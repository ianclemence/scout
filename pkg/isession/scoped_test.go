package isession

import (
	"path/filepath"
	"testing"

	"github.com/ianclemence/scout/pkg/config"
	"github.com/ianclemence/scout/pkg/runtime"
	"github.com/ianclemence/scout/pkg/store"
)

func testCore(t *testing.T) *runtime.Core {
	t.Helper()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	db, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	core, err := runtime.New(cfg, db)
	if err != nil {
		t.Fatal(err)
	}
	return core
}

func TestScopedModelsPersistence(t *testing.T) {
	core := testCore(t)
	if sc := LoadScopedModels(core); !sc.AllEnabled() {
		t.Fatal("default should be all-enabled")
	}
	ids := []string{"ollama/qwen3:0.6b", "anthropic/claude-haiku-4-5"}
	if err := SaveScopedModels(core, ids); err != nil {
		t.Fatal(err)
	}
	sc := LoadScopedModels(core)
	if sc.AllEnabled() {
		t.Fatal("expected explicit scope")
	}
	got := sc.IDs()
	if len(got) != 2 || got[0] != ids[0] || got[1] != ids[1] {
		t.Fatalf("order not preserved: %v", got)
	}
	if !sc.IsEnabled("ollama/qwen3:0.6b") || sc.IsEnabled("deepseek/deepseek-flash") {
		t.Fatal("IsEnabled wrong")
	}
	// Saving nil clears the setting -> all enabled.
	if err := SaveScopedModels(core, nil); err != nil {
		t.Fatal(err)
	}
	if !LoadScopedModels(core).AllEnabled() {
		t.Fatal("nil should clear to all-enabled")
	}
}

func TestScopedModelsCoveringAllCollapses(t *testing.T) {
	core := testCore(t)
	all := AvailableModels(core)
	ids := make([]string, 0, len(all))
	for _, m := range all {
		ids = append(ids, m.Provider+"/"+m.ID)
	}
	if err := SaveScopedModels(core, ids); err != nil {
		t.Fatal(err)
	}
	// An explicit list covering every model collapses to "all enabled".
	if !LoadScopedModels(core).AllEnabled() {
		t.Fatal("full coverage should collapse to all-enabled")
	}
}

func TestFilterScopedModels(t *testing.T) {
	core := testCore(t)
	all := AvailableModels(core)
	var sc ScopedModels
	got := FilterScoped(all, sc)
	if len(got) != len(all) {
		t.Fatalf("all-enabled should return full catalog: %d vs %d", len(got), len(all))
	}
	sc.Set([]string{"ollama/qwen3:0.6b"})
	got = FilterScoped(all, sc)
	if len(got) != 1 || got[0].Provider != "ollama" {
		t.Fatalf("scoped filter wrong: %+v", got)
	}
}

func TestCycleModel(t *testing.T) {
	core := testCore(t)
	all := AvailableModels(core)
	var sc ScopedModels
	next, ok := CycleModel(all, sc, all[0].Provider, all[0].ID)
	if !ok {
		t.Fatal("expected cycle")
	}
	if next.Provider == all[0].Provider && next.ID == all[0].ID {
		t.Fatal("cycle should advance")
	}
	// Wrap-around at the end.
	last := all[len(all)-1]
	next, ok = CycleModel(all, sc, last.Provider, last.ID)
	if !ok || next.Provider != all[0].Provider || next.ID != all[0].ID {
		t.Fatalf("expected wrap to first, got %+v", next)
	}
}
