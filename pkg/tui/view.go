package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ianclemence/scout/pkg/config"
	"github.com/ianclemence/scout/pkg/isession"
	"github.com/ianclemence/scout/pkg/runtime"
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
	if m.login != nil {
		b.WriteString(m.login.view(m.width))
	} else if m.approval != nil {
		b.WriteString(m.approvalCard())
	} else {
		b.WriteString(m.promptBox())
	}
	b.WriteString("\n")
	if m.scopedSel != nil {
		b.WriteString(m.scopedSel.view(m.width))
		b.WriteString("\n")
	} else if m.modelSel != nil {
		b.WriteString(m.modelSel.view(m.width))
		b.WriteString("\n")
	} else if m.picker != nil {
		b.WriteString(m.picker.view(m.width))
		b.WriteString("\n")
	} else if m.sel != nil {
		b.WriteString(m.selectorView())
		b.WriteString("\n")
	}
	b.WriteString(m.footerStats())
	b.WriteString("\n")
	b.WriteString(m.footerKeys())
	return b.String()
}

// dockPreview is one reserved line above the composer: the tail of the
// streaming reply with a caret while a turn runs, blank when idle. Tool
// internals are never shown here — the live activity is named in the
// composer's top rule ("Working", "Searching work…") instead.
func (m *model) dockPreview() string {
	w := m.width
	if w < 10 {
		w = 10
	}
	if !m.working {
		return ""
	}
	line := "▍"
	if m.stream.Len() > 0 {
		flat := strings.ReplaceAll(m.stream.String(), "\n", " ") + "▍"
		lines := wrap(flat, w)
		line = lines[len(lines)-1]
	}
	return styleAssistant.Render(cellTruncate(line, w))
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

// activityWord is the live status in the composer's top rule: what Scout is
// doing right now in product language — the active tool ("Searching work…",
// "Drafting a proposal…") when one is running, otherwise "Working" — then
// how long. The tool count is deliberately omitted; that detail lives in the
// footer stats line.
func (m *model) activityWord() string {
	word := isession.ActivityLabel(m.toolName)
	if word == "" {
		word = "Working"
	}
	s := fmt.Sprintf("%s %s", spinnerFrames[m.spin%len(spinnerFrames)], word)
	if d := time.Since(m.turnFrom); d > 0 {
		s += " · " + formatElapsed(d)
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
		return styleTool.Render(wrapPrefixed(e.text, "  ✓ ", w, styleTool))
	case eNotice:
		// Notices (including release notes) wrap; they are never truncated.
		wrapped := wrapANSI(e.text, tw-2)
		if len(wrapped) == 0 {
			return styleNotice.Render("  ·")
		}
		var lines []string
		for i, wl := range wrapped {
			if i == 0 {
				lines = append(lines, styleNotice.Render("  · "+wl))
			} else {
				lines = append(lines, styleNotice.Render("    "+wl))
			}
		}
		return strings.Join(lines, "\n")
	case eApproval:
		return styleApproval.Render(wrapPrefixed(e.text, "  ◆ ", w, styleApproval))
	case eErr:
		var lines []string
		for _, wl := range wrap(e.text, tw-2) {
			lines = append(lines, "  "+wl)
		}
		return styleErrorCard.Render(strings.Join(lines, "\n"))
	}
	return e.text
}

// assistantBlock renders a model response as wrapped markdown inside the
// Scout transcript column. Width is the terminal minus the one-cell left
// margin so long prose wraps instead of overflowing.
func (m *model) assistantBlock(text string) string {
	w := m.width
	if w < 20 {
		w = 80
	}
	body := RenderMarkdownWidth(text, w-1)
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
	cmds := styleWelcomeCmds.Render("  /help           commands & keys\n  /login          connect a provider\n  /model          select conversation model (ctrl+p cycles)\n  /scoped-models  pick models to cycle\n  /profile        who Scout thinks you are")
	out := center(art) + "\n" + center(tag) + "\n\n" + center(cmds)
	// Name configured work sources so the startup itself answers "what MCP is
	// configured"; the /sources picker manages them.
	if m.st != nil && m.st.Core != nil {
		if conns, err := m.st.Core.Connections(); err == nil && len(conns) > 0 {
			out += "\n\n" + center(styleWelcomeSources.Render(connectorsSummary(conns)))
		}
	}
	return out
}

// connectorsSummary renders a one-line work-source digest for the welcome
// card: name, kind, and auth state, joined for a compact footer.
func connectorsSummary(conns []runtime.Connection) string {
	var parts []string
	for _, conn := range conns {
		if !conn.Enabled {
			continue
		}
		state := conn.Auth
		switch state {
		case "authenticated":
			state = "ready"
		case "open":
			state = "ready"
		case "token_stored":
			state = "token stored"
		case "unauthenticated", "token_rejected":
			state = "needs auth"
		default:
			state = "configured"
		}
		target := conn.Endpoint
		if conn.Kind == "mcp-stdio" {
			target = conn.Command
		}
		parts = append(parts, fmt.Sprintf("%s (%s on %s)", conn.Name, state, target))
	}
	if len(parts) == 0 {
		return ""
	}
	return "· " + strings.Join(parts, "  ·  ")
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
	digest := "ready"
	if m.turns > 0 {
		digest = fmt.Sprintf("%d turn(s)", m.turns)
	}
	if m.connHint != "" {
		digest += " · " + m.connHint
	}
	return digest
}

// refreshConnHint recomputes the cached source-auth hint. It is cheap
// (one list query) and called only when sources change or at startup.
func (m *model) refreshConnHint() {
	m.connHint = ""
	if m.st == nil || m.st.Core == nil {
		return
	}
	conns, err := m.st.Core.Connections()
	if err != nil {
		return
	}
	var needs []string
	for _, c := range conns {
		if !c.Enabled {
			continue
		}
		switch c.Auth {
		case "unauthenticated", "token_rejected":
			needs = append(needs, c.Name)
		}
	}
	if len(needs) > 0 {
		m.connHint = strings.Join(needs, ", ") + " needs auth · /sources"
	}
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
	case m.login != nil:
		keys = "↑↓ pick · type to filter · enter select · esc cancel"
	case m.approval != nil:
		keys = "1 approve · 2 reject · esc leaves pending"
	case m.scopedSel != nil:
		keys = "↑↓ move · enter toggle · ctrl+a/x all/clear · ctrl+p provider · alt+↑↓ reorder · ctrl+s save · esc close"
	case m.modelSel != nil:
		keys = "↑↓ pick · tab scope · enter select · ctrl+s default · esc close"
	case m.picker != nil:
		keys = "↑↓ pick · type to filter · enter select · esc cancel"
	case m.sel != nil:
		keys = "↑↓ pick · enter select · esc close"
	case m.working:
		keys = "esc aborts · / commands"
	default:
		// Idle: justify the command hint left and the exit hint right, the
		// same way the stats line pairs the digest with the model name.
		return m.footerEnds("/ commands", "esc quit")
	}
	return styleFooterHint.Render(cellTruncate(keys, m.width))
}

// footerEnds renders a left hint and a right hint on one line, separated by
// the remaining width — the alignment used for the idle key bar (and mirrored
// by footerStats for the digest/model pair).
func (m *model) footerEnds(left, right string) string {
	lw, rw := lipgloss.Width(left), lipgloss.Width(right)
	const minGap = 2
	if lw+minGap+rw <= m.width {
		return styleFooterHint.Render(left+strings.Repeat(" ", m.width-lw-rw)) + styleFooterHint.Render(right)
	}
	return styleFooterHint.Render(cellTruncate(left+" · "+right, m.width))
}

// ---------- selector (palette + model picker) ----------

const paletteMaxRows = 8

func (m *model) openPalette(filter string) {
	items := []selItem{}
	for _, c := range isession.Registry() {
		if filter == "" || strings.HasPrefix(c.Name, filter) || strings.Contains(strings.ToLower(c.Description), strings.ToLower(filter)) {
			label := "/" + c.Name
			if c.ArgHint != "" {
				label += " " + c.ArgHint
			}
			items = append(items, selItem{label: label, detail: "[" + c.Group + "] " + c.Description, value: "/" + c.Name})
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

// openModelSelector opens the searchable model selector, pre-filled
// with search. It mirrors the /model command's interactive behaviour.
func (m *model) openModelSelector(search string) {
	defProv, defModel := m.st.Sess.Provider, m.st.Sess.Model
	if r, ok := m.st.Core.Cfg.Models[config.RoleConversation]; ok && r.Provider != "" {
		defProv, defModel = r.Provider, r.Model
	}
	m.modelSel = newModelPickerUI(m.st.Core, m.st.ScopedModels, m.st.Sess.Provider, m.st.Sess.Model, defProv, defModel, search)
}

// openScopedModels opens the enable/disable + reorder selector.
func (m *model) openScopedModels() {
	m.scopedSel = newScopedModelsUI(m.st.Core, m.st.ScopedModels, m.st.Sess.Provider, m.st.Sess.Model)
}

// applyModelSelection switches the session model (and optionally records it as
// the conversation default).
func (m *model) applyModelSelection(prov, model string, asDefault bool) (tea.Model, tea.Cmd) {
	m.st.Sess.Provider, m.st.Sess.Model = prov, model
	saveSessionModel(m.st)
	if asDefault {
		// Persisting the default model role updates the config file/env model.
		if err := m.st.Core.SetRoleModel(config.RoleConversation, prov, model); err != nil {
			return m, tea.Println(renderEntryStatic(entry{kind: eErr, text: err.Error()}))
		}
		return m, tea.Println(styleNotice.Render("Default model → " + prov + "/" + model))
	}
	return m, tea.Println(styleNotice.Render("Session model → " + prov + "/" + model))
}

// cycleModel rotates the session model through the scoped set (Ctrl+P /
// Shift+Ctrl+P).
func (m *model) cycleModel(delta int) (tea.Model, tea.Cmd) {
	models := isession.AvailableModels(m.st.Core)
	scoped := isession.FilterScoped(models, m.st.ScopedModels)
	if len(scoped) < 2 {
		msg := "Only one model available"
		if !m.st.ScopedModels.AllEnabled() {
			msg = "Only one model in scope"
		}
		return m, tea.Println(styleNotice.Render(msg))
	}
	cur := -1
	for i, mm := range scoped {
		if mm.Provider == m.st.Sess.Provider && mm.ID == m.st.Sess.Model {
			cur = i
			break
		}
	}
	next := scoped[((cur+delta)%len(scoped)+len(scoped))%len(scoped)]
	m.st.Sess.Provider, m.st.Sess.Model = next.Provider, next.ID
	saveSessionModel(m.st)
	return m, tea.Println(styleNotice.Render("Switched to " + next.Provider + "/" + next.ID))
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
	b.WriteString("\n")
	b.WriteString(bar.Render("┃") + " " + styleFooterHint.Render("approving records your decision; it does not submit externally unless a connected source can execute it"))
	return b.String()
}

// ---------- width helpers ----------

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// wrapANSI word-wraps already-styled text without splitting escape sequences.
// It measures with lipgloss.Width (so wide runes count correctly) and resets
// style at the end of each wrapped line so colors do not bleed.
func wrapANSI(s string, width int) []string {
	if width <= 0 {
		width = 80
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
				cur.WriteString(word)
				curLen = wl
				continue
			}
			if curLen+1+wl > width {
				flush()
			}
			if curLen > 0 {
				cur.WriteString(" ")
				curLen++
			}
			cur.WriteString(word)
			curLen += wl
		}
		flush()
	}
	return out
}

func cellTruncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	runes := []rune(s)
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi) / 2
		if lipgloss.Width(string(runes[:mid])) < w-1 {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo > 1 {
		return string(runes[:lo-1]) + "…"
	}
	return "…"
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
