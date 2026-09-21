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
	"github.com/ianclemence/scout/pkg/oauth"
	"github.com/ianclemence/scout/pkg/runtime"
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

type turnDoneMsg struct {
	final string
	dur   time.Duration
}

// oauthResultMsg carries the outcome of a background account sign-in.
type oauthResultMsg struct {
	provider string
	cred     *oauth.Credential
	err      error
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
	st        *isession.ReplState
	ta        textarea.Model
	prog      *tea.Program
	entries   []entry
	width     int
	height    int
	ready     bool
	working   bool
	turnFrom  time.Time
	stream    strings.Builder
	toolLine  string
	toolName  string
	tools     int
	turns     int
	spin      int
	cancel    context.CancelFunc
	sel       *selector
	selMode   string // palette, login, logout
	palFilter string
	login     *loginFlowUI
	approval  *pendingApproval
	// oauthFlow is the live account sign-in flow, if any.
	oauthFlow *oauth.Flow
	// connHint is a cached left-footer suffix naming a configured source
	// that needs authentication (e.g. "Upwork needs auth"). It is refreshed
	// when sources change, never queried every frame.
	connHint string
	// scopedSel and modelSel are the interactive model dialogs. Only one may
	// be open at a time; both render in the dock below the composer.
	scopedSel *scopedModelsUI
	modelSel  *modelPickerUI
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
	return &model{st: st, ta: ta}
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
	return tea.Batch(textarea.Blink, m.welcomeCmd())
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

type welcomeMsg struct{}

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
	case welcomeMsg:
		// Release notes are available on demand (/changelog, `scout update`);
		// they are deliberately not injected into the welcome card.
		return m, tea.Println(m.welcomeCard())
	case oauthResultMsg:
		return m.finishOAuthLogin(msg)
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
	if m.scopedSel != nil {
		key := msg.String()
		if key == "esc" || key == "ctrl+c" {
			m.scopedSel = nil
			return m, nil
		}
		if persist := m.scopedSel.handleKey(key); persist {
			ids := m.scopedSel.ids
			if err := m.st.SetScopedModels(ids); err != nil {
				m.scopedSel = nil
				return m, tea.Println(renderEntryStatic(entry{kind: eErr, text: err.Error()}))
			}
			m.scopedSel.dirty = false
			return m, tea.Println(styleNotice.Render("Model selection saved to settings"))
		}
		return m, nil
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
				return m, m.flushCmds()
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
		return m, nil
	case "ctrl+p":
		return m.cycleModel(1)
	case "shift+ctrl+p":
		return m.cycleModel(-1)
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
	switch name {
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
			m.openApprovals()
			return m, nil
		}
	case "opportunities", "opps":
		if strings.TrimSpace(args) == "" {
			m.openOpportunities()
			return m, m.flushCmds()
		}
	case "skills":
		if strings.TrimSpace(args) == "" {
			m.openSkills()
			return m, m.flushCmds()
		}
	case "applications", "apps":
		if strings.TrimSpace(args) == "" {
			m.openApplications()
			return m, m.flushCmds()
		}
	case "sources", "integrations":
		if strings.TrimSpace(args) == "" {
			m.openSources()
			return m, m.flushCmds()
		}
	}
	m.st.Width = m.width
	m.st.SwitchSession = m.switchSession
	m.st.OpenScopedModels = m.openScopedModels
	m.st.OpenModelSelector = m.openModelSelector
	m.st.OpenThinking = m.openThinking
	m.st.OpenSessions = m.openSessions
	m.st.OpenApprovals = m.openApprovals
	m.st.OpenSources = m.openSources
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
	return m, m.flushCmds()
}

func (m *model) println(e entry) *model {
	m.entries = append(m.entries, e)
	return m
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
	return m, tea.Batch(printUser, tickSpin())
}

func (m *model) handleEvent(ev runtime.Event) (tea.Model, tea.Cmd) {
	switch ev.Type {
	case "turn_start":
		m.toolLine = ""
		m.toolName = ""
	case "token":
		m.stream.WriteString(ev.Text)
	case "tool_start":
		// The loop filters tool fences out of the token stream; defensively
		// drop any buffered preview that still contains one so tool machinery
		// can never linger on screen. The composer names the activity instead.
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
	case "error":
		m.println(entry{kind: eErr, text: ev.Err.Error(), at: time.Now()})
	}
	return m, nil
}

func (m *model) finishTurn(final string, dur time.Duration) (tea.Model, tea.Cmd) {
	m.working = false
	m.cancel = nil
	if strings.TrimSpace(final) == "" {
		if n := len(m.st.History); n > 0 {
			m.st.History = m.st.History[:n-1]
		}
		m.stream.Reset()
		return m, m.flushCmds()
	}
	m.st.History = append(m.st.History, llm.Message{Role: "assistant", Content: final})
	_ = csession.AppendMessages(m.st.Core.DB, m.st.Sess.ID, []csession.Message{
		{Role: "assistant", Content: final},
	})
	if len(m.st.History) > 40 {
		m.st.History = m.st.History[len(m.st.History)-40:]
	}
	m.turns++
	m.println(entry{kind: eScout, text: final, dur: dur, at: time.Now()})
	csession.Touch(m.st.Core.DB, m.st.Sess.ID, m.st.Sess.Provider, m.st.Sess.Model)
	flush := m.flushCmds()
	// Inline approval card for newly created pending actions.
	if pend, _ := m.st.Core.PendingApprovals(); len(pend) > 0 && m.approval == nil {
		p := pend[0]
		m.approval = &pendingApproval{id: p.ID, title: p.ActionType + " → " + p.Target, risk: p.RiskLevel}
	}
	m.stream.Reset()
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
