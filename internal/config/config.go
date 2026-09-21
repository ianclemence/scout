// Package config holds one coherent configuration model.
// Precedence: environment variables > database/app settings > defaults.
// Secrets never live here; see internal/secret.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

// FileConfig is the optional JSON config file (~/.config/scout/config.json
// or $SCOUT_CONFIG). Env vars override file values.
type FileConfig struct {
	Addr       string             `json:"addr"`
	DataDir    string             `json:"data_dir"`
	OllamaHost string             `json:"ollama_host"`
	DryRun     *bool              `json:"dry_run"`
	Models     map[string]LLMRole `json:"models"`
}

// Load merges defaults <- config file <- environment.
func Load() Config {
	cfg := Default()
	var path string
	if v := os.Getenv("SCOUT_CONFIG"); v != "" {
		path = v
	} else if home, err := os.UserHomeDir(); err == nil {
		path = filepath.Join(home, ".config", "scout", "config.json")
	}
	if path != "" {
		if raw, err := os.ReadFile(path); err == nil {
			var fc FileConfig
			if json.Unmarshal(raw, &fc) == nil {
				if fc.Addr != "" {
					cfg.Addr = fc.Addr
				}
				if fc.DataDir != "" {
					cfg.DataDir = fc.DataDir
					cfg.DBPath = filepath.Join(fc.DataDir, "scout.db")
				}
				if fc.OllamaHost != "" {
					cfg.OllamaHost = fc.OllamaHost
				}
				if fc.DryRun != nil {
					cfg.DryRun = *fc.DryRun
				}
				for k, v := range fc.Models {
					cfg.Models[k] = v
				}
			}
		}
	}
	// Env overrides everything (model roles handled below).
	cfg.Addr = envOr("SCOUT_ADDR", cfg.Addr)
	cfg.OllamaHost = envOr("OLLAMA_HOST", cfg.OllamaHost)
	if v := os.Getenv("SCOUT_DATA_DIR"); v != "" {
		cfg.DataDir = v
		cfg.DBPath = filepath.Join(v, "scout.db")
	}
	if v := os.Getenv("SCOUT_MASTER_KEY"); v != "" {
		cfg.ScoutEnvKey = v
	}
	if _, ok := os.LookupEnv("SCOUT_DRY_RUN"); ok {
		cfg.DryRun = envBool("SCOUT_DRY_RUN", cfg.DryRun)
	}
	// Per-role models: env vars override file values only when set.
	for role := range cfg.Models {
		up := strings.ToUpper(role)
		if p, ok := os.LookupEnv("SCOUT_MODEL_" + up + "_PROVIDER"); ok {
			r := cfg.Models[role]
			r.Provider = p
			cfg.Models[role] = r
		}
		if m, ok := os.LookupEnv("SCOUT_MODEL_" + up); ok {
			r := cfg.Models[role]
			r.Model = m
			cfg.Models[role] = r
		}
	}
	return cfg
}
