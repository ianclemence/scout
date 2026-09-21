// Package mcpserver exposes Scout itself as an MCP server (stdio + Streamable HTTP).
// Domain-level tools only; no dangerous low-level primitives.
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ianclemence/scout/internal/approve"
	"github.com/ianclemence/scout/internal/domain"
	"github.com/ianclemence/scout/internal/match"
	"github.com/ianclemence/scout/internal/profile"
	"github.com/ianclemence/scout/internal/store"
)

type Deps struct {
	Store *store.Store
}

func New(deps Deps) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "scout", Version: "v0.1.0"}, nil)
	mcp.AddTool(s, &mcp.Tool{Name: "scout_status", Description: "Scout health, counts of opportunities, pending approvals, applications."},
		func(ctx context.Context, req *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
			return textResult(statusJSON(deps.Store)), nil, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "scout_search", Description: "Search local opportunities by keyword."},
		func(ctx context.Context, req *mcp.CallToolRequest, args struct {
			Query string `json:"query"`
		}) (*mcp.CallToolResult, any, error) {
			return textResult(searchJSON(deps.Store, args.Query)), nil, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "scout_review_opportunity", Description: "Show opportunity detail with match evaluation."},
		func(ctx context.Context, req *mcp.CallToolRequest, args struct {
			ID string `json:"id"`
		}) (*mcp.CallToolResult, any, error) {
			return textResult(reviewJSON(deps.Store, args.ID)), nil, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "scout_match_opportunity", Description: "Run deterministic match evaluation on an opportunity."},
		func(ctx context.Context, req *mcp.CallToolRequest, args struct {
			ID string `json:"id"`
		}) (*mcp.CallToolResult, any, error) {
			return textResult(matchJSON(deps.Store, args.ID)), nil, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "scout_list_applications", Description: "List applications with stage."},
		func(ctx context.Context, req *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
			return textResult(listJSON(deps.Store, `SELECT id,opportunity_id,source,stage,cost_connects,submitted_at FROM applications ORDER BY submitted_at DESC LIMIT 100`)), nil, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "scout_review_pending_actions", Description: "List draft and pending-approval actions."},
		func(ctx context.Context, req *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
			acts, _ := approve.List(deps.Store, true)
			b, _ := json.Marshal(acts)
			return textResult(string(b)), nil, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "scout_approve_action", Description: "Approve a pending action (does not execute external writes by itself)."},
		func(ctx context.Context, req *mcp.CallToolRequest, args struct {
			ID string `json:"id"`
		}) (*mcp.CallToolResult, any, error) {
			if err := approve.SetStatus(deps.Store, args.ID, "approved"); err != nil {
				return textResult(`{"error":` + jsonStr(err.Error()) + `}`), nil, nil
			}
			return textResult(`{"id":` + jsonStr(args.ID) + `,"status":"approved"}`), nil, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "scout_reject_action", Description: "Reject a pending action."},
		func(ctx context.Context, req *mcp.CallToolRequest, args struct {
			ID string `json:"id"`
		}) (*mcp.CallToolResult, any, error) {
			if err := approve.SetStatus(deps.Store, args.ID, "rejected"); err != nil {
				return textResult(`{"error":` + jsonStr(err.Error()) + `}`), nil, nil
			}
			return textResult(`{"id":` + jsonStr(args.ID) + `,"status":"rejected"}`), nil, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "scout_pipeline", Description: "Application pipeline grouped by stage."},
		func(ctx context.Context, req *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
			return textResult(pipelineJSON(deps.Store)), nil, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "scout_profile", Description: "Show the structured professional profile summary."},
		func(ctx context.Context, req *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
			p, _ := profile.Load(deps.Store)
			b, _ := json.Marshal(p)
			return textResult(string(b)), nil, nil
		})
	return s
}

// Handler returns a Streamable HTTP handler for mounting.
func Handler(deps Deps) http.Handler {
	s := New(deps)
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s }, nil)
}

// RunStdio serves Scout MCP over stdio.
func RunStdio(deps Deps) error {
	return New(deps).Run(context.Background(), mcp.NewStdioTransport())
}

func textResult(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func statusJSON(db *store.Store) string {
	var opps, pending, apps int
	db.DB.QueryRow(`SELECT COUNT(*) FROM opportunities`).Scan(&opps)
	db.DB.QueryRow(`SELECT COUNT(*) FROM pending_actions WHERE status IN ('draft','pending_approval')`).Scan(&pending)
	db.DB.QueryRow(`SELECT COUNT(*) FROM applications`).Scan(&apps)
	return fmt.Sprintf(`{"scout":"v0.1.0","opportunities":%d,"pending_approvals":%d,"applications":%d}`, opps, pending, apps)
}

func searchJSON(db *store.Store, q string) string {
	rows, err := db.DB.Query(`SELECT id,source,title,status FROM opportunities WHERE title LIKE ? OR description LIKE ? ORDER BY updated_at DESC LIMIT 50`, "%"+q+"%", "%"+q+"%")
	if err != nil {
		return `{"error":"query failed"}`
	}
	defer rows.Close()
	type r struct {
		ID, Source, Title, Status string
	}
	var out []r
	for rows.Next() {
		var x r
		rows.Scan(&x.ID, &x.Source, &x.Title, &x.Status)
		out = append(out, x)
	}
	b, _ := json.Marshal(out)
	return string(b)
}

func reviewJSON(db *store.Store, id string) string {
	var title, desc, status, source string
	err := db.DB.QueryRow(`SELECT title,description,status,source FROM opportunities WHERE id=?`, id).Scan(&title, &desc, &status, &source)
	if err != nil {
		return `{"error":"not found"}`
	}
	var evalData string
	db.DB.QueryRow(`SELECT data FROM evaluations WHERE opportunity_id=? ORDER BY created_at DESC LIMIT 1`, id).Scan(&evalData)
	o := map[string]any{"id": id, "title": title, "description": desc, "status": status, "source": source, "evaluation": evalData}
	b, _ := json.Marshal(o)
	return string(b)
}

func matchJSON(db *store.Store, id string) string {
	var title, desc, skills, budgetType string
	var bmin, bmax float64
	var connects int
	err := db.DB.QueryRow(`SELECT title,description,COALESCE(skills,''),budget_type,COALESCE(budget_min,0),COALESCE(budget_max,0),COALESCE(connects_cost,0) FROM opportunities WHERE id=?`, id).Scan(&title, &desc, &skills, &budgetType, &bmin, &bmax, &connects)
	if err != nil {
		return `{"error":"not found"}`
	}
	p, _ := profile.Load(db)
	o := &domain.Opportunity{ID: id, Title: title, Description: desc, BudgetType: budgetType, BudgetMin: bmin, BudgetMax: bmax, ConnectsCost: connects}
	ev := match.HeuristicEvaluate(p, o)
	b, _ := json.Marshal(ev)
	return string(b)
}

func listJSON(db *store.Store, q string) string {
	rows, err := db.DB.Query(q)
	if err != nil {
		return `{"error":"query failed"}`
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	var out []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		rows.Scan(ptrs...)
		m := map[string]any{}
		for i, c := range cols {
			m[c] = vals[i]
		}
		out = append(out, m)
	}
	b, _ := json.Marshal(out)
	return string(b)
}

func pipelineJSON(db *store.Store) string {
	rows, _ := db.DB.Query(`SELECT stage, COUNT(*) FROM applications GROUP BY stage`)
	defer func() {
		if rows != nil {
			rows.Close()
		}
	}()
	m := map[string]int{}
	if rows != nil {
		for rows.Next() {
			var s string
			var n int
			rows.Scan(&s, &n)
			m[s] = n
		}
	}
	b, _ := json.Marshal(m)
	return string(b)
}
