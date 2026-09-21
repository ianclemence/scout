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

// configureProvider stores a credential so the provider counts as configured.
func configureProvider(t *testing.T, core *runtime.Core, provider string) {
	t.Helper()
	if err := core.SaveSecret("llm:"+provider, "test-key"); err != nil {
		t.Fatal(err)
	}
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
	// Normalization is against the full catalog, so an explicit list covering
	// every known model collapses to "all enabled".
	all := AllModels(core)
	ids := make([]string, 0, len(all))
	for _, m := range all {
		ids = append(ids, m.Provider+"/"+m.ID)
	}
	if err := SaveScopedModels(core, ids); err != nil {
		t.Fatal(err)
	}
	if !LoadScopedModels(core).AllEnabled() {
		t.Fatal("full coverage should collapse to all-enabled")
	}
}

func TestAvailableModelsFiltersConfiguredProviders(t *testing.T) {
	core := testCore(t)
	// The available set contains exactly the models from providers reported as
	// configured (which includes a reachable Ollama, if one is running).
	configured := core.ConfiguredProviders()
	avail := AvailableModels(core)
	for _, m := range avail {
		if !configured[m.Provider] {
			t.Fatalf("available set leaked unconfigured provider %q", m.Provider)
		}
	}
	// Every model from a configured provider must be present.
	for _, m := range AllModels(core) {
		if !configured[m.Provider] {
			continue
		}
		found := false
		for _, a := range avail {
			if a.Provider == m.Provider && a.ID == m.ID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("configured model %s/%s missing from available set", m.Provider, m.ID)
		}
	}
	// A newly configured provider contributes its models and nothing else.
	configureProvider(t, core, "deepseek")
	configured = core.ConfiguredProviders()
	avail = AvailableModels(core)
	sawDeepseek := false
	for _, m := range avail {
		if !configured[m.Provider] {
			t.Fatalf("available set leaked unconfigured provider %q", m.Provider)
		}
		if m.Provider == "deepseek" {
			sawDeepseek = true
		}
	}
	if !sawDeepseek {
		t.Fatal("expected deepseek models after configuring it")
	}
	// AllModels must be a superset that still includes unconfigured providers
	// as long as one exists.
	for _, m := range AllModels(core) {
		if !configured[m.Provider] {
			return // found the expected superset member
		}
	}
	t.Log("all known providers are configured in this environment; superset check skipped")
}

func TestFilterScopedModels(t *testing.T) {
	core := testCore(t)
	configureProvider(t, core, "deepseek")
	all := AvailableModels(core)
	var sc ScopedModels
	got := FilterScoped(all, sc)
	if len(got) != len(all) {
		t.Fatalf("all-enabled should return the input catalog: %d vs %d", len(got), len(all))
	}
	sc.Set([]string{"deepseek/deepseek-flash"})
	got = FilterScoped(all, sc)
	if len(got) != 1 || got[0].ID != "deepseek-flash" {
		t.Fatalf("scoped filter wrong: %+v", got)
	}
}

func TestCycleModel(t *testing.T) {
	core := testCore(t)
	configureProvider(t, core, "deepseek")
	all := AvailableModels(core)
	if len(all) < 2 {
		t.Skip("need at least two configured models to test cycling")
	}
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
