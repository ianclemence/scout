package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/ianclemence/scout/pkg/csession"
)

// shortID trims an id to a readable prefix.
func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// This file holds the general interactive pickers that make Scout's slash
// commands behave like the reference terminal agents: an inline, searchable
// list with a marked current row, dim descriptions, and a footer that names
// exactly what each key does. Typing filters; arrows move; Enter acts; Esc
// cancels. Nothing is "just printed words".

// pickItem is one row in a listP icker.
type pickItem struct {
	label   string // primary column
	detail  string // dim description after the label
	current bool   // marks the active/selected value (✓)
	value   string // what act() receives
}

// listPickerUI is a searchable, keyboard-driven list.
type listPickerUI struct {
	title    string
	status   string // optional line under the title
	footer   string
	items    []pickItem
	filtered []pickItem
	search   string
	cur      int
	// act is invoked with the chosen value. It may return a message to print.
	act func(value string) string
	// secondary, when set, is bound to Ctrl+S (e.g. "set as default").
	secondary     func(value string) string
	secondaryHint string
}

func newListPicker(title, status, footer string, items []pickItem, act func(string) string) *listPickerUI {
	u := &listPickerUI{title: title, status: status, footer: footer, items: items, act: act}
	u.rebuild()
	return u
}

func (u *listPickerUI) rebuild() {
	q := strings.ToLower(strings.TrimSpace(u.search))
	if q == "" {
		u.filtered = u.items
	} else {
		var out []pickItem
		for _, it := range u.items {
			hay := strings.ToLower(it.label + " " + it.detail)
			if strings.Contains(hay, q) {
				out = append(out, it)
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

// picked returns the highlighted item, or ok=false when the list is empty.
func (u *listPickerUI) picked() (pickItem, bool) {
	if len(u.filtered) == 0 {
		return pickItem{}, false
	}
	return u.filtered[u.cur], true
}

// handlePickerKey processes a key. It returns a command string for the model
// to run ("pick", "secondary", "cancel"), or "" to stay open.
func (u *listPickerUI) handleKey(key string) string {
	switch key {
	case "esc", "ctrl+c":
		return "cancel"
	case "enter", "tab":
		return "pick"
	case "ctrl+s":
		if u.secondary != nil {
			return "secondary"
		}
	case "up":
		if len(u.filtered) > 0 {
			u.cur = (u.cur - 1 + len(u.filtered)) % len(u.filtered)
		}
	case "down":
		if len(u.filtered) > 0 {
			u.cur = (u.cur + 1) % len(u.filtered)
		}
	case "pgup":
		u.cur -= 5
		if u.cur < 0 {
			u.cur = 0
		}
	case "pgdown":
		u.cur += 5
		if u.cur >= len(u.filtered) {
			u.cur = maxInt(0, len(u.filtered)-1)
		}
	case "home":
		u.cur = 0
	case "end":
		u.cur = maxInt(0, len(u.filtered)-1)
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
	return ""
}

func (u *listPickerUI) view(width int) string {
	if width < 20 {
		width = 20
	}
	var b strings.Builder
	esc := stylePaletteScroll.Render("esc")
	gap := width - lipgloss.Width(u.title) - lipgloss.Width("esc")
	if gap < 1 {
		gap = 1
	}
	b.WriteString(styleModalTitle.Render(u.title) + strings.Repeat(" ", gap) + esc + "\n")
	if u.status != "" {
		b.WriteString(styleModelScopeHint.Render("  "+u.status) + "\n")
	}
	search := u.search
	if search == "" {
		search = "type to filter…"
	}
	b.WriteString(styleModelSearch.Render("  "+search+"▍") + "\n")
	if len(u.filtered) == 0 {
		b.WriteString(stylePaletteNoMatch.Render("  No matches") + "\n")
	} else {
		const maxRows = 10
		off := scrollOffset(u.cur, len(u.filtered), maxRows)
		end := minInt(off+maxRows, len(u.filtered))
		for i := off; i < end; i++ {
			it := u.filtered[i]
			cursor := "  "
			if i == u.cur {
				cursor = stylePaletteSel.Render("→ ")
			}
			mark := "  "
			if it.current {
				mark = styleModelEnabled.Render("✓ ")
			}
			primary := cellTruncate(it.label, 30)
			spacing := strings.Repeat(" ", maxInt(1, 32-lipgloss.Width(primary)))
			row := cursor + mark + primary + spacing
			if rem := width - lipgloss.Width(row) - 1; rem > 8 && it.detail != "" {
				row += stylePaletteDesc.Render(cellTruncate(it.detail, rem))
			}
			if i == u.cur {
				row = cursor + mark + stylePaletteSel.Render(primary) + spacing
				if it.detail != "" {
					row += stylePaletteDesc.Render(cellTruncate(it.detail, maxInt(0, width-lipgloss.Width(row)-1)))
				}
			}
			b.WriteString(row + "\n")
		}
		if off > 0 || end < len(u.filtered) {
			b.WriteString(stylePaletteScroll.Render(fmt.Sprintf("  (%d/%d)", u.cur+1, len(u.filtered))) + "\n")
		}
		if it, ok := u.picked(); ok && it.detail != "" {
			b.WriteString("\n" + stylePaletteDesc.Render("  "+it.detail) + "\n")
		}
	}
	footer := u.footer
	if u.secondary != nil && u.secondaryHint != "" {
		footer += " · ctrl+s " + u.secondaryHint
	}
	b.WriteString(styleModelScopeFooter.Render("  " + footer + " · esc cancel"))
	return strings.TrimRight(b.String(), "\n")
}

// ---------- TUI openers ----------

// openThinking opens the reasoning-level selector. Enter selects for the
// session; Ctrl+S also records it as the conversation default.
func (m *model) openThinking() {
	cur := m.st.Sess.Thinking
	p := newThinkingPicker(cur, m.st.Sess.Provider, m.st.Sess.Model, func(level string) string {
		m.st.Sess.Thinking = level
		csession.SetThinking(m.st.Core.DB, m.st.Sess.ID, level)
		return "Reasoning level → " + level
	})
	m.picker = p
}

// openSessions opens the interactive session picker. Selecting a session
// switches to it in place (the TUI's resume path).
func (m *model) openSessions() {
	list, err := csession.List(m.st.Core.DB)
	if err != nil {
		m.println(entry{kind: eErr, text: err.Error(), at: time.Now()})
		return
	}
	var items []pickItem
	for _, s := range list {
		detail := s.Provider + "/" + s.Model
		if s.Name != "" {
			detail = s.Name + " · " + detail
		}
		items = append(items, pickItem{
			label:   shortID(s.ID),
			detail:  detail,
			current: s.ID == m.st.Sess.ID,
			value:   s.ID,
		})
	}
	m.picker = newListPicker("Sessions", "enter switches session", "enter resume", items, func(id string) string {
		sess, err := m.st.Core.ResolveSession(id)
		if err != nil {
			return "could not resume: " + err.Error()
		}
		if m.st.SwitchSession != nil {
			if err := m.st.SwitchSession(sess); err != nil {
				return "could not switch: " + err.Error()
			}
			return "resumed session " + shortID(sess.ID)
		}
		return "session " + shortID(sess.ID)
	})
}

// openApprovals opens the interactive approval picker: each pending action is
// a row; Enter approves, Ctrl+R rejects. This is the trust boundary, made
// reachable without typing an id.
func (m *model) openApprovals() {
	acts, err := m.st.Core.PendingApprovals()
	if err != nil {
		m.println(entry{kind: eErr, text: err.Error(), at: time.Now()})
		return
	}
	if len(acts) == 0 {
		m.println(entry{kind: eNotice, text: "Nothing awaiting approval.", at: time.Now()})
		m.flushCmds()
		return
	}
	var items []pickItem
	for _, a := range acts {
		items = append(items, pickItem{
			label:  a.ActionType,
			detail: "→ " + a.Target + "  [risk " + a.RiskLevel + "]",
			value:  a.ID,
		})
	}
	p := newListPicker("Approvals", "enter approves · ctrl+r rejects", "enter approve", items, func(id string) string {
		if err := m.st.Core.SetApprovalStatus(id, "approved"); err != nil {
			return "approve failed: " + err.Error()
		}
		return "approved " + shortID(id)
	})
	p.secondaryHint = "reject"
	p.secondary = func(id string) string {
		if err := m.st.Core.SetApprovalStatus(id, "rejected"); err != nil {
			return "reject failed: " + err.Error()
		}
		return "rejected " + shortID(id)
	}
	m.picker = p
}

// ---------- thinking picker (/thinking) ----------

var thinkingLevels = []string{"off", "low", "medium", "high", "max"}

var thinkingDesc = map[string]string{
	"off":    "No reasoning — direct answers",
	"low":    "Light reasoning",
	"medium": "Moderate reasoning",
	"high":   "Deep reasoning",
	"max":    "Maximum reasoning",
}

// newThinkingPicker builds the reasoning-level selector, mirroring the
// reference picking experience: the current level is marked, each level is
// described, typing filters, and Enter selects.
func newThinkingPicker(current, provider, model string, onSelect func(level string) string) *listPickerUI {
	var items []pickItem
	for _, lvl := range thinkingLevels {
		items = append(items, pickItem{
			label:   lvl,
			detail:  thinkingDesc[lvl],
			current: lvl == current,
			value:   lvl,
		})
	}
	status := fmt.Sprintf("Reasoning level for %s/%s", provider, model)
	if current == "" {
		status += " (default)"
	} else {
		status += " (current: " + current + ")"
	}
	u := newListPicker("Thinking Level", status, "enter select", items, onSelect)
	return u
}
