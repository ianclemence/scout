// Package config holds one coherent configuration model.
// Precedence: environment variables > database/app settings > defaults.
// Secrets never live here; see internal/secret.
package config

import (
	"os"
	"path/filepath"
	"strconv"
)

type LLMRole struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type Config struct {
	DataDir      string
	DBPath       string
	Addr         string
	AuthDisabled bool // localhost-first dev only, never for network exposure

	ScoutEnvKey  string // SCOUT_MASTER_KEY for secret encryption; file perms enforced
	ScoutDataDir string

	OllamaHost string

	Models map[string]LLMRole // screening, analysis, proposal, conversation, deep_analysis

	DryRun bool
}

func Default() Config {
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".local", "share", "scout")
	if v := os.Getenv("SCOUT_DATA_DIR"); v != "" {
		dataDir = v
	}
	return Config{
		DataDir:     dataDir,
		DBPath:      filepath.Join(dataDir, "scout.db"),
		Addr:        envOr("SCOUT_ADDR", "127.0.0.1:3210"),
		OllamaHost:  envOr("OLLAMA_HOST", "http://127.0.0.1:11434"),
		ScoutEnvKey: os.Getenv("SCOUT_MASTER_KEY"),
		DryRun:      envBool("SCOUT_DRY_RUN", false),
		Models: map[string]LLMRole{
			"screening":     {Provider: envOr("SCOUT_MODEL_SCREENING_PROVIDER", "ollama"), Model: envOr("SCOUT_MODEL_SCREENING", "qwen3:0.6b")},
			"analysis":      {Provider: envOr("SCOUT_MODEL_ANALYSIS_PROVIDER", "ollama"), Model: envOr("SCOUT_MODEL_ANALYSIS", "qwen3:0.6b")},
			"proposal":      {Provider: envOr("SCOUT_MODEL_PROPOSAL_PROVIDER", "ollama"), Model: envOr("SCOUT_MODEL_PROPOSAL", "qwen3:0.6b")},
			"conversation":  {Provider: envOr("SCOUT_MODEL_CONVERSATION_PROVIDER", "ollama"), Model: envOr("SCOUT_MODEL_CONVERSATION", "qwen3:0.6b")},
			"deep_analysis": {Provider: envOr("SCOUT_MODEL_DEEP_PROVIDER", "ollama"), Model: envOr("SCOUT_MODEL_DEEP", "qwen3:0.6b")},
		},
	}
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func envBool(k string, d bool) bool {
	v := os.Getenv(k)
	if v == "" {
		return d
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return d
	}
	return b
}
