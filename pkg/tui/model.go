package tui

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ianclemence/scout/pkg/csession"
	"github.com/ianclemence/scout/pkg/isession"
	"github.com/ianclemence/scout/pkg/llm"
	"github.com/ianclemence/scout/pkg/mcpauth"
	"github.com/ianclemence/scout/pkg/runtime"
	"github.com/ianclemence/scout/pkg/sources"
	"github.com/ianclemence/scout/pkg/version"
)

type entryKind int

const (
	eUser entryKind = iota
	eScout
	eTool
	eNotice
	eErr
	eApproval
	// eCommand is a local slash-command result. It is rendered as Scout's
	// own response (same prose styling as a model reply), because a command
	// result is Scout answering, not an aside.
	eCommand
)

type entry struct {
	kind entryKind
	text string
	dur  time.Duration
	at   time.Time
}

// evMsg carries runtime agent events into Update.
type evMsg struct{ ev runtime.Event }

// flushMsg requests a coalesced repaint of the live dock. Token events mark the
// dirty flag; this tick is what actually schedules a frame, so a fast stream
// cannot force one render per token (which flickers on a full-screen dock).
type flushMsg struct{}

type turnDoneMsg struct {
	final string
	dur   time.Duration
}

// modelsRefreshedMsg carries the outcome of a background model-catalog refresh.
type modelsRefreshedMsg struct {
	count int
	err   error
}

type spinTickMsg struct{}

// pendingApproval drives the inline approval card.
type pendingApproval struct {
	id    string
	title string
	risk  string
	sel   int // 0 approve, 1 reject
}

type selector struct {
	title string
	items []selItem
	cur   int
}

type selItem struct {
	label  string
	detail string
	value  string
}

type model struct {
	st       *isession.ReplState
	ta       textarea.Model
	prog     *tea.Program
	entries  []entry
	width    int
	height   int
	ready    bool
	working  bool
	turnFrom time.Time
	// stream holds the prose of the turn currently in flight. It is reset at
	// every turn_start so one turn's narration can never concatenate with the
	// next (the bug that produced a wall of "Let me pull…" preambles).
	stream strings.Builder
	// answer holds the final answer segment — the one turn that ended without
	// a tool call. Only this is committed to the transcript; per-turn
	// preambles are process narration and are discarded once superseded.
	answer strings.Builder
	// answerSet reports whether the final answer has been captured this run.
	answerSet bool
	toolLine  string
	toolName  string
	tools     int
	turns     int
	// streamDirty is set when a token arrives and cleared on the coalesced
	// flush tick; while dirty the dock keeps the last rendered frame.
	streamDirty bool
	// streamHeaderShown tracks whether the assistant header line has been
	// printed for the current turn; streamFlushedLines counts how many
	// completed streaming lines already reached the scrollback. Together
	// they let the reply grow line-by-line in the conversation area (Ghost
	// parity) with no reprint at completion.
	streamHeaderShown  bool
	streamFlushedLines int
	streamSty          streamStyler
	// lastFlush is the most recent block printed to the scrollback. It lets
	// tests assert on committed output after the buffer drains.
	lastFlush string
	spin      int
	cancel    context.CancelFunc
	sel       *selector
	selMode   string // palette, login, logout
	palFilter string
	login     *loginFlowUI
	approval  *pendingApproval
	// mcpLogin/mcpFlow track an in-flight MCP OAuth sign-in.
	mcpLogin      *mcpLoginUI
	mcpFlow       *mcpauth.Flow
	mcpFlowCancel context.CancelFunc
	// nextCmd is a command produced by a picker action (e.g. starting an MCP
	// login) that the next Update should run alongside the flushed output.
	nextCmd tea.Cmd
	// connHint is a cached left-footer suffix naming a configured source
	// that needs authentication (e.g. "Upwork needs auth"). It is refreshed
	// when sources change, never queried every frame.
	connHint string
	// modelSel is the interactive model dialog; it renders in the dock below
	// the composer.
	modelSel *modelPickerUI
	// picker is the general searchable list used by /thinking, /sessions,
	// /approvals, /skills, /tools. It renders in the dock like the others.
	picker   *listPickerUI
	welcomed bool
	quitting bool
}

func initialModel(st *isession.ReplState) *model {
	ta := textarea.New()
	ta.Placeholder = ""
	ta.Prompt = ""
	ta.CharLimit = 8000
	ta.SetWidth(80)
	ta.SetHeight(1)
	ta.ShowLineNumbers = false
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.BlurredStyle.CursorLine = lipgloss.NewStyle()
	ta.Focus()
	m := &model{st: st, ta: ta}
	// Bind the UI callbacks up front so any picker opened from the palette has
	// a live action to run; without this a session picker could print a notice
	// instead of switching.
	if st != nil {
		st.SwitchSession = m.switchSession
		st.OpenModelSelector = m.openModelSelector
		st.OpenThinking = m.openThinking
		st.OpenSessions = func() { m.nextCmd = m.openSessions() }
		st.OpenApprovals = func() { m.nextCmd = m.openApprovals() }
		st.OpenSources = func() { m.nextCmd = m.openSources() }
	}
	return m
}

// silenceStderr parks the process stderr on /dev/null and silences the std
// logger, returning a restore function. A stray write from any dependency can
// otherwise corrupt the live composer frame; model-loop internals must never
// reach the chat. Stdout stays untouched (the renderer writes there).
func silenceStderr() func() {
	log.SetOutput(io.Discard)
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return func() { log.SetOutput(os.Stderr) }
	}
	orig := os.Stderr
	os.Stderr = devnull
	return func() {
		os.Stderr = orig
		log.SetOutput(os.Stderr)
		_ = devnull.Close()
	}
}

// Run starts the full-screen session. Callers must ensure a TTY.
func Run(st *isession.ReplState) error {
	// Park stderr on /dev/null for the lifetime of the TUI so a stray write
	// from any dependency (the std logger, a subprocess, a provider client)
	// can never paint over the live composer frame. Stdout is untouched: the
	// renderer writes there. This keeps a stray write from corrupting the frame.
	restore := silenceStderr()
	defer restore()

	m := initialModel(st)
	p := tea.NewProgram(m)
	m.prog = p
	_, err := p.Run()
	return err
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.welcomeCmd(), m.historyCmd())
}

func (m *model) welcomeCmd() tea.Cmd {
	return func() tea.Msg {
		m.refreshConnHint()
		n, _ := m.st.Core.SessionMessageCount(m.st.Sess.ID)
		if n == 0 && !m.welcomed {
			m.welcomed = true
			return welcomeMsg{}
		}
		return nil
	}
}

// historyCmd loads the session's prior turns and returns them for rendering
// into the transcript, so opening or resuming a session shows the conversation
// instead of an empty screen. The same history is already loaded into model
// context; this makes it visible to the human.
type historyMsg struct{ rendered string }

func (m *model) historyCmd() tea.Cmd {
	return func() tea.Msg {
		return historyMsg{rendered: m.renderHistory()}
	}
}

// renderHistory builds the scrollback block for a session's prior turns: a
// header, then each turn rendered exactly like a live one (user bubble,
// assistant block), so restored history is visually identical to how it looked
// when first exchanged.
func (m *model) renderHistory() string {
	msgs, err := csession.LoadMessages(m.st.Core.DB, m.st.Sess.ID, 40)
	if err != nil || len(msgs) == 0 {
		return ""
	}
	var blocks []string
	blocks = append(blocks, styleDayDivider.Render("── Earlier in this session "+strings.Repeat("─", maxInt(1, m.width-28))))
	for _, mm := range msgs {
		switch mm.Role {
		case "user":
			blocks = append(blocks, m.renderEntry(entry{kind: eUser, text: mm.Content, at: time.Now()}))
		case "assistant":
			if strings.TrimSpace(mm.Content) == "" {
				continue
			}
			blocks = append(blocks, m.renderEntry(entry{kind: eScout, text: mm.Content, at: time.Now()}))
		}
	}
	return strings.Join(blocks, "\n\n")
}

type welcomeMsg struct{}

// discoverDoneMsg carries the outcome of an async /discover run, so the source
// search streams progress and never freezes the UI.
type discoverDoneMsg struct {
	found, stored int
	sources       int
	warnings      []string
	err           error
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.ta.SetWidth(msg.Width)
		m.layoutComposer()
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case evMsg:
		return m.handleEvent(msg.ev)
	case turnDoneMsg:
		nm, cmd := m.finishTurn(msg.final, msg.dur)
		return nm, cmd
	case spinTickMsg:
		if m.working {
			m.spin++
			return m, tickSpin()
		}
		return m, nil
	case flushMsg:
		m.streamDirty = false
		return m, nil
	case welcomeMsg:
		// Release notes are available on demand (/changelog, `scout update`);
		// they are deliberately not injected into the welcome card.
		return m, tea.Println(m.welcomeCard())
	case historyMsg:
		if msg.rendered == "" {
			return m, nil
		}
		return m, tea.Println(msg.rendered)
	case discoverDoneMsg:
		return m.finishDiscover(msg)
	case modelsRefreshedMsg:
		return m.handleModelsRefreshed(msg)
	case mcpLoginResultMsg:
		return m.finishMCPLogin(msg)
	}
	if m.sel == nil && m.approval == nil {
		var cmd tea.Cmd
		m.ta, cmd = m.ta.Update(msg)
		m.layoutComposer()
		return m, cmd
	}
	return m, nil
}

func tickSpin() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return spinTickMsg{} })
}

// tickFlush schedules a coalesced repaint of the live dock. Each token marks
// the stream dirty; at most one frame is scheduled per interval, so a burst of
// tokens becomes one render instead of hundreds.
func tickFlush() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg { return flushMsg{} })
}

// layoutComposer grows the composer with content, capped like a real editor.
func (m *model) layoutComposer() {
	rows := 1
	w := m.width
	if w < 20 {
		w = 20
	}
	for _, line := range strings.Split(m.ta.Value(), "\n") {
		n := visualRows(line, w)
		rows += n
	}
	_ = rows
	n := composerRows(m.ta.Value(), w)
	m.ta.SetHeight(n)
}

func composerRows(v string, w int) int {
	n := 0
	for _, line := range strings.Split(v, "\n") {
		n += visualRows(line, w)
	}
	if n < 1 {
		n = 1
	}
	if cap := composerCap(w); n > cap {
		n = cap
	}
	return n
}

func composerCap(w int) int {
	_ = w
	return 7
}

func visualRows(s string, w int) int {
	if w < 1 {
		w = 1
	}
	if s == "" {
		return 1
	}
	rows, col := 1, 0
	for _, word := range strings.Fields(s) {
		ww := len([]rune(word))
		if col == 0 {
			for ww > w {
				rows++
				ww -= w
			}
			col = ww
			continue
		}
		if col+1+ww > w {
			rows++
			col = 0
			for ww > w {
				rows++
				ww -= w
			}
			col = ww
			continue
		}
		col += 1 + ww
	}
	return rows
}

func (m *model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mcpLogin != nil {
		return m.handleMCPLoginKey(msg)
	}
	if m.login != nil {
		return m.handleLoginKey(msg)
	}
	if m.approval != nil {
		switch msg.String() {
		case "1":
			nm, cmd := m.resolveApproval(true)
			return nm, cmd
		case "2":
			nm, cmd := m.resolveApproval(false)
			return nm, cmd
		case "left", "right", "tab":
			m.approval.sel = 1 - m.approval.sel
			return m, nil
		case "enter":
			nm, cmd := m.resolveApproval(m.approval.sel == 0)
			return nm, cmd
		case "esc":
			m.approval = nil
			return m, nil
		default:
			return m, nil
		}
	}
	if m.modelSel != nil {
		key := msg.String()
		sel, doSelect, setDefault, cancel := m.modelSel.handleKey(key)
		if cancel {
			m.modelSel = nil
			return m, nil
		}
		if doSelect || setDefault {
			m.modelSel = nil
			return m.applyModelSelection(sel.Provider, sel.ID, setDefault)
		}
		return m, nil
	}
	if m.picker != nil {
		switch m.picker.handleKey(msg.String()) {
		case "cancel":
			m.picker = nil
			return m, nil
		case "pick":
			if it, ok := m.picker.picked(); ok {
				act := m.picker.act
				m.picker = nil
				if act != nil {
					if out := act(it.value); out != "" {
						m.println(entry{kind: eNotice, text: out, at: time.Now()})
					}
				}
				return m, tea.Batch(m.flushCmds(), m.takeNextCmd())
			}
			return m, nil
		case "secondary":
			if it, ok := m.picker.picked(); ok && m.picker.secondary != nil {
				if out := m.picker.secondary(it.value); out != "" {
					m.println(entry{kind: eNotice, text: out, at: time.Now()})
				}
				return m, m.flushCmds()
			}
			return m, nil
		}
		return m, nil
	}
	if m.sel != nil && m.selMode == "palette" {
		switch msg.String() {
		case "esc", "ctrl+c":
			m.sel = nil
			m.palFilter = ""
			m.ta.Reset()
			m.layoutComposer()
			return m, nil
		case "up", "ctrl+p":
			if m.sel.cur > 0 {
				m.sel.cur--
			}
			return m, nil
		case "down", "ctrl+n":
			if m.sel.cur < len(m.sel.items)-1 {
				m.sel.cur++
			}
			return m, nil
		case "enter", "tab":
			nm, cmd := m.pickSelected()
			return nm, cmd
		}
		// All other keys edit the visible composer text; the palette
		// filters from what is actually typed. Clearing the "/" (or
		// typing a space) dismisses the palette.
		var cmd tea.Cmd
		m.ta, cmd = m.ta.Update(msg)
		m.layoutComposer()
		v := strings.TrimSpace(m.ta.Value())
		if !strings.HasPrefix(v, "/") || strings.Contains(v, " ") {
			m.sel = nil
			m.palFilter = ""
			return m, cmd
		}
		m.palFilter = strings.TrimPrefix(v, "/")
		m.refilterPalette()
		return m, cmd
	}
	if m.sel != nil {
		// Model picker (and any non-palette selector): navigation only.
		switch msg.String() {
		case "esc", "ctrl+c":
			m.sel = nil
			m.palFilter = ""
			return m, nil
		case "up", "ctrl+p":
			if m.sel.cur > 0 {
				m.sel.cur--
			}
			return m, nil
		case "down", "ctrl+n":
			if m.sel.cur < len(m.sel.items)-1 {
				m.sel.cur++
			}
			return m, nil
		case "enter", "tab":
			nm, cmd := m.pickSelected()
			return nm, cmd
		default:
			return m, nil
		}
	}
	switch msg.String() {
	case "ctrl+c":
		if m.working && m.cancel != nil {
			m.cancel()
			return m, nil
		}
		m.quitting = true
		return m, tea.Quit
	case "esc":
		if m.working && m.cancel != nil {
			m.cancel()
			return m, nil
		}
		m.quitting = true
		return m, tea.Quit
	case "ctrl+l":
		m.openModelSelector("")
		return m, m.takeModelRefresh()
	case "enter":
		if m.working {
			return m, nil
		}
		line := strings.TrimSpace(m.ta.Value())
		m.ta.Reset()
		m.layoutComposer()
		if line == "" {
			return m, nil
		}
		if strings.HasPrefix(line, "/") {
			nm, cmd := m.runCommand(strings.TrimPrefix(line, "/"))
			return nm, cmd
		}
		nm, cmd := m.startTurn(line)
		return nm, cmd
	}
	var cmd tea.Cmd
	m.ta, cmd = m.ta.Update(msg)
	m.layoutComposer()
	// Typing "/" opens the palette; the slash stays visible and editable.
	// Backspacing it away (or typing a space) dismisses the palette.
	if v := strings.TrimSpace(m.ta.Value()); strings.HasPrefix(v, "/") && !strings.Contains(v, " ") && len(v) <= 14 {
		m.openPalette(strings.TrimPrefix(v, "/"))
	}
	return m, cmd
}

// runCommand executes a slash command, capturing output into entries.
// login/logout open staged TUI flows, never the cooked prompt (which cannot
// work in raw terminal mode).
func (m *model) runCommand(line string) (tea.Model, tea.Cmd) {
	name := line
	args := ""
	if i := strings.Index(line, " "); i >= 0 {
		name, args = line[:i], strings.TrimSpace(line[i+1:])
	}
	// Bind the UI callbacks BEFORE any branch can open a picker. Several
	// commands (sessions, resume, model, …) open a picker that later invokes
	// these; binding them here means the picker's action can never observe a
	// nil callback and fall back to printing a notice instead of acting.
	m.st.SwitchSession = m.switchSession
	m.st.OpenModelSelector = m.openModelSelector
	m.st.OpenThinking = m.openThinking
	m.st.OpenSessions = func() { m.nextCmd = m.openSessions() }
	m.st.OpenApprovals = func() { m.nextCmd = m.openApprovals() }
	m.st.OpenSources = func() { m.nextCmd = m.openSources() }
	switch name {
	case "discover":
		// Discovery hits a live source and can take seconds; run it async so the
		// dock keeps its spinner and never freezes.
		return m.startDiscover(args)
	case "login":
		m.openLoginFlow(args)
		return m, m.flushCmds()
	case "logout":
		if strings.TrimSpace(args) != "" {
			nm, cmd := m.removeStoredKey(strings.ToLower(strings.TrimSpace(args)))
			return nm, cmd
		}
		m.openLogoutFlow()
		return m, m.flushCmds()
	case "thinking", "think":
		// Bare /thinking opens the interactive reasoning selector; an
		// argument is handled by the shared command (validated + applied).
		if strings.TrimSpace(args) == "" {
			m.openThinking()
			return m, nil
		}
	case "sessions":
		if strings.TrimSpace(args) == "" {
			m.openSessions()
			return m, nil
		}
	case "resume":
		if strings.TrimSpace(args) == "" {
			m.openSessions()
			return m, nil
		}
	case "approvals":
		if strings.TrimSpace(args) == "" {
			return m, m.openApprovals()
		}
	case "opportunities", "opps":
		if strings.TrimSpace(args) == "" {
			return m, m.openOpportunities()
		}
	case "applications", "apps":
		if strings.TrimSpace(args) == "" {
			return m, m.openApplications()
		}
	case "sources", "integrations":
		if strings.TrimSpace(args) == "" {
			return m, m.openSources()
		}
		if sub := strings.Fields(args); len(sub) >= 2 && sub[0] == "login" {
			return m.startMCPLogin(sub[1])
		}
	}
	m.st.Width = m.width
	var out strings.Builder
	st := m.st
	prev := st.Out
	st.Out = func(f string, a ...any) { fmt.Fprintf(&out, f, a...) }
	err := isession.Dispatch(st, line)
	st.Out = prev
	if err != nil {
		m.println(entry{kind: eErr, text: err.Error(), at: time.Now()})
		return m, m.flushCmds()
	}
	if s := strings.TrimRight(out.String(), "\n"); s != "" {
		// A local command's result is Scout answering: render it in the same
		// prose style as a model reply. Approvals keep their distinct card.
		kind := eCommand
		if strings.HasPrefix(line, "approvals") {
			kind = eApproval
		}
		m.println(entry{kind: kind, text: s, at: time.Now()})
	}
	cmds := m.flushCmds()
	if cmd := m.takeModelRefresh(); cmd != nil {
		cmds = tea.Batch(cmds, cmd)
	}
	return m, cmds
}

func (m *model) println(e entry) *model {
	m.entries = append(m.entries, e)
	return m
}

// takeNextCmd returns and clears a command produced by a picker action.
func (m *model) takeNextCmd() tea.Cmd {
	c := m.nextCmd
	m.nextCmd = nil
	return c
}

// flushCmds prints committed entries to scrollback as one block, separated
// by a blank line. Entries are grouped so the transcript reads as discrete
// messages, not a wall of text, and no date divider is emitted.
func (m *model) flushCmds() tea.Cmd {
	if len(m.entries) == 0 {
		return nil
	}
	var blocks []string
	for _, e := range m.entries {
		blocks = append(blocks, m.renderEntry(e))
	}
	m.entries = nil
	return tea.Println(strings.Join(blocks, "\n\n"))
}

// startDiscover launches a discovery run in the background so the UI keeps
// streaming (spinner + activity) instead of freezing while the source search
// completes. The result arrives as discoverDoneMsg.
func (m *model) startDiscover(query string) (tea.Model, tea.Cmd) {
	m.working = true
	m.turnFrom = time.Now()
	m.toolName = ""
	m.println(entry{kind: eUser, text: "/discover " + strings.TrimSpace(query), at: time.Now()})
	prog := m.prog
	core := m.st.Core
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		res, err := core.DiscoverSources(ctx, sources.SearchFilter{Query: strings.TrimSpace(query), Limit: 20})
		if prog == nil {
			return // headless (tests): no UI to notify
		}
		prog.Send(discoverDoneMsg{
			found: res.Found, stored: res.Stored, sources: len(res.Sources),
			warnings: res.Warnings, err: err,
		})
	}()
	return m, tea.Batch(m.flushCmds(), tickSpin())
}

// finishDiscover commits the discovery result to the transcript.
func (m *model) finishDiscover(msg discoverDoneMsg) (tea.Model, tea.Cmd) {
	m.working = false
	if msg.err != nil {
		m.println(entry{kind: eErr, text: "discovery failed: " + msg.err.Error(), at: time.Now()})
		return m, m.flushCmds()
	}
	if msg.sources == 0 {
		m.println(entry{kind: eNotice, text: "No connected sources yet. Add one in your shell: scout integrations add Upwork https://mcp.upwork.com/mcp", at: time.Now()})
		return m, m.flushCmds()
	}
	var b strings.Builder
	b.WriteString("**Search**\n\n")
	b.WriteString(fmt.Sprintf("- Searched **%d** source(s)\n", msg.sources))
	b.WriteString(fmt.Sprintf("- Found **%d** · stored **%d** new\n", msg.found, msg.stored))
	for _, w := range msg.warnings {
		b.WriteString("- ⚠ " + w + "\n")
	}
	if msg.stored > 0 {
		b.WriteString("\nReview them with `/opportunities`.\n")
	}
	m.println(entry{kind: eCommand, text: b.String(), at: time.Now()})
	return m, m.flushCmds()
}

// startTurn launches the agent goroutine.
func (m *model) startTurn(line string) (tea.Model, tea.Cmd) {
	eng := m.st.Core.EngineFor(m.st.Sess.Provider, m.st.Sess.Model)
	if eng.LLM == nil {
		m.println(entry{kind: eUser, text: line, at: time.Now()})
		m.println(entry{kind: eErr, text: "No model configured. /login <provider> or /model ollama/<model>.", at: time.Now()})
		return m, m.flushCmds()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.working = true
	m.turnFrom = time.Now()
	m.stream.Reset()
	m.answer.Reset()
	m.answerSet = false
	m.streamDirty = false
	m.streamHeaderShown = false
	m.streamFlushedLines = 0
	m.streamSty = streamStyler{width: streamWidth(m)}
	m.toolLine = ""
	m.toolName = ""
	m.tools = 0
	m.st.History = append(m.st.History, llm.Message{Role: "user", Content: line})
	_ = csession.AppendMessages(m.st.Core.DB, m.st.Sess.ID, []csession.Message{{Role: "user", Content: line}})
	// Auto-name untitled sessions from the first message.
	if m.st.Sess.Name == "interactive" || m.st.Sess.Name == "session" {
		if name := autoName(line); name != "" {
			_ = csession.Rename(m.st.Core.DB, m.st.Sess.ID, name)
			m.st.Sess.Name = name
		}
	}
	msgs := append([]llm.Message{}, m.st.History...)
	prog := m.prog
	printUser := tea.Println(m.renderEntry(entry{kind: eUser, text: line, at: time.Now()}))
	go func() {
		var final strings.Builder
		_, _ = m.st.Core.RunAgent(ctx, eng, msgs, m.st.Sess.Thinking, func(ev runtime.Event) {
			if ev.Type == "token" {
				final.WriteString(ev.Text)
			}
			prog.Send(evMsg{ev: ev})
		})
		prog.Send(turnDoneMsg{final: final.String(), dur: time.Since(m.turnFrom)})
	}()
	return m, tea.Batch(printUser, tickSpin(), tickFlush())
}

func (m *model) handleEvent(ev runtime.Event) (tea.Model, tea.Cmd) {
	switch ev.Type {
	case "turn_start":
		// A new turn starts a fresh prose segment. Without this reset, every
		// turn's narration accumulated into one blob and was shown as if it
		// were a single answer.
		m.stream.Reset()
		m.toolLine = ""
		m.toolName = ""
		// The progressive printer counts against the buffer: a reset buffer
		// restarts the count (the header stays as-is — one header per turn,
		// never one per tool iteration).
		m.streamFlushedLines = 0
	case "token":
		m.stream.WriteString(ev.Text)
		// Coalesce renders: mark dirty and let the frame tick repaint, rather
		// than scheduling a render for every token. The reply itself grows
		// line-by-line in the scrollback via flushStreamLines.
		if !m.streamDirty {
			m.streamDirty = true
		}
		return m, m.flushStreamLines()
	case "tool_start":
		// The turn's prose ended where the tool call began: this segment is
		// process narration, not the answer, so it is superseded rather than
		// accumulated. Defensively drop any buffered tool fence too.
		if strings.Contains(m.stream.String(), "```tool") {
			m.stream.Reset()
		}
		m.toolName = ev.Name
		m.tools++
	case "tool_end":
		m.toolLine = ""
		m.toolName = ""
	case "skill":
		m.toolLine = "skill: " + ev.Name
	case "agent_end":
		// The loop only reaches agent_end when a turn produced no tool call:
		// this is the answer. Capture it as the committed text.
		m.answer.Reset()
		m.answer.WriteString(ev.Text)
		m.answerSet = true
	case "error":
		m.println(entry{kind: eErr, text: ev.Err.Error(), at: time.Now()})
	}
	return m, nil
}

func (m *model) finishTurn(final string, dur time.Duration) (tea.Model, tea.Cmd) {
	m.working = false
	m.cancel = nil
	// Prefer the text captured from agent_end: that is the turn with no tool
	// call. The loop's return value is a fallback for paths that bypass the
	// event (or an interrupted run).
	commit := strings.TrimSpace(m.answer.String())
	if commit == "" {
		commit = strings.TrimSpace(final)
	}
	if commit == "" {
		if n := len(m.st.History); n > 0 {
			m.st.History = m.st.History[:n-1]
		}
		m.stream.Reset()
		m.answer.Reset()
		m.answerSet = false
		m.streamHeaderShown = false
		m.streamFlushedLines = 0
		return m, m.flushCmds()
	}
	m.st.History = append(m.st.History, llm.Message{Role: "assistant", Content: commit})
	_ = csession.AppendMessages(m.st.Core.DB, m.st.Sess.ID, []csession.Message{
		{Role: "assistant", Content: commit},
	})
	if len(m.st.History) > 40 {
		m.st.History = m.st.History[len(m.st.History)-40:]
	}
	m.turns++
	var tailCmd tea.Cmd
	if m.streamHeaderShown {
		// The reply already grew line-by-line in the scrollback; print
		// only the unprinted tail, never the whole text again.
		if tail := m.streamTail(); tail != "" {
			m.lastFlush += "\n" + tail
			tailCmd = tea.Println(tail)
		}
	} else {
		m.println(entry{kind: eScout, text: commit, dur: dur, at: time.Now()})
	}
	csession.Touch(m.st.Core.DB, m.st.Sess.ID, m.st.Sess.Provider, m.st.Sess.Model)
	flush := m.flushCmds()
	if tailCmd != nil {
		if flush != nil {
			flush = tea.Batch(tailCmd, flush)
		} else {
			flush = tailCmd
		}
	}
	// Inline approval card for newly created pending actions.
	if pend, _ := m.st.Core.PendingApprovals(); len(pend) > 0 && m.approval == nil {
		p := pend[0]
		m.approval = &pendingApproval{id: p.ID, title: p.ActionType + " → " + p.Target, risk: p.RiskLevel}
	}
	m.stream.Reset()
	m.answer.Reset()
	m.answerSet = false
	m.streamDirty = false
	m.streamHeaderShown = false
	m.streamFlushedLines = 0
	m.streamSty = streamStyler{width: streamWidth(m)}
	return m, flush
}

// removeStoredKey deletes a stored credential. Environment variables are
// never touched — logout only removes credentials saved by /login.
func (m *model) removeStoredKey(provider string) (tea.Model, tea.Cmd) {
	if !validLoginProvider(provider) {
		return m, tea.Println(renderEntryStatic(entry{kind: eErr, text: "Unknown provider: " + provider}))
	}
	if _, err := m.st.Core.DB.DB.Exec(`DELETE FROM secrets WHERE key=?`, "llm:"+provider); err != nil {
		return m, tea.Println(renderEntryStatic(entry{kind: eErr, text: err.Error()}))
	}
	return m, tea.Println(renderEntryStatic(entry{kind: eNotice, text: "Removed stored key for " + provider + ". Environment variables are unchanged."}))
}

// autoName derives a session title from the first user message.
func autoName(line string) string {
	s := strings.Join(strings.Fields(line), " ")
	r := []rune(s)
	if len(r) > 40 {
		s = string(r[:40]) + "…"
	}
	return s
}
func (m *model) switchSession(s *csession.Session) error {
	m.st.Sess = s
	m.st.History = nil
	m.st.LastOpps = nil
	if msgs, err := csession.LoadMessages(m.st.Core.DB, s.ID, 40); err == nil {
		for _, mm := range msgs {
			m.st.History = append(m.st.History, llm.Message{Role: mm.Role, Content: mm.Content})
		}
	}
	return nil
}

func (m *model) resolveApproval(approve bool) (tea.Model, tea.Cmd) {
	a := m.approval
	m.approval = nil
	if a == nil {
		return m, nil
	}
	status := "rejected"
	verb := "rejected"
	if approve {
		status = "approved"
		verb = "approved"
	}
	if err := m.st.Core.SetApprovalStatus(a.id, status); err != nil {
		return m, tea.Println(renderEntryStatic(entry{kind: eErr, text: err.Error()}))
	}
	return m, tea.Println(renderEntryStatic(entry{kind: eNotice, text: fmt.Sprintf("%s %s.", a.title, verb)}))
}

func renderEntryStatic(e entry) string {
	return (&model{width: 80}).renderEntry(e)
}

var _ = version.Version
