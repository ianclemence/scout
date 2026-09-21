// Package selfupdate implements the mechanics of keeping a Scout install
// current: resolve the target version, download and verify a release
// artifact (or build a pinned tag for the dev channel), smoke-test it, and
// atomically swap it into place. It never builds in place and never
// overwrites a running binary.
package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Release describes a published Scout release.
type Release struct {
	Version    string  `json:"tag_name"` // e.g. "v0.6.0"
	Name       string  `json:"name"`
	Notes      string  `json:"body"`
	Assets     []Asset `json:"assets"`
	Prerelease bool    `json:"prerelease"`
}

// Asset is one downloadable file attached to a release.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

// Client talks to the release host. It is an interface so tests can inject a
// fake without a network.
type Client interface {
	// Latest returns the newest stable release.
	Latest() (*Release, error)
	// ByTag returns a specific release tag (e.g. "v0.6.0").
	ByTag(tag string) (*Release, error)
	// Download fetches an asset to dst.
	Download(url, dst string) error
}

// GitHubClient is the production Client backed by the GitHub Releases API.
type GitHubClient struct {
	Repo   string // "owner/repo"
	HTTP   *http.Client
	APIURL string // override for tests; defaults to the public API
}

// NewGitHubClient builds a client for a repository.
func NewGitHubClient(repo string) *GitHubClient {
	return &GitHubClient{
		Repo: repo,
		HTTP: &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *GitHubClient) api(path string) string {
	base := c.APIURL
	if base == "" {
		base = "https://api.github.com"
	}
	return strings.TrimSuffix(base, "/") + path
}

func (c *GitHubClient) get(url string, into any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "scout-selfupdate")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("release host returned HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(into)
}

// Latest fetches the newest stable release.
func (c *GitHubClient) Latest() (*Release, error) {
	var r Release
	if err := c.get(c.api("/repos/"+c.Repo+"/releases/latest"), &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// ByTag fetches a release by tag name.
func (c *GitHubClient) ByTag(tag string) (*Release, error) {
	var r Release
	if err := c.get(c.api("/repos/"+c.Repo+"/releases/tags/"+tag), &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// Download streams an asset to dst, following redirects.
func (c *GitHubClient) Download(url, dst string) error {
	resp, err := c.HTTP.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("asset download returned HTTP %d", resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}
	return f.Sync()
}

// ErrNotFound is returned when a release or tag does not exist.
var ErrNotFound = errors.New("not found")

// ---------- version comparison ----------

var semverRe = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)`)

func parseSemver(s string) (major, minor, patch int, rest string, ok bool) {
	m := semverRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, 0, 0, "", false
	}
	major, _ = strconv.Atoi(m[1])
	minor, _ = strconv.Atoi(m[2])
	patch, _ = strconv.Atoi(m[3])
	rest = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), m[0]))
	return major, minor, patch, rest, true
}

// IsNewer reports whether candidate is a strictly newer semantic version than
// current. The candidate must be a real semantic version; a non-version target
// (e.g. "dev") is never considered an upgrade. When current is not a version
// (a dev build), any valid candidate is newer.
func IsNewer(candidate, current string) bool {
	cm, cn, cp, _, cok := parseSemver(candidate)
	if !cok {
		return false
	}
	um, un, up, _, uok := parseSemver(current)
	if !uok {
		return true
	}
	if cm != um {
		return cm > um
	}
	if cn != un {
		return cn > un
	}
	return cp > up
}

// NormalizeVersion trims a leading "v".
func NormalizeVersion(s string) string { return strings.TrimPrefix(strings.TrimSpace(s), "v") }

// ---------- checksums ----------

// SHA256File returns the hex sha256 of a file.
func SHA256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Checksums parses a sha256sums file ("<hex>  <name>") into name->hex.
func Checksums(data string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 {
			out[strings.TrimPrefix(fields[1], "*")] = strings.ToLower(fields[0])
		}
	}
	return out
}

// VerifySHA256 checks a file against an expected hex digest (empty = skip).
func VerifySHA256(path, wantHex string) error {
	if wantHex == "" {
		return nil
	}
	got, err := SHA256File(path)
	if err != nil {
		return err
	}
	if !strings.EqualFold(got, wantHex) {
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s", filepath.Base(path), got, wantHex)
	}
	return nil
}

// ---------- atomic install ----------

// AtomicReplace installs src as dst by writing a sibling ".new" file and
// renaming it over dst. A rename within the same directory is atomic, so the
// running binary is never partially overwritten. The file is made executable.
func AtomicReplace(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".new"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// ---------- changelog ----------

// ChangelogEntry is one versioned section of a CHANGELOG file.
type ChangelogEntry struct {
	Version string // "0.6.0"
	Body    string
}

var changelogHeaderRe = regexp.MustCompile(`(?m)^##\s+\[?v?(\d+\.\d+\.\d+)\]?.*$`)

// ParseChangelog splits a CHANGELOG.md into versioned entries, newest first.
func ParseChangelog(md string) []ChangelogEntry {
	idx := changelogHeaderRe.FindAllStringSubmatchIndex(md, -1)
	var out []ChangelogEntry
	for i, loc := range idx {
		ver := md[loc[2]:loc[3]]
		start := loc[1]
		end := len(md)
		if i+1 < len(idx) {
			end = idx[i+1][0]
		}
		out = append(out, ChangelogEntry{Version: ver, Body: strings.TrimSpace(md[start:end])})
	}
	return out
}

// NewEntries returns the entries newer than lastSeen (exclusive), newest
// first. When lastSeen is empty, all entries are returned.
func NewEntries(entries []ChangelogEntry, lastSeen string) []ChangelogEntry {
	if lastSeen == "" {
		return entries
	}
	var out []ChangelogEntry
	for _, e := range entries {
		if e.Version == NormalizeVersion(lastSeen) {
			break
		}
		out = append(out, e)
	}
	return out
}
