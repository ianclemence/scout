// Package changelog embeds the release notes and tracks the last version the
// user has seen, so Scout can print only what is new after an update.
package changelog

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"

	"github.com/ianclemence/scout/pkg/selfupdate"
)

//go:embed CHANGELOG.md
var raw string

// markFile is the per-data-dir marker holding the last seen version.
const markFile = "changelog.seen"

// Raw returns the full embedded changelog.
func Raw() string { return raw }

// Entries returns the parsed changelog, newest first.
func Entries() []selfupdate.ChangelogEntry { return selfupdate.ParseChangelog(raw) }

// ForVersion returns the body for an exact version (no "v"), or "".
func ForVersion(version string) string {
	want := selfupdate.NormalizeVersion(version)
	for _, e := range Entries() {
		if e.Version == want {
			return e.Body
		}
	}
	return ""
}

func markPath(dataDir string) string { return filepath.Join(dataDir, markFile) }

// LastSeen returns the version the user has already been shown, or "".
func LastSeen(dataDir string) string {
	b, err := os.ReadFile(markPath(dataDir))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// MarkSeen records that the given version's notes have been shown. The base
// version is stored so a dev build ("0.7.0-3-gabc") is not later mistaken for
// a distinct release.
func MarkSeen(dataDir, version string) error {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(markPath(dataDir), []byte(selfupdate.BaseVersion(version)+"\n"), 0o600)
}

// NewSince returns the entries the user has not yet seen, given the current
// version. On a first run (no marker) it returns nothing and records the
// current version, so a fresh install is not greeted with old history.
func NewSince(dataDir, current string) []selfupdate.ChangelogEntry {
	last := LastSeen(dataDir)
	if last == "" {
		_ = MarkSeen(dataDir, current)
		return nil
	}
	return selfupdate.NewEntries(Entries(), last)
}
