package profile

import (
	"path/filepath"
	"testing"

	"github.com/ianclemence/scout/pkg/store"
)

func TestImportTextCV(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cv := "Jane Doe\nSenior Go Developer\nBuilt APIs with Go, Postgres and Docker. React frontend work.\n"
	p, ev, err := ImportDocument(db, "cv.txt", []byte(cv))
	if err != nil {
		t.Fatal(err)
	}
	if p.DisplayName == "" {
		t.Fatal("expected name extraction")
	}
	if len(p.Skills) < 2 {
		t.Fatalf("expected skills, got %v", p.Skills)
	}
	if len(ev) != 1 || ev[0].Kind != "cv_section" {
		t.Fatal("expected cv evidence")
	}
	loaded, err := Load(db)
	if err != nil || loaded.DisplayName != p.DisplayName {
		t.Fatal("profile not persisted as source of truth")
	}
}
