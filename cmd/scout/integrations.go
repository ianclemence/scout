// Command scout — terminal-native AI work acquisition agent.
// Subcommands are scriptable; the interactive session is the primary surface.
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ianclemence/scout/pkg/runtime"
)

// integrationsCmd is the CLI view of the shared connector surface
// (runtime.Connections / ProbeConnection / Add* / StoreConnectionToken).
// The /sources slash command and the agent tools call the same Core methods,
// so the three interfaces can never disagree.
func integrationsCmd(c *runtime.Core, args []string) error {
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "list", "ls":
		conns, err := c.Connections()
		if err != nil {
			return err
		}
		if len(conns) == 0 {
			fmt.Println("No work sources configured. Add one:")
			fmt.Println("  scout integrations add Upwork https://mcp.upwork.com/mcp")
			return nil
		}
		fmt.Printf("%-14s %-10s %-16s %-18s %s\n", "NAME", "KIND", "STATE", "AUTH", "CAPABILITIES")
		for _, conn := range conns {
			state := "off"
			if conn.Enabled {
				state = conn.Status
				if state == "" {
					state = "configured"
				}
			}
			endpoint := conn.Endpoint
			if conn.Kind == "mcp-stdio" {
				endpoint = conn.Command
			}
			fmt.Printf("%-14s %-10s %-16s %-18s %s\n", conn.Name, conn.Kind, state, conn.Auth, runtime.CapabilityLabels(conn.Capabilities))
			if endpoint != "" {
				fmt.Printf("%-14s %s\n", "", endpoint)
			}
			if conn.Detail != "" && conn.Status != "configured" && conn.Status != "" {
				fmt.Printf("%-14s %s\n", "", conn.Detail)
			}
		}
		return nil
	case "add":
		if len(args) < 3 {
			return fmt.Errorf("usage: scout integrations add <name> <https-url> | scout integrations add <name> --command \"prog args...\"")
		}
		name := args[1]
		if args[2] == "--command" {
			if len(args) < 4 {
				return fmt.Errorf("usage: scout integrations add <name> --command \"prog args...\"")
			}
			if err := c.AddStdioConnection(name, strings.Join(args[3:], " ")); err != nil {
				return err
			}
			fmt.Println("added stdio source", name)
			return nil
		}
		if err := c.AddMCPConnection(name, args[2]); err != nil {
			return err
		}
		fmt.Println("added", name)
		return nil
	case "test":
		ref := "Upwork"
		if len(args) > 1 {
			ref = args[1]
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		conn, err := c.ProbeConnection(ctx, ref, 20*time.Second)
		if err != nil {
			return err
		}
		fmt.Printf("%s: status=%s auth=%s tools=%d capabilities=%s\n",
			conn.Name, conn.Status, conn.Auth, conn.ToolCount,
			runtime.CapabilityLabels(conn.Capabilities))
		if conn.Detail != "" {
			fmt.Printf("  %s\n", conn.Detail)
		}
		return nil
	case "remove", "rm", "delete":
		if len(args) < 2 {
			return fmt.Errorf("usage: scout integrations remove <name>")
		}
		if err := c.RemoveConnection(args[1]); err != nil {
			return err
		}
		fmt.Println("removed", args[1])
		return nil
	case "enable", "disable":
		if len(args) < 2 {
			return fmt.Errorf("usage: scout integrations %s <name>", sub)
		}
		if err := c.SetConnectionEnabled(args[1], sub == "enable"); err != nil {
			return err
		}
		fmt.Printf("%s %sd\n", args[1], sub)
		return nil
	case "token":
		return integrationsTokenCmd(c, args[1:])
	default:
		return fmt.Errorf("usage: scout integrations [list|add|test|token|enable|disable|remove]")
	}
}

// integrationsTokenCmd stores an MCP access token (masked prompt, encrypted).
func integrationsTokenCmd(c *runtime.Core, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: scout integrations token <name>")
	}
	conn, err := c.FindConnection(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("Access token for %s: ", conn.Name)
	key, err := readPassword()
	if err != nil || key == "" {
		return fmt.Errorf("no token entered")
	}
	if err := c.StoreConnectionToken(args[0], key); err != nil {
		return err
	}
	fmt.Println("token stored (encrypted, never displayed).")
	return nil
}
