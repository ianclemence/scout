package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ianclemence/scout/internal/csession"
	"github.com/ianclemence/scout/internal/isession"
	"github.com/ianclemence/scout/internal/llm"
	"github.com/ianclemence/scout/internal/runtime"
	"github.com/ianclemence/scout/internal/version"
)

type entryKind int

const (
	eUser entryKind = iota
	eScout
	eTool
	eNotice
	eErr
	eApproval
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
	selMode   string // palette, model
	palFilter string
	approval  *pendingApproval
	lastDay   string
	welcomed  bool
	quitting  bool
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

// Run starts the full-screen session. Callers must ensure a TTY.
func Run(st *isession.ReplState) error {
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
		return m, tea.Println(m.welcomeCard())
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
	if m.sel != nil {
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
			if m.selMode == "palette" && len(msg.Runes) > 0 {
				m.palFilter += string(msg.Runes)
				m.refilterPalette()
				return m, nil
			}
			if m.selMode == "palette" && msg.String() == "backspace" && len(m.palFilter) > 0 {
				m.palFilter = m.palFilter[:len(m.palFilter)-1]
				m.refilterPalette()
				return m, nil
			}
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
		m.openModelPicker()
		return m, nil
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
	if v := strings.TrimSpace(m.ta.Value()); strings.HasPrefix(v, "/") && !strings.Contains(v, " ") && len(v) <= 14 {
		m.openPalette(strings.TrimPrefix(v, "/"))
		m.ta.Reset()
		m.layoutComposer()
	}
	return m, cmd
}

// runCommand executes a slash command, capturing output into entries.
func (m *model) runCommand(line string) (tea.Model, tea.Cmd) {
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
		kind := eNotice
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

// flushCmds prints committed entries (with day dividers) to scrollback.
func (m *model) flushCmds() tea.Cmd {
	var cmds []tea.Cmd
	for _, e := range m.entries {
		for _, d := range m.dividerFor(e) {
			d := d
			cmds = append(cmds, tea.Println(d))
		}
		e := e
		cmds = append(cmds, tea.Println(m.renderEntry(e)))
	}
	m.entries = nil
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (m *model) dividerFor(e entry) []string {
	if e.at.IsZero() {
		return nil
	}
	d := dayLabel(e.at)
	if d == "" || d == m.lastDay {
		return nil
	}
	m.lastDay = d
	return []string{m.renderDayDivider(d)}
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
	case "token":
		m.stream.WriteString(ev.Text)
	case "tool_start":
		m.toolLine = ev.Name + " " + ev.Args
		m.toolName = ev.Name
		m.tools++
	case "tool_end":
		m.toolLine = ""
		m.toolName = ""
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

// resolveApproval settles the inline card: true = approve.
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
