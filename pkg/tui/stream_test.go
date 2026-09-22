package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ianclemence/scout/pkg/csession"
	"github.com/ianclemence/scout/pkg/isession"
	"github.com/ianclemence/scout/pkg/runtime"
)

// nextStreamBlock is pure accounting: the header prints once, completed
// lines print once each, the trailing partial line is always held back.
func TestNextStreamBlockAccounting(t *testing.T) {
	wh, lines, flushed := nextStreamBlock("", false, 0)
	if wh || len(lines) != 0 || flushed != 0 {
		t.Fatalf("empty stream must emit nothing, got %v %v %d", wh, lines, flushed)
	}
	wh, lines, flushed = nextStreamBlock("Hello", false, 0)
	if !wh || len(lines) != 0 || flushed != 0 {
		t.Fatalf("partial line: header due, no lines, got %v %v %d", wh, lines, flushed)
	}
	wh, lines, flushed = nextStreamBlock("Hello\nWorld", false, 0)
	if !wh || len(lines) != 1 || lines[0] != "Hello" || flushed != 1 {
		t.Fatalf("first line completes, got %v %q %d", wh, lines, flushed)
	}
	wh, lines, flushed = nextStreamBlock("Hello\nWorld", true, 1)
	if wh || len(lines) != 0 || flushed != 1 {
		t.Fatalf("same buffer twice must emit nothing new, got %v %v %d", wh, lines, flushed)
	}
}

// A streamed reply grows in the scrollback and is never reprinted whole at
// completion: tokens print completed lines, finishTurn prints only the tail.
func TestStreamsReplyProgressivelyNoDup(t *testing.T) {
	core := testCore(t)
	st := &isession.ReplState{Core: core, Sess: &csession.Session{Provider: "ollama", Model: "qwen3:0.6b"}}
	m := initialModel(st)
	m.width, m.height, m.ready = 80, 24, true
	m.working = true
	m.streamSty = streamStyler{width: streamWidth(m)}
	var shown strings.Builder
	feed := func(text string) {
		m.handleEvent(runtime.Event{Type: "token", Text: text})
		shown.WriteString(m.lastFlush)
		m.lastFlush = ""
	}
	feed("## Head\n")
	feed("Body line\n")
	if !m.streamHeaderShown || m.streamFlushedLines != 2 {
		t.Fatalf("header + two lines must print progressively, shown=%v flushed=%d", m.streamHeaderShown, m.streamFlushedLines)
	}
	m.handleEvent(runtime.Event{Type: "agent_end", Text: "## Head\nBody line"})
	m.finishTurn("", 0)
	got := shown.String() + m.lastFlush
	for _, want := range []string{"Head", "Body line"} {
		if n := strings.Count(got, want); n != 1 {
			t.Fatalf("content %q printed %d times, want exactly once\ngot=%q", want, n, got)
		}
	}
}

// The dock is a blank anchor, never a tail preview: replies grow in the
// scrollback instead. The composer and footer never move either way.
func TestDockIsBlankAnchorMidStream(t *testing.T) {
	m := testModel()
	m.width, m.height, m.ready = 80, 24, true
	m.working = true
	for _, tok := range []string{"## Head\n", "Body line\n", "Tail."} {
		m.handleEvent(runtime.Event{Type: "token", Text: tok})
	}
	if got := m.dockPreview(); strings.TrimSpace(got) != "" {
		t.Fatalf("dock must stay a blank anchor mid-stream, got %q", got)
	}
	if n := len(strings.Split(m.dockPreview(), "\n")); n != dockPreviewRows {
		t.Fatalf("dock height must stay constant at %d, got %d", dockPreviewRows, n)
	}
}

// User messages render as `You ┃ text` on one row.
func TestBubbleSameRow(t *testing.T) {
	m := testModel()
	out := m.renderEntry(entry{kind: eUser, text: "hello"})
	rows := strings.Split(out, "\n")
	if len(rows) != 1 {
		t.Fatalf("single-line message must render one row, got %q", out)
	}
	you, bar := strings.Index(rows[0], "You"), strings.Index(rows[0], "┃")
	if you < 0 || bar < 0 || you > bar {
		t.Fatalf("name and pipe must share the first row in order, got %q", out)
	}
}

// Typing a long message grows the composer (layout runs on the key path).
func TestComposerGrowsWhileTyping(t *testing.T) {
	m := testModel()
	long := strings.Repeat("word ", 40)
	for _, r := range long {
		nm, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(*model)
	}
	if h := m.ta.Height(); h <= 1 {
		t.Fatalf("composer must grow while typing long input, height=%d", h)
	}
}

// The resume picker never lists empty sessions: launching scout without
// messaging must leave nothing to resume.
func TestResumePickerHidesEmptySessions(t *testing.T) {
	core := testCore(t)
	full, err := csession.Create(core.DB, "work", "ollama", "qwen3:0.6b")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := csession.Create(core.DB, "interactive", "ollama", "qwen3:0.6b"); err != nil {
		t.Fatal(err)
	}
	if err := csession.AppendMessages(core.DB, full.ID, []csession.Message{{Role: "user", Content: "hi"}}); err != nil {
		t.Fatal(err)
	}
	st := &isession.ReplState{Core: core, Sess: &csession.Session{ID: full.ID, Provider: "ollama", Model: "qwen3:0.6b"}}
	m := initialModel(st)
	m.width, m.height, m.ready = 80, 24, true
	m.openSessions()
	if m.picker == nil {
		t.Fatal("picker must open with a content session present")
	}
	lp := m.picker
	if len(lp.items) != 1 {
		t.Fatalf("picker must list only the messaged session, got %v", lp.items)
	}
	for _, it := range lp.items {
		if strings.Contains(it.label, "empty session") {
			t.Fatalf("picker must never show an empty session: %v", lp.items)
		}
	}
}
