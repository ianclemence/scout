// Command scout — terminal-native AI work acquisition agent.
// Bare `scout` enters the interactive session; subcommands are scriptable.
package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ianclemence/scout/pkg/config"
	"github.com/ianclemence/scout/pkg/runtime"
)

func providersCmd(c *runtime.Core) error {
	fmt.Printf("%-16s %-10s %-6s %s\n", "PROVIDER", "CONFIGURED", "MODELS", "DETAIL / ROLES")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, p := range c.ProviderStatus(ctx) {
		mark := "no"
		if p.Configured {
			mark = "yes"
		}
		roles := ""
		if len(p.Roles) > 0 {
			roles = " [" + strings.Join(p.Roles, ",") + "]"
		}
		fmt.Printf("%-16s %-10s %-6d %s%s\n", p.Provider, mark, p.Models, p.Detail, roles)
	}
	fmt.Printf("\nroles:\n")
	for _, role := range []string{config.RoleConversation, config.RoleWorker} {
		r := c.Cfg.Models[role]
		fmt.Printf("  %-13s %s/%s\n", role, r.Provider, r.Model)
	}
	return nil
}

// modelsCmd lists the registry catalog; `scout models refresh [provider]`.

func modelsCmd(c *runtime.Core, args []string) error {
	if len(args) > 0 && args[0] == "refresh" {
		prov := ""
		if len(args) > 1 {
			prov = args[1]
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		n, err := c.Registry().Refresh(ctx, prov)
		fmt.Printf("refreshed %d models\n", n)
		return err
	}
	if len(args) == 0 {
		return providersCmd(c)
	}
	return fmt.Errorf("usage: scout models [refresh [provider]]")
}

func loginCmd(c *runtime.Core, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: scout login <openai|anthropic|deepseek|moonshot>")
	}
	p := strings.ToLower(args[0])
	switch p {
	case "openai", "anthropic", "deepseek", "moonshot":
	default:
		return fmt.Errorf("unknown provider %q", p)
	}
	fmt.Printf("%s API key: ", p)
	key, err := readPassword()
	if err != nil || strings.TrimSpace(key) == "" {
		return fmt.Errorf("no key entered")
	}
	if err := c.SaveSecret("llm:"+p, strings.TrimSpace(key)); err != nil {
		return err
	}
	fmt.Println("stored (encrypted).")
	return nil
}

func logoutCmd(c *runtime.Core, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: scout logout <openai|anthropic|deepseek|moonshot>")
	}
	p := strings.ToLower(args[0])
	switch p {
	case "openai", "anthropic", "deepseek", "moonshot":
	default:
		return fmt.Errorf("unknown provider %q", p)
	}
	if _, err := c.DB.DB.Exec(`DELETE FROM secrets WHERE key=?`, "llm:"+p); err != nil {
		return err
	}
	fmt.Println("Removed stored key for " + p + ". Environment variables are unchanged.")
	return nil
}

func ollamaUp(host string) bool {
	cl := http.Client{Timeout: 5 * time.Second}
	r, err := cl.Get(strings.TrimSuffix(host, "/") + "/api/tags")
	if err != nil {
		return false
	}
	defer r.Body.Close()
	return r.StatusCode < 500
}
