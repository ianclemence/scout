package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ianclemence/scout/internal/csession"
	"github.com/ianclemence/scout/internal/isession"
	"github.com/ianclemence/scout/internal/llm"
	"github.com/ianclemence/scout/internal/runtime"
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
}

// evMsg carries runtime agent events into Update.
type evMsg struct{ ev runtime.Event }

type turnDoneMsg struct{ final string }

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
	entries   []entry // committed this view (also flushed to scrollback)
	width     int
	height    int
	ready     bool
	working   bool
	stream    strings.Builder
	toolLine  string
	tools     int
	turns     int
	cancel    context.CancelFunc
	sel       *selector
	selMode   string // palette, model
	palFilter string
	quitting  bool
}

func initialModel(st *isession.ReplState) *model {
	ta := textarea.New()
	ta.Placeholder = ""
	ta.Prompt = "› "
	ta.CharLimit = 4000
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
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
	return textarea.Blink
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.ta.SetWidth(msg.Width - 6)
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case evMsg:
		return m.handleEvent(msg.ev)
	case turnDoneMsg:
		nm, cmd := m.finishTurn(msg.final)
		return nm, cmd
	}
	if m.sel == nil {
		var cmd tea.Cmd
		m.ta, cmd = m.ta.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Selector open: navigation keys belong to it.
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
		case "enter":
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
	// Typing "/" first opens the palette.
	if strings.HasPrefix(strings.TrimSpace(m.ta.Value()), "/") && m.ta.Value() != "" {
		v := strings.TrimSpace(m.ta.Value())
		if !strings.Contains(v, " ") && len(v) <= 12 {
			m.openPalette(strings.TrimPrefix(v, "/"))
			m.ta.Reset()
		}
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
		m.println(entry{kind: eErr, text: err.Error()})
		return m, m.flushCmds()
	}
	if s := strings.TrimRight(out.String(), "\n"); s != "" {
		kind := eNotice
		if strings.HasPrefix(line, "approvals") {
			kind = eApproval
		}
		m.println(entry{kind: kind, text: s})
	}
	return m, m.flushCmds()
}

func (m *model) println(e entry) *model {
	m.entries = append(m.entries, e)
	return m
}

// flushCmds prints committed entries to the terminal scrollback and clears
// the buffer. The transcript lives in scrollback; the live view only shows
// the streaming preview, composer, and footer.
func (m *model) flushCmds() tea.Cmd {
	var cmds []tea.Cmd
	for _, e := range m.entries {
		e := e
		cmds = append(cmds, tea.Println(renderEntry(e)))
	}
	m.entries = nil
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// startTurn prints the user message and launches the agent goroutine.
func (m *model) startTurn(line string) (tea.Model, tea.Cmd) {
	eng := m.st.Core.EngineFor(m.st.Sess.Provider, m.st.Sess.Model)
	if eng.LLM == nil {
		m.println(entry{kind: eUser, text: line})
		m.println(entry{kind: eErr, text: "No model configured. /login <provider> or /model ollama/<model>."})
		return m, m.flushCmds()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.working = true
	m.stream.Reset()
	m.toolLine = ""
	m.tools = 0
	m.st.History = append(m.st.History, llm.Message{Role: "user", Content: line})
	_ = csession.AppendMessages(m.st.Core.DB, m.st.Sess.ID, []csession.Message{{Role: "user", Content: line}})
	msgs := append([]llm.Message{}, m.st.History...)
	prog := m.prog
	printUser := tea.Println(renderEntry(entry{kind: eUser, text: line}))
	go func() {
		var final strings.Builder
		_, _ = m.st.Core.RunAgent(ctx, eng, msgs, m.st.Sess.Thinking, func(ev runtime.Event) {
			if ev.Type == "token" {
				final.WriteString(ev.Text)
			}
			prog.Send(evMsg{ev: ev})
		})
		prog.Send(turnDoneMsg{final: final.String()})
	}()
	return m, printUser
}

func (m *model) handleEvent(ev runtime.Event) (tea.Model, tea.Cmd) {
	switch ev.Type {
	case "token":
		m.stream.WriteString(ev.Text)
	case "tool_start":
		m.toolLine = ev.Name + " " + ev.Args
		m.tools++
	case "tool_end":
		m.toolLine = ""
	case "error":
		m = m.println(entry{kind: eErr, text: ev.Err.Error()})
	}
	return m, nil
}

func (m *model) finishTurn(final string) (tea.Model, tea.Cmd) {
	m.working = false
	m.cancel = nil
	if strings.TrimSpace(final) == "" {
		// Interrupted/failed with no answer: drop the dangling user turn.
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
	m.println(entry{kind: eScout, text: final})
	// User message persistence (assistant persisted above).
	csession.Touch(m.st.Core.DB, m.st.Sess.ID, m.st.Sess.Provider, m.st.Sess.Model)
	// Surface new approvals without claiming execution.
	if pend, _ := m.st.Core.PendingApprovals(); len(pend) > 0 {
		m.println(entry{kind: eApproval, text: fmt.Sprintf("%d action(s) awaiting approval — /approvals to review.", len(pend))})
	}
	m.stream.Reset()
	return m, m.flushCmds()
}

func renderEntry(e entry) string {
	switch e.kind {
	case eUser:
		return styleUserLabel.Render("You") + "\n" + e.text
	case eScout:
		return styleScout.Render("👷 Scout") + "\n" + RenderMarkdown(e.text)
	case eTool:
		return styleTool.Render("◐ " + e.text)
	case eNotice:
		return styleNotice.Render(e.text)
	case eApproval:
		return styleRiskHigh.Render("ACTION REQUIRES APPROVAL") + "\n" + e.text
	case eErr:
		return styleErr.Render("error: " + e.text)
	default:
		return e.text
	}
}
