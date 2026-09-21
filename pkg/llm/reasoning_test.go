package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReasoningFromMap(t *testing.T) {
	for _, f := range reasoningFields {
		if got := reasoningFromMap(map[string]any{f: "thought"}); got != "thought" {
			t.Fatalf("reasoningFromMap(%q) = %q", f, got)
		}
	}
	if reasoningFromMap(map[string]any{"content": "hi"}) != "" {
		t.Fatal("content must not be reported as reasoning")
	}
}

func TestStripReasoningTags(t *testing.T) {
	cases := map[string]string{
		"answer":                                       "answer",
		"<think>secret</think>answer":                  "answer",
		"<thinking>secret</thinking>answer":            "answer",
		"a<think>secret</think>b":                      "ab",
		"<think>unclosed reasoning":                    "",
		"answer<think>reasoning</think> more":          "answer more",
		"<thinking>x</thinking>one<think>y</think>two": "onetwo",
	}
	for in, want := range cases {
		if got := stripReasoning(in); got != want {
			t.Fatalf("stripReasoning(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReasoningSanitizerAcrossTokens(t *testing.T) {
	san := &reasoningSanitizer{}
	var out strings.Builder
	// The open/close tags are deliberately split across token boundaries.
	for _, tok := range []string{"Hel", "lo <thi", "nk>SECRET", "</thi", "nk> world"} {
		out.WriteString(san.write(tok))
	}
	out.WriteString(san.flush())
	if got := out.String(); got != "Hello  world" {
		t.Fatalf("streamed sanitize = %q", got)
	}
}

// openaiSSE builds a chat-completions SSE body where reasoning_content and
// content interleave; only content must surface.
func openaiSSE() string {
	deltas := []string{
		`{"choices":[{"delta":{"reasoning_content":"think "}}]}`,
		`{"choices":[{"delta":{"content":"Hello"}}]}`,
		`{"choices":[{"delta":{"reasoning_content":"more thinking"}}]}`,
		`{"choices":[{"delta":{"content":" world"}}]}`,
		`{"choices":[{"delta":{"reasoning":"alt field"}}]}`,
		`{"choices":[{"delta":{"content":"!"}}]}`,
	}
	var b strings.Builder
	for _, d := range deltas {
		fmt.Fprintf(&b, "data: %s\n\n", d)
	}
	b.WriteString("data: [DONE]\n\n")
	return b.String()
}

func TestOpenAIStreamDropsReasoning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(openaiSSE()))
	}))
	defer srv.Close()
	p, err := New(Config{Provider: "deepseek", Model: "deepseek-flash", APIKey: "k", Endpoint: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	var got strings.Builder
	if err := p.Stream(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}}, func(tok string) error {
		got.WriteString(tok)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got.String() != "Hello world!" {
		t.Fatalf("stream leaked reasoning: %q", got.String())
	}
}

func TestOpenAICompleteDropsReasoning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"answer","reasoning_content":"secret","reasoning":"alt"}}]}`))
	}))
	defer srv.Close()
	p, _ := New(Config{Provider: "deepseek", Model: "deepseek-flash", APIKey: "k", Endpoint: srv.URL})
	out, err := p.Complete(Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if out != "answer" {
		t.Fatalf("complete leaked reasoning: %q", out)
	}
}

func anthropicSSE() string {
	// Anthropic interleaves thinking_delta and text_delta on the same shape.
	events := []string{
		`{"type":"content_block_delta","delta":{"type":"thinking_delta","text":"SECRET THOUGHT"}}`,
		`{"type":"content_block_delta","delta":{"type":"text_delta","text":"Hello"}}`,
		`{"type":"content_block_delta","delta":{"type":"signature_delta","text":"opaque"}}`,
		`{"type":"content_block_delta","delta":{"type":"text_delta","text":" world"}}`,
	}
	var b strings.Builder
	for _, e := range events {
		fmt.Fprintf(&b, "data: %s\n\n", e)
	}
	b.WriteString("data: [DONE]\n\n")
	return b.String()
}

func TestAnthropicStreamDropsThinking(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(anthropicSSE()))
	}))
	defer srv.Close()
	p, _ := New(Config{Provider: "anthropic", Model: "claude-x", APIKey: "k", Endpoint: srv.URL})
	var got strings.Builder
	if err := p.Stream(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}}, func(tok string) error {
		got.WriteString(tok)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got.String() != "Hello world" {
		t.Fatalf("anthropic stream leaked thinking: %q", got.String())
	}
}

func TestAnthropicCompleteDropsThinking(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"content":[{"type":"thinking","text":"SECRET"},{"type":"text","text":"answer"}]}`))
	}))
	defer srv.Close()
	p, _ := New(Config{Provider: "anthropic", Model: "claude-x", APIKey: "k", Endpoint: srv.URL})
	out, err := p.Complete(Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if out != "answer" {
		t.Fatalf("anthropic complete leaked thinking: %q", out)
	}
}

func TestDeepSeekOffDisablesThinking(t *testing.T) {
	if b := captureBody(t, "deepseek", "deepseek-flash", "off"); b["thinking"] == nil {
		t.Fatal("deepseek off must send thinking disabled")
	} else if m := b["thinking"].(map[string]any); m["type"] != "disabled" {
		t.Fatalf("deepseek off wrong: %v", m)
	}
	if b := captureBody(t, "deepseek", "deepseek-flash", ""); b["thinking"] == nil {
		t.Fatal("deepseek empty default must send thinking disabled")
	} else if m := b["thinking"].(map[string]any); m["type"] != "disabled" {
		t.Fatalf("deepseek empty default wrong: %v", m)
	}
}
