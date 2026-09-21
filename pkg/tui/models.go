package tui

import (
	"fmt"
	"strings"

	"github.com/ianclemence/scout/pkg/isession"
	"github.com/ianclemence/scout/pkg/registry"
	"github.com/ianclemence/scout/pkg/runtime"
)

// This file implements Scout's model dialog for the dock-style TUI. Its layout
// and behavior follow the model selector in the Pi coding agent: a bordered
// panel with a live search field, a current/default/provider-sorted list with
// cursor, current-model and default markers, a scroll indicator, the selected
// model's display name, and a background catalog refresh.

// pickerMaxVisible is the number of list rows shown at once (Pi uses 10).
const pickerMaxVisible = 10

// modelPickerUI is the searchable /model selector.
type modelPickerUI struct {
	all        []registry.ModelInfo
	filtered   []registry.ModelInfo
	cur        int
	search     string
	errMsg     string
	curProv    string
	curModel   string
	defProv    string
	defModel   string
	configured bool // whether at least one provider is configured

	// refreshStatus is the background catalog-refresh message (Pi shows one).
	refreshStatus  string
	refreshSuccess bool
	needsRefresh   bool
}

func newModelPickerUI(core *runtime.Core, curProv, curModel, defProv, defModel, initial string) *modelPickerUI {
	available := isession.AvailableModels(core)
	configured := len(available) > 0
	all := available
	if !configured {
		// Nothing configured yet: show the full catalog so the picker is still
		// useful for browsing, and the view says why.
		all = isession.AllModels(core)
	}
	u := &modelPickerUI{
		all:          all,
		curProv:      curProv,
		curModel:     curModel,
		defProv:      defProv,
		defModel:     defModel,
		search:       initial,
		configured:   configured,
		needsRefresh: true,
	}
	u.reload(all)
	if initial != "" {
		u.rebuild()
	} else {
		u.filtered = u.all
		u.selectCurrent()
	}
	return u
}

// reload replaces the catalog (after an initial snapshot or a refresh).
func (u *modelPickerUI) reload(all []registry.ModelInfo) {
	u.all = u.sortModels(all)
}

func (u *modelPickerUI) sortModels(models []registry.ModelInfo) []registry.ModelInfo {
	sorted := append([]registry.ModelInfo{}, models...)
	sortSliceStable(sorted, func(a, b registry.ModelInfo) bool {
		aCur, bCur := u.isCurrent(a), u.isCurrent(b)
		if aCur != bCur {
			return aCur
		}
		aDef, bDef := u.isDefault(a), u.isDefault(b)
		if aDef != bDef {
			return aDef
		}
		return a.Provider < b.Provider
	})
	return sorted
}

func (u *modelPickerUI) selectCurrent() {
	idx := -1
	for i, m := range u.filtered {
		if u.isCurrent(m) {
			idx = i
			break
		}
	}
	if idx >= 0 {
		u.cur = idx
		return
	}
	if u.cur >= len(u.filtered) {
		u.cur = maxInt(0, len(u.filtered)-1)
	}
	if u.cur < 0 {
		u.cur = 0
	}
}

// modelSearchText mirrors Pi's selector search text: provider-prefixed forms
// first so "provider/id" ranks above a proxy id that merely contains it.
func modelSearchText(m registry.ModelInfo) string {
	name := ""
	if m.DisplayName != "" {
		name = " " + m.DisplayName
	}
	return m.Provider + " " + m.Provider + "/" + m.ID + " " + m.Provider + " " + m.ID + name
}

func (u *modelPickerUI) isDefaultSearch(q string) bool {
	q = strings.ToLower(strings.TrimSpace(q))
	return q != "" && strings.HasPrefix("default", q)
}

func (u *modelPickerUI) rebuild() {
	q := strings.TrimSpace(u.search)
	if q == "" {
		u.filtered = u.all
	} else {
		filtered := fuzzyFilter(u.all, q, modelSearchText)
		if u.isDefaultSearch(q) {
			var defs []registry.ModelInfo
			keys := map[string]bool{}
			for _, m := range u.all {
				if u.isDefault(m) {
					defs = append(defs, m)
					keys[m.Provider+"\x00"+m.ID] = true
				}
			}
			rest := make([]registry.ModelInfo, 0, len(filtered))
			for _, m := range filtered {
				if !keys[m.Provider+"\x00"+m.ID] {
					rest = append(rest, m)
				}
			}
			u.filtered = append(defs, rest...)
		} else {
			u.filtered = filtered
		}
	}
	if q != "" {
		u.cur = 0
	} else {
		u.selectCurrent()
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

// handleKey processes a key. It returns the model to switch to (zero value
// when none), whether Ctrl+S requested making it the default, and whether the
// selector was dismissed.
func (u *modelPickerUI) handleKey(key string) (sel registry.ModelInfo, doSelect, setDefault, cancel bool) {
	switch key {
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
	if width < 20 {
		width = 20
	}
	rule := stylePromptBar.Render(strings.Repeat("─", width))
	var b strings.Builder
	b.WriteString(rule + "\n\n")
	if u.configured {
		b.WriteString(styleModelScopeHint.Render("Only showing models from configured providers. Use /login to add providers.") + "\n")
	} else {
		b.WriteString(styleModelScopeWarn.Render("No providers configured — showing all known models. Use /login to add providers.") + "\n")
	}
	b.WriteString("\n")
	b.WriteString(styleModelSearch.Render(u.search) + "\n\n")

	if u.errMsg != "" {
		for _, ln := range strings.Split(u.errMsg, "\n") {
			b.WriteString(styleError.Render(ln) + "\n")
		}
	} else if len(u.filtered) == 0 {
		b.WriteString(stylePaletteNoMatch.Render("  No matching models") + "\n")
	} else {
		start := scrollOffset(u.cur, len(u.filtered), pickerMaxVisible)
		end := minInt(start+pickerMaxVisible, len(u.filtered))
		for i := start; i < end; i++ {
			m := u.filtered[i]
			selected := i == u.cur
			cursor := "  "
			if selected {
				cursor = stylePaletteSel.Render("→ ")
			}
			mark := "  "
			if u.isCurrent(m) {
				mark = styleModelEnabled.Render("✓ ")
			}
			modelText := m.ID
			if selected {
				modelText = stylePaletteSel.Render(modelText)
			}
			badge := stylePaletteDesc.Render(" [" + m.Provider + "]")
			def := ""
			if u.isDefault(m) {
				def = stylePaletteDesc.Render(" · default")
			}
			b.WriteString(cursor + mark + modelText + badge + def + "\n")
		}
		if start > 0 || end < len(u.filtered) {
			b.WriteString(stylePaletteScroll.Render(fmt.Sprintf("  (%d/%d)", u.cur+1, len(u.filtered))) + "\n")
		}
		b.WriteString("\n" + stylePaletteDesc.Render("  Model Name: "+displayName(u.filtered[u.cur])) + "\n")
	}
	if u.refreshStatus != "" {
		style := stylePaletteDesc
		if u.refreshSuccess {
			style = styleModelEnabled
		}
		b.WriteString("\n" + style.Render("  "+u.refreshStatus) + "\n")
	}
	b.WriteString("\n" + styleModelScopeFooter.Render("  enter to select · ctrl+s to set as default · esc to cancel") + "\n")
	b.WriteString(rule)
	return b.String()
}

// ---------- helpers ----------

func displayName(m registry.ModelInfo) string {
	if m.DisplayName != "" {
		return m.DisplayName
	}
	return m.ID
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// sortSliceStable sorts in place, keeping equal elements in order.
func sortSliceStable[T any](s []T, less func(a, b T) bool) {
	// Insertion sort is fine for the catalog sizes Scout deals with and keeps
	// the "current then default" ordering stable without a comparator adapter.
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && less(s[j], s[j-1]); j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
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
