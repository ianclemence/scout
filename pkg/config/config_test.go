package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFileAndEnv(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"ollama_host":"http://x:11434","models":{"analysis":{"provider":"openai","model":"gpt-x"}}}`), 0o600)
	t.Setenv("SCOUT_CONFIG", filepath.Join(dir, "config.json"))
	t.Setenv("SCOUT_ADDR", "127.0.0.1:9999")
	cfg := Load()
	if cfg.OllamaHost != "http://x:11434" {
		t.Fatalf("file not applied: %s", cfg.OllamaHost)
	}
	if cfg.Addr != "127.0.0.1:9999" {
		t.Fatalf("env override failed: %s", cfg.Addr)
	}
	if cfg.Models["analysis"].Provider != "openai" {
		t.Fatal("file models not applied")
	}
}

func TestSaveRolesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	t.Setenv("SCOUT_CONFIG", path)
	// Seed an unrelated field so we can verify SaveRoles preserves it.
	os.WriteFile(path, []byte(`{"ollama_host":"http://keep:11434"}`), 0o600)

	if err := SaveRoles(map[string]LLMRole{"conversation": {Provider: "moonshot", Model: "kimi-k3"}}); err != nil {
		t.Fatal(err)
	}
	cfg := Load()
	if cfg.Models["conversation"].Provider != "moonshot" || cfg.Models["conversation"].Model != "kimi-k3" {
		t.Fatalf("role not persisted: %+v", cfg.Models["conversation"])
	}
	if cfg.OllamaHost != "http://keep:11434" {
		t.Fatalf("existing fields not preserved: %s", cfg.OllamaHost)
	}
}
