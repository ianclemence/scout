package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// View renders only the live dock: streaming preview, prompt box (or
// selector), and footer. Committed transcript lines go to the terminal's
// scrollback, so native scrolling reaches every previous message.
func (m *model) View() string {
	if m.quitting {
		return ""
	}
	if !m.ready {
		return "starting Scout…"
	}
	var b strings.Builder
	b.WriteString(m.previewLine())
	b.WriteString("\n")
	if m.sel != nil {
		b.WriteString(m.selectorView())
	} else {
		b.WriteString(m.promptBox())
	}
	b.WriteString("\n")
	b.WriteString(m.footerStats())
	b.WriteString("\n")
	b.WriteString(m.footerKeys())
	return b.String()
}

// previewLine shows the tail of the streaming reply with a caret, or the
// active tool. Blank when idle so the dock keeps a stable height.
func (m *model) previewLine() string {
	if !m.working {
		return ""
	}
	if m.toolLine != "" {
		return styleTool.Render("◐ " + truncate(m.toolLine, m.width-4))
	}
	s := strings.ReplaceAll(m.stream.String(), "\n", " ") + "▍"
	lines := wrap(s, m.width-2)
	if len(lines) == 0 {
		return "▍"
	}
	return styleDim.Render(lines[len(lines)-1])
}

func (m *model) promptBox() string {
	title := "› ask"
	if m.working {
		title = "◐ working… (esc aborts)"
	}
	box := styleBox.Render(m.ta.View())
	_ = title
	return box
}

func (m *model) footerStats() string {
	left := m.sessionDigest()
	local, prov, mod := m.modelInfo()
	right := fmt.Sprintf("(%s) %s/%s", local, prov, mod)
	if think := m.st.Sess.Thinking; think != "" {
		right += " · " + think
	}
	if pend, _ := m.st.Core.PendingApprovals(); len(pend) > 0 {
		left += fmt.Sprintf(" · %d approval(s)", len(pend))
	}
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 2 {
		return styleFooter.Render(truncate(left, m.width))
	}
	return styleFooter.Render(left+strings.Repeat(" ", gap)) + styleModel(local).Render(right)
}

func (m *model) sessionDigest() string {
	if m.working {
		return fmt.Sprintf("working · %d tool(s)", m.tools)
	}
	if m.turns == 0 {
		return "ready"
	}
	return fmt.Sprintf("%d turn(s)", m.turns)
}

func (m *model) modelInfo() (local, prov, mod string) {
	prov, mod = m.st.Sess.Provider, m.st.Sess.Model
	if prov == "ollama" {
		return "local", prov, mod
	}
	return "cloud", prov, mod
}

func styleModel(local string) lipgloss.Style {
	if local == "local" {
		return lipgloss.NewStyle().Foreground(userColor)
	}
	return lipgloss.NewStyle().Foreground(accent)
}

func (m *model) footerKeys() string {
	var keys string
	switch {
	case m.sel != nil:
		keys = "↑↓ pick · enter select · esc close"
	case m.working:
		keys = "esc aborts · / commands"
	default:
		keys = "/ commands · tab complete · ctrl+l model · esc quit"
	}
	return styleDim.Render(truncate(keys, m.width))
}

// ---------- selector (palette + model picker) ----------

func (m *model) openPalette(filter string) {
	items := []selItem{}
	for _, c := range commandList() {
		if filter == "" || strings.HasPrefix(c.Name, filter) {
			items = append(items, selItem{label: "/" + c.Name, detail: c.Description, value: "/" + c.Name})
		}
	}
	m.sel = &selector{title: "Commands", items: items}
	m.selMode = "palette"
	m.palFilter = filter
}

func (m *model) refilterPalette() {
	m.openPalette(m.palFilter)
}

func (m *model) openModelPicker() {
	items := []selItem{}
	for _, o := range modelOptionsFor(m.st) {
		mark := ""
		if o.prov == m.st.Sess.Provider && o.model == m.st.Sess.Model {
			mark = " ●"
		}
		items = append(items, selItem{
			label:  o.prov + "/" + o.model + mark,
			detail: o.note,
			value:  o.prov + "/" + o.model,
		})
	}
	m.sel = &selector{title: "Model (session)", items: items}
	m.selMode = "model"
}

func (m *model) pickSelected() (tea.Model, tea.Cmd) {
	if m.sel == nil || len(m.sel.items) == 0 {
		m.sel = nil
		return m, nil
	}
	it := m.sel.items[m.sel.cur]
	mode := m.selMode
	m.sel = nil
	m.palFilter = ""
	if mode == "model" {
		prov, mod := splitRef(it.value)
		m.st.Sess.Provider, m.st.Sess.Model = prov, mod
		saveSessionModel(m.st)
		return m, tea.Println(styleNotice.Render(fmt.Sprintf("Session model → %s", it.value)))
	}
	// Palette: run the command (strip leading "/").
	nm, cmd := m.runCommand(strings.TrimPrefix(it.value, "/"))
	return nm, cmd
}

func (m *model) selectorView() string {
	var b strings.Builder
	b.WriteString(styleScout.Render(m.sel.title))
	b.WriteString("\n")
	max := 10
	start := 0
	if m.sel.cur >= max {
		start = m.sel.cur - max + 1
	}
	for i := start; i < len(m.sel.items) && i < start+max; i++ {
		it := m.sel.items[i]
		line := fmt.Sprintf("  %-28s %s", it.label, styleDim.Render(it.detail))
		if i == m.sel.cur {
			line = styleSelect.Render(fmt.Sprintf("▶ %-28s %s", it.label, it.detail))
		}
		b.WriteString(line + "\n")
	}
	return styleBox.Render(strings.TrimRight(b.String(), "\n"))
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func wrap(s string, w int) []string {
	if w <= 0 {
		return []string{s}
	}
	var out []string
	for len(s) > 0 {
		if len(s) <= w {
			out = append(out, s)
			break
		}
		out = append(out, s[:w])
		s = s[w:]
	}
	return out
}
