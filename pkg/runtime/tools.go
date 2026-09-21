package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ianclemence/scout/pkg/agent"
	"github.com/ianclemence/scout/pkg/approve"
)

// Permission classes, from least to most consequential. The runtime
// enforces them; they are not hidden inside individual skills.
type Permission string

const (
	PermRead        Permission = "read"            // search, inspect, retrieve
	PermAnalyze     Permission = "analyze"         // evaluate, match, research
	PermDraft       Permission = "draft"           // prepared but never sent
	PermMutateLocal Permission = "mutate_local"    // local DB writes (explicit, audited)
	PermExternal    Permission = "external_action" // submit, send — approval required
	PermFinancial   Permission = "financial"       // binding/financial — strict approval
)

// ToolResult is the structured envelope tools return. Results stay
// structured so the agent never reverse-engineers CLI text.
type ToolResult struct {
	Success    bool              `json:"success"`
	Data       any               `json:"data,omitempty"`
	Source     string            `json:"source,omitempty"`
	Warnings   []string          `json:"warnings,omitempty"`
	Errors     []string          `json:"errors,omitempty"`
	Provenance string            `json:"provenance,omitempty"`
	Meta       map[string]string `json:"meta,omitempty"`
}

func okResult(data any) string {
	b, _ := json.Marshal(ToolResult{Success: true, Data: data})
	return string(b)
}

func errResult(errs ...string) string {
	b, _ := json.Marshal(ToolResult{Success: false, Errors: errs})
	return string(b)
}

// Tool is a Scout domain capability with a stable typed contract.
type Tool struct {
	Name        string
	Description string
	ArgsHint    string            // short arg documentation for prompts and help
	ArgsSchema  map[string]string // arg name -> type hint
	Permission  Permission
	Timeout     time.Duration
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
		{Name: "get_profile", Permission: PermRead, Description: "Show the structured professional profile.", ArgsHint: "{}", ReadOnly: true,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				p, err := c.Profile()
				if err != nil {
					return "", err
				}
				return toJSON(map[string]any{"name": p.DisplayName, "title": p.Title, "skills": p.Skills,
					"min_budget": p.MinProjectBudget, "min_hourly": p.MinHourlyRate, "excluded": p.ExcludedWork,
					"proposal_style": p.ProposalStyle}), nil
			}},
		{Name: "list_evidence", Permission: PermRead, Description: "List profile evidence items.", ArgsHint: `{"limit": 8}`, ReadOnly: true,
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
		{Name: "search_opportunities", Permission: PermRead, Description: "Search stored opportunities.", ArgsHint: `{"query": "go api", "status": "review"}`, ReadOnly: true,
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
		{Name: "get_opportunity", Permission: PermRead, Description: "Full opportunity detail.", ArgsHint: `{"id": "opp-..."}`, ReadOnly: true,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				o, err := c.GetOpportunity(str(args, "id"))
				if err != nil {
					return "", err
				}
				return toJSON(o), nil
			}},
		{Name: "analyze_opportunity", Permission: PermAnalyze, Description: "Run filter + match evaluation on an opportunity.", ArgsHint: `{"id": "opp-..."}`, ReadOnly: true,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				ev, f, err := c.Analyze(ctx, str(args, "id"), c.engineFor("analysis"))
				if err != nil {
					return "", err
				}
				return toJSON(map[string]any{"filter_pass": f.Pass, "filter_reason": f.Reason, "evaluation": ev}), nil
			}},
		{Name: "prepare_proposal", Permission: PermDraft, Description: "Draft a tailored proposal (draft only, no external writes).", ArgsHint: `{"id": "opp-..."}`, ReadOnly: false,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				pr, err := c.DraftProposal(ctx, str(args, "id"), c.engineFor("proposal"))
				if err != nil {
					return "", err
				}
				return toJSON(pr), nil
			}},
		{Name: "request_approval", Permission: PermDraft, Description: "Create a pending approval for a consequential action (e.g. submit_proposal). Does not execute.", ArgsHint: `{"opportunity_id": "opp-..."}`, ReadOnly: false,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				a, err := c.RequestSubmitApproval(str(args, "opportunity_id"))
				if err != nil {
					return "", err
				}
				return toJSON(a), nil
			}},
		{Name: "list_pending_approvals", Permission: PermRead, Description: "Show actions awaiting human decision.", ArgsHint: "{}", ReadOnly: true,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				acts, err := c.PendingApprovals()
				if err != nil {
					return "", err
				}
				return toJSON(acts), nil
			}},
		{Name: "get_pipeline", Permission: PermRead, Description: "Application counts by stage.", ArgsHint: "{}", ReadOnly: true,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				m, err := c.Pipeline()
				if err != nil {
					return "", err
				}
				return toJSON(m), nil
			}},
		{Name: "list_applications", Permission: PermRead, Description: "List applications.", ArgsHint: "{}", ReadOnly: true,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				apps, err := c.ListApplications(num(args, "limit", 50))
				if err != nil {
					return "", err
				}
				return toJSON(apps), nil
			}},
		{Name: "list_messages", Permission: PermRead, Description: "List stored messages.", ArgsHint: "{}", ReadOnly: true,
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
		{Name: "run_discovery", Permission: PermRead, Description: "Summarize discovery over stored opportunities (no external writes).", ArgsHint: "{}", ReadOnly: true,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				s, err := c.RunDiscovery(true)
				if err != nil {
					return "", err
				}
				return toJSON(s), nil
			}},
		{Name: "load_skill", Permission: PermRead, Description: "Load a skill's full procedure before acting on it.", ArgsHint: `{"name": "evaluate-opportunity"}`, ArgsSchema: map[string]string{"name": "string"}, ReadOnly: true,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				reg, err := c.SkillRegistry()
				if err != nil {
					return "", err
				}
				name, _ := args["name"].(string)
				s, ok := reg.Find(name)
				if !ok {
					return "", fmt.Errorf("unknown skill %q", name)
				}
				return okResult(map[string]any{"name": s.Name, "procedure": s.Body}), nil
			}},
		{Name: "add_feedback", Permission: PermMutateLocal, Description: "Record feedback on an opportunity (good_match, bad_match, too_low_budget, ...).", ArgsHint: `{"opportunity_id": "...", "signal": "bad_match", "note": "too much wordpress"}`, ReadOnly: false,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				if err := c.AddFeedback(str(args, "opportunity_id"), str(args, "signal"), str(args, "note")); err != nil {
					return "", err
				}
				return `{"ok":true}`, nil
			}},
	}
	tools = append(tools, sourceTools(c)...)
	tools = append(tools, memoryTools(c)...)
	tools = append(tools, researchTools(c)...)
	tools = append(tools, profileTools(c)...)
	tools = append(tools, githubTools(c)...)
	tools = append(tools, applyTools(c)...)
	seen := map[string]bool{}
	var dedup []*Tool
	for _, t := range tools {
		if seen[t.Name] {
			continue
		}
		seen[t.Name] = true
		dedup = append(dedup, t)
	}
	tools = dedup
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

// ToolCatalog describes tools for the system prompt: names + one line.
// Full argument shapes live behind load_tool_detail to keep every turn
// cheap on small local models.
func (c *Core) ToolCatalog() string {
	var sb strings.Builder
	for _, t := range c.Tools() {
		fmt.Fprintf(&sb, "- %s: %s\n", t.Name, t.Description)
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
