package runtime

import (
	"fmt"
	"sort"
	"strings"
)

// coreTools are always offered: profile/evidence grounding, the core work loop,
// approvals, and document parsing. They are the tools almost every real request
// can touch.
var coreTools = []string{
	"load_skill", "get_profile", "list_evidence", "search_opportunities",
	"get_opportunity", "analyze_opportunity", "analyze_opportunities", "prepare_proposal",
	"request_approval", "list_pending_approvals", "parse_document",
	"search_learned_preferences", "list_tools",
}

const catalogCap = 16

// ToolCatalogFor returns a compact tool catalog scoped to the request. Sending
// all ~50 tool descriptions every turn wastes context and degrades tool choice
// on smaller models; scoring by the request keeps the catalog small while
// list_tools exposes the full set when the model needs more.
func (c *Core) ToolCatalogFor(request string) string {
	tokens := requestTokens(request)
	if len(tokens) == 0 {
		return c.toolCatalog(c.Tools())
	}
	core := map[string]bool{}
	for _, n := range coreTools {
		core[n] = true
	}
	type scored struct {
		t     *Tool
		score int
	}
	var ranked []scored
	for _, t := range c.Tools() {
		if core[t.Name] {
			ranked = append(ranked, scored{t, 0})
			continue
		}
		hay := strings.ToLower(t.Name + " " + t.Description)
		score := 0
		for _, tok := range tokens {
			if hasToken(hay, tok) {
				score++
			}
		}
		if score > 0 {
			ranked = append(ranked, scored{t, score})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })
	if len(ranked) > catalogCap {
		ranked = ranked[:catalogCap]
	}
	// Keep a stable, alphabetical presentation.
	selected := make([]*Tool, 0, len(ranked))
	for _, r := range ranked {
		selected = append(selected, r.t)
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].Name < selected[j].Name })
	return c.toolCatalog(selected)
}

func (c *Core) toolCatalog(tools []*Tool) string {
	var sb strings.Builder
	for _, t := range tools {
		fmt.Fprintf(&sb, "- %s: %s\n", t.Name, t.Description)
	}
	return sb.String()
}

// FullToolCatalog is the unscoped catalog, for list_tools and diagnostics.
func (c *Core) FullToolCatalog() string {
	return c.toolCatalog(c.Tools())
}

// RecordTrajectory stores one agent turn's process: the request, the tools it
// used, how many turns it took, its final answer, and any error. It is the raw
// material for evaluation and future learning, and it never contains secrets
// (summaries are already redacted elsewhere).
func (c *Core) RecordTrajectory(request string, tools []string, turns int, final string, runErr error) {
	errText := ""
	if runErr != nil {
		errText = runErr.Error()
	}
	_, _ = c.DB.DB.Exec(`INSERT INTO trajectories(id,created_at,request,tools,turns,final,error) VALUES(?,?,?,?,?,?,?)`,
		newID("traj"), now(), truncate(strings.Join(strings.Fields(request), " "), 500),
		strings.Join(tools, ","), turns, truncate(final, 2000), errText)
}

var catalogStopwords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "you": true, "your": true,
	"our": true, "are": true, "this": true, "that": true, "from": true, "will": true,
	"have": true, "has": true, "not": true, "but": true, "all": true, "any": true,
	"can": true, "who": true, "what": true, "new": true, "work": true, "role": true,
	"job": true, "need": true, "looking": true, "must": true, "able": true, "using": true,
	"help": true, "into": true, "over": true, "they": true, "them": true, "their": true,
	"about": true, "would": true, "should": true, "could": true, "when": true, "where": true,
	"which": true, "than": true, "then": true, "also": true, "more": true, "most": true,
	"other": true, "some": true, "such": true, "only": true, "just": true, "very": true,
	"each": true, "does": true, "how": true, "why": true, "well": true, "make": true,
}

func requestTokens(s string) []string {
	seen := map[string]bool{}
	var out []string
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '.' && r != '#' && r != '+' && r != '-'
	}) {
		w = strings.Trim(w, ".-+")
		if len(w) < 3 || catalogStopwords[w] || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	return out
}

func hasToken(hay, tok string) bool {
	from := 0
	for from <= len(hay)-len(tok) {
		i := strings.Index(hay[from:], tok)
		if i < 0 {
			return false
		}
		i += from
		end := i + len(tok)
		leftOK := i == 0 || !isWordByte(hay[i-1])
		rightOK := end >= len(hay) || !isWordByte(hay[end])
		if leftOK && rightOK {
			return true
		}
		from = i + 1
	}
	return false
}

func isWordByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}
