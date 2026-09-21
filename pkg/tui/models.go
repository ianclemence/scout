package tui

import (
	"fmt"
	"strings"

	"github.com/ianclemence/scout/pkg/isession"
	"github.com/ianclemence/scout/pkg/registry"
	"github.com/ianclemence/scout/pkg/runtime"
)

// This file implements Scout's model dialog for the dock-style TUI:
//
//   * modelPickerUI — /model: a searchable selector. Enter switches the
//     session model; Ctrl+S also sets it as the default.
//
// It renders in the dock below the composer, so it is modeled as a plain state
// struct driven by handleKey and rendered by view.

// Row cap for the model dialog.
const pickerMaxVisible = 10

// ---------- model picker (/model) ----------

// modelPickerUI is the searchable /model selector. It offers configured-
// provider models when any provider is configured, and falls back to the full
// catalog (with a note) when nothing is.
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
		all:        all,
		curProv:    curProv,
		curModel:   curModel,
		defProv:    defProv,
		defModel:   defModel,
		search:     initial,
		configured: configured,
	}
	u.rebuild()
	return u
}

func (u *modelPickerUI) rebuild() {
	q := strings.ToLower(strings.TrimSpace(u.search))
	var base []registry.ModelInfo
	if q == "" {
		base = u.all
	} else {
		for _, m := range u.all {
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
		for _, m := range u.all {
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
	var b strings.Builder
	if u.configured {
		b.WriteString(styleModelScopeHint.Render("Showing models from configured providers.") + "\n")
	} else {
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
