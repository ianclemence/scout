package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// updateCmd deploys the latest checkout to the installed Scout:
// refuse dirty tree (unless --force), pull, skip when already current,
// rebuild, reinstall, restart the user service.
func updateCmd(args []string) error {
	dryRun, force := false, false
	for _, a := range args {
		switch a {
		case "--dry-run":
			dryRun = true
		case "--force":
			force = true
		default:
			return fmt.Errorf("usage: scout update [--dry-run] [--force]")
		}
	}
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
	git := func(gargs ...string) (string, error) {
		cmd := exec.Command("git", append([]string{"-C", repo}, gargs...)...)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		err := cmd.Run()
		return out.String(), err
	}
	before, _ := git("rev-parse", "--short", "HEAD")

	if !force {
		if out, err := git("status", "--porcelain"); err == nil && strings.TrimSpace(out) != "" {
			return fmt.Errorf("refusing to update: uncommitted changes in %s (commit them or use --force)\n%s", repo, shortDirty(out))
		}
	}
	fmt.Println("Fetching...")
	if out, err := git("fetch", "origin"); err != nil {
		return fmt.Errorf("fetch failed: %v\n%s", err, out)
	}
	local, _ := git("rev-parse", "HEAD")
	remote, _ := git("rev-parse", "@{upstream}")
	upToDate := strings.TrimSpace(local) == strings.TrimSpace(remote) && strings.TrimSpace(remote) != ""
	if upToDate && !force {
		fmt.Printf("Already up to date (%s) — nothing to deploy. Use --force to rebuild anyway.\n", strings.TrimSpace(before))
		return nil
	}
	if dryRun {
		fmt.Printf("[dry-run] would update %s → %s, rebuild, reinstall ~/.local/bin/scout, restart user service.\n",
			strings.TrimSpace(before), shortRef(remote))
		return nil
	}
	fmt.Println("Pulling...")
	if out, err := git("pull", "--ff-only"); err != nil {
		return fmt.Errorf("pull failed: %v\n%s", err, out)
	}
	after, _ := git("rev-parse", "--short", "HEAD")
	fmt.Printf("Building %s...\n", strings.TrimSpace(after))
	home, _ := os.UserHomeDir()
	bin := filepath.Join(home, ".local", "bin", "scout")
	build := exec.Command("go", "build", "-o", bin, "./cmd/scout")
	build.Dir = repo
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("build failed: %w (checkout left at %s)", err, strings.TrimSpace(after))
	}
	fmt.Println("Restarting service...")
	if err := exec.Command("systemctl", "--user", "restart", "scout").Run(); err != nil {
		return fmt.Errorf("restart failed: %w (binary updated; start service manually: systemctl --user start scout)", err)
	}
	fmt.Printf("Update complete: %s → %s\n", strings.TrimSpace(before), strings.TrimSpace(after))
	return nil
}

func isScoutCheckout(dir string) bool {
	raw, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return false
	}
	return strings.HasPrefix(string(raw), "module github.com/ianclemence/scout")
}

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
