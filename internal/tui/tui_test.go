package tui

import (
	"strings"
	"testing"
)

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
	for _, e := range []entry{
		{kind: eUser, text: "hi"},
		{kind: eScout, text: "hello **there**"},
		{kind: eTool, text: "search"},
		{kind: eNotice, text: "note"},
		{kind: eApproval, text: "approve me"},
		{kind: eErr, text: "boom"},
	} {
		if s := renderEntry(e); s == "" {
			t.Fatalf("empty render for kind %d", e.kind)
		}
	}
	if s := renderEntry(entry{kind: eApproval, text: "x"}); !strings.Contains(s, "APPROVAL") {
		t.Fatal("approval must be prominent")
	}
	if s := renderEntry(entry{kind: eScout, text: "hi"}); !strings.Contains(s, "👷 Scout") {
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
