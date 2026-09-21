package tui

import (
	"fmt"
	"strings"

	"github.com/ianclemence/scout/pkg/isession"
	"github.com/ianclemence/scout/pkg/registry"
	"github.com/ianclemence/scout/pkg/runtime"
)

// This file implements Scout's two model dialogs for the dock-style TUI:
//
//   * scopedModelsUI  — /scoped-models: enable/disable and reorder the models
//     available for Ctrl+P cycling. Session-only until Ctrl+S persists.
//   * modelPickerUI   — /model: a searchable selector with an all/scoped scope
//     toggle (Tab), Enter to switch the session model, Ctrl+S to set default.
//
// They render in the dock below the composer, so they are modeled here as
// plain state structs driven by handleKey and rendered by view.

// Row caps for the model dialogs.
const (
	scopedMaxVisible = 10
	pickerMaxVisible = 10
)

// ---------- scoped models (/scoped-models) ----------

// scopedModelsUI is the interactive enable/disable + reorder selector.
type scopedModelsUI struct {
	all      []registry.ModelInfo
	byID     map[string]registry.ModelInfo
	ids      []string // explicit ordered enabled list; nil = all enabled
	search   string
	filtered []string // full ids after search filtering
	cur      int
	dirty    bool
	curProv  string
	curModel string
}

func newScopedModelsUI(core *runtime.Core, sc isession.ScopedModels, curProv, curModel string) *scopedModelsUI {
	// The scoped selector lists the full catalog so any model can be enabled,
	// even ones whose provider will be configured later.
	all := isession.AllModels(core)
	u := &scopedModelsUI{
		all:      all,
		byID:     map[string]registry.ModelInfo{},
		curProv:  curProv,
		curModel: curModel,
	}
	for _, m := range all {
		u.byID[m.Provider+"/"+m.ID] = m
	}
	u.ids = sc.IDs() // nil when all enabled
	u.rebuild()
	return u
}

// ordered returns the enabled ids first (in their explicit order) followed by
// the remaining available models.
func (u *scopedModelsUI) ordered() []string {
	all := make([]string, 0, len(u.all))
	for _, m := range u.all {
		all = append(all, m.Provider+"/"+m.ID)
	}
	if u.ids == nil {
		return all
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(all))
	for _, id := range u.ids {
		out = append(out, id)
		seen[id] = true
	}
	for _, id := range all {
		if !seen[id] {
			out = append(out, id)
		}
	}
	return out
}

func (u *scopedModelsUI) rebuild() {
	items := u.ordered()
	q := strings.ToLower(strings.TrimSpace(u.search))
	if q == "" {
		u.filtered = items
	} else {
		var out []string
		for _, id := range items {
			m, ok := u.byID[id]
			hay := id
			if ok {
				hay = id + " " + m.DisplayName
			}
			if strings.Contains(strings.ToLower(hay), q) {
				out = append(out, id)
			}
		}
		u.filtered = out
	}
	if u.cur >= len(u.filtered) {
		u.cur = maxInt(0, len(u.filtered)-1)
	}
	if u.cur < 0 {
		u.cur = 0
	}
}

func (u *scopedModelsUI) enabled(id string) bool {
	if u.ids == nil {
		return true
	}
	for _, x := range u.ids {
		if x == id {
			return true
		}
	}
	return false
}

func (u *scopedModelsUI) toggle(id string) {
	if u.ids == nil {
		// Collapse "all" to an explicit list minus the toggled id.
		next := make([]string, 0, len(u.filtered))
		for _, x := range u.ordered() {
			if x != id {
				next = append(next, x)
			}
		}
		u.ids = u.normalize(next)
		return
	}
	idx := indexOf(u.ids, id)
	if idx >= 0 {
		u.ids = append(append([]string{}, u.ids[:idx]...), u.ids[idx+1:]...)
	} else {
		u.ids = u.normalize(append(append([]string{}, u.ids...), id))
	}
}

func (u *scopedModelsUI) enableAll(targets []string) {
	if u.ids == nil {
		return
	}
	if targets == nil {
		// Enable everything.
		u.ids = nil
		return
	}
	next := append([]string{}, u.ids...)
	for _, id := range targets {
		if indexOf(next, id) < 0 {
			next = append(next, id)
		}
	}
	u.ids = u.normalize(next)
}

func (u *scopedModelsUI) clearAll(targets []string) {
	if u.ids == nil {
		if targets == nil {
			u.ids = []string{}
			return
		}
		// Clearing a filtered subset from "all": everything else stays.
		drop := map[string]bool{}
		for _, id := range targets {
			drop[id] = true
		}
		next := []string{}
		for _, id := range u.ordered() {
			if !drop[id] {
				next = append(next, id)
			}
		}
		u.ids = next
		return
	}
	drop := map[string]bool{}
	if targets == nil {
		for _, id := range u.ids {
			drop[id] = true
		}
	} else {
		for _, id := range targets {
			drop[id] = true
		}
	}
	next := []string{}
	for _, id := range u.ids {
		if !drop[id] {
			next = append(next, id)
		}
	}
	u.ids = u.normalize(next)
}

// normalize collapses to nil (all enabled) when the list covers every model.
func (u *scopedModelsUI) normalize(ids []string) []string {
	// De-duplicate, preserving order.
	seen := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if len(out) == len(u.all) {
		covered := true
		for _, m := range u.all {
			if indexOf(out, m.Provider+"/"+m.ID) < 0 {
				covered = false
				break
			}
		}
		if covered {
			return nil
		}
	}
	return out
}

func (u *scopedModelsUI) move(delta int) {
	if u.ids == nil || len(u.filtered) == 0 {
		return
	}
	id := u.filtered[u.cur]
	idx := indexOf(u.ids, id)
	if idx < 0 {
		return
	}
	ni := idx + delta
	if ni < 0 || ni >= len(u.ids) {
		return
	}
	u.ids[idx], u.ids[ni] = u.ids[ni], u.ids[idx]
	u.cur += delta
	u.dirty = true
}

func (u *scopedModelsUI) toggleProvider() {
	if len(u.filtered) == 0 {
		return
	}
	m, ok := u.byID[u.filtered[u.cur]]
	if !ok {
		return
	}
	prov := m.Provider
	targets := []string{}
	allEnabled := true
	for _, x := range u.all {
		if x.Provider == prov {
			id := x.Provider + "/" + x.ID
			targets = append(targets, id)
			if !u.enabled(id) {
				allEnabled = false
			}
		}
	}
	if allEnabled {
		u.clearAll(targets)
	} else {
		u.enableAll(targets)
	}
}

func (u *scopedModelsUI) footer() string {
	enabled := len(u.all)
	if u.ids != nil {
		enabled = 0
		for _, id := range u.ids {
			if _, ok := u.byID[id]; ok {
				enabled++
			}
		}
	}
	unavail := 0
	if u.ids != nil {
		for _, id := range u.ids {
			if _, ok := u.byID[id]; !ok {
				unavail++
			}
		}
	}
	count := "all enabled"
	if u.ids != nil {
		count = fmt.Sprintf("%d/%d enabled", enabled, len(u.all))
		if unavail > 0 {
			count += fmt.Sprintf(" · %d unavailable", unavail)
		}
	}
	parts := []string{
		"enter toggle", "ctrl+a all", "ctrl+x clear", "ctrl+p provider",
		"alt+↑/↓ reorder", "ctrl+s save", count,
	}
	if u.dirty {
		return "  " + strings.Join(parts, " · ") + " (unsaved)"
	}
	return "  " + strings.Join(parts, " · ")
}

// handleScopedKey processes a key for the scoped-models selector. The returned
// persist flag is true when Ctrl+S was pressed.
func (u *scopedModelsUI) handleKey(key string) (persist bool) {
	switch key {
	case "up":
		if len(u.filtered) > 0 {
			u.cur = (u.cur - 1 + len(u.filtered)) % len(u.filtered)
		}
	case "down":
		if len(u.filtered) > 0 {
			u.cur = (u.cur + 1) % len(u.filtered)
		}
	case "alt+up":
		u.move(-1)
	case "alt+down":
		u.move(1)
	case "enter", "tab":
		if len(u.filtered) > 0 {
			u.toggle(u.filtered[u.cur])
			u.dirty = true
			u.rebuild()
		}
	case "ctrl+a":
		if strings.TrimSpace(u.search) != "" {
			u.enableAll(append([]string{}, u.filtered...))
		} else {
			u.enableAll(nil)
		}
		u.dirty = true
		u.rebuild()
	case "ctrl+x":
		if strings.TrimSpace(u.search) != "" {
			u.clearAll(append([]string{}, u.filtered...))
		} else {
			u.clearAll(nil)
		}
		u.dirty = true
		u.rebuild()
	case "ctrl+p":
		u.toggleProvider()
		u.dirty = true
		u.rebuild()
	case "ctrl+s":
		return true
	case "backspace":
		if u.search != "" {
			r := []rune(u.search)
			u.search = string(r[:len(r)-1])
			u.rebuild()
		}
	default:
		if isPrintableKey(key) {
			u.search += printableText(key)
			u.rebuild()
		}
	}
	return false
}

// view renders the scoped-models selector as dock lines.
func (u *scopedModelsUI) view(width int) string {
	var b strings.Builder
	b.WriteString(styleModelScopeTitle.Render("Model Configuration") + "\n")
	b.WriteString(styleModelScopeHint.Render("Session-only. ctrl+s to save to settings.") + "\n")
	b.WriteString(styleModelSearch.Render("  /"+u.search+"_") + "\n")

	if len(u.filtered) == 0 {
		b.WriteString(stylePaletteNoMatch.Render("  No matching models") + "\n")
	} else {
		off := scrollOffset(u.cur, len(u.filtered), scopedMaxVisible)
		end := minInt(off+scopedMaxVisible, len(u.filtered))
		for i := off; i < end; i++ {
			id := u.filtered[i]
			mark := "  "
			if u.enabled(id) {
				mark = styleModelEnabled.Render("✓ ")
			}
			line := "  "
			if i == u.cur {
				line = stylePaletteSel.Render("→ ")
			}
			// Unavailable models: keep the explicit id but mark them.
			name := id
			badge := stylePaletteDesc.Render(" [" + providerOf(id) + "]")
			if _, ok := u.byID[id]; !ok {
				name = styleModelUnavail.Render(id)
				badge = stylePaletteDesc.Render(" [unavailable]")
			} else if i == u.cur {
				name = stylePaletteSel.Render(name)
			}
			b.WriteString(line + mark + name + badge)
			if id == u.curProv+"/"+u.curModel {
				b.WriteString(styleModelCurrent.Render("  ← current"))
			}
			b.WriteString("\n")
		}
		if off > 0 || end < len(u.filtered) {
			b.WriteString(stylePaletteScroll.Render(fmt.Sprintf("  (%d/%d)", u.cur+1, len(u.filtered))) + "\n")
		}
		if len(u.filtered) > 0 {
			if m, ok := u.byID[u.filtered[u.cur]]; ok {
				b.WriteString("\n" + stylePaletteDesc.Render("  Model Name: "+displayName(m)) + "\n")
			}
		}
	}
	b.WriteString(styleModelScopeFooter.Render(u.footer()))
	return strings.TrimRight(b.String(), "\n")
}

// ---------- model picker (/model) ----------

type modelScope int

const (
	scopeAll modelScope = iota
	scopeScoped
)

// modelPickerUI is the searchable /model selector with an all/scoped toggle.
// "all" means every model from a configured provider (the usable catalog);
// "scoped" is the user's enabled subset. Unconfigured providers are never
// offered here — they remain resolvable by exact reference via AllModels.
type modelPickerUI struct {
	all        []registry.ModelInfo // configured-provider catalog ("all" scope)
	scoped     []registry.ModelInfo // scoped subset, intersected with configured
	active     []registry.ModelInfo
	filtered   []registry.ModelInfo
	cur        int
	scope      modelScope
	search     string
	errMsg     string
	curProv    string
	curModel   string
	defProv    string
	defModel   string
	configured bool // whether at least one provider is configured
}

func newModelPickerUI(core *runtime.Core, sc autoScope, curProv, curModel, defProv, defModel, initial string) *modelPickerUI {
	available := isession.AvailableModels(core)
	configured := len(available) > 0
	all := available
	if !configured {
		// Nothing configured yet: show the full catalog so the picker is still
		// useful for browsing, and the view says why.
		all = isession.AllModels(core)
	}
	u := &modelPickerUI{
		all:        all,
		curProv:    curProv,
		curModel:   curModel,
		defProv:    defProv,
		defModel:   defModel,
		search:     initial,
		configured: configured,
	}
	if !sc.AllEnabled() {
		u.scoped = isession.FilterScoped(all, sc)
		u.scope = scopeScoped
	}
	if len(u.scoped) == 0 {
		u.scope = scopeAll
	}
	u.applyScope()
	u.rebuild()
	return u
}

// autoScope is the scoped-model state passed into the picker.
type autoScope = isession.ScopedModels

func (u *modelPickerUI) applyScope() {
	if u.scope == scopeScoped && len(u.scoped) > 0 {
		u.active = u.scoped
	} else {
		u.active = u.all
	}
}

func (u *modelPickerUI) rebuild() {
	q := strings.ToLower(strings.TrimSpace(u.search))
	var base []registry.ModelInfo
	if q == "" {
		base = u.active
	} else {
		for _, m := range u.active {
			hay := strings.ToLower(m.Provider + "/" + m.ID + " " + m.DisplayName)
			if strings.Contains(hay, q) {
				base = append(base, m)
			}
		}
	}
	// "default" search: filter normally, but pin default-model matches to the
	// top even when the literal query does not match their text.
	if q != "" && strings.HasPrefix("default", q) {
		var defs []registry.ModelInfo
		seen := map[string]bool{}
		for _, m := range u.active {
			if u.isDefault(m) {
				defs = append(defs, m)
				seen[m.Provider+"\x00"+m.ID] = true
			}
		}
		rest := make([]registry.ModelInfo, 0, len(base))
		for _, m := range base {
			if !seen[m.Provider+"\x00"+m.ID] {
				rest = append(rest, m)
			}
		}
		base = append(defs, rest...)
	}
	u.filtered = base
	if q != "" {
		u.cur = 0
	} else if u.cur >= len(u.filtered) {
		u.cur = maxInt(0, len(u.filtered)-1)
	}
	if u.cur < 0 {
		u.cur = 0
	}
}

func (u *modelPickerUI) isCurrent(m registry.ModelInfo) bool {
	return m.Provider == u.curProv && m.ID == u.curModel
}

func (u *modelPickerUI) isDefault(m registry.ModelInfo) bool {
	if u.defProv == "" || u.defModel == "" {
		return false
	}
	return m.Provider == u.defProv && m.ID == u.defModel
}

func (u *modelPickerUI) scopeText() string {
	allS, scS := "all", "scoped"
	if u.scope == scopeAll {
		allS = styleModelScopeActive.Render("all")
		scS = styleModelScopeInactive.Render("scoped")
	} else {
		allS = styleModelScopeInactive.Render("all")
		scS = styleModelScopeActive.Render("scoped")
	}
	return styleModelScopeHint.Render("Scope: ") + allS + styleModelScopeHint.Render(" | ") + scS
}

// handlePickerKey processes a key. It returns:
//   - selectModel: the model to switch to (zero value when none)
//   - setDefault:  true when Ctrl+S requested making it the default
//   - cancel:      true when the selector was dismissed
func (u *modelPickerUI) handleKey(key string) (sel registry.ModelInfo, doSelect, setDefault, cancel bool) {
	switch key {
	case "tab":
		if len(u.scoped) > 0 {
			if u.scope == scopeAll {
				u.scope = scopeScoped
			} else {
				u.scope = scopeAll
			}
			u.applyScope()
			u.rebuild()
		}
	case "up":
		if len(u.filtered) > 0 {
			u.cur = (u.cur - 1 + len(u.filtered)) % len(u.filtered)
		}
	case "down":
		if len(u.filtered) > 0 {
			u.cur = (u.cur + 1) % len(u.filtered)
		}
	case "enter":
		if len(u.filtered) > 0 {
			return u.filtered[u.cur], true, false, false
		}
	case "ctrl+s":
		if len(u.filtered) > 0 {
			return u.filtered[u.cur], false, true, false
		}
	case "esc", "ctrl+c":
		return registry.ModelInfo{}, false, false, true
	case "backspace":
		if u.search != "" {
			r := []rune(u.search)
			u.search = string(r[:len(r)-1])
			u.rebuild()
		}
	default:
		if isPrintableKey(key) {
			u.search += printableText(key)
			u.rebuild()
		}
	}
	return registry.ModelInfo{}, false, false, false
}

func (u *modelPickerUI) view(width int) string {
	var b strings.Builder
	switch {
	case len(u.scoped) > 0:
		b.WriteString(u.scopeText() + "\n")
		b.WriteString(styleModelScopeHint.Render("  tab scope (all/scoped)") + "\n")
	case u.configured:
		// "all" scope, configured providers present: the list really is the
		// configured subset.
		b.WriteString(styleModelScopeHint.Render("Showing models from configured providers.") + "\n")
	default:
		// Nothing configured: we fall back to the full catalog, and say so.
		b.WriteString(styleModelScopeWarn.Render("No providers configured — showing all known models. Use /login to add providers.") + "\n")
	}
	b.WriteString(styleModelSearch.Render("  /"+u.search+"_") + "\n")

	if u.errMsg != "" {
		b.WriteString(styleError.Render("  "+u.errMsg) + "\n")
	}
	if len(u.filtered) == 0 {
		b.WriteString(stylePaletteNoMatch.Render("  No matching models") + "\n")
	} else {
		off := scrollOffset(u.cur, len(u.filtered), pickerMaxVisible)
		end := minInt(off+pickerMaxVisible, len(u.filtered))
		for i := off; i < end; i++ {
			m := u.filtered[i]
			cursor := "  "
			if i == u.cur {
				cursor = stylePaletteSel.Render("→ ")
			}
			curMark := "  "
			if u.isCurrent(m) {
				curMark = styleModelEnabled.Render("✓ ")
			}
			name := m.ID
			if i == u.cur {
				name = stylePaletteSel.Render(name)
			}
			badge := stylePaletteDesc.Render(" [" + m.Provider + "]")
			def := ""
			if u.isDefault(m) {
				def = stylePaletteDesc.Render(" · default")
			}
			b.WriteString(cursor + curMark + name + badge + def + "\n")
		}
		if off > 0 || end < len(u.filtered) {
			b.WriteString(stylePaletteScroll.Render(fmt.Sprintf("  (%d/%d)", u.cur+1, len(u.filtered))) + "\n")
		}
		b.WriteString("\n" + stylePaletteDesc.Render("  Model Name: "+displayName(u.filtered[u.cur])) + "\n")
	}
	b.WriteString(styleModelScopeFooter.Render("  enter select · ctrl+s set default · esc cancel"))
	return strings.TrimRight(b.String(), "\n")
}

// ---------- helpers ----------

func displayName(m registry.ModelInfo) string {
	if m.DisplayName != "" {
		return m.DisplayName
	}
	return m.ID
}

func providerOf(id string) string {
	if i := strings.Index(id, "/"); i >= 0 {
		return id[:i]
	}
	return ""
}

func scrollOffset(cur, total, maxVisible int) int {
	if total <= maxVisible {
		return 0
	}
	off := cur - maxVisible/2
	if off+maxVisible > total {
		off = total - maxVisible
	}
	if off < 0 {
		off = 0
	}
	return off
}

func indexOf(xs []string, x string) int {
	for i, s := range xs {
		if s == x {
			return i
		}
	}
	return -1
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// isPrintableKey reports whether a key string is printable text that should be
// appended to a search field. It accepts a single rune and also multi-rune
// strings (pasted text), while rejecting control/navigation keys. The named
// "space" key maps to a real space so searches can contain spaces.
func isPrintableKey(key string) bool {
	if key == "" {
		return false
	}
	if key == "space" {
		return true
	}
	if strings.HasPrefix(key, "ctrl+") || strings.HasPrefix(key, "alt+") {
		return false
	}
	switch key {
	case "enter", "tab", "esc", "backspace", "up", "down", "left", "right", "delete", "home", "end", "pgup", "pgdown":
		return false
	}
	r := []rune(key)
	if len(r) == 0 {
		return false
	}
	// Reject control characters; accept everything else.
	for _, c := range r {
		if c < 0x20 || c == 0x7f {
			return false
		}
	}
	return true
}

// printableText normalizes a printable key into the text it contributes.
func printableText(key string) string {
	if key == "space" {
		return " "
	}
	return key
}
