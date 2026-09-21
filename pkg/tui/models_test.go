package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ianclemence/scout/pkg/config"
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

func TestModelPicker_SearchAndSelect(t *testing.T) {
	core := uiTestCore(t)
	uiConfigure(t, core, "anthropic")
	p := newModelPickerUI(core, "", "", "", "", "")
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
	p := newModelPickerUI(core, "", "", "deepseek", "deepseek-flash", "default")
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
	p := newModelPickerUI(core, "", "", "", "", "")
	for _, m := range p.all {
		if !configured[m.Provider] {
			t.Fatalf("picker offered unconfigured provider %q", m.Provider)
		}
	}
	if !p.configured {
		t.Fatal("configured should be true when a provider has a key")
	}
	sawDeepseek := false
	for _, m := range p.all {
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
	p := newModelPickerUI(core, "", "", "", "", "")
	if p.configured {
		t.Fatal("configured should be false with no credentials")
	}
	if len(p.all) == 0 {
		t.Fatal("fallback catalog should not be empty")
	}
	v := p.view(100)
	if !strings.Contains(v, "No providers configured") {
		t.Fatalf("fallback warning missing:\n%s", v)
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

// TestModelPickerViewPiLayout locks the Pi-style selector layout: bordered
// panel, configured-providers hint, cursor/current/default markers, provider
// badge, Model Name line, and the key hint.
func TestModelPickerViewPiLayout(t *testing.T) {
	core := uiTestCore(t)
	uiConfigure(t, core, "deepseek")
	p := newModelPickerUI(core, "deepseek", "deepseek-flash", "deepseek", "deepseek-flash", "")
	v := p.view(100)
	for _, want := range []string{
		"─",
		"Only showing models from configured providers",
		"→ ",
		"✓ ",
		"[deepseek]",
		"· default",
		"Model Name:",
		"enter to select",
		"esc to cancel",
	} {
		if !strings.Contains(v, want) {
			t.Fatalf("picker view missing %q:\n%s", want, v)
		}
	}
}
