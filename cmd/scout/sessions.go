// Command scout — terminal-native AI work acquisition agent.
// Bare `scout` enters the interactive session; subcommands are scriptable.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ianclemence/scout/pkg/csession"
	"github.com/ianclemence/scout/pkg/llm"
	"github.com/ianclemence/scout/pkg/runtime"
	"github.com/ianclemence/scout/pkg/skills"
)

func sessionsCmd(c *runtime.Core, args []string) error {
	list, err := csession.List(c.DB)
	if err != nil {
		return err
	}
	asJSON := len(args) > 0 && args[0] == "--json"
	if asJSON {
		b, _ := json.Marshal(list)
		fmt.Println(string(b))
		return nil
	}
	for _, s := range list {
		fmt.Printf("%s\t%s\t%s/%s\t%s\n", s.ID, s.Name, s.Provider, s.Model, s.UpdatedAt.Format(time.RFC3339))
	}
	return nil
}

func skillsCmd(c *runtime.Core, args []string) error {
	_ = c
	reg, err := skills.Load()
	if err != nil {
		return err
	}
	if len(args) > 0 {
		for _, s := range reg.Select(strings.Join(args, " "), 5) {
			fmt.Printf("%s\n", s.Name)
		}
		return nil
	}
	for _, s := range reg.List() {
		fmt.Printf("%-28s %s\n", s.Name, strings.Join(s.Triggers, ", "))
	}
	return nil
}

func toolsCmd(c *runtime.Core) error {
	for _, t := range c.Tools() {
		fmt.Printf("%-26s %-14s %s\n", t.Name, t.Permission, t.Description)
	}
	return nil
}

// askCmd runs one agent turn non-interactively; --json emits the final text as JSON.

func askCmd(c *runtime.Core, args []string) error {
	asJSON := false
	var q []string
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		} else {
			q = append(q, a)
		}
	}
	if len(q) == 0 {
		return fmt.Errorf("usage: scout ask [--json] \"question\"")
	}
	sess, err := csession.Create(c.DB, "ask", c.Cfg.Models["conversation"].Provider, c.Cfg.Models["conversation"].Model)
	if err != nil {
		return err
	}
	eng := c.EngineFor(sess.Provider, sess.Model)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	final, err := c.RunAgent(ctx, eng, []llm.Message{{Role: "user", Content: strings.Join(q, " ")}}, "", func(ev runtime.Event) {
		switch ev.Type {
		case "tool_start":
			fmt.Fprintf(os.Stderr, "◐ %s %s\n", ev.Name, ev.Args)
		case "tool_end":
			if ev.Err != nil {
				fmt.Fprintf(os.Stderr, "✗ %s: %s\n", ev.Name, ev.Err)
			} else {
				fmt.Fprintf(os.Stderr, "✓ %s\n", ev.Text)
			}
		case "error":
			fmt.Fprintf(os.Stderr, "error: %s\n", ev.Err)
		default:
			if ev.Type == "token" && !asJSON {
				fmt.Print(ev.Text)
			}
		}
	})
	if err != nil {
		return err
	}
	_ = csession.AppendMessages(c.DB, sess.ID, []csession.Message{{Role: "user", Content: strings.Join(q, " ")}, {Role: "assistant", Content: final}})
	if asJSON {
		b, _ := json.Marshal(map[string]string{"session": sess.ID, "response": final})
		fmt.Println(string(b))
	} else {
		fmt.Println()
	}
	return nil
}
