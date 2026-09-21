// Package mcpserver exposes Scout Core as an MCP server (stdio + Streamable HTTP).
// Domain-level tools only, backed by the same runtime.Core the CLI uses.
package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ianclemence/scout/pkg/approve"
	"github.com/ianclemence/scout/pkg/runtime"
	"github.com/ianclemence/scout/pkg/version"
)

type Deps struct {
	Core *runtime.Core
}

// exposed tools: read + draft only. Approval decisions stay human-driven;
// approve_action/reject_action record intent and never execute external writes.
var exposed = map[string]bool{
	"get_profile": true, "list_evidence": true,
	"search_opportunities": true, "get_opportunity": true,
	"analyze_opportunity": true, "match_opportunity": true,
	"prepare_proposal": true, "list_proposals": false,
	"list_applications": true, "list_messages": true, "get_pipeline": true,
	"list_pending_approvals": true, "get_status": true,
	"discover_opportunities": true, "list_sources": true,
}

func New(deps Deps) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "scout", Version: version.Version}, nil)
	coreTools := map[string]*runtime.Tool{}
	for _, t := range deps.Core.Tools() {
		coreTools[t.Name] = t
	}
	add := func(name, desc string) {
		t, ok := coreTools[name]
		if !ok {
			return
		}
		tool := t
		mcp.AddTool(s, &mcp.Tool{Name: name, Description: desc},
			func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
				out, err := tool.Handler(ctx, args)
				if err != nil {
					return textResult(`{"error":` + jsonStr(err.Error()) + `}`), nil, nil
				}
				return textResult(out), nil, nil
			})
	}
	for _, t := range deps.Core.Tools() {
		if !exposed[t.Name] {
			continue
		}
		add(t.Name, t.Description)
	}
	// Aliases with Scout-domain names.
	mcp.AddTool(s, &mcp.Tool{Name: "match_opportunity", Description: "Alias of analyze_opportunity: structured fit evaluation."},
		func(ctx context.Context, req *mcp.CallToolRequest, args struct {
			ID string `json:"id"`
		}) (*mcp.CallToolResult, any, error) {
			out, err := coreTools["analyze_opportunity"].Handler(ctx, map[string]any{"id": args.ID})
			if err != nil {
				return textResult(`{"error":` + jsonStr(err.Error()) + `}`), nil, nil
			}
			return textResult(out), nil, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "discover_opportunities", Description: "Discovery summary over stored opportunities."},
		func(ctx context.Context, req *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
			out, err := coreTools["run_discovery"].Handler(ctx, map[string]any{})
			if err != nil {
				return textResult(`{"error":` + jsonStr(err.Error()) + `}`), nil, nil
			}
			return textResult(out), nil, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "approve_action", Description: "Record approval intent for a pending action. Never executes external writes by itself."},
		func(ctx context.Context, req *mcp.CallToolRequest, args struct {
			ID string `json:"id"`
		}) (*mcp.CallToolResult, any, error) {
			if err := deps.Core.SetApprovalStatus(args.ID, "approved"); err != nil {
				return textResult(`{"error":` + jsonStr(err.Error()) + `}`), nil, nil
			}
			return textResult(`{"id":` + jsonStr(args.ID) + `,"status":"approved"}`), nil, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "reject_action", Description: "Reject a pending action."},
		func(ctx context.Context, req *mcp.CallToolRequest, args struct {
			ID string `json:"id"`
		}) (*mcp.CallToolResult, any, error) {
			if err := deps.Core.SetApprovalStatus(args.ID, "rejected"); err != nil {
				return textResult(`{"error":` + jsonStr(err.Error()) + `}`), nil, nil
			}
			return textResult(`{"id":` + jsonStr(args.ID) + `,"status":"rejected"}`), nil, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_status", Description: "Scout health and counts."},
		func(ctx context.Context, req *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
			return textResult(statusJSON(deps.Core)), nil, nil
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

func statusJSON(core *runtime.Core) string {
	var opps, pending, apps int
	_ = core.DB.DB.QueryRow(`SELECT COUNT(*) FROM opportunities`).Scan(&opps)
	_ = core.DB.DB.QueryRow(`SELECT COUNT(*) FROM pending_actions WHERE status IN ('draft','pending_approval')`).Scan(&pending)
	_ = core.DB.DB.QueryRow(`SELECT COUNT(*) FROM applications`).Scan(&apps)
	return `{"scout":` + jsonStr(version.Version) + `,"opportunities":` + strconv.Itoa(opps) + `,"pending_approvals":` + strconv.Itoa(pending) + `,"applications":` + strconv.Itoa(apps) + `}`
}

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

var _ = approve.List
