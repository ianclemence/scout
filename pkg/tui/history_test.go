package tui

import (
	"strings"
	"testing"

	"github.com/ianclemence/scout/pkg/csession"
	"github.com/ianclemence/scout/pkg/isession"
)

// TestRenderHistoryShowsPriorTurns ensures opening a session renders its prior
// user/assistant turns into the transcript, so the conversation is visible.
func TestRenderHistoryShowsPriorTurns(t *testing.T) {
	core := testCore(t)
	sess, err := csession.Create(core.DB, "interactive", "deepseek", "deepseek-flash")
	if err != nil {
		t.Fatal(err)
	}
	if err := csession.AppendMessages(core.DB, sess.ID, []csession.Message{
		{Role: "user", Content: "find me go work"},
		{Role: "assistant", Content: "Here are three matches."},
	}); err != nil {
		t.Fatal(err)
	}
	m := initialModel(&isession.ReplState{Core: core, Sess: sess})
	m.width, m.height, m.ready = 80, 24, true

	got := m.renderHistory()
	if !strings.Contains(got, "Earlier in this session") {
		t.Fatalf("history should have a divider:\n%s", got)
	}
	if !strings.Contains(got, "find me go work") {
		t.Fatalf("history should show the user turn:\n%s", got)
	}
	if !strings.Contains(got, "Here are three matches.") {
		t.Fatalf("history should show the assistant turn:\n%s", got)
	}
}

// TestRenderHistoryEmptyForNewSession ensures a fresh session shows nothing.
func TestRenderHistoryEmptyForNewSession(t *testing.T) {
	core := testCore(t)
	sess, _ := csession.Create(core.DB, "interactive", "deepseek", "deepseek-flash")
	m := initialModel(&isession.ReplState{Core: core, Sess: sess})
	m.width, m.ready = 80, true
	if h := m.renderHistory(); h != "" {
		t.Fatalf("a new session should render no history, got %q", h)
	}
}
