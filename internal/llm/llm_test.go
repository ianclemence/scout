package llm

import "testing"

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
