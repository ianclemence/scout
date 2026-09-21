package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/ianclemence/scout/internal/csession"
	"github.com/ianclemence/scout/internal/isession"
)

func timeNow() time.Time { return time.Now() }

func TestRenderMarkdown(t *testing.T) {
	out := RenderMarkdown("# Title\n\nSome **bold** and `code` text.\n\n- item one\n- item two\n\n```go\nfmt.Println()\n```\n\n> quoted")
	if !strings.Contains(out, "Title") || !strings.Contains(out, "bold") || !strings.Contains(out, "code") {
		t.Fatalf("lost content: %q", out)
	}
	if strings.Contains(out, "**") || strings.Contains(out, "# Title") {
		t.Fatalf("markup not rendered: %q", out)
	}
	if RenderMarkdown("plain") != "plain" {
		t.Fatal("plain text altered")
	}
}

func TestRenderEntryKinds(t *testing.T) {
	m := &model{width: 80, st: &isession.ReplState{Sess: &csession.Session{Provider: "ollama", Model: "qwen3:0.6b"}}}
	render := func(e entry) string { return m.renderEntry(e) }
	for _, e := range []entry{
		{kind: eUser, text: "hi"},
		{kind: eScout, text: "hello **there**"},
		{kind: eTool, text: "search"},
		{kind: eNotice, text: "note"},
		{kind: eApproval, text: "approve me"},
		{kind: eErr, text: "boom"},
	} {
		if s := render(e); s == "" {
			t.Fatalf("empty render for kind %d", e.kind)
		}
	}
	if s := render(entry{kind: eApproval, text: "x"}); !strings.Contains(s, "◆") {
		t.Fatal("approval must be prominent")
	}
	if s := render(entry{kind: eScout, text: "hi"}); !strings.Contains(s, "👷 Scout") {
		t.Fatal("assistant label must carry the builder mark")
	}
}

func TestTruncateWrap(t *testing.T) {
	if truncate("abcdef", 4) != "abc…" {
		t.Fatal("truncate wrong")
	}
	if len(wrap("abcdefghij", 4)) != 3 {
		t.Fatal("wrap wrong")
	}
}

func testModel() *model {
	return &model{width: 80, height: 24, ready: true,
		st: &isession.ReplState{Sess: &csession.Session{Provider: "ollama", Model: "qwen3:0.6b"}}}
}

func TestComposerRules(t *testing.T) {
	m := testModel()
	idle := m.composerTopRule()
	if !strings.Contains(idle, "──") {
		t.Fatal("idle composer must be a plain rule")
	}
	m.working = true
	m.turnFrom = timeNow()
	w := m.composerTopRule()
	if !strings.Contains(w, "Working") && !strings.Contains(w, "──") {
		t.Fatalf("working rule must carry status: %q", w)
	}
}

func TestWelcomeCard(t *testing.T) {
	m := testModel()
	card := m.welcomeCard()
	for _, want := range []string{"S C O U T", "Find work worth doing.", "/help", "/model"} {
		if !strings.Contains(card, want) {
			t.Fatalf("welcome card missing %q", want)
		}
	}
}

func TestApprovalCard(t *testing.T) {
	m := testModel()
	m.approval = &pendingApproval{id: "a1", title: "submit_proposal → opp-1", risk: "high"}
	card := m.approvalCard()
	for _, want := range []string{"Approval required", "high", "[1] approve", "[2] reject"} {
		if !strings.Contains(card, want) {
			t.Fatalf("approval card missing %q", want)
		}
	}
}

func TestFooter(t *testing.T) {
	m := testModel()
	if s := m.footerStats(); !strings.Contains(s, "ollama/qwen3:0.6b") || !strings.Contains(s, "local") {
		t.Fatalf("footer must show locality + model: %q", s)
	}
	if s := m.footerKeys(); !strings.Contains(s, "/ commands") {
		t.Fatalf("footer keys missing: %q", s)
	}
}
