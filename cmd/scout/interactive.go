// Command scout — terminal-native AI work acquisition agent.
// Bare `scout` enters the interactive session; subcommands are scriptable.
package main

import (
	"fmt"

	"github.com/ianclemence/scout/pkg/config"
	"github.com/ianclemence/scout/pkg/csession"
	"github.com/ianclemence/scout/pkg/isession"
	"github.com/ianclemence/scout/pkg/llm"
	"github.com/ianclemence/scout/pkg/tui"
)

func runInteractive(resumeRef string) error {
	c, err := openCore()
	if err != nil {
		return err
	}
	defer c.DB.Close()
	var sess *csession.Session
	if resumeRef != "" {
		sess, err = c.ResolveSession(resumeRef)
		if err != nil {
			return err
		}
		fmt.Printf("Resumed session %s (%s).\n", sess.ID[:12], sess.Name)
	} else {
		sess, err = csession.Create(c.DB, "interactive", c.Cfg.Models[config.RoleConversation].Provider, c.Cfg.Models[config.RoleConversation].Model)
		if err != nil {
			return err
		}
	}
	st := &isession.ReplState{Core: c, Sess: sess}
	if msgs, err := csession.LoadMessages(c.DB, sess.ID, 20); err == nil {
		for _, m := range msgs {
			st.History = append(st.History, llm.Message{Role: m.Role, Content: m.Content})
		}
	}
	if isTerminal() {
		return tui.Run(st)
	}
	return isession.Run(c, sess)
}
