package isession

import (
	"github.com/ianclemence/scout/pkg/registry"
	"github.com/ianclemence/scout/pkg/runtime"
)

// AllModels returns the full registry catalog — builtins plus cached/discovered
// rows plus live Ollama tags — sorted by provider then id. This is the floor of
// what Scout knows about, and is used to resolve exact model references.
func AllModels(core *runtime.Core) []registry.ModelInfo {
	return core.Registry().List(ctxBg(), "")
}

// AvailableModels returns the subset of the catalog whose provider is actually
// configured right now (credentials present, Ollama reachable, or a compatible
// endpoint set). This is what the model selector offers by default, so
// unusable models are not selectable.
func AvailableModels(core *runtime.Core) []registry.ModelInfo {
	all := AllModels(core)
	configured := core.ConfiguredProviders()
	out := make([]registry.ModelInfo, 0, len(all))
	for _, m := range all {
		if configured[m.Provider] {
			out = append(out, m)
		}
	}
	return out
}
