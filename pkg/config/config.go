// Package config holds one coherent configuration model.
// Precedence: environment variables > database/app settings > defaults.
// Secrets never live here; see pkg/secret.
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

	// Models maps a role to its provider/model. Scout has exactly two roles:
	// "conversation" (the interactive/session model, runtime-switchable via
	// /model) and "worker" (background tasks: fit analysis, proposal drafting,
	// and future tool work). The worker inherits the conversation model unless
	// explicitly overridden. See DefaultRoles and RoleCanonical.
	Models map[string]LLMRole

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
		Models:      DefaultRoles(),
	}
}

// DefaultProvider and DefaultModel are the built-in role defaults. They point
// at a cloud model with a broad, cheap availability (DeepSeek Flash) rather
// than a specific local Ollama tag: a built-in local default goes stale the
// moment that tag is not pulled, leaving every role dead with no signal. Local
// engines remain first-class via SCOUT_MODEL_* env vars or config.json.
const (
	DefaultProvider = "deepseek"
	DefaultModel    = "deepseek-flash"
)

// Role names. Scout keeps exactly the distinction that earns its keep: the
// interactive session model, and the background worker model (which inherits
// the session model unless overridden).
const (
	RoleConversation = "conversation"
	RoleWorker       = "worker"
)

// legacyRole maps the old five-role names onto today's two roles, so existing
// SCOUT_MODEL_* env vars and config.json keys keep working.
var legacyRole = map[string]string{
	"screening":     RoleWorker,
	"analysis":      RoleWorker,
	"proposal":      RoleWorker,
	"deep_analysis": RoleWorker,
	"deep":          RoleWorker,
}

// RoleCanonical resolves a role name (current or legacy) to a canonical role.
// Unknown names fall back to the worker role.
func RoleCanonical(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == RoleConversation {
		return RoleConversation
	}
	if role == RoleWorker {
		return RoleWorker
	}
	if canon, ok := legacyRole[role]; ok {
		return canon
	}
	return RoleWorker
}

// DefaultRoles returns the role default provider/model, honoring the
// SCOUT_MODEL_* environment overrides. The worker defaults to the
// conversation model unless explicitly overridden, so there is one model
// everywhere by default and a worker override only when a user wants a
// cheaper or different background model.
func DefaultRoles() map[string]LLMRole {
	conv := LLMRole{
		Provider: envOr("SCOUT_MODEL_CONVERSATION_PROVIDER", envOr("SCOUT_MODEL_PROVIDER", DefaultProvider)),
		Model:    envOr("SCOUT_MODEL_CONVERSATION", envOr("SCOUT_MODEL", DefaultModel)),
	}
	worker := conv
	if p := envFirst("SCOUT_MODEL_WORKER_PROVIDER", "SCOUT_MODEL_ANALYSIS_PROVIDER", "SCOUT_MODEL_PROPOSAL_PROVIDER", "SCOUT_MODEL_SCREENING_PROVIDER", "SCOUT_MODEL_DEEP_PROVIDER"); p != "" {
		worker.Provider = p
	}
	if m := envFirst("SCOUT_MODEL_WORKER", "SCOUT_MODEL_ANALYSIS", "SCOUT_MODEL_PROPOSAL", "SCOUT_MODEL_SCREENING", "SCOUT_MODEL_DEEP"); m != "" {
		worker.Model = m
	}
	return map[string]LLMRole{
		RoleConversation: conv,
		RoleWorker:       worker,
	}
}

// envFirst returns the first non-empty environment value among keys.
func envFirst(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
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
	// A worker override may come from a file or from the environment, under
	// either the canonical name or a legacy one; any of these counts as
	// explicit so the worker does not simply inherit the conversation model.
	workerExplicit := envFirst("SCOUT_MODEL_WORKER_PROVIDER", "SCOUT_MODEL_WORKER",
		"SCOUT_MODEL_ANALYSIS_PROVIDER", "SCOUT_MODEL_ANALYSIS",
		"SCOUT_MODEL_PROPOSAL_PROVIDER", "SCOUT_MODEL_PROPOSAL",
		"SCOUT_MODEL_SCREENING_PROVIDER", "SCOUT_MODEL_SCREENING",
		"SCOUT_MODEL_DEEP_PROVIDER", "SCOUT_MODEL_DEEP") != ""
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
					// Map legacy role keys (analysis/proposal/screening/deep) onto
					// the worker role so old config files keep working. A file value
					// for either canonical role counts as an explicit override.
					if RoleCanonical(k) == RoleWorker {
						workerExplicit = true
					}
					cfg.Models[RoleCanonical(k)] = v
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
	// Precedence is defaults < file < environment. cfg.Models starts as the
	// env-aware defaults (from Default); the file merge above overrode the
	// roles it mentioned. Resolve inheritance, then per-field env overrides.
	conv := cfg.Models[RoleConversation]
	worker := cfg.Models[RoleWorker]
	if !workerExplicit {
		// The worker follows the conversation model unless deliberately set.
		worker = conv
	}
	// Environment overrides win over both file and default, per field.
	if v, ok := os.LookupEnv("SCOUT_MODEL_CONVERSATION_PROVIDER"); ok {
		conv.Provider = v
	}
	if v, ok := os.LookupEnv("SCOUT_MODEL_CONVERSATION"); ok {
		conv.Model = v
	}
	if !workerExplicit {
		// Inherit the (possibly env-overridden) conversation model.
		worker = conv
	}
	// Worker env override: canonical name first, then the legacy names so an
	// existing SCOUT_MODEL_ANALYSIS/PROPOSAL/SCREENING/DEEP keeps working.
	if v := envFirst("SCOUT_MODEL_WORKER_PROVIDER", "SCOUT_MODEL_ANALYSIS_PROVIDER", "SCOUT_MODEL_PROPOSAL_PROVIDER", "SCOUT_MODEL_SCREENING_PROVIDER", "SCOUT_MODEL_DEEP_PROVIDER"); v != "" {
		worker.Provider = v
	}
	if v := envFirst("SCOUT_MODEL_WORKER", "SCOUT_MODEL_ANALYSIS", "SCOUT_MODEL_PROPOSAL", "SCOUT_MODEL_SCREENING", "SCOUT_MODEL_DEEP"); v != "" {
		worker.Model = v
	}
	cfg.Models[RoleConversation] = conv
	cfg.Models[RoleWorker] = worker
	return cfg
}

// ConfigPath returns the active config file path ($SCOUT_CONFIG or the default).
func ConfigPath() string {
	if v := os.Getenv("SCOUT_CONFIG"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "scout", "config.json")
}

// SaveRoles persists the role → provider/model map into the config file,
// preserving other existing fields when present.
func SaveRoles(models map[string]LLMRole) error {
	path := ConfigPath()
	if path == "" {
		return nil
	}
	fc := FileConfig{}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &fc)
	}
	if fc.Models == nil {
		fc.Models = map[string]LLMRole{}
	}
	// Write canonical role keys only; drop any legacy keys a prior version
	// left behind so the file converges on the two-role model.
	cleaned := map[string]LLMRole{}
	for k, v := range fc.Models {
		canon := RoleCanonical(k)
		if k == canon {
			cleaned[canon] = v
		}
	}
	fc.Models = cleaned
	for k, v := range models {
		fc.Models[RoleCanonical(k)] = v
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}
