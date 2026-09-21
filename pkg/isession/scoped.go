package isession

import (
	"encoding/json"
	"sort"

	"github.com/ianclemence/scout/pkg/registry"
	"github.com/ianclemence/scout/pkg/runtime"
)

// scopedModelsSetting is the app_settings key holding the persisted ordered
// list of enabled model ids ("provider/model"). Absent = all models enabled.
const scopedModelsSetting = "scoped_models"

// ScopedModels holds the session-local view of which models are enabled for
// model cycling. A nil slice means "all enabled" (no filter); a non-nil slice
// is an explicit ordered allow-list of "provider/model" ids.
type ScopedModels struct {
	ids []string // nil = all enabled
}

// AllEnabled reports whether no explicit filter is active.
func (s ScopedModels) AllEnabled() bool { return s.ids == nil }

// IDs returns a copy of the explicit ordered ids, or nil when all are enabled.
func (s ScopedModels) IDs() []string {
	if s.ids == nil {
		return nil
	}
	out := make([]string, len(s.ids))
	copy(out, s.ids)
	return out
}

// Set replaces the enabled set. A nil slice means all enabled.
func (s *ScopedModels) Set(ids []string) {
	if ids == nil {
		s.ids = nil
		return
	}
	cp := make([]string, len(ids))
	copy(cp, ids)
	s.ids = cp
}

// IsEnabled reports whether id is part of the active set.
func (s ScopedModels) IsEnabled(id string) bool {
	if s.ids == nil {
		return true
	}
	for _, x := range s.ids {
		if x == id {
			return true
		}
	}
	return false
}

// LoadScopedModels reads the persisted scoped-model selection from the store.
// A missing or malformed value yields the all-enabled default.
func LoadScopedModels(core *runtime.Core) ScopedModels {
	var sc ScopedModels
	v, ok, err := core.DB.GetSetting(scopedModelsSetting)
	if err != nil || !ok {
		return sc
	}
	var ids []string
	if json.Unmarshal([]byte(v), &ids) != nil {
		return sc
	}
	// An explicit list that covers every available model collapses to "all".
	all := allModelIDs(core)
	if coversAll(ids, all) {
		return sc
	}
	sc.ids = ids
	return sc
}

// SaveScopedModels persists the explicit selection. A nil list clears the
// key so the default (all enabled) applies again.
func SaveScopedModels(core *runtime.Core, ids []string) error {
	if ids == nil {
		return core.DB.DeleteSetting(scopedModelsSetting)
	}
	raw, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	return core.DB.SetSetting(scopedModelsSetting, string(raw))
}

// allModelIDs returns every registry model as "provider/id", sorted. This is
// the full catalog (AllModels), used to normalize the scoped list.
func allModelIDs(core *runtime.Core) []string {
	ms := AllModels(core)
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.Provider+"/"+m.ID)
	}
	sort.Strings(out)
	return out
}

func coversAll(ids, all []string) bool {
	if len(all) == 0 {
		return false
	}
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}
	for _, id := range all {
		if !set[id] {
			return false
		}
	}
	return true
}

// AllModels returns the full registry catalog — builtins plus cached/discovered
// rows plus live Ollama tags — sorted by provider then id. This is the floor of
// what Scout knows about, and is used to resolve exact references and to
// normalize the scoped selection.
func AllModels(core *runtime.Core) []registry.ModelInfo {
	return core.Registry().List(ctxBg(), "")
}

// AvailableModels returns the subset of the catalog whose provider is actually
// configured right now (credentials present, Ollama reachable, or a compatible
// endpoint set). It may be empty when nothing is configured; callers that want
// a browsing fallback should use AllModels explicitly. This is what the model
// selectors offer by default, so the "configured providers" claim is true and
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

// FilterScoped returns the models eligible for cycling given the scoped
// selection. When all are enabled (or the explicit list references nothing
// available) the full catalog is returned.
func FilterScoped(models []registry.ModelInfo, sc ScopedModels) []registry.ModelInfo {
	if sc.AllEnabled() {
		return models
	}
	byID := map[string]registry.ModelInfo{}
	for _, m := range models {
		byID[m.Provider+"/"+m.ID] = m
	}
	out := make([]registry.ModelInfo, 0, len(sc.ids))
	for _, id := range sc.ids {
		if m, ok := byID[id]; ok {
			out = append(out, m)
		}
	}
	if len(out) == 0 {
		return models
	}
	return out
}

// CycleModel returns the next eligible model after the current one, honoring
// the scoped ordering. It returns ok=false when there is nothing to cycle to.
func CycleModel(models []registry.ModelInfo, sc ScopedModels, curProvider, curModel string) (registry.ModelInfo, bool) {
	eligible := FilterScoped(models, sc)
	if len(eligible) < 2 {
		return registry.ModelInfo{}, false
	}
	cur := -1
	for i, m := range eligible {
		if m.Provider == curProvider && m.ID == curModel {
			cur = i
			break
		}
	}
	next := eligible[(cur+1+len(eligible))%len(eligible)]
	return next, true
}
