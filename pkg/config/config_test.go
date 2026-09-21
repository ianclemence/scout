package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFileAndEnv(t *testing.T) {
	dir := t.TempDir()
	// A legacy role key (analysis) must map onto the worker role so old config
	// files keep working, while the conversation role keeps its default.
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
	if cfg.Models[RoleWorker].Provider != "openai" || cfg.Models[RoleWorker].Model != "gpt-x" {
		t.Fatalf("legacy analysis role should map to worker: %+v", cfg.Models[RoleWorker])
	}
	if _, ok := cfg.Models["analysis"]; ok {
		t.Fatal("legacy role key must not survive in the resolved config")
	}
}

func TestRolesAreTwo(t *testing.T) {
	cfg := Load()
	if len(cfg.Models) != 2 {
		t.Fatalf("expected exactly two roles, got %d: %+v", len(cfg.Models), cfg.Models)
	}
	if _, ok := cfg.Models[RoleConversation]; !ok {
		t.Fatal("conversation role missing")
	}
	if _, ok := cfg.Models[RoleWorker]; !ok {
		t.Fatal("worker role missing")
	}
}

func TestWorkerInheritsConversationByDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	t.Setenv("SCOUT_CONFIG", path)
	os.WriteFile(path, []byte(`{"models":{"conversation":{"provider":"moonshot","model":"kimi-k3"}}}`), 0o600)
	cfg := Load()
	if cfg.Models[RoleWorker] != cfg.Models[RoleConversation] {
		t.Fatalf("worker should inherit conversation: %+v vs %+v", cfg.Models[RoleWorker], cfg.Models[RoleConversation])
	}
}

func TestWorkerOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	t.Setenv("SCOUT_CONFIG", path)
	os.WriteFile(path, []byte(`{"models":{"conversation":{"provider":"moonshot","model":"kimi-k3"},"worker":{"provider":"deepseek","model":"deepseek-flash"}}}`), 0o600)
	cfg := Load()
	if cfg.Models[RoleConversation].Provider != "moonshot" {
		t.Fatalf("conversation wrong: %+v", cfg.Models[RoleConversation])
	}
	if cfg.Models[RoleWorker].Provider != "deepseek" {
		t.Fatalf("explicit worker override ignored: %+v", cfg.Models[RoleWorker])
	}
}

func TestLegacyEnvMapsToWorker(t *testing.T) {
	t.Setenv("SCOUT_CONFIG", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("SCOUT_MODEL_ANALYSIS", "legacy-model")
	t.Setenv("SCOUT_MODEL_ANALYSIS_PROVIDER", "openai")
	cfg := Load()
	if cfg.Models[RoleWorker].Provider != "openai" || cfg.Models[RoleWorker].Model != "legacy-model" {
		t.Fatalf("legacy SCOUT_MODEL_ANALYSIS should map to worker: %+v", cfg.Models[RoleWorker])
	}
}

func TestRoleCanonical(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"conversation", RoleConversation},
		{"worker", RoleWorker},
		{"analysis", RoleWorker},
		{"proposal", RoleWorker},
		{"screening", RoleWorker},
		{"deep_analysis", RoleWorker},
		{"Deep", RoleWorker},
		{"nonsense", RoleWorker},
	} {
		if got := RoleCanonical(tc.in); got != tc.want {
			t.Errorf("RoleCanonical(%q) = %q, want %q", tc.in, got, tc.want)
		}
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
	if cfg.Models[RoleConversation].Provider != "moonshot" || cfg.Models[RoleConversation].Model != "kimi-k3" {
		t.Fatalf("role not persisted: %+v", cfg.Models[RoleConversation])
	}
	if cfg.OllamaHost != "http://keep:11434" {
		t.Fatalf("existing fields not preserved: %s", cfg.OllamaHost)
	}
}
