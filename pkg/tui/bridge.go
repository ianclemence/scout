package tui

import (
	"context"

	"github.com/ianclemence/scout/pkg/csession"
	"github.com/ianclemence/scout/pkg/isession"
)

func bg() context.Context { return context.Background() }

func saveSessionModel(st *isession.ReplState) {
	csession.Touch(st.Core.DB, st.Sess.ID, st.Sess.Provider, st.Sess.Model)
}
