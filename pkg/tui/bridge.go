package tui

import (
	"context"

	"github.com/ianclemence/scout/pkg/csession"
	"github.com/ianclemence/scout/pkg/isession"
)

func bg() context.Context { return context.Background() }

type cmdInfo struct {
	Name        string
	Description string
}

func commandList() []cmdInfo {
	var out []cmdInfo
	for _, c := range isession.Registry() {
		out = append(out, cmdInfo{c.Name, c.Description})
	}
	return out
}

func saveSessionModel(st *isession.ReplState) {
	csession.Touch(st.Core.DB, st.Sess.ID, st.Sess.Provider, st.Sess.Model)
}
