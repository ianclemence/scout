package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ianclemence/scout/pkg/changelog"
	"github.com/ianclemence/scout/pkg/csession"
	"github.com/ianclemence/scout/pkg/isession"
)

// The thinking picker lists every level, marks the current one, filters as you
// type, and applies the selection on Enter.
func TestThinkingPicker(t *testing.T) {
	core := testCore(t)
	sess, err := csession.Create(core.DB, "t", "deepseek", "deepseek-flash")
	if err != nil {
		t.Fatal(err)
	}
	sess.Thinking = "low"
	st := &isession.ReplState{Core: core, Sess: sess}
	m := initialModel(st)
	m.width, m.height, m.ready = 90, 30, true

	m.openThinking()
	if m.picker == nil {
		t.Fatal("openThinking should open the picker")
	}
	v := m.picker.view(90)
	if !strings.Contains(v, "Thinking Level") || !strings.Contains(v, "Moderate reasoning") {
		t.Fatalf("picker not rendered:\n%s", v)
	}
	if len(m.picker.filtered) != 5 {
		t.Fatalf("expected 5 levels, got %d", len(m.picker.filtered))
	}
	// Current level is marked.
	var marked string
	for _, it := range m.picker.items {
		if it.current {
			marked = it.value
		}
	}
	if marked != "low" {
		t.Fatalf("current level should be marked, got %q", marked)
	}

	// Filter to "med" and select.
	for _, r := range "med" {
		m.picker.handleKey(string(r))
	}
	if len(m.picker.filtered) != 1 || m.picker.filtered[0].value != "medium" {
		t.Fatalf("filter failed: %+v", m.picker.filtered)
	}
	// Drive through the model so the action runs and persists.
	nm, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(*model)
	if m.picker != nil {
		t.Fatal("picker should close after selection")
	}
	if sess.Thinking != "medium" {
		t.Fatalf("thinking not applied: %q", sess.Thinking)
	}
}

// Bare /thinking opens the picker; /thinking <level> sets directly; an unknown
// level errors with the available list.
func TestThinkingCommandDispatch(t *testing.T) {
	core := testCore(t)
	sess, _ := csession.Create(core.DB, "t", "deepseek", "deepseek-flash")
	st := &isession.ReplState{Core: core, Sess: sess}
	m := initialModel(st)
	m.width, m.height, m.ready = 90, 30, true

	nm, _ := m.runCommand("thinking")
	m = nm.(*model)
	if m.picker == nil {
		t.Fatal("bare /thinking should open the picker")
	}
	m.picker = nil

	nm, _ = m.runCommand("thinking high")
	m = nm.(*model)
	if sess.Thinking != "high" {
		t.Fatalf("/thinking high should set the level, got %q", sess.Thinking)
	}
}

// The session picker lists sessions and marks the current one.
func TestSessionPicker(t *testing.T) {
	core := testCore(t)
	cur, _ := csession.Create(core.DB, "cur", "deepseek", "deepseek-flash")
	if _, err := csession.Create(core.DB, "other", "deepseek", "deepseek-flash"); err != nil {
		t.Fatal(err)
	}
	st := &isession.ReplState{Core: core, Sess: cur}
	m := initialModel(st)
	m.width, m.height, m.ready = 90, 30, true
	m.openSessions()
	if m.picker == nil || len(m.picker.items) < 2 {
		t.Fatalf("session picker should list sessions: %+v", m.picker)
	}
	curCount := 0
	for _, it := range m.picker.items {
		if it.current {
			curCount++
		}
	}
	if curCount != 1 {
		t.Fatalf("exactly one session should be marked current, got %d", curCount)
	}
}

// The approval picker offers approvals with approve/reject actions.
func TestApprovalPicker(t *testing.T) {
	core := testCore(t)
	// Create a pending approval directly.
	if _, err := core.DB.DB.Exec(`INSERT INTO pending_actions(id,action_type,target,payload,risk_level,status,created_at) VALUES('pa-1','submit_proposal','opp-1','{}','high','pending_approval','x')`); err != nil {
		t.Fatal(err)
	}
	sess, _ := csession.Create(core.DB, "t", "deepseek", "deepseek-flash")
	st := &isession.ReplState{Core: core, Sess: sess}
	m := initialModel(st)
	m.width, m.height, m.ready = 90, 30, true
	m.openApprovals()
	if m.picker == nil {
		t.Fatal("approval picker should open when actions are pending")
	}
	if len(m.picker.items) != 1 || m.picker.items[0].value != "pa-1" {
		t.Fatalf("picker items wrong: %+v", m.picker.items)
	}
	// Approve via the primary action.
	_ = m.picker.act("pa-1")
	pending, _ := core.PendingApprovals()
	if len(pending) != 0 {
		t.Fatalf("approval should be cleared, got %d pending", len(pending))
	}
}

// The changelog marker suppresses already-seen notes and shows new ones.
func TestChangelogNewSince(t *testing.T) {
	dir := t.TempDir()
	// First run records the current version and shows nothing.
	if got := changelog.NewSince(dir, "0.6.0"); len(got) != 0 {
		t.Fatalf("fresh install should show no notes, got %d", len(got))
	}
	// After the marker is older, newer entries surface.
	if err := changelog.MarkSeen(dir, "0.5.0"); err != nil {
		t.Fatal(err)
	}
	got := changelog.NewSince(dir, "0.6.0")
	if len(got) == 0 {
		t.Fatal("expected new entries after a bump")
	}
}
