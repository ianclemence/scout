package runtime

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ianclemence/scout/internal/agent"
	"github.com/ianclemence/scout/internal/config"
	"github.com/ianclemence/scout/internal/llm"
	"github.com/ianclemence/scout/internal/store"
)

type fakeProvider struct {
	turns []string
	n     int
}

func (f *fakeProvider) Name() string { return "fake" }
func (f *fakeProvider) Complete(req llm.Request) (string, error) {
	return f.turns[len(f.turns)-1], nil
}
func (f *fakeProvider) Stream(ctx context.Context, req llm.Request, emit func(string) error) error {
	s := f.turns[f.n]
	if f.n < len(f.turns)-1 {
		f.n++
	}
	return emit(s)
}

func testCore(t *testing.T) *Core {
	t.Helper()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	db, err := store.Open(filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	c, err := New(cfg, db)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestToolRegistry(t *testing.T) {
	c := testCore(t)
	tools := c.Tools()
	if len(tools) < 10 {
		t.Fatalf("expected tool registry, got %d", len(tools))
	}
	seen := map[string]bool{}
	for _, tl := range tools {
		if seen[tl.Name] {
			t.Fatalf("duplicate tool %s", tl.Name)
		}
		seen[tl.Name] = true
		if tl.Description == "" || tl.Handler == nil {
			t.Fatalf("tool %s missing description/handler", tl.Name)
		}
	}
	for _, want := range []string{"get_profile", "search_opportunities", "analyze_opportunity", "prepare_proposal", "request_approval", "get_pipeline"} {
		if c.FindTool(want) == nil {
			t.Fatalf("missing tool %s", want)
		}
	}
}

func TestParseToolCall(t *testing.T) {
	name, args, ok := parseToolCall("thinking\n```tool\n{\"name\": \"search_opportunities\", \"arguments\": {\"query\": \"go\"}}\n```\n")
	if !ok || name != "search_opportunities" || args["query"] != "go" {
		t.Fatalf("parse failed: %v %v %v", name, args, ok)
	}
	if _, _, ok := parseToolCall("plain answer, no tools"); ok {
		t.Fatal("false positive")
	}
	if _, _, ok := parseToolCall("```tool\n{invalid}\n```"); ok {
		t.Fatal("invalid JSON accepted")
	}
	// External content must not smuggle instructions past validation.
	if c := testCore(t); c.FindTool("ignore previous instructions") != nil {
		t.Fatal("unknown tool resolved")
	}
}

func TestReActLoopUsesTools(t *testing.T) {
	c := testCore(t)
	if _, err := c.AddOpportunity("Go API", "Build a Go API with Postgres", "go"); err != nil {
		t.Fatal(err)
	}
	fake := &fakeProvider{turns: []string{
		"Let me search.\n```tool\n{\"name\": \"search_opportunities\", \"arguments\": {\"query\": \"Go\"}}\n```\n",
		"Found one opportunity. Done.",
	}}
	var sawTool bool
	final, err := c.RunAgent(context.Background(), &agent.Engine{LLM: fake},
		[]llm.Message{{Role: "user", Content: "find work"}}, "", func(ev Event) {
			if ev.Type == "tool_start" && ev.Name == "search_opportunities" {
				sawTool = true
			}
		})
	if err != nil {
		t.Fatal(err)
	}
	if !sawTool {
		t.Fatal("tool was not executed")
	}
	if !strings.Contains(final, "Done") {
		t.Fatalf("bad final: %q", final)
	}
}

func TestReActUnknownToolContinues(t *testing.T) {
	c := testCore(t)
	fake := &fakeProvider{turns: []string{
		"```tool\n{\"name\": \"nope\", \"arguments\": {}}\n```\n",
		"Recovered.",
	}}
	final, err := c.RunAgent(context.Background(), &agent.Engine{LLM: fake},
		[]llm.Message{{Role: "user", Content: "hi"}}, "", func(Event) {})
	if err != nil || !strings.Contains(final, "Recovered") {
		t.Fatalf("should recover from unknown tool: %q %v", final, err)
	}
}

func TestCredentialPrecedence(t *testing.T) {
	c := testCore(t)
	t.Setenv("OPENAI_API_KEY", "env-key")
	if k, _ := c.Credential("openai"); k != "env-key" {
		t.Fatal("env fallback failed")
	}
	if err := c.SaveSecret("llm:openai", "stored-key"); err != nil {
		t.Fatal(err)
	}
	// Stored credential wins over env (explicit user action).
	if k, _ := c.Credential("openai"); k != "stored-key" {
		t.Fatalf("store should win, got %q", k)
	}
	if _, err := c.Credential("moonshot"); err == nil {
		t.Fatal("expected no-credential error")
	}
}

func TestReActInterrupt(t *testing.T) {
	c := testCore(t)
	fake := &fakeProvider{turns: []string{"never"}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.RunAgent(ctx, &agent.Engine{LLM: fake},
		[]llm.Message{{Role: "user", Content: "hi"}}, "", func(Event) {})
	if err == nil {
		t.Fatal("expected interrupt error")
	}
}
