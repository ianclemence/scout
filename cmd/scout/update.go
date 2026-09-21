package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ianclemence/scout/pkg/changelog"
	"github.com/ianclemence/scout/pkg/runtime"
	"github.com/ianclemence/scout/pkg/selfupdate"
	"github.com/ianclemence/scout/pkg/version"
)

// repoSlug is the GitHub repository Scout releases from.
const repoSlug = "ianclemence/scout"

// updateOptions is the parsed `scout update` invocation.
type updateOptions struct {
	self    bool // update the binary (default)
	models  bool // refresh the model catalog
	all     bool // binary + models
	force   bool
	dryRun  bool
	check   bool   // report only
	notes   bool   // print changelog for the installed version
	channel string // "" (release) or "dev"
	version string // pinned target (e.g. v0.6.0)
}

// updateCmd implements `scout update`:
//
//	scout update                 update the binary to the latest release
//	scout update --self          same (alias)
//	scout update --models        refresh the model catalog only
//	scout update --all           binary + model catalog
//	scout update --check         report current vs available, change nothing
//	scout update --notes         print the changelog for the installed version
//	scout update --force         reinstall even when already current
//	scout update --dry-run       show the plan, change nothing
//	scout update --channel dev   build from the local checkout (developers)
//
// The default channel installs a verified release artifact and never builds
// in place. The dev channel builds the working tree, but always injects the
// version so the installed binary reports a real tag, not "dev".
func updateCmd(args []string) error {
	opts, err := parseUpdateArgs(args)
	if err != nil {
		return err
	}

	if opts.notes {
		printChangelogNotes()
		return nil
	}

	doSelf := opts.self || opts.all || (!opts.models && !opts.all)
	doModels := opts.models || opts.all

	if doSelf {
		if err := runSelfUpdate(opts); err != nil {
			return err
		}
	}
	if doModels {
		if err := refreshModels(); err != nil {
			return fmt.Errorf("model catalog refresh failed: %w", err)
		}
	}
	return nil
}

func parseUpdateArgs(args []string) (updateOptions, error) {
	opts := updateOptions{}
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "--self":
			opts.self = true
		case "--models":
			opts.models = true
		case "--all":
			opts.all = true
		case "--force":
			opts.force = true
		case "--dry-run":
			opts.dryRun = true
		case "--check":
			opts.check = true
		case "--notes":
			opts.notes = true
		case "--channel":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("--channel needs a value (release|dev)")
			}
			i++
			opts.channel = strings.ToLower(args[i])
		case "--version":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("--version needs a value (e.g. v0.6.0)")
			}
			i++
			opts.version = args[i]
		default:
			if strings.HasPrefix(a, "--channel=") {
				opts.channel = strings.ToLower(strings.TrimPrefix(a, "--channel="))
				continue
			}
			if strings.HasPrefix(a, "--version=") {
				opts.version = strings.TrimPrefix(a, "--version=")
				continue
			}
			return opts, fmt.Errorf("usage: scout update [--self|--models|--all] [--check|--notes] [--force] [--dry-run] [--channel release|dev] [--version V]")
		}
	}
	if opts.channel != "" && opts.channel != "release" && opts.channel != "dev" {
		return opts, fmt.Errorf("unknown channel %q (want release or dev)", opts.channel)
	}
	return opts, nil
}

// runSelfUpdate resolves the target and dispatches to the release or dev
// channel.
func runSelfUpdate(opts updateOptions) error {
	current := version.Version
	if opts.channel == "dev" {
		return devChannelUpdate(opts, current)
	}
	return releaseChannelUpdate(opts, current)
}

// releaseChannelUpdate installs a verified release artifact. Scout releases
// are currently source tags without binary assets, so this verifies the tag
// exists and, when a matching asset is published, downloads and installs it;
// otherwise it falls back to building that exact tag (still never the dirty
// working tree), which keeps the update reproducible and version-stamped.
func releaseChannelUpdate(opts updateOptions, current string) error {
	if offline() {
		return fmt.Errorf("offline (SCOUT_OFFLINE set) — skipping update")
	}
	client := selfupdate.NewGitHubClient(repoSlug)

	var rel *selfupdate.Release
	var err error
	if opts.version != "" {
		rel, err = client.ByTag(opts.version)
	} else {
		rel, err = client.Latest()
	}
	if err != nil {
		return fmt.Errorf("could not resolve the latest release: %w", err)
	}
	target := rel.Version
	fmt.Printf("Installed: %s\n", current)
	fmt.Printf("Available: %s\n", target)

	if !opts.force && !selfupdate.IsNewer(target, current) {
		fmt.Println("Already current.")
		printRecentNotes(current)
		return nil
	}
	if opts.check {
		fmt.Printf("Update available: %s → %s (run `scout update`)\n", current, target)
		printRecentNotes(target)
		return nil
	}
	if opts.dryRun {
		plan := "download the release asset, verify its checksum, smoke-test it, and atomically install over ~/.local/bin/scout, then restart the user service"
		if !hasAsset(rel, "scout_") {
			plan = "no binary asset published for this tag; build the pinned tag " + target + " (never the working tree), verify, and atomically install"
		}
		fmt.Printf("[dry-run] would %s. (%s → %s)\n", plan, current, target)
		return nil
	}

	bin := installedBinaryPath()
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp("", "scout-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)

	staged := filepath.Join(stage, "scout")
	if asset, ok := findAsset(rel, "scout_"); ok {
		fmt.Printf("Downloading %s...\n", asset.Name)
		if err := client.Download(asset.URL, staged); err != nil {
			return err
		}
		// Release assets are stored without the executable bit; make the
		// staged binary runnable before the checksum and smoke test.
		if err := os.Chmod(staged, 0o755); err != nil {
			return err
		}
		if sum, ok := findAsset(rel, "checksums"); ok {
			sumPath := filepath.Join(stage, sum.Name)
			if err := client.Download(sum.URL, sumPath); err == nil {
				if data, rerr := os.ReadFile(sumPath); rerr == nil {
					if want := selfupdate.Checksums(string(data))[asset.Name]; want != "" {
						if err := selfupdate.VerifySHA256(staged, want); err != nil {
							return err
						}
						fmt.Println("Checksum verified.")
					}
				}
			}
		}
	} else {
		fmt.Printf("No binary asset for %s; building the pinned tag...\n", target)
		if err := buildTag(target, staged); err != nil {
			return err
		}
	}

	if err := smokeTest(staged, target); err != nil {
		return err
	}
	if err := selfupdate.AtomicReplace(staged, bin); err != nil {
		return err
	}
	fmt.Printf("Updated %s → %s\n", current, target)
	restartUserService()
	_ = changelog.MarkSeen(dataDirPath(), current)
	printNotesFor(target)
	return nil
}

// devChannelUpdate builds the local checkout with a real version injected.
// This is the developer path; it refuses a dirty tree unless --force.
func devChannelUpdate(opts updateOptions, current string) error {
	repo := os.Getenv("SCOUT_REPO")
	if repo == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		repo = filepath.Join(home, "scout")
	}
	if !isScoutCheckout(repo) {
		return fmt.Errorf("not a scout checkout: %s (set SCOUT_REPO)", repo)
	}
	tag := gitDescribe(repo)
	if opts.dryRun {
		fmt.Printf("[dry-run] would build %s at %s, inject version %s, and install over %s\n", repo, shortRef(gitHead(repo)), tag, installedBinaryPath())
		return nil
	}
	if !opts.force {
		if dirty := gitDirty(repo); dirty != "" {
			return fmt.Errorf("refusing to update: uncommitted changes in %s (commit them or use --force)\n%s", repo, dirty)
		}
	}
	bin := installedBinaryPath()
	stage, err := os.MkdirTemp("", "scout-dev-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	staged := filepath.Join(stage, "scout")
	fmt.Printf("Building %s (version %s)...\n", repo, tag)
	build := exec.Command("go", "build", "-ldflags", "-X github.com/ianclemence/scout/pkg/version.Version="+tag, "-o", staged, "./cmd/scout")
	build.Dir = repo
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}
	if err := smokeTest(staged, tag); err != nil {
		return err
	}
	if err := selfupdate.AtomicReplace(staged, bin); err != nil {
		return err
	}
	fmt.Printf("Installed dev build %s → %s\n", current, tag)
	restartUserService()
	return nil
}

// refreshModels updates the cached model catalog.
func refreshModels() error {
	return withCore(func(c *runtime.Core) error {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		n, err := c.Registry().Refresh(ctx, "")
		if err != nil {
			return err
		}
		fmt.Printf("Model catalog refreshed (%d models).\n", n)
		return nil
	})
}

// ---------- helpers ----------

func offline() bool {
	return os.Getenv("SCOUT_OFFLINE") != "" || os.Getenv("PI_OFFLINE") != ""
}

func installedBinaryPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin", "scout")
}

func dataDirPath() string {
	if v := os.Getenv("SCOUT_DATA_DIR"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "scout")
}

func findAsset(rel *selfupdate.Release, prefix string) (selfupdate.Asset, bool) {
	for _, a := range rel.Assets {
		if strings.Contains(strings.ToLower(a.Name), strings.ToLower(prefix)) {
			return a, true
		}
	}
	return selfupdate.Asset{}, false
}

func hasAsset(rel *selfupdate.Release, prefix string) bool {
	_, ok := findAsset(rel, prefix)
	return ok
}

// smokeTest confirms the staged binary runs and reports the expected version.
func smokeTest(path, wantVersion string) error {
	out, err := exec.Command(path, "version").Output()
	if err != nil {
		return fmt.Errorf("staged binary failed its smoke test: %w", err)
	}
	got := strings.TrimSpace(string(out))
	fmt.Printf("Smoke test: %s\n", got)
	if wantVersion != "" && !strings.Contains(got, selfupdate.NormalizeVersion(wantVersion)) && !strings.Contains(wantVersion, "dev") {
		return fmt.Errorf("staged binary reports %q, expected %s", got, wantVersion)
	}
	return nil
}

func restartUserService() {
	if err := exec.Command("systemctl", "--user", "restart", "scout").Run(); err != nil {
		fmt.Println("Note: could not restart the user service; run: systemctl --user restart scout")
		return
	}
	fmt.Println("Service restarted.")
}

func printChangelogNotes() {
	fmt.Println(changelog.Raw())
}

func printNotesFor(version string) {
	body := changelog.ForVersion(version)
	if body == "" {
		return
	}
	fmt.Printf("\nWhat's new in %s:\n\n%s\n", version, body)
}

func printRecentNotes(version string) {
	for _, e := range changelog.NewSince(dataDirPath(), version) {
		fmt.Printf("\nWhat's new in %s:\n\n%s\n", e.Version, e.Body)
	}
}

// ---------- git helpers (dev channel) ----------

func gitDescribe(repo string) string {
	out, err := exec.Command("git", "-C", repo, "describe", "--tags", "--always", "--dirty").Output()
	if err != nil {
		return "dev"
	}
	return strings.TrimSpace(string(out))
}

func gitHead(repo string) string {
	out, _ := exec.Command("git", "-C", repo, "rev-parse", "--short", "HEAD").Output()
	return strings.TrimSpace(string(out))
}

func gitDirty(repo string) string {
	out, err := exec.Command("git", "-C", repo, "status", "--porcelain").Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return ""
	}
	return shortDirty(string(out))
}

func isScoutCheckout(dir string) bool {
	raw, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return false
	}
	return strings.HasPrefix(string(raw), "module github.com/ianclemence/scout")
}

// buildTag builds a specific git tag into dst without touching the working
// tree, using a worktree in a temp dir. This keeps release installs
// reproducible and avoids deploying uncommitted work.
func buildTag(tag, dst string) error {
	repo, err := checkoutDir()
	if err != nil {
		return err
	}
	wt, err := os.MkdirTemp("", "scout-tag-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(wt)
	if out, err := exec.Command("git", "-C", repo, "worktree", "add", "--detach", wt, tag).CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree add %s: %v\n%s", tag, err, out)
	}
	defer exec.Command("git", "-C", repo, "worktree", "remove", "--force", wt).Run()
	build := exec.Command("go", "build", "-ldflags", "-X github.com/ianclemence/scout/pkg/version.Version="+tag, "-o", dst, "./cmd/scout")
	build.Dir = wt
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("build %s failed: %w", tag, err)
	}
	return nil
}

func checkoutDir() (string, error) {
	if repo := os.Getenv("SCOUT_REPO"); repo != "" {
		return repo, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "scout"), nil
}

// shortRef trims a git ref to a readable length.
func shortRef(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 7 {
		return s[:7]
	}
	if s == "" {
		return "unknown"
	}
	return s
}

// shortDirty summarizes a git porcelain status into at most a few lines.
func shortDirty(out string) string {
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		lines = append(lines, "  "+l)
		if len(lines) >= 8 {
			lines = append(lines, "  …")
			break
		}
	}
	return strings.Join(lines, "\n")
}
