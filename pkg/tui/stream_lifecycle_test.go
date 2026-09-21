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

// TestLiveBlockIsBounded ensures the dock preview never grows past the cap,
// so the composer and footer stay put while text streams.
func TestLiveBlockIsBounded(t *testing.T) {
	m := testModel()
	m.width, m.height, m.ready = 60, 24, true
	m.working = true
	for i := 0; i < 200; i++ {
		m.handleEvent(runtime.Event{Type: "token", Text: "word "})
	}
	lines := strings.Split(m.dockPreview(), "\n")
	if len(lines) > 4 {
		t.Fatalf("live block exceeded cap: %d lines\n%s", len(lines), m.dockPreview())
	}
}

// TestLiveBlockOnlyShowsCurrentTurn ensures a superseded preamble disappears
// from the dock once the next turn starts.
func TestLiveBlockOnlyShowsCurrentTurn(t *testing.T) {
	m := testModel()
	m.width, m.height, m.ready = 60, 24, true
	m.working = true
	m.handleEvent(runtime.Event{Type: "turn_start"})
	m.handleEvent(runtime.Event{Type: "token", Text: "ANCIENT PREAMBLE"})
	m.handleEvent(runtime.Event{Type: "tool_start", Name: "search"})
	m.handleEvent(runtime.Event{Type: "turn_start"})
	m.handleEvent(runtime.Event{Type: "token", Text: "fresh answer"})

	got := m.dockPreview()
	if strings.Contains(got, "ANCIENT PREAMBLE") {
		t.Fatalf("superseded preamble leaked into the live dock: %q", got)
	}
	if !strings.Contains(got, "fresh answer") {
		t.Fatalf("current turn text missing from dock: %q", got)
	}
}
