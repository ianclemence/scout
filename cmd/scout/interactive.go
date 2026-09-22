// Command scout — terminal-native AI work acquisition agent.
// Bare `scout` enters the interactive session; subcommands are scriptable.
package main

import (
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
		outErr := tui.Run(st)
		// Empty sessions are not tracked: sweep rows that never gained a
		// message (this run if untouched, plus any legacy orphans) so the
		// resume list never fills with contentless launches.
		_, _ = csession.DeleteEmpty(c.DB)
		return outErr
	}
	outErr := isession.Run(c, sess)
	_, _ = csession.DeleteEmpty(c.DB)
	return outErr
}
