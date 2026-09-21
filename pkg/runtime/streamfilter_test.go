package runtime

import (
	"strings"
	"testing"
)

// A tool call must never stream its fenced JSON to the user.
func TestStreamFilterHidesToolCall(t *testing.T) {
	var got strings.Builder
	f := newStreamFilter(func(s string) { got.WriteString(s) })
	// Stream a realistic ReAct turn: a short preamble, then the fence.
	for _, tok := range []string{
		"I'll ", "check ", "the ", "sources ",
		"```tool\n", `{"name": "source_health", "arguments": {}}`, "\n```",
	} {
		f.write(tok)
	}
	f.flushRemaining()
	out := got.String()
	if !strings.Contains(out, "I'll check the sources") {
		t.Fatalf("visible preamble should stream, got %q", out)
	}
	if strings.Contains(out, "```tool") || strings.Contains(out, "source_health") || strings.Contains(out, "arguments") {
		t.Fatalf("tool machinery leaked into the stream: %q", out)
	}
}

// The fence can be split across tokens; the filter must still catch it.
func TestStreamFilterFenceAcrossTokens(t *testing.T) {
	var got strings.Builder
	f := newStreamFilter(func(s string) { got.WriteString(s) })
	for _, tok := range []string{"answer ```", "to", "ol", ` {"name":"x"}`} {
		f.write(tok)
	}
	f.flushRemaining()
	if strings.Contains(got.String(), "tool") {
		t.Fatalf("split fence leaked: %q", got.String())
	}
}

// A plain answer with no tool call streams in full.
func TestStreamFilterPlainAnswer(t *testing.T) {
	var got strings.Builder
	f := newStreamFilter(func(s string) { got.WriteString(s) })
	for _, tok := range []string{"Hello", ", ", "world", "!"} {
		f.write(tok)
	}
	f.flushRemaining()
	if got.String() != "Hello, world!" {
		t.Fatalf("plain answer mangled: %q", got.String())
	}
}
