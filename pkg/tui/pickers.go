package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/ianclemence/scout/pkg/config"
	"github.com/ianclemence/scout/pkg/csession"
	"github.com/ianclemence/scout/pkg/domain"
	"github.com/ianclemence/scout/pkg/llm"
	"github.com/ianclemence/scout/pkg/runtime"
)

// shortID trims an id to a readable prefix.
func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// This file holds the general interactive pickers that make Scout's slash
// commands first-class: an inline, searchable list with a marked current row,
// dim descriptions, and a footer that names exactly what each key does.
// Typing filters; arrows move; Enter acts; Esc cancels.

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
	u.filtered = pickerFilter(u.items, u.search)
	if u.cur >= len(u.filtered) {
		u.cur = maxInt(0, len(u.filtered)-1)
	}
	if u.cur < 0 {
		u.cur = 0
	}
}

// pickerFilter ranks items by a fuzzy match on the primary label, then adds
// items whose label+detail contains the query as a substring. This keeps the
// model-selector's fuzzy ranking while still letting users search secondary
// text (a session name, a source endpoint) without fuzzy matching across
// unrelated words.
func pickerFilter(items []pickItem, query string) []pickItem {
	q := strings.TrimSpace(query)
	if q == "" {
		return items
	}
	matched := fuzzyFilter(items, q, func(it pickItem) string { return it.label })
	seen := map[pickItem]bool{}
	for _, it := range matched {
		seen[it] = true
	}
	low := strings.ToLower(q)
	for _, it := range items {
		if seen[it] {
			continue
		}
		if strings.Contains(strings.ToLower(it.label+" "+it.detail), low) {
			matched = append(matched, it)
		}
	}
	return matched
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
	b.WriteString("\n")
	search := u.search
	b.WriteString(styleModelSearch.Render("  "+search) + "\n")
	b.WriteString("\n")
	if len(u.filtered) == 0 {
		b.WriteString(stylePaletteNoMatch.Render("  No matches") + "\n")
	} else {
		const maxRows = 10
		off := scrollOffset(u.cur, len(u.filtered), maxRows)
		end := minInt(off+maxRows, len(u.filtered))
		col := listColumnWidth(u.filtered)
		for i := off; i < end; i++ {
			it := u.filtered[i]
			selected := i == u.cur
			cursor := "  "
			if selected {
				cursor = stylePaletteSel.Render("→ ")
			}
			mark := "  "
			if it.current {
				mark = styleModelEnabled.Render("✓ ")
			}
			label := cellTruncate(it.label, col-2)
			spacing := strings.Repeat(" ", maxInt(2, col-lipgloss.Width(label)))
			descWidth := width - 4 - col - 2
			desc := ""
			if descWidth > 8 {
				desc = cellTruncate(it.detail, descWidth)
			}
			if selected {
				b.WriteString(cursor + mark + stylePaletteSel.Render(label+spacing+desc) + "\n")
			} else {
				b.WriteString(cursor + mark + label + stylePaletteDesc.Render(spacing+desc) + "\n")
			}
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
	b.WriteString("\n" + styleModelScopeFooter.Render("  "+footer+" · esc cancel"))
	return strings.TrimRight(b.String(), "\n")
}

// listColumnWidth returns the primary-column width for a picker list: the
// widest visible label plus the gap, clamped to [paletteMinCol, paletteMaxCol].
func listColumnWidth(items []pickItem) int {
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

// openSources opens the work-source manager. Selecting a source opens a
// second-level action picker: test connectivity, enable/disable, show
// capabilities, or remove. This is the terminal answer to "what MCP is
// configured" — with actions, not just a list.
func (m *model) openSources() {
	conns, err := m.st.Core.Connections()
	if err != nil {
		m.println(entry{kind: eErr, text: err.Error(), at: time.Now()})
		return
	}
	if len(conns) == 0 {
		m.println(entry{kind: eNotice, text: "No work sources configured.\nAdd one in your shell: scout integrations add Upwork https://mcp.upwork.com/mcp", at: time.Now()})
		return
	}
	var items []pickItem
	for _, conn := range conns {
		state := "disabled"
		if conn.Enabled {
			state = "enabled"
			if conn.Status != "" {
				state = conn.Status
			}
		}
		detail := conn.Kind + " · " + state
		if a := conn.HumanAuth(); a != "" {
			detail += " · " + a
		}
		items = append(items, pickItem{label: conn.Name, detail: detail, value: conn.Name})
	}
	m.picker = newListPicker("Work Sources", "enter opens actions · MCP connectors & capabilities", "enter actions", items, func(name string) string {
		m.openSourceActions(name)
		return ""
	})
}

// openSourceActions shows the actions available for one configured source.
func (m *model) openSourceActions(name string) {
	conn, err := m.st.Core.FindConnection(name)
	if err != nil {
		m.println(entry{kind: eErr, text: err.Error(), at: time.Now()})
		return
	}
	status := conn.Status
	if status == "" {
		status = "not probed"
	}
	var items []pickItem
	if conn.Kind != "mcp-stdio" {
		items = append(items, pickItem{label: "Sign in", detail: "OAuth 2.1 — opens a browser to authorize", value: "signin"})
	}
	items = append(items, pickItem{label: "Test connection", detail: "read-only capability discovery", value: "test"})
	if conn.Enabled {
		items = append(items, pickItem{label: "Disable", detail: "hide from discovery and the agent", value: "disable"})
	} else {
		items = append(items, pickItem{label: "Enable", detail: "include in discovery and the agent", value: "enable"})
	}
	items = append(items, pickItem{label: "Show capabilities", detail: status + " · " + runtime.CapabilityLabels(conn.Capabilities), value: "capabilities"})
	items = append(items, pickItem{label: "Remove", detail: "delete the connector and its stored token", value: "remove"})
	statusLine := conn.Kind + " · endpoint " + sourceTarget(conn)
	m.picker = newListPicker(conn.Name, statusLine, "enter run", items, func(action string) string {
		return m.runSourceAction(name, action)
	})
	m.picker.secondaryHint = ""
}

func sourceTarget(conn *runtime.Connection) string {
	if conn.Kind == "mcp-stdio" {
		return conn.Command
	}
	return conn.Endpoint
}

// runSourceAction dispatches a source-manager action.
func (m *model) runSourceAction(name, action string) string {
	defer m.refreshConnHint()
	switch action {
	case "signin":
		_, cmd := m.startMCPLogin(name)
		m.nextCmd = cmd
		return ""
	case "test":
		conn, err := m.st.Core.ProbeConnection(context.Background(), name, 10*time.Second)
		if err != nil {
			return "test failed: " + err.Error()
		}
		line := fmt.Sprintf("%s: %s · %s · %d tools · %s", conn.Name, conn.Status, conn.HumanAuth(), conn.ToolCount, runtime.CapabilityLabels(conn.Capabilities))
		if conn.Detail != "" {
			line += "\n" + conn.Detail
		}
		m.entries = append(m.entries, entry{kind: eNotice, text: line, at: time.Now()})
		return ""
	case "enable":
		if err := m.st.Core.SetConnectionEnabled(name, true); err != nil {
			return "enable failed: " + err.Error()
		}
		return name + " enabled"
	case "disable":
		if err := m.st.Core.SetConnectionEnabled(name, false); err != nil {
			return "disable failed: " + err.Error()
		}
		return name + " disabled"
	case "capabilities":
		conn, err := m.st.Core.FindConnection(name)
		if err != nil {
			return err.Error()
		}
		m.entries = append(m.entries, entry{kind: eNotice, text: fmt.Sprintf("%s\n  kind: %s\n  target: %s\n  status: %s\n  auth: %s\n  capabilities: %s",
			conn.Name, conn.Kind, sourceTarget(conn), conn.Status, conn.HumanAuth(), runtime.CapabilityLabels(conn.Capabilities)), at: time.Now()})
		return ""
	case "remove":
		if err := m.st.Core.RemoveConnection(name); err != nil {
			return "remove failed: " + err.Error()
		}
		return name + " removed"
	}
	return ""
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

// openOpportunities opens the opportunity picker. Selecting one opens a
// second-level action picker: analyze fit, draft a proposal, or show detail.
func (m *model) openOpportunities() {
	opps, err := m.st.Core.ListOpportunities(runtime.OpportunityFilter{Limit: 50})
	if err != nil {
		m.println(entry{kind: eErr, text: err.Error(), at: time.Now()})
		return
	}
	if len(opps) == 0 {
		m.println(entry{kind: eNotice, text: "No opportunities yet. Add one with: scout opportunity add --title … --description-file …", at: time.Now()})
		return
	}
	var items []pickItem
	for _, o := range opps {
		items = append(items, pickItem{
			label:  o.Title,
			detail: o.HumanListDetail(),
			value:  o.ID,
		})
	}
	m.picker = newListPicker("Opportunities", "enter opens actions", "enter actions", items, func(id string) string {
		m.openOpportunityActions(id)
		return ""
	})
}

// openOpportunityActions shows the analyze/propose/detail choices for one
// opportunity.
func (m *model) openOpportunityActions(id string) {
	o, err := m.st.Core.GetOpportunity(id)
	if err != nil {
		m.println(entry{kind: eErr, text: err.Error(), at: time.Now()})
		return
	}
	acts := []pickItem{
		{label: "Analyze fit", detail: "score skills, budget, scope, risks", value: "analyze"},
		{label: "Draft proposal", detail: "grounded in your resume", value: "proposal"},
		{label: "Show detail", detail: "posting + evaluation + proposal", value: "detail"},
	}
	m.picker = newListPicker(o.Title, "choose an action", "enter run", acts, func(action string) string {
		return m.runOpportunityAction(action, id)
	})
}

// runOpportunityAction dispatches the chosen opportunity action. Analyze and
// draft run the worker model synchronously; the result prints inline as
// rendered markdown, so labels are bold and any markdown from the model is
// styled rather than shown as literal ** markers.
func (m *model) runOpportunityAction(action, id string) string {
	switch action {
	case "analyze":
		o, err := m.st.Core.GetOpportunity(id)
		if err != nil {
			return "analyze failed: " + err.Error()
		}
		ev, f, err := m.st.Core.Analyze(context.Background(), id, m.st.Core.EngineForRole(config.RoleWorker))
		if err != nil {
			return "analyze failed: " + err.Error()
		}
		body := "### " + strings.TrimSpace(o.Title) + "\n\n" + domain.HumanEvaluation(ev, f.Pass, f.Reason)
		m.entries = append(m.entries, entry{kind: eCommand, text: body, at: time.Now()})
		return ""
	case "proposal":
		pr, err := m.st.Core.DraftProposal(context.Background(), id, m.st.Core.EngineForRole(config.RoleWorker))
		if err != nil {
			return "draft failed: " + err.Error()
		}
		var b strings.Builder
		b.WriteString("### Proposal draft\n\n")
		b.WriteString(strings.TrimSpace(pr.CoverLetter))
		if len(pr.EvidenceIDs) > 0 {
			b.WriteString("\n\n**Grounded in:** " + strings.Join(pr.EvidenceIDs, ", "))
		}
		m.entries = append(m.entries, entry{kind: eCommand, text: b.String(), at: time.Now()})
		return ""
	case "detail":
		o, err := m.st.Core.GetOpportunity(id)
		if err != nil {
			return err.Error()
		}
		m.entries = append(m.entries, entry{kind: eCommand, text: domain.HumanOpportunity(o), at: time.Now()})
		return ""
	}
	return ""
}

// openApplications opens the application picker. Selecting one shows its
// detail; a follow-up draft is offered when it is submitted.
func (m *model) openApplications() {
	apps, err := m.st.Core.ListApplications(50)
	if err != nil {
		m.println(entry{kind: eErr, text: err.Error(), at: time.Now()})
		return
	}
	if len(apps) == 0 {
		m.println(entry{kind: eNotice, text: "No applications yet.", at: time.Now()})
		return
	}
	var items []pickItem
	for _, a := range apps {
		items = append(items, pickItem{
			label:  a.Stage + " · " + shortID(a.OpportunityID),
			detail: a.Source,
			value:  a.ID,
		})
	}
	m.picker = newListPicker("Applications", "enter shows detail", "enter detail", items, func(id string) string {
		for _, a := range apps {
			if a.ID == id {
				m.entries = append(m.entries, entry{kind: eNotice, text: fmt.Sprintf("Application %s\nOpportunity %s · %s\nStage %s · source %s", shortID(a.ID), shortID(a.OpportunityID), "", a.Stage, a.Source), at: time.Now()})
				return ""
			}
		}
		return ""
	})
}

// ---------- thinking picker (/thinking) ----------

// newThinkingPicker builds the reasoning-level selector: the current level is
// marked, each level is described with what it maps to for the current
// provider, typing filters, and Enter selects.
func newThinkingPicker(current, provider, model string, onSelect func(level string) string) *listPickerUI {
	var items []pickItem
	for _, lvl := range llm.ThinkLevels {
		items = append(items, pickItem{
			label:   lvl,
			detail:  llm.ThinkDescription(provider, lvl),
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
	return newListPicker("Thinking Level", status, "enter select", items, onSelect)
}
