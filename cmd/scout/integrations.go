// Command scout — terminal-native AI work acquisition agent.
// Bare `scout` enters the interactive session; subcommands are scriptable.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ianclemence/scout/pkg/mcpclient"
	"github.com/ianclemence/scout/pkg/runtime"
	"github.com/ianclemence/scout/pkg/upwork"
)

func integrationsCmd(c *runtime.Core, args []string) error {
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "list":
		srcs, err := c.ListSources()
		if err != nil {
			return err
		}
		for _, s := range srcs {
			fmt.Printf("%s\t%s\t%s\tenabled=%v\tcaps=%v\n", s.Name, s.Kind, s.Endpoint, s.Enabled, s.Capabilities)
		}
	case "add":
		if len(args) < 3 {
			return fmt.Errorf("usage: scout integrations add <name> <endpoint> | scout integrations add <name> --command \"prog args...\"")
		}
		if len(args) >= 3 && args[1] == "--command" {
			return fmt.Errorf("usage: scout integrations add <name> --command \"prog args...\"")
		}
		if args[2] == "--command" {
			if len(args) < 4 {
				return fmt.Errorf("usage: scout integrations add <name> --command \"prog args...\"")
			}
			_, err := c.DB.DB.Exec(`INSERT OR REPLACE INTO sources(id,name,kind,endpoint,command,enabled,capabilities) VALUES(?,?,?,?,?,1,'')`,
				"src-"+strings.ToLower(strings.ReplaceAll(args[1], " ", "-")), args[1], "mcp-stdio", "", args[3])
			fmt.Println("added stdio source", args[1])
			return err
		}
		if !strings.HasPrefix(args[2], "https://") && !strings.HasPrefix(args[2], "http://localhost") && !strings.HasPrefix(args[2], "http://127.0.0.1") {
			return fmt.Errorf("endpoint must be https (or localhost http)")
		}
		_, err := c.DB.DB.Exec(`INSERT OR REPLACE INTO sources(id,name,kind,endpoint,enabled,capabilities) VALUES(?,?,?,?,1,'')`,
			"src-"+strings.ToLower(strings.ReplaceAll(args[1], " ", "-")), args[1], "mcp", args[2])
		fmt.Println("added", args[1])
		return err
	case "test":
		name := "Upwork"
		if len(args) > 1 {
			name = args[1]
		}
		var endpoint, kind, command string
		if err := c.DB.DB.QueryRow(`SELECT endpoint,kind,COALESCE(command,'') FROM sources WHERE name=?`, name).Scan(&endpoint, &kind, &command); err != nil {
			return fmt.Errorf("source %q not found", name)
		}
		tok, _ := c.LoadSecret("mcp:" + name)
		conn := &mcpclient.Connector{ID: name, Endpoint: endpoint, Token: tok}
		if kind == "mcp-stdio" {
			if command == "" {
				return fmt.Errorf("stdio source %q has no command", name)
			}
			conn.Command = strings.Fields(command)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		tools, err := conn.ListTools(ctx)
		if err != nil {
			return fmt.Errorf("capability discovery failed: %w", err)
		}
		caps := upwork.Discover(tools)
		fmt.Printf("%s: %d tools, capabilities=%v\n", name, len(tools), caps)
		for _, t := range tools {
			fmt.Printf("  - %s\n", t.Name)
		}
		cb, _ := json.Marshal(caps)
		_, _ = c.DB.DB.Exec(`UPDATE sources SET capabilities=? WHERE name=?`, string(cb), name)
	case "token":
		return integrationsTokenCmd(c, args[1:])
	default:
		return fmt.Errorf("usage: scout integrations [list|add|test|token]")
	}
	return nil
}

// integrationsTokenCmd stores an MCP access token (masked prompt, encrypted).

func integrationsTokenCmd(c *runtime.Core, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: scout integrations token <name>")
	}
	fmt.Printf("Access token for %s: ", args[0])
	key, err := readPassword()
	if err != nil || key == "" {
		return fmt.Errorf("no token entered")
	}
	if err := c.SaveSecret("mcp:"+args[0], key); err != nil {
		return err
	}
	fmt.Println("token stored (encrypted, never displayed).")
	return nil
}
