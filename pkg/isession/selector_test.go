package isession

import "testing"

// TestExactModelResolvesUnconfigured verifies that an explicitly named model is
// still resolvable even when its provider has no credentials (Pi keeps the full
// catalog for exact-reference resolution).
func TestExactModelResolvesUnconfigured(t *testing.T) {
	core := testCore(t)
	ctx := &SessionCtx{Core: core}
	if !exactModelExists(ctx, "anthropic", "claude-haiku-4-5") {
		t.Fatal("exact reference to an unconfigured provider's builtin should resolve")
	}
	if exactModelExists(ctx, "nope", "does-not-exist") {
		t.Fatal("unknown model must not resolve")
	}
}

// TestSelectorModelsExcludesUnconfigured verifies the /model selector offers
// only configured providers' models.
func TestSelectorModelsExcludesUnconfigured(t *testing.T) {
	core := testCore(t)
	configureProvider(t, core, "deepseek")
	ctx := &SessionCtx{Core: core}
	configured := core.ConfiguredProviders()
	for _, m := range selectorModels(ctx) {
		if !configured[m.Provider] {
			t.Fatalf("selector offered unconfigured provider %q", m.Provider)
		}
	}
}
