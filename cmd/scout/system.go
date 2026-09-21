// Command scout — terminal-native AI work acquisition agent.
// Bare `scout` enters the interactive session; subcommands are scriptable.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ianclemence/scout/pkg/config"
	"github.com/ianclemence/scout/pkg/mcpserver"
	"github.com/ianclemence/scout/pkg/runtime"
)

func configCmd() error {
	cfg := config.Load()
	fmt.Printf("data_dir=%s\ndb=%s\nollama=%s\ndry_run=%v\nmodels=%v\n",
		cfg.DataDir, cfg.DBPath, cfg.OllamaHost, cfg.DryRun, cfg.Models)
	fmt.Println("(secrets redacted)")
	return nil
}

func doctorCmd(c *runtime.Core) error {
	fmt.Println("scout doctor")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, ch := range c.Doctor(ctx) {
		s := "OK"
		if !ch.OK {
			s = "MISSING"
		}
		fmt.Printf("  [%s] %s %s\n", s, ch.Name, ch.Detail)
	}
	return nil
}

// resetCmd wipes Scout's saved data and starts it fresh. It is irreversible,
// so it requires --yes; without it, it prints exactly what would be destroyed.
// Provider API keys and connector configuration are preserved unless --all is
// given, so a reset does not force a re-login.
func resetCmd(c *runtime.Core, args []string) error {
	all, yes := false, false
	for _, a := range args {
		switch a {
		case "--all":
			all = true
		case "--yes", "-y":
			yes = true
		}
	}
	scope := runtime.ResetScope{IncludeCredentials: all}
	if !yes {
		fmt.Println("scout reset will permanently delete:")
		fmt.Println("  - profile and evidence (your CV data)")
		fmt.Println("  - opportunities, evaluations, proposals, applications, messages")
		fmt.Println("  - all sessions and their message history")
		fmt.Println("  - learned preferences and feedback")
		fmt.Println("  - trajectories, tool audit, and cached models")
		if all {
			fmt.Println("  - stored provider keys and connector sign-in (--all)")
			fmt.Println("  - the local master key and the workspace overlay (SCOUT.md, skills)")
		} else {
			fmt.Println("\nKept: provider API keys and configured connectors (use --all to wipe them too).")
		}
		return fmt.Errorf("nothing was deleted — re-run with --yes to confirm")
	}
	rep, err := c.Reset(scope)
	if err != nil {
		return err
	}
	fmt.Printf("Scout reset: %s.\n", rep.Summary())
	if all {
		fmt.Println("Stored credentials were cleared; run `scout login <provider>` and re-add connectors.")
	} else {
		fmt.Println("Provider keys and connectors were kept. Starting fresh.")
	}
	return nil
}

func backupCmd(c *runtime.Core, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: scout backup <file>")
	}
	if _, err := c.DB.DB.Exec(`VACUUM INTO ?`, args[0]); err != nil {
		return err
	}
	fmt.Println("backup written to", args[0], "(includes encrypted secrets table — keep private)")
	return nil
}

func restoreCmd(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: scout restore <file>")
	}
	cfg := config.Load()
	src, err := os.Open(args[0])
	if err != nil {
		return err
	}
	defer src.Close()
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o700); err != nil {
		return err
	}
	dst, err := os.OpenFile(cfg.DBPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer dst.Close()
	if _, err := copyFile(dst, src); err != nil {
		return err
	}
	fmt.Println("restored to", cfg.DBPath)
	return nil
}

func mcpCmd(c *runtime.Core, args []string) error {
	sub := "stdio"
	if len(args) > 0 {
		sub = args[0]
	}
	deps := mcpserver.Deps{Core: c}
	switch sub {
	case "stdio":
		return mcpserver.RunStdio(deps)
	case "serve":
		addr := c.Cfg.Addr
		if len(args) > 1 {
			addr = args[1]
		}
		fmt.Fprintf(os.Stderr, "scout MCP (Streamable HTTP) on http://%s/mcp\n", addr)
		return http.ListenAndServe(addr, mcpserver.Handler(deps))
	default:
		return fmt.Errorf("usage: scout mcp [stdio|serve]")
	}
}
