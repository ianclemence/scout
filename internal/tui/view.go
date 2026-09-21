package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Spinner cube, rotating while a turn runs.
var spinnerFrames = []string{"▖", "▘", "▝", "▗"}

// View renders only the live dock: streaming preview, composer (or approval
// card), palette/modal, footer. Committed transcript lives in scrollback.
func (m *model) View() string {
	if m.quitting {
		return ""
	}
	if !m.ready {
		return "starting Scout…"
	}
	var b strings.Builder
	b.WriteString(m.dockPreview())
	b.WriteString("\n")
	if m.auth != nil {
		b.WriteString(m.authCard())
	} else if m.approval != nil {
		b.WriteString(m.approvalCard())
	} else {
		b.WriteString(m.promptBox())
	}
	b.WriteString("\n")
	if m.sel != nil {
		b.WriteString(m.selectorView())
		b.WriteString("\n")
	}
	b.WriteString(m.footerStats())
	b.WriteString("\n")
	b.WriteString(m.footerKeys())
	return b.String()
}

// dockPreview is one reserved line: streaming tail with caret, else blank.
func (m *model) dockPreview() string {
	w := m.width
	if w < 10 {
		w = 10
	}
	if !m.working {
		return ""
	}
	line := ""
	if m.toolLine != "" {
		line = "◐ " + m.toolLine
	} else if m.stream.Len() > 0 {
		flat := strings.ReplaceAll(m.stream.String(), "\n", " ") + "▍"
		lines := wrap(flat, w)
		line = lines[len(lines)-1]
	} else {
		line = "▍"
	}
	return styleTool.Render(truncate(line, w))
}

// promptBox is the rule-framed composer: top rule carries live status.
func (m *model) promptBox() string {
	var b strings.Builder
	b.WriteString(m.composerTopRule())
	for _, ln := range strings.Split(m.ta.View(), "\n") {
		b.WriteString("\n")
		b.WriteString(ln)
	}
	b.WriteString("\n")
	b.WriteString(stylePromptBar.Render(strings.Repeat("─", m.width)))
	return b.String()
}

// composerTopRule is `── <spinner> <activity> … ──` while working, plain idle.
func (m *model) composerTopRule() string {
	w := m.width
	if w < 10 {
		w = 10
	}
	if !m.working {
		return stylePromptBar.Render(strings.Repeat("─", w))
	}
	status := m.activityWord()
	sw := lipgloss.Width(status)
	if sw+6 > w {
		status = cellTruncate(status, w-6)
		sw = lipgloss.Width(status)
	}
	fill := w - 3 - sw - 1
	if fill < 0 {
		fill = 0
	}
	return stylePromptBar.Render("── ") + styleWorking.Render(status) +
		stylePromptBar.Render(" "+strings.Repeat("─", fill))
}

// activityWord names what Scout is doing: the active tool in Scout language.
func (m *model) activityWord() string {
	word := "Working"
	switch m.toolName {
	case "search_opportunities", "discover_opportunities", "run_discovery":
		word = "Searching"
	case "analyze_opportunity", "get_opportunity":
		word = "Analyzing"
	case "prepare_proposal":
		word = "Drafting"
	case "list_messages", "list_applications", "get_pipeline":
		word = "Reading"
	case "get_profile", "list_evidence":
		word = "Reviewing"
	}
	s := fmt.Sprintf("%s %s", spinnerFrames[m.spin%len(spinnerFrames)], word)
	if d := time.Since(m.turnFrom); d > 0 {
		s += " · " + formatElapsed(d)
	}
	if m.tools > 0 {
		s += fmt.Sprintf(" · %d tool(s)", m.tools)
	}
	return s
}

func formatElapsed(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}

// ---------- transcript entries ----------

func (m *model) renderEntry(e entry) string {
	w := m.width
	if w < 20 {
		w = 20
	}
	tw := w - 2
	switch e.kind {
	case eUser:
		var lines []string
		lines = append(lines, styleUserName.Render("You"))
		for _, para := range strings.Split(e.text, "\n") {
			if strings.TrimSpace(para) == "" {
				lines = append(lines, styleUserPanel.Render(styleUserBar.Render("┃")))
				continue
			}
			for _, wl := range wrap(para, w-4) {
				lines = append(lines, styleUserPanel.Render(styleUserBar.Render("┃")+" "+styleUserText.Render(wl)))
			}
		}
		return strings.Join(lines, "\n")
	case eScout:
		head := " " + styleAssistantName.Render("👷 Scout · "+m.st.Sess.Provider+"/"+m.st.Sess.Model)
		if e.dur > 0 {
			head += styleAssistantMeta.Render(" · " + formatElapsed(e.dur))
		}
		return head + "\n" + m.assistantBlock(e.text)
	case eTool:
		return styleTool.Render("  ✓ " + cellTruncate(e.text, tw-4))
	case eNotice:
		return styleNotice.Render("  · " + cellTruncate(e.text, tw-2))
	case eApproval:
		return styleApproval.Render("  ◆ " + cellTruncate(e.text, tw-2))
	case eErr:
		var lines []string
		for _, wl := range wrap(e.text, tw-2) {
			lines = append(lines, "  "+wl)
		}
		return styleErrorCard.Render(strings.Join(lines, "\n"))
	}
	return e.text
}

func (m *model) assistantBlock(text string) string {
	body := RenderMarkdown(text)
	lines := strings.Split(body, "\n")
	for i, ln := range lines {
		if ln == "" {
			continue
		}
		lines[i] = " " + ln
	}
	return strings.Join(lines, "\n")
}

// ---------- welcome + day dividers ----------

func (m *model) welcomeCard() string {
	w := m.width
	if w < 40 {
		w = 80 // first paint can precede sizing; never wrap for width 0
	}
	center := func(s string) string {
		var lines []string
		for _, ln := range strings.Split(s, "\n") {
			lines = append(lines, lipgloss.PlaceHorizontal(w, lipgloss.Center, ln))
		}
		return strings.Join(lines, "\n")
	}
	art := styleScoutArt.Render("▓▒░  👷  S C O U T  ░▒▓")
	tag := styleWelcomeTitle.Render(wrapFirst("Find work worth doing.", minInt(w-2, 64)))
	cmds := styleWelcomeCmds.Render("  /help      commands & keys\n  /model     switch thinking engine\n  /profile   who Scout thinks you are\n  /sources   work sources & auth")
	return center(art) + "\n" + center(tag) + "\n\n" + center(cmds)
}

func dayLabel(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	y, mo, d := t.Date()
	ny, nmo, nd := time.Now().Date()
	if y == ny && mo == nmo && d == nd {
		return "Today"
	}
	yy, ymo, yd := time.Now().AddDate(0, 0, -1).Date()
	if y == yy && mo == ymo && d == yd {
		return "Yesterday"
	}
	return t.Format("2 January 2006")
}

func (m *model) renderDayDivider(label string) string {
	w := m.width
	core := " " + label + " "
	fill := w - lipgloss.Width(core)
	if fill < 0 {
		return styleDayDivider.Render(cellTruncate(label, w))
	}
	left := fill / 2
	return styleDayDivider.Render(strings.Repeat("─", left) + core + strings.Repeat("─", fill-left))
}

// ---------- footer ----------

func (m *model) footerStats() string {
	left := m.sessionDigest()
	local, prov, mod := m.modelInfo()
	right := fmt.Sprintf("(%s) %s/%s", local, prov, mod)
	if think := m.st.Sess.Thinking; think != "" {
		right += " · " + think
	}
	if m.approval != nil {
		left = "waiting for you"
	} else if m.st.Core != nil {
		if pend, _ := m.st.Core.PendingApprovals(); len(pend) > 0 {
			left += fmt.Sprintf(" · %d approval(s)", len(pend))
		}
	}
	lw, rw := lipgloss.Width(left), lipgloss.Width(right)
	const minGap = 2
	if lw+minGap+rw <= m.width {
		return styleFooter.Render(left+strings.Repeat(" ", m.width-lw-rw)) + styleModel(local).Render(right)
	}
	return styleFooter.Render(cellTruncate(left, m.width))
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
		return styleModelLocal
	}
	return styleModelCloud
}

func (m *model) footerKeys() string {
	var keys string
	switch {
	case m.auth != nil:
		keys = "enter submit · esc cancel"
	case m.approval != nil:
		keys = "1 approve · 2 reject · esc leaves pending"
	case m.sel != nil:
		keys = "↑↓ pick · enter select · esc close"
	case m.working:
		keys = "esc aborts · / commands"
	default:
		keys = "/ commands · tab complete · ctrl+l model · esc quit"
	}
	return styleFooterHint.Render(cellTruncate(keys, m.width))
}

// ---------- selector (palette + model picker) ----------

const paletteMaxRows = 5

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
	cur := 0
	if m.sel != nil {
		cur = m.sel.cur
	}
	m.openPalette(m.palFilter)
	if cur < len(m.sel.items) {
		m.sel.cur = cur
	}
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
		return m, tea.Println(styleNotice.Render("Session model → " + it.value))
	}
	if mode == "login" {
		d := newAuthDialog(it.value)
		m.auth = &d
		return m, nil
	}
	if mode == "logout" {
		nm, cmd := m.removeStoredKey(it.value)
		return nm, cmd
	}
	nm, cmd := m.runCommand(strings.TrimPrefix(it.value, "/"))
	m.ta.Reset()
	m.layoutComposer()
	return nm, cmd
}

func (m *model) selectorView() string {
	w := m.width
	if w < 20 {
		w = 20
	}
	items := m.sel.items
	off := 0
	if len(items) > paletteMaxRows {
		off = m.sel.cur - paletteMaxRows/2
		if off+paletteMaxRows > len(items) {
			off = len(items) - paletteMaxRows
		}
		if off < 0 {
			off = 0
		}
	}
	end := off + paletteMaxRows
	if end > len(items) {
		end = len(items)
	}
	var b strings.Builder
	if len(items) == 0 {
		b.WriteString(stylePaletteNoMatch.Render("  No matching commands"))
		return b.String()
	}
	for i := off; i < end; i++ {
		it := items[i]
		row := fmt.Sprintf("  %-30s %s", it.label, stylePaletteDesc.Render(cellTruncate(it.detail, w-36)))
		if i == m.sel.cur {
			row = stylePaletteSel.Render(fmt.Sprintf("→ %-30s %s", it.label, cellTruncate(it.detail, w-36)))
		}
		b.WriteString(row + "\n")
	}
	if len(items) > paletteMaxRows {
		b.WriteString(stylePaletteScroll.Render(fmt.Sprintf("  (%d/%d)", m.sel.cur+1, len(items))))
	}
	return strings.TrimRight(b.String(), "\n")
}

// ---------- approval card ----------

// authCard is the Pi-style login dialog: titled box, masked key prompt.
func (m *model) authCard() string {
	a := m.auth
	var b strings.Builder
	b.WriteString(stylePromptBar.Render(strings.Repeat("─", m.width)))
	b.WriteString("\n")
	b.WriteString(" " + styleModalTitle.Render("Login to "+providerDisplay(a.provider)))
	b.WriteString("\n\n")
	b.WriteString(" " + styleAssistant.Render("Enter "+providerDisplay(a.provider)+" API key"))
	b.WriteString("\n")
	b.WriteString(" " + a.input.View())
	if a.err != "" {
		b.WriteString("\n")
		b.WriteString(" " + styleError.Render(a.err))
	}
	b.WriteString("\n\n")
	b.WriteString(" " + styleFooterHint.Render("(esc to cancel, enter to submit)"))
	b.WriteString("\n")
	b.WriteString(stylePromptBar.Render(strings.Repeat("─", m.width)))
	return b.String()
}

func providerDisplay(p string) string {
	switch p {
	case "openai":
		return "OpenAI"
	case "anthropic":
		return "Anthropic"
	case "deepseek":
		return "DeepSeek"
	case "moonshot":
		return "Moonshot"
	}
	return p
}

func (m *model) approvalCard() string {
	a := m.approval
	risk := strings.ToLower(a.risk)
	badge := styleRiskDefault.Render(" " + risk + " ")
	switch risk {
	case "high":
		badge = styleRiskHigh.Render(" ◆ high ")
	case "medium":
		badge = styleRiskMid.Render(" ◆ medium ")
	default:
		badge = styleRiskLow.Render(" ◆ low ")
	}
	bar := styleApprovalBar
	var b strings.Builder
	b.WriteString(bar.Render("┃") + " " + styleApprovalTitle.Render("△ Approval required") + "  " + badge)
	b.WriteString("\n")
	b.WriteString(bar.Render("┃") + " " + styleAssistant.Render(cellTruncate(a.title, m.width-4)))
	b.WriteString("\n")
	labels := []string{"[1] approve", "[2] reject"}
	row := bar.Render("┃") + " "
	for i, l := range labels {
		if i == a.sel {
			row += styleApprovalSel.Render(" " + l + " ")
		} else {
			row += styleApprovalKeys.Render(" " + l + " ")
		}
		row += "  "
	}
	row += styleNotice.Render("←→ select · enter confirm · esc leaves pending")
	b.WriteString(row)
	return b.String()
}

// ---------- width helpers ----------

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func cellTruncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	return truncate(s, w)
}

func wrapFirst(s string, w int) string {
	lines := wrap(s, w)
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
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
		w = 80
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		if strings.TrimSpace(para) == "" {
			out = append(out, "")
			continue
		}
		var cur strings.Builder
		curLen := 0
		flush := func() {
			if curLen > 0 {
				out = append(out, cur.String())
				cur.Reset()
				curLen = 0
			}
		}
		for _, word := range strings.Fields(para) {
			wl := lipgloss.Width(word)
			if curLen == 0 {
				// Hard-break words wider than the width.
				for wl > w {
					out = append(out, word[:w])
					word = word[w:]
					wl = lipgloss.Width(word)
				}
				cur.WriteString(word)
				curLen = wl
				continue
			}
			if curLen+1+wl > w {
				flush()
				for wl > w {
					out = append(out, word[:w])
					word = word[w:]
					wl = lipgloss.Width(word)
				}
				cur.WriteString(word)
				curLen = wl
				continue
			}
			cur.WriteString(" " + word)
			curLen += 1 + wl
		}
		flush()
	}
	return out
}
