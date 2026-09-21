package tui

import (
	"context"
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
	// The streaming preview line is only present while a turn is producing
	// text, so the idle dock stays tight.
	if p := m.dockPreview(); p != "" {
		b.WriteString(p)
		b.WriteString("\n")
	}
	if m.login != nil {
		b.WriteString(m.login.view(m.width))
	} else if m.approval != nil {
		b.WriteString(m.approvalCard())
	} else {
		b.WriteString(m.promptBox())
	}
	b.WriteString("\n")
	if m.modelSel != nil {
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
// streaming reply while a turn runs, blank when idle. No cursor marker is
// shown — the assistant text alone is the live preview. Tool internals are
// never shown here; the live activity is named in the composer's top rule.
func (m *model) dockPreview() string {
	w := m.width
	if w < 10 {
		w = 10
	}
	if !m.working || m.stream.Len() == 0 {
		return ""
	}
	flat := strings.ReplaceAll(m.stream.String(), "\n", " ")
	lines := wrap(flat, w)
	line := lines[len(lines)-1]
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
	case eCommand:
		// Local command results read as Scout's own answer, in the same
		// wrapped-prose style as a model reply.
		return " " + styleAssistantName.Render("👷 Scout") + "\n" + m.assistantBlock(e.text)
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
	center := func(s string) string { return lipgloss.PlaceHorizontal(w, lipgloss.Center, s) }
	art := styleScoutArt.Render("▓▒░  👷  S C O U T  ░▒▓")
	tag := styleWelcomeTitle.Render(wrapFirst("Find work worth doing.", minInt(w-2, 64)))

	// Get-started hints, aligned as a two-column block and centered as one
	// composition so the list reads as a unit rather than floating lines.
	cmds := []struct{ name, desc string }{
		{"/help", "commands & keys"},
		{"/login", "connect a provider"},
		{"/model", "select conversation model"},
		{"/profile", "who Scout thinks you are"},
		{"/sources", "work sources & MCP connectors"},
	}
	block := make([]string, 0, len(cmds))
	for _, c := range cmds {
		pad := strings.Repeat(" ", maxInt(1, 13-len(c.name)))
		block = append(block, styleWelcomeCmd.Render(c.name)+pad+styleWelcomeDesc.Render(c.desc))
	}
	out := center(art) + "\n" + center(tag) + "\n\n" + centerBlock(block, w)

	// Name configured work sources so the startup itself answers "what MCP is
	// configured"; the /sources picker manages them.
	if m.st != nil && m.st.Core != nil {
		if conns, err := m.st.Core.Connections(); err == nil && len(conns) > 0 {
			src := styleWelcomeSources.Render("Work sources · " + connectorsSummary(conns))
			out += "\n\n" + centerBlock([]string{src}, w)
		}
	}
	return out
}

// centerBlock centers a group of lines as a single block: every line shares
// one left offset, so the block stays internally aligned. Widths are measured
// with lipgloss so ANSI styling does not skew the centering.
func centerBlock(lines []string, w int) string {
	max := 0
	for _, ln := range lines {
		if n := lipgloss.Width(ln); n > max {
			max = n
		}
	}
	off := (w - max) / 2
	if off < 0 {
		off = 0
	}
	pad := strings.Repeat(" ", off)
	var out []string
	for _, ln := range lines {
		out = append(out, pad+ln)
	}
	return strings.Join(out, "\n")
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
	return strings.Join(parts, "  ·  ")
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
	case m.modelSel != nil:
		keys = "↑↓ pick · enter select · ctrl+s default · esc close"
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
	commands := isession.Registry()
	if f := strings.TrimSpace(filter); f != "" {
		// Fuzzy-rank command names, then add any whose name or description
		// contains the query as a substring (so "connect" finds /login).
		matched := fuzzyFilter(commands, f, func(c *isession.Command) string { return c.Name })
		seen := map[*isession.Command]bool{}
		for _, c := range matched {
			seen[c] = true
		}
		low := strings.ToLower(f)
		for _, c := range commands {
			if seen[c] {
				continue
			}
			if strings.Contains(strings.ToLower(c.Name+" "+c.Description), low) {
				matched = append(matched, c)
			}
		}
		commands = matched
	}
	items := make([]selItem, 0, len(commands))
	for _, c := range commands {
		// The palette shows the command name and a one-line description.
		// The argument hint folds into the description ("<id> — …"),
		// the way Pi's slash-command autocomplete composes it. No group
		// or source tags: they are noise next to every row.
		detail := c.Description
		if c.ArgHint != "" {
			if detail != "" {
				detail = c.ArgHint + " — " + detail
			} else {
				detail = c.ArgHint
			}
		}
		items = append(items, selItem{label: "/" + c.Name, detail: detail, value: "/" + c.Name})
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
	m.modelSel = newModelPickerUI(m.st.Core, m.st.Sess.Provider, m.st.Sess.Model, defProv, defModel, search)
}

// takeModelRefresh consumes the picker's pending-refresh flag and returns a
// command to refresh model catalogs in the background (Pi refreshes on open).
func (m *model) takeModelRefresh() tea.Cmd {
	if m.modelSel == nil || !m.modelSel.needsRefresh || m.st == nil || m.st.Core == nil {
		return nil
	}
	m.modelSel.needsRefresh = false
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		n, err := m.st.Core.Registry().Refresh(ctx, "")
		return modelsRefreshedMsg{count: n, err: err}
	}
}

// handleModelsRefreshed applies a background catalog refresh to the picker.
func (m *model) handleModelsRefreshed(msg modelsRefreshedMsg) (tea.Model, tea.Cmd) {
	if m.modelSel == nil {
		return m, nil
	}
	if msg.err != nil && msg.count == 0 {
		m.modelSel.errMsg = "Could not refresh model catalogs; showing cached models."
		return m, nil
	}
	available := isession.AvailableModels(m.st.Core)
	m.modelSel.configured = len(available) > 0
	all := available
	if !m.modelSel.configured {
		all = isession.AllModels(m.st.Core)
	}
	m.modelSel.reload(all)
	m.modelSel.errMsg = ""
	m.modelSel.refreshStatus = "Model catalogs refreshed."
	m.modelSel.refreshSuccess = true
	m.modelSel.rebuild()
	return m, nil
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

// pickSelected applies the highlighted palette command.
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
	// Primary column is as wide as the widest visible command, clamped to a
	// readable range with a fixed gap before the description — the layout the
	// Pi slash-command autocomplete uses.
	col := paletteColumnWidth(items)
	for i := off; i < end; i++ {
		it := items[i]
		label := cellTruncate(it.label, col-paletteColGap)
		spacing := strings.Repeat(" ", maxInt(1, col-lipgloss.Width(label)))
		descWidth := w - 2 - col - 2
		desc := ""
		if descWidth > 10 {
			desc = cellTruncate(it.detail, descWidth)
		}
		if i == m.sel.cur {
			b.WriteString(stylePaletteSel.Render("→ "+label+spacing+desc) + "\n")
		} else {
			b.WriteString("  " + label + stylePaletteDesc.Render(spacing+desc) + "\n")
		}
	}
	if len(items) > paletteMaxRows {
		b.WriteString(stylePaletteScroll.Render(fmt.Sprintf("  (%d/%d)", m.sel.cur+1, len(items))))
	}
	return strings.TrimRight(b.String(), "\n")
}

// Palette column bounds and gap, matching the slash-command autocomplete in
// the Pi coding agent (12–32 cells, two-space gap).
const (
	paletteMinCol = 12
	paletteMaxCol = 32
	paletteColGap = 2
)

// paletteColumnWidth returns the primary-column width: the widest visible
// command label plus the gap, clamped to [paletteMinCol, paletteMaxCol].
func paletteColumnWidth(items []selItem) int {
	widest := 0
	for _, it := range items {
		if n := lipgloss.Width(it.label); n > widest {
			widest = n
		}
	}
	widest += paletteColGap
	if widest < paletteMinCol {
		widest = paletteMinCol
	}
	if widest > paletteMaxCol {
		widest = paletteMaxCol
	}
	return widest
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
