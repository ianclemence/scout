package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ianclemence/scout/pkg/config"
	"github.com/ianclemence/scout/pkg/isession"
	"github.com/ianclemence/scout/pkg/runtime"
	"github.com/ianclemence/scout/pkg/store"
)

func uiTestCore(t *testing.T) *runtime.Core {
	t.Helper()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	db, err := store.Open(filepath.Join(t.TempDir(), "tui.db"))
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

// uiConfigure marks a provider configured by storing a credential.
func uiConfigure(t *testing.T, core *runtime.Core, provider string) {
	t.Helper()
	if err := core.SaveSecret("llm:"+provider, "test-key"); err != nil {
		t.Fatal(err)
	}
}

func TestScopedUI_ToggleFromAll(t *testing.T) {
	core := uiTestCore(t)
	u := newScopedModelsUI(core, isession.ScopedModels{}, "", "")
	if !u.enabled("ollama/qwen3:0.6b") {
		t.Fatal("all should be enabled initially")
	}
	// Move cursor onto a known model and toggle it off.
	target := "ollama/qwen3:0.6b"
	for i, id := range u.filtered {
		if id == target {
			u.cur = i
			break
		}
	}
	u.handleKey("enter")
	if u.enabled(target) {
		t.Fatal("toggle should disable the model")
	}
	if u.enabled("anthropic/claude-haiku-4-5") == false {
		t.Fatal("other models should remain enabled")
	}
	// Toggle back on; covering all again should collapse to nil.
	for i, id := range u.filtered {
		if id == target {
			u.cur = i
			break
		}
	}
	u.handleKey("enter")
	if u.ids != nil {
		t.Fatalf("re-enabling all should collapse to nil, got %v", u.ids)
	}
}

func TestScopedUI_Reorder(t *testing.T) {
	core := uiTestCore(t)
	sc := isession.ScopedModels{}
	sc.Set([]string{"ollama/qwen3:0.6b", "anthropic/claude-haiku-4-5"})
	u := newScopedModelsUI(core, sc, "", "")
	if len(u.ids) != 2 || u.ids[0] != "ollama/qwen3:0.6b" {
		t.Fatalf("initial order wrong: %v", u.ids)
	}
	u.cur = 0
	u.handleKey("alt+down")
	if u.ids[0] != "anthropic/claude-haiku-4-5" || u.ids[1] != "ollama/qwen3:0.6b" {
		t.Fatalf("reorder failed: %v", u.ids)
	}
}

func TestScopedUI_ToggleProvider(t *testing.T) {
	core := uiTestCore(t)
	u := newScopedModelsUI(core, isession.ScopedModels{}, "", "")
	// Find the deepseek row and toggle the whole provider off.
	for i, id := range u.filtered {
		if providerOf(id) == "deepseek" {
			u.cur = i
			break
		}
	}
	u.handleKey("ctrl+p")
	for _, id := range u.filtered {
		if providerOf(id) == "deepseek" && u.enabled(id) {
			t.Fatalf("provider toggle should disable %s", id)
		}
	}
}

func TestScopedUI_ClearAllThenEnableAll(t *testing.T) {
	core := uiTestCore(t)
	u := newScopedModelsUI(core, isession.ScopedModels{}, "", "")
	u.handleKey("ctrl+x")
	if u.ids == nil || len(u.ids) != 0 {
		t.Fatalf("clear should yield an explicit empty list, got %v", u.ids)
	}
	u.handleKey("ctrl+a")
	if u.ids != nil {
		t.Fatalf("enable-all should collapse to nil, got %v", u.ids)
	}
}

func TestScopedUI_PersistKey(t *testing.T) {
	core := uiTestCore(t)
	u := newScopedModelsUI(core, isession.ScopedModels{}, "", "")
	if !u.handleKey("ctrl+s") {
		t.Fatal("ctrl+s should request persist")
	}
	if u.handleKey("enter") {
		t.Fatal("enter should not request persist")
	}
}

func TestModelPicker_ScopeToggle(t *testing.T) {
	core := uiTestCore(t)
	uiConfigure(t, core, "deepseek")
	uiConfigure(t, core, "anthropic")
	sc := isession.ScopedModels{}
	sc.Set([]string{"deepseek/deepseek-flash"})
	p := newModelPickerUI(core, sc, "deepseek", "deepseek-flash", "", "", "")
	if p.scope != scopeScoped {
		t.Fatal("picker should start scoped when a scope is set")
	}
	if len(p.active) != 1 {
		t.Fatalf("scoped active set should have 1 model, got %d", len(p.active))
	}
	p.handleKey("tab")
	if p.scope != scopeAll {
		t.Fatal("tab should switch to all")
	}
	if len(p.active) < 2 {
		t.Fatalf("all scope should show the configured catalog, got %d", len(p.active))
	}
	// With no scope configured the toggle is inert.
	sc2 := isession.ScopedModels{}
	p2 := newModelPickerUI(core, sc2, "", "", "", "", "")
	if p2.scope != scopeAll {
		t.Fatal("no scope should default to all")
	}
	p2.handleKey("tab")
	if p2.scope != scopeAll {
		t.Fatal("tab should be inert without a scoped set")
	}
}

func TestModelPicker_SearchAndSelect(t *testing.T) {
	core := uiTestCore(t)
	uiConfigure(t, core, "anthropic")
	p := newModelPickerUI(core, isession.ScopedModels{}, "", "", "", "", "")
	// Type "haiku" and confirm the filtered list narrows.
	for _, r := range "haiku" {
		p.handleKey(string(r))
	}
	if len(p.filtered) != 1 || p.filtered[0].ID != "claude-haiku-4-5" {
		t.Fatalf("search failed: %+v", p.filtered)
	}
	sel, doSelect, _, _ := p.handleKey("enter")
	if !doSelect || sel.ID != "claude-haiku-4-5" {
		t.Fatalf("select failed: %+v", sel)
	}
}

func TestModelPicker_DefaultPin(t *testing.T) {
	core := uiTestCore(t)
	uiConfigure(t, core, "deepseek")
	p := newModelPickerUI(core, isession.ScopedModels{}, "", "", "deepseek", "deepseek-flash", "default")
	if len(p.filtered) == 0 {
		t.Fatal("default search should match its model")
	}
	if p.filtered[0].ID != "deepseek-flash" {
		t.Fatalf("default model should pin first, got %s", p.filtered[0].ID)
	}
}

// TestModelPicker_HidesUnconfiguredProviders is the core regression: the picker
// must not offer models whose provider is not configured (Ollama counts as
// configured when reachable, so it may legitimately appear).
func TestModelPicker_HidesUnconfiguredProviders(t *testing.T) {
	core := uiTestCore(t)
	uiConfigure(t, core, "deepseek")
	configured := core.ConfiguredProviders()
	p := newModelPickerUI(core, isession.ScopedModels{}, "", "", "", "", "")
	for _, m := range p.active {
		if !configured[m.Provider] {
			t.Fatalf("picker offered unconfigured provider %q", m.Provider)
		}
	}
	if !p.configured {
		t.Fatal("configured should be true when a provider has a key")
	}
	sawDeepseek := false
	for _, m := range p.active {
		if m.Provider == "deepseek" {
			sawDeepseek = true
		}
	}
	if !sawDeepseek {
		t.Fatal("configured deepseek models should be offered")
	}
}

// TestModelPicker_FallsBackWhenNothingConfigured ensures a fresh install can
// still browse the catalog, with the honest "no providers" warning.
func TestModelPicker_FallsBackWhenNothingConfigured(t *testing.T) {
	core := uiTestCore(t)
	if core.ConfiguredProviders()["ollama"] {
		t.Skip("Ollama is reachable in this environment; fallback path not exercised")
	}
	p := newModelPickerUI(core, isession.ScopedModels{}, "", "", "", "", "")
	if p.configured {
		t.Fatal("configured should be false with no credentials")
	}
	if len(p.active) == 0 {
		t.Fatal("fallback catalog should not be empty")
	}
	v := p.view(100)
	if !strings.Contains(v, "No providers configured") {
		t.Fatalf("fallback warning missing:\n%s", v)
	}
	if strings.Contains(v, "Only showing models from configured providers") {
		t.Fatalf("fallback must not claim it is showing configured providers:\n%s", v)
	}
}

func TestScrollOffset(t *testing.T) {
	if got := scrollOffset(0, 5, 10); got != 0 {
		t.Fatalf("short list offset = %d", got)
	}
	if got := scrollOffset(20, 30, 10); got != 15 {
		t.Fatalf("centered offset = %d", got)
	}
	if got := scrollOffset(29, 30, 10); got != 20 {
		t.Fatalf("clamped offset = %d", got)
	}
}
