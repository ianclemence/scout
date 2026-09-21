package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitCopiesTemplate(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"README.md", "SCOUT.md", "skills/README.md", "skills/rate-check/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(Dir(dir), f)); err != nil {
			t.Fatalf("missing template file %s", f)
		}
	}
	// Never overwrites user material.
	marker := filepath.Join(Dir(dir), "SCOUT.md")
	os.WriteFile(marker, []byte("mine"), 0o600)
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(marker)
	if string(b) != "mine" {
		t.Fatal("init overwrote user file")
	}
}

func TestOwnerNotesCapped(t *testing.T) {
	if OwnerNotes(t.TempDir()) != "" {
		t.Fatal("absent notes should be empty")
	}
}
