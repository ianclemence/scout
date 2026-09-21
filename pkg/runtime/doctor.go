package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Check is one diagnostic result.
type Check struct {
	Name   string
	OK     bool
	Detail string
}

// Doctor runs the full diagnostic suite: database, providers, models,
// disk, service, configuration. Same checks in-session (/doctor) and in
// `scout doctor` — commands execute, they don't describe.
func (c *Core) Doctor(ctx context.Context) []Check {
	check := func(name string, ok bool, detail string) Check {
		return Check{Name: name, OK: ok, Detail: detail}
	}
	var out []Check
	_, err := os.Stat(c.Cfg.DataDir)
	out = append(out, check("data-dir", err == nil, c.Cfg.DataDir))
	out = append(out, check("sqlite", c.DB.DB.Ping() == nil, c.Cfg.DBPath))
	out = append(out, check("ollama", probeOllama(c.Cfg.OllamaHost), c.Cfg.OllamaHost))
	out = append(out, check("openai-key", c.HasCredential("OPENAI_API_KEY", "llm:openai"), "env or stored"))
	out = append(out, check("anthropic-key", c.HasCredential("ANTHROPIC_API_KEY", "llm:anthropic"), "env or stored"))
	out = append(out, check("deepseek-key", c.HasCredential("DEEPSEEK_API_KEY", "llm:deepseek"), "env or stored"))
	out = append(out, check("moonshot-key", c.HasCredential("MOONSHOT_API_KEY", "llm:moonshot"), "env or stored"))
	if sk, err := c.SkillRegistry(); err == nil {
		out = append(out, check("skills", true, fmt.Sprintf("%d embedded", len(sk.List()))))
	} else {
		out = append(out, check("skills", false, err.Error()))
	}
	out = append(out, check("tools", true, fmt.Sprintf("%d registered", len(c.Tools()))))
	if free, total, err := DiskUsage(c.Cfg.DataDir); err == nil {
		out = append(out, check("disk", free > 256<<20, fmt.Sprintf("%.1fG free of %.1fG", float64(free)/1e9, float64(total)/1e9)))
	}
	if st, err := ServiceState(); err == nil {
		out = append(out, check("service", st == "active", "scout user unit "+st))
	} else {
		out = append(out, check("service", false, "user unit not found"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		cf := filepath.Join(home, ".config", "scout", "config.json")
		if _, err := os.Stat(cf); err == nil {
			out = append(out, check("config-file", true, cf))
		} else {
			out = append(out, check("config-file", true, "defaults+env (no file)"))
		}
	}
	return out
}

// HasCredential reports env-or-stored key presence without revealing anything.
func (c *Core) HasCredential(env, secretKey string) bool {
	if os.Getenv(env) != "" {
		return true
	}
	s, err := c.LoadSecret(secretKey)
	return err == nil && s != ""
}
