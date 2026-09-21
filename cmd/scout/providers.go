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
	"github.com/ianclemence/scout/pkg/termui"
)

func providersCmd(c *runtime.Core) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var rows [][]string
	for _, p := range c.ProviderStatus(ctx) {
		mark := termui.Dim("no")
		if p.Configured {
			mark = termui.Good("yes")
		}
		roles := ""
		if len(p.Roles) > 0 {
			roles = termui.Accent("[" + strings.Join(p.Roles, ", ") + "]")
		}
		detail := p.Detail
		if roles != "" {
			detail += "  " + roles
		}
		rows = append(rows, []string{p.Provider, mark, fmt.Sprintf("%d", p.Models), detail})
	}
	termui.Print(termui.Table([]string{"Provider", "Ready", "Models", "Detail / roles"}, rows))
	fmt.Println()
	var roleRows [][]string
	for _, role := range []string{config.RoleConversation, config.RoleWorker} {
		r := c.Cfg.Models[role]
		roleRows = append(roleRows, []string{role, termui.Bold(r.Provider + "/" + r.Model)})
	}
	termui.Print(termui.Table([]string{"Role", "Model"}, roleRows))
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
