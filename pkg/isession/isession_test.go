package isession

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ianclemence/scout/pkg/config"
	"github.com/ianclemence/scout/pkg/domain"
	"github.com/ianclemence/scout/pkg/runtime"
	"github.com/ianclemence/scout/pkg/store"
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
	for _, want := range []string{"help", "status", "model", "approvals", "proposal", "compact", "quit", "profile", "doctor", "opportunities", "sources", "login", "thinking", "sessions", "new", "discover", "applications", "feedback", "export"} {
		if FindCommand(want) == nil {
			t.Fatalf("missing /%s", want)
		}
	}
	// Commands deliberately removed from the interactive surface.
	for _, gone := range []string{"providers", "scoped-models", "skills", "tools", "copy", "keys", "session", "pipeline", "cv", "clear", "changelog", "inbox"} {
		if FindCommand(gone) != nil {
			t.Fatalf("/%s should be removed", gone)
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

func TestDoctorCommandExecutes(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	db, err := store.Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	core, err := runtime.New(cfg, db)
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	ctx := &SessionCtx{Core: core, Out: func(f string, a ...any) { fmt.Fprintf(&sb, f, a...) }}
	if err := FindCommand("doctor").Handler(ctx, ""); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	for _, want := range []string{"data-dir", "sqlite", "skills", "tools", "disk"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Run `scout doctor`") {
		t.Fatal("doctor must execute, not redirect")
	}
}

func TestProfileEvidence(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	db, err := store.Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	core, err := runtime.New(cfg, db)
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	ctx := &SessionCtx{Core: core, Out: func(f string, a ...any) { fmt.Fprintf(&sb, f, a...) }}
	if err := FindCommand("profile").Handler(ctx, "evidence"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), "No resume content") {
		t.Fatalf("unexpected /profile evidence output: %q", sb.String())
	}
}
