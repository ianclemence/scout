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

	"github.com/ianclemence/scout/pkg/config"
	"github.com/ianclemence/scout/pkg/csession"
	"github.com/ianclemence/scout/pkg/isession"
	"github.com/ianclemence/scout/pkg/llm"
	"github.com/ianclemence/scout/pkg/runtime"
	"github.com/ianclemence/scout/pkg/termui"
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
	var rows [][]string
	for _, s := range list {
		name := s.Name
		if name == "" {
			name = termui.Dim("untitled")
		}
		rows = append(rows, []string{shortSessionID(s.ID), name, termui.Dim(s.Provider + "/" + s.Model), termui.Dim(s.UpdatedAt.Format("2006-01-02 15:04"))})
	}
	termui.Print(termui.Table([]string{"Session", "Name", "Model", "Updated"}, rows))
	return nil
}

// shortSessionID trims an id to a readable prefix for list output.
func shortSessionID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func skillsCmd(c *runtime.Core, args []string) error {
	_ = c
	reg, err := c.SkillRegistry()
	if err != nil {
		return err
	}
	if len(args) > 0 {
		for _, s := range reg.Select(strings.Join(args, " "), 5) {
			fmt.Println(s.Name)
		}
		return nil
	}
	var srows [][]string
	for _, s := range reg.List() {
		srows = append(srows, []string{s.Name, termui.Dim(strings.Join(s.Triggers, ", "))})
	}
	termui.Print(termui.Table([]string{"Skill", "Triggers"}, srows))
	return nil
}

func toolsCmd(c *runtime.Core) error {
	var rows [][]string
	for _, t := range c.Tools() {
		rows = append(rows, []string{t.Name, termui.Dim(string(t.Permission)), termui.Dim(t.Description)})
	}
	termui.Print(termui.Table([]string{"Tool", "Permission", "Description"}, rows))
	return nil
}

// askCmd runs one agent turn non-interactively; --json emits the final text as JSON.

func askCmd(c *runtime.Core, args []string) error {
	// Sweep message-less rows on the way out: a run that errors before
	// storing anything must not leave an empty session behind.
	defer func() { _, _ = csession.DeleteEmpty(c.DB) }()
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
	sess, err := csession.Create(c.DB, "ask", c.Cfg.Models[config.RoleConversation].Provider, c.Cfg.Models[config.RoleConversation].Model)
	if err != nil {
		return err
	}
	eng := c.EngineFor(sess.Provider, sess.Model)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	final, err := c.RunAgent(ctx, eng, []llm.Message{{Role: "user", Content: strings.Join(q, " ")}}, "", func(ev runtime.Event) {
		switch ev.Type {
		case "tool_start":
			// Product-language status on stderr (diagnostics for scripts);
			// raw tool names/args are never printed.
			if a := isession.ActivityLabel(ev.Name); a != "" && a != "Thinking" {
				fmt.Fprintf(os.Stderr, "◐ %s…\n", a)
			}
		case "tool_end":
			// silent: tool results are internals, not the answer.
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
