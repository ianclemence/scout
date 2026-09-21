// Package workspace owns the user workspace: a tracked template shipped
// with Scout, copied once into the data dir by `scout init`, never
// overwritten, never committed (it lives outside the repository).
package workspace

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed template/*
var templateFS embed.FS

// Dir returns the runtime workspace root for a data dir.
func Dir(dataDir string) string { return filepath.Join(dataDir, "workspace") }

// SkillsDir is the user skill overlay.
func SkillsDir(dataDir string) string { return filepath.Join(Dir(dataDir), "skills") }

// NotesFile is the owner notes file (SCOUT.md).
func NotesFile(dataDir string) string { return filepath.Join(Dir(dataDir), "SCOUT.md") }

// Init copies the template on first run. Existing files are never touched.
func Init(dataDir string) error {
	root := Dir(dataDir)
	return fs.WalkDir(templateFS, "template", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel("template", path)
		dst := filepath.Join(root, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o700)
		}
		if _, err := os.Stat(dst); err == nil {
			return nil // never overwrite user material
		}
		raw, err := templateFS.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, raw, 0o600)
	})
}

// OwnerNotes reads SCOUT.md (capped), empty when absent.
func OwnerNotes(dataDir string) string {
	raw, err := os.ReadFile(NotesFile(dataDir))
	if err != nil {
		return ""
	}
	s := string(raw)
	if len(s) > 2000 {
		s = s[:2000]
	}
	return s
}
