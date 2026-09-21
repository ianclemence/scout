package tui

import (
	"context"
	"sort"
	"strings"

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

type modelOpt struct {
	prov  string
	model string
	note  string
}

func modelOptionsFor(st *isession.ReplState) []modelOpt {
	var out []modelOpt
	for _, role := range []string{"screening", "analysis", "proposal", "conversation", "deep_analysis"} {
		r := st.Core.Cfg.Models[role]
		out = append(out, modelOpt{r.Provider, r.Model, "role:" + role})
	}
	for _, m := range st.Core.Registry().List(bg(), "") {
		note := "reasoning:" + m.Reasoning
		if m.Provider == "ollama" {
			note = "local"
		}
		if m.Source == "builtin" {
			note += " · builtin"
		}
		out = append(out, modelOpt{m.Provider, m.ID, note})
	}
	seen := map[string]bool{}
	var dedup []modelOpt
	for _, o := range out {
		if o.prov == "" || o.model == "" {
			continue
		}
		if k := o.prov + "/" + o.model; !seen[k] {
			seen[k] = true
			dedup = append(dedup, o)
		}
	}
	sort.Slice(dedup, func(i, j int) bool { return dedup[i].prov < dedup[j].prov })
	return dedup
}

func splitRef(s string) (string, string) {
	p := strings.SplitN(s, "/", 2)
	if len(p) != 2 {
		return "", ""
	}
	return p[0], p[1]
}

func saveSessionModel(st *isession.ReplState) {
	csession.Touch(st.Core.DB, st.Sess.ID, st.Sess.Provider, st.Sess.Model)
}
