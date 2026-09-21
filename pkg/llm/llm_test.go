package llm

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewUnknownProvider(t *testing.T) {
	if _, err := New(Config{Provider: "nope"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewCompatRequiresEndpoint(t *testing.T) {
	if _, err := New(Config{Provider: "openai_compatible"}); err == nil {
		t.Fatal("expected endpoint error")
	}
}

func TestNewOllamaDefault(t *testing.T) {
	p, err := New(Config{Provider: "ollama", Model: "qwen3:0.6b"})
	if err != nil || p.Name() != "ollama" {
		t.Fatalf("expected ollama provider, %v", err)
	}
}

func TestNewDeepseekEndpoint(t *testing.T) {
	p, err := New(Config{Provider: "deepseek", Model: "deepseek-flash", APIKey: "test"})
	if err != nil || p.Name() != "deepseek" {
		t.Fatalf("expected deepseek provider, %v", err)
	}
}

func TestNewMoonshotEndpoint(t *testing.T) {
	p, err := New(Config{Provider: "moonshot", Model: "kimi-k2.6", APIKey: "test"})
	if err != nil || p.Name() != "moonshot" {
		t.Fatalf("expected moonshot provider, %v", err)
	}
}

func captureBody(t *testing.T, provider, model, thinking string) map[string]any {
	t.Helper()
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &got)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()
	p, err := New(Config{Provider: provider, Model: model, APIKey: "k", Endpoint: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Complete(Request{Messages: []Message{{Role: "user", Content: "hi"}}, Thinking: thinking}); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestThinkingMapping(t *testing.T) {
	if b := captureBody(t, "moonshot", "kimi-k3", "high"); b["reasoning_effort"] != "high" {
		t.Fatalf("k3 high should map to high, got %v", b)
	}
	if b := captureBody(t, "moonshot", "kimi-k3", "xhigh"); b["reasoning_effort"] != "max" {
		t.Fatalf("k3 xhigh should map to max, got %v", b)
	}
	if b := captureBody(t, "moonshot", "kimi-k3", "minimal"); b["reasoning_effort"] != "low" {
		t.Fatalf("k3 minimal should map to low, got %v", b)
	}
	if b := captureBody(t, "moonshot", "kimi-k2.6", "off"); b["thinking"] == nil {
		t.Fatal("k2 off should send thinking disabled")
	} else if m := b["thinking"].(map[string]any); m["type"] != "disabled" {
		t.Fatalf("k2 off wrong: %v", m)
	}
	if b := captureBody(t, "deepseek", "deepseek-flash", "high"); b["reasoning_effort"] != "high" {
		t.Fatalf("deepseek high wrong: %v", b)
	}
	if b := captureBody(t, "openai", "gpt-x", ""); seams(b) {
		t.Fatal("empty thinking must not add fields")
	}
}

func seams(b map[string]any) bool {
	_, a := b["reasoning_effort"]
	_, c := b["thinking"]
	return a || c
}
