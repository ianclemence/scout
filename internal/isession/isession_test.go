package isession

import (
	"path/filepath"
	"testing"

	"github.com/ianclemence/scout/internal/config"
	"github.com/ianclemence/scout/internal/domain"
	"github.com/ianclemence/scout/internal/runtime"
	"github.com/ianclemence/scout/internal/store"
)

func TestCommandRegistry(t *testing.T) {
	cmds := Registry()
	if len(cmds) < 15 {
		t.Fatalf("expected full command set, got %d", len(cmds))
	}
	seen := map[string]bool{}
	for _, c := range cmds {
		if seen[c.Name] {
			t.Fatalf("duplicate command /%s", c.Name)
		}
		seen[c.Name] = true
		if c.Description == "" || c.Handler == nil {
			t.Fatalf("command /%s incomplete", c.Name)
		}
	}
	for _, want := range []string{"help", "status", "model", "approvals", "proposal", "compact", "quit"} {
		if FindCommand(want) == nil {
			t.Fatalf("missing /%s", want)
		}
	}
	if FindCommand("nope") != nil {
		t.Fatal("unknown command resolved")
	}
}

func TestResolveOpp(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	db, err := store.Open(filepath.Join(t.TempDir(), "i.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	core, err := runtime.New(cfg, db)
	if err != nil {
		t.Fatal(err)
	}
	st := &ReplState{Core: core}
	o, err := core.AddOpportunity("Go API", "Build a Go API backend", "go")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.resolveOpp(o.ID); err != nil {
		t.Fatal("full id failed:", err)
	}
	if _, err := st.resolveOpp(o.ID[:8]); err != nil {
		t.Fatal("prefix failed:", err)
	}
	st.LastOpps = []domain.Opportunity{{ID: o.ID, Title: o.Title}}
	if _, err := st.resolveOpp("1"); err != nil {
		t.Fatal("index failed:", err)
	}
	if _, err := st.resolveOpp(""); err == nil {
		t.Fatal("empty ref should fail")
	}
	if _, err := st.resolveOpp("zzz"); err == nil {
		t.Fatal("unknown ref should fail")
	}
}
