//go:build liveupwork

// Live Upwork MCP capability probe. Not part of the default test run; it
// performs real read-only calls against the configured Upwork connector and
// reports which capabilities answer. Run with:
//
//	go test -tags liveupwork ./pkg/runtime/ -run TestLiveUpworkCapabilities -v
//
// It never confirms a write: proposal submission stops at the draft/preview
// step, so no Connects are spent and nothing is sent.
package runtime

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ianclemence/scout/pkg/config"
	"github.com/ianclemence/scout/pkg/mcpclient"
	"github.com/ianclemence/scout/pkg/store"
)

// jsonField pulls the first occurrence of a string field from an arbitrary
// nested JSON document, used to read org_uid out of a tool payload.
func jsonField(raw, key string) string {
	var v any
	if json.Unmarshal([]byte(raw), &v) != nil {
		return ""
	}
	var walk func(any) string
	walk = func(x any) string {
		switch t := x.(type) {
		case map[string]any:
			if s, ok := t[key].(string); ok && s != "" {
				return s
			}
			for _, v := range t {
				if s := walk(v); s != "" {
					return s
				}
			}
		case []any:
			for _, v := range t {
				if s := walk(v); s != "" {
					return s
				}
			}
		}
		return ""
	}
	return walk(v)
}

func liveCore(t *testing.T) *Core {
	t.Helper()
	cfg := config.Default()
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	c, err := New(cfg, db)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLiveUpworkCapabilities(t *testing.T) {
	c := liveCore(t)
	conns, err := c.Connections()
	if err != nil {
		t.Fatal(err)
	}
	var up *Connection
	for i := range conns {
		if conns[i].Name == "Upwork" {
			up = &conns[i]
		}
	}
	if up == nil {
		t.Skip("no Upwork connector configured")
	}

	tok := c.mcpAccessToken(up.Name)
	if tok == "" {
		t.Fatal("no Upwork access token stored; run `scout integrations login Upwork`")
	}
	mc := &mcpclient.Connector{ID: up.Name, Endpoint: up.Endpoint, Token: tok}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tools, err := mc.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	t.Logf("Upwork MCP exposes %d tools:", len(tools))
	for _, tl := range tools {
		t.Logf("  - %s", tl.Name)
	}

	// Resolve the freelancer org_uid, which most tool calls require.
	acctRaw, err := mc.CallTool(ctx, "upwork__list_accounts", map[string]any{})
	if err != nil {
		t.Fatalf("list_accounts: %v", err)
	}
	org := jsonField(acctRaw, "org_uid")
	if org == "" {
		t.Fatalf("no org_uid in account listing: %s", acctRaw)
	}
	t.Logf("freelancer org_uid = %s", org)

	// Read-only smoke calls. Names come from live discovery, so this adapts.
	probe := func(name string, args map[string]any) {
		c, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		out, err := mc.CallTool(c, name, args)
		if err != nil {
			t.Logf("[fail] %s: %v", name, err)
			return
		}
		preview := out
		if len(preview) > 300 {
			preview = preview[:300] + "…"
		}
		t.Logf("[ok]   %s -> %s", name, preview)
	}

	for _, tl := range tools {
		switch tl.Name {
		case "upwork__list_accounts":
			probe(tl.Name, map[string]any{})
		case "upwork__get_account":
			probe(tl.Name, map[string]any{"org_uid": org, "action": "get_organization"})
		case "upwork__get_freelancer_dashboard":
			probe(tl.Name, map[string]any{"org_uid": org, "action": "check"})
		case "upwork__get_profile":
			probe(tl.Name, map[string]any{"org_uid": org, "action": "get"})
		case "upwork__find_jobs":
			probe(tl.Name, map[string]any{"org_uid": org, "action": "search", "params": map[string]any{"query": "golang developer", "limit": 3}})
		case "upwork__find_saved_jobs":
			probe(tl.Name, map[string]any{"org_uid": org, "action": "list"})
		case "upwork__list_freelancer_proposals":
			probe(tl.Name, map[string]any{"org_uid": org, "action": "list"})
		case "upwork__list_contracts":
			probe(tl.Name, map[string]any{"org_uid": org, "action": "list"})
		case "upwork__get_messages":
			probe(tl.Name, map[string]any{"org_uid": org, "action": "list_rooms", "params": map[string]any{"limit": 3}})
		case "upwork__list_offers":
			probe(tl.Name, map[string]any{"org_uid": org, "action": "list"})
		case "upwork__get_freelancer_financials":
			probe(tl.Name, map[string]any{"org_uid": org, "action": "get"})
		}
	}
}

func TestLiveUpworkToolHelp(t *testing.T) {
	c := liveCore(t)
	conns, _ := c.Connections()
	var up *Connection
	for i := range conns {
		if conns[i].Name == "Upwork" {
			up = &conns[i]
		}
	}
	if up == nil {
		t.Skip("no Upwork connector")
	}
	mc := &mcpclient.Connector{ID: up.Name, Endpoint: up.Endpoint, Token: c.mcpAccessToken(up.Name)}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	for _, name := range []string{"get_freelancer_dashboard", "find_jobs", "manage_proposals", "get_messages", "get_account", "confirm_preview", "send_message"} {
		out, err := mc.CallTool(ctx, "upwork__get_tool_help", map[string]any{"tool_name": name})
		if err != nil {
			t.Logf("help %s: ERR %v", name, err)
			continue
		}
		t.Logf("help %s:\n%s\n", name, out)
	}
}

// TestLiveUpworkSubmissionFlow exercises the most important capability end to
// end: search a real job, read its detail, then create a proposal DRAFT and
// STOP. It never confirms, so no Connects are spent and nothing is sent.
func TestLiveUpworkSubmissionFlow(t *testing.T) {
	c := liveCore(t)
	conns, _ := c.Connections()
	var up *Connection
	for i := range conns {
		if conns[i].Name == "Upwork" {
			up = &conns[i]
		}
	}
	if up == nil {
		t.Skip("no Upwork connector")
	}
	mc := &mcpclient.Connector{ID: up.Name, Endpoint: up.Endpoint, Token: c.mcpAccessToken(up.Name)}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	org := jsonField(mustCall(t, ctx, mc, "upwork__list_accounts", map[string]any{}), "org_uid")

	// 1. Search a real job.
	raw := mustCall(t, ctx, mc, "upwork__find_jobs", map[string]any{
		"org_uid": org, "action": "search",
		"params": map[string]any{"query": "golang backend developer", "limit": 5},
	})
	jobID := jsonField(raw, "id")
	if jobID == "" {
		// Try the jobs array specifically.
		jobID = jsonField(raw, "uid")
	}
	t.Logf("search returned job id=%q", jobID)

	// 2. Read detail (connects cost, can_apply).
	if jobID != "" {
		detail := mustCall(t, ctx, mc, "upwork__find_jobs", map[string]any{
			"org_uid": org, "action": "get", "params": map[string]any{"id": jobID},
		})
		t.Logf("detail: %s", truncateStr(detail, 600))

		// 3. MANDATORY pre-check: invitations + existing proposals.
		inv := mustCall(t, ctx, mc, "upwork__list_freelancer_proposals", map[string]any{
			"org_uid": org, "action": "invitations",
		})
		t.Logf("invitations: %s", truncateStr(inv, 200))
		existing := mustCall(t, ctx, mc, "upwork__list_freelancer_proposals", map[string]any{
			"org_uid": org, "action": "list",
		})
		t.Logf("existing proposals: %s", truncateStr(existing, 200))

		// 4. Create the DRAFT only. Never confirm.
		preview := mustCallErr(mc, ctx, "upwork__manage_proposals", map[string]any{
			"org_uid": org, "action": "create",
			"params": map[string]any{
				"job_reference":  jobID,
				"cover_letter":   "This is a capability test draft. It will not be submitted.",
				"charged_amount": 55,
			},
		})
		t.Logf("proposal draft preview: %s", truncateStr(preview, 400))
	}
}

func mustCall(t *testing.T, ctx context.Context, mc *mcpclient.Connector, name string, args map[string]any) string {
	t.Helper()
	out, err := mc.CallTool(ctx, name, args)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return out
}

func mustCallErr(mc *mcpclient.Connector, ctx context.Context, name string, args map[string]any) string {
	out, err := mc.CallTool(ctx, name, args)
	if err != nil {
		return "ERR: " + err.Error()
	}
	return out
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
