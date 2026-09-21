package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ianclemence/scout/internal/agent"
	"github.com/ianclemence/scout/internal/approve"
)

// Tool is a Scout domain capability. Consequential tools never execute
// external writes; they create PendingActions for human approval.
type Tool struct {
	Name        string
	Description string
	ArgsHint    string // short arg documentation for prompts and help
	ReadOnly    bool
	Handler     func(ctx context.Context, args map[string]any) (string, error)
}

func (c *Core) engineFor(role string) *agent.Engine {
	return engineFromEnv(c, role)
}

// Tools returns the registry in stable order.
func (c *Core) Tools() []*Tool {
	str := func(args map[string]any, k string) string {
		if v, ok := args[k].(string); ok {
			return v
		}
		return ""
	}
	num := func(args map[string]any, k string, d int) int {
		if v, ok := args[k].(float64); ok {
			return int(v)
		}
		return d
	}
	tools := []*Tool{
		{Name: "get_profile", Description: "Show the structured professional profile.", ArgsHint: "{}", ReadOnly: true,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				p, err := c.Profile()
				if err != nil {
					return "", err
				}
				return toJSON(map[string]any{"name": p.DisplayName, "title": p.Title, "skills": p.Skills,
					"min_budget": p.MinProjectBudget, "min_hourly": p.MinHourlyRate, "excluded": p.ExcludedWork,
					"proposal_style": p.ProposalStyle}), nil
			}},
		{Name: "list_evidence", Description: "List profile evidence items.", ArgsHint: `{"limit": 8}`, ReadOnly: true,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				ev, err := c.Evidence(num(args, "limit", 8))
				if err != nil {
					return "", err
				}
				type e struct {
					ID, Kind, Ref, Content string
				}
				out := []e{}
				for _, x := range ev {
					out = append(out, e{x.ID, x.Kind, x.Reference, truncate(x.Content, 300)})
				}
				return toJSON(out), nil
			}},
		{Name: "search_opportunities", Description: "Search stored opportunities.", ArgsHint: `{"query": "go api", "status": "review"}`, ReadOnly: true,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				opps, err := c.ListOpportunities(OpportunityFilter{Query: str(args, "query"), Status: str(args, "status"), Limit: num(args, "limit", 20)})
				if err != nil {
					return "", err
				}
				type o struct {
					ID, Source, Title, Status string
				}
				out := []o{}
				for _, x := range opps {
					out = append(out, o{x.ID, x.Source, x.Title, x.Status})
				}
				return toJSON(out), nil
			}},
		{Name: "get_opportunity", Description: "Full opportunity detail.", ArgsHint: `{"id": "opp-..."}`, ReadOnly: true,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				o, err := c.GetOpportunity(str(args, "id"))
				if err != nil {
					return "", err
				}
				return toJSON(o), nil
			}},
		{Name: "analyze_opportunity", Description: "Run filter + match evaluation on an opportunity.", ArgsHint: `{"id": "opp-..."}`, ReadOnly: true,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				ev, f, err := c.Analyze(ctx, str(args, "id"), c.engineFor("analysis"))
				if err != nil {
					return "", err
				}
				return toJSON(map[string]any{"filter_pass": f.Pass, "filter_reason": f.Reason, "evaluation": ev}), nil
			}},
		{Name: "prepare_proposal", Description: "Draft a tailored proposal (draft only, no external writes).", ArgsHint: `{"id": "opp-..."}`, ReadOnly: false,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				pr, err := c.DraftProposal(ctx, str(args, "id"), c.engineFor("proposal"))
				if err != nil {
					return "", err
				}
				return toJSON(pr), nil
			}},
		{Name: "request_approval", Description: "Create a pending approval for a consequential action (e.g. submit_proposal). Does not execute.", ArgsHint: `{"opportunity_id": "opp-..."}`, ReadOnly: false,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				a, err := c.RequestSubmitApproval(str(args, "opportunity_id"))
				if err != nil {
					return "", err
				}
				return toJSON(a), nil
			}},
		{Name: "list_pending_approvals", Description: "Show actions awaiting human decision.", ArgsHint: "{}", ReadOnly: true,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				acts, err := c.PendingApprovals()
				if err != nil {
					return "", err
				}
				return toJSON(acts), nil
			}},
		{Name: "get_pipeline", Description: "Application counts by stage.", ArgsHint: "{}", ReadOnly: true,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				m, err := c.Pipeline()
				if err != nil {
					return "", err
				}
				return toJSON(m), nil
			}},
		{Name: "list_applications", Description: "List applications.", ArgsHint: "{}", ReadOnly: true,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				apps, err := c.ListApplications(num(args, "limit", 50))
				if err != nil {
					return "", err
				}
				return toJSON(apps), nil
			}},
		{Name: "list_messages", Description: "List stored messages.", ArgsHint: "{}", ReadOnly: true,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				rows, err := c.DB.DB.Query(`SELECT id,source,thread_id,from_party,substr(body,1,500) FROM messages ORDER BY created_at DESC LIMIT 50`)
				if err != nil {
					return "", err
				}
				defer rows.Close()
				out := []map[string]any{}
				for rows.Next() {
					var id, src, th, from, body string
					rows.Scan(&id, &src, &th, &from, &body)
					out = append(out, map[string]any{"id": id, "source": src, "thread": th, "from": from, "body": body})
				}
				return toJSON(out), nil
			}},
		{Name: "list_sources", Description: "Work sources and discovered capabilities.", ArgsHint: "{}", ReadOnly: true,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				srcs, err := c.ListSources()
				if err != nil {
					return "", err
				}
				return toJSON(srcs), nil
			}},
		{Name: "run_discovery", Description: "Summarize discovery over stored opportunities (no external writes).", ArgsHint: "{}", ReadOnly: true,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				s, err := c.RunDiscovery(true)
				if err != nil {
					return "", err
				}
				return toJSON(s), nil
			}},
		{Name: "add_feedback", Description: "Record feedback on an opportunity (good_match, bad_match, too_low_budget, ...).", ArgsHint: `{"opportunity_id": "...", "signal": "bad_match", "note": "too much wordpress"}`, ReadOnly: false,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				if err := c.AddFeedback(str(args, "opportunity_id"), str(args, "signal"), str(args, "note")); err != nil {
					return "", err
				}
				return `{"ok":true}`, nil
			}},
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools
}

func (c *Core) FindTool(name string) *Tool {
	for _, t := range c.Tools() {
		if t.Name == name {
			return t
		}
	}
	return nil
}

// ToolCatalog describes tools for the system prompt.
func (c *Core) ToolCatalog() string {
	var sb strings.Builder
	for _, t := range c.Tools() {
		fmt.Fprintf(&sb, "- %s %s: %s\n", t.Name, t.ArgsHint, t.Description)
	}
	return sb.String()
}

func toJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"error":"encode failed"}`
	}
	return string(b)
}

var _ = approve.List
