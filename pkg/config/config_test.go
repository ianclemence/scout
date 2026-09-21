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
