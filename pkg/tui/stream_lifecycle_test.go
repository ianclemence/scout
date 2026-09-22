package tui

import (
	"strings"
	"testing"

	"github.com/ianclemence/scout/pkg/runtime"
)

// TestStreamDoesNotAccumulateAcrossTurns guards the fix for the "wall of
// preambles" bug: each ReAct turn's prose is its own segment, not a
// concatenation of every turn's narration.
func TestStreamDoesNotAccumulateAcrossTurns(t *testing.T) {
	m := &model{}
	m.handleEvent(runtime.Event{Type: "turn_start"})
	m.handleEvent(runtime.Event{Type: "token", Text: "first preamble"})
	m.handleEvent(runtime.Event{Type: "tool_start", Name: "get_profile"})
	m.handleEvent(runtime.Event{Type: "tool_end", Name: "get_profile"})
	m.handleEvent(runtime.Event{Type: "turn_start"})
	m.handleEvent(runtime.Event{Type: "token", Text: "final answer"})

	if got := m.stream.String(); got != "final answer" {
		t.Fatalf("stream should hold only the current turn's prose, got %q", got)
	}
}

// TestAnswerCapturedFromAgentEnd verifies the committed text comes from the
// turn that ended without a tool call, not from any preamble.
func TestAnswerCapturedFromAgentEnd(t *testing.T) {
	m := &model{}
	m.handleEvent(runtime.Event{Type: "turn_start"})
	m.handleEvent(runtime.Event{Type: "token", Text: "Let me pull your CV."})
	m.handleEvent(runtime.Event{Type: "tool_start", Name: "get_user_cv"})
	m.handleEvent(runtime.Event{Type: "turn_start"})
	m.handleEvent(runtime.Event{Type: "token", Text: "Here is the answer."})
	m.handleEvent(runtime.Event{Type: "agent_end", Text: "Here is the answer."})

	if !m.answerSet {
		t.Fatal("answer should be captured from agent_end")
	}
	if got := m.answer.String(); got != "Here is the answer." {
		t.Fatalf("answer = %q", got)
	}
}

// TestLiveBlockIsBounded ensures the dock stays a constant blank anchor no
// matter how much text streams: replies grow in the scrollback, never in a
// fixed preview container.
func TestLiveBlockIsBounded(t *testing.T) {
	m := testModel()
	m.width, m.height, m.ready = 60, 24, true
	m.working = true
	for i := 0; i < 200; i++ {
		m.handleEvent(runtime.Event{Type: "token", Text: "word "})
	}
	lines := strings.Split(m.dockPreview(), "\n")
	if len(lines) != dockPreviewRows {
		t.Fatalf("dock must stay exactly %d rows, got %d", dockPreviewRows, len(lines))
	}
	if strings.TrimSpace(m.dockPreview()) != "" {
		t.Fatalf("dock must stay blank while streaming, got %q", m.dockPreview())
	}
}

// TestLiveBlockOnlyShowsCurrentTurn ensures a superseded preamble never
// reaches the scrollback twice: the stream resets every turn_start, so the
// progressive printer starts each turn from a clean buffer.
func TestLiveBlockOnlyShowsCurrentTurn(t *testing.T) {
	m := testModel()
	m.width, m.height, m.ready = 60, 24, true
	m.working = true
	m.streamSty = streamStyler{width: streamWidth(m)}
	var shown strings.Builder
	feed := func(text string) {
		m.handleEvent(runtime.Event{Type: "token", Text: text})
		shown.WriteString(m.lastFlush)
		m.lastFlush = ""
	}
	feed("ANCIENT PREAMBLE\n")
	m.handleEvent(runtime.Event{Type: "tool_start", Name: "search"})
	m.handleEvent(runtime.Event{Type: "turn_start"})
	feed("fresh answer\n")

	got := shown.String()
	if n := strings.Count(got, "ANCIENT PREAMBLE"); n != 1 {
		t.Fatalf("superseded preamble must print exactly once, got %d in %q", n, got)
	}
	if !strings.Contains(got, "fresh answer") {
		t.Fatalf("current turn text missing from scrollback: %q", got)
	}
}
