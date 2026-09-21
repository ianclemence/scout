package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ianclemence/scout/pkg/csession"
	"github.com/ianclemence/scout/pkg/isession"
)

// TestApprovalsEmptyShowsProse guards the bug where /approvals with nothing
// pending printed nothing at all, because the flush command was discarded.
func TestApprovalsEmptyShowsProse(t *testing.T) {
	core := testCore(t)
	s, _ := csession.Create(core.DB, "interactive", "deepseek", "deepseek-flash")
	m := initialModel(&isession.ReplState{Core: core, Sess: s})
	m.width, m.height, m.ready = 80, 24, true

	cmd := m.openApprovals()
	if cmd == nil {
		t.Fatal("openApprovals must return a flush command even when empty")
	}
	msg := cmd()
	out := friendlyMsg(msg)
	if !strings.Contains(out, "Nothing awaiting approval") {
		t.Fatalf("expected a plain 'nothing awaiting' message, got %q", out)
	}
	if strings.Contains(out, "· Nothing") {
		t.Fatalf("empty state must not render as a notice bullet: %q", out)
	}
}

// TestApplicationsEmptyShowsProse ensures /applications empty state is prose.
func TestApplicationsEmptyShowsProse(t *testing.T) {
	core := testCore(t)
	s, _ := csession.Create(core.DB, "interactive", "deepseek", "deepseek-flash")
	m := initialModel(&isession.ReplState{Core: core, Sess: s})
	m.width, m.height, m.ready = 80, 24, true
	cmd := m.openApplications()
	if cmd == nil {
		t.Fatal("openApplications must return a flush command")
	}
	out := friendlyMsg(cmd())
	if !strings.Contains(out, "No applications yet") {
		t.Fatalf("expected applications empty prose, got %q", out)
	}
}

// TestDiscoverRunsAsync ensures /discover starts a background run (working
// state) instead of blocking the UI.
func TestDiscoverRunsAsync(t *testing.T) {
	core := testCore(t)
	s, _ := csession.Create(core.DB, "interactive", "deepseek", "deepseek-flash")
	m := initialModel(&isession.ReplState{Core: core, Sess: s})
	m.width, m.height, m.ready = 80, 24, true
	nm, cmd := m.startDiscover("go")
	mm := nm.(*model)
	if !mm.working {
		t.Fatal("discover must set the working state so the UI keeps streaming")
	}
	if cmd == nil {
		t.Fatal("discover must return a command (flush + spinner tick)")
	}
}

// friendlyMsg renders a tea message for assertions.
func friendlyMsg(msg any) string {
	type stringer interface{ String() string }
	if s, ok := msg.(stringer); ok {
		return s.String()
	}
	if m, ok := msg.(interface{ Msg() any }); ok {
		return friendlyMsg(m.Msg())
	}
	// tea.Println returns a printLineMessage with unexported fields; fall back
	// to formatting the value.
	return fmt.Sprintf("%v", msg)
}
