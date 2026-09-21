package isession

import (
	"strings"

	"github.com/ianclemence/scout/pkg/registry"
	"github.com/ianclemence/scout/pkg/runtime"
)

// findExactModel resolves an exact model reference the way the Pi agent does:
// first the canonical "provider/id", then "provider/id" split forms, then a
// bare id. Ambiguous references (more than one match) do not resolve.
func findExactModel(models []registry.ModelInfo, ref string) (registry.ModelInfo, bool) {
	r := strings.ToLower(strings.TrimSpace(ref))
	if r == "" {
		return registry.ModelInfo{}, false
	}
	single := func(matches []registry.ModelInfo) (registry.ModelInfo, bool) {
		if len(matches) == 1 {
			return matches[0], true
		}
		return registry.ModelInfo{}, false
	}
	var canon []registry.ModelInfo
	for _, m := range models {
		if strings.ToLower(m.Provider+"/"+m.ID) == r {
			canon = append(canon, m)
		}
	}
	if len(canon) > 0 {
		return single(canon)
	}
	if i := strings.Index(ref, "/"); i != -1 {
		p := strings.ToLower(strings.TrimSpace(ref[:i]))
		id := strings.ToLower(strings.TrimSpace(ref[i+1:]))
		var matches []registry.ModelInfo
		for _, m := range models {
			if strings.ToLower(m.Provider) == p && strings.ToLower(m.ID) == id {
				matches = append(matches, m)
			}
		}
		if len(matches) > 0 {
			return single(matches)
		}
	}
	var ids []registry.ModelInfo
	for _, m := range models {
		if strings.ToLower(m.ID) == r {
			ids = append(ids, m)
		}
	}
	return single(ids)
}

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
