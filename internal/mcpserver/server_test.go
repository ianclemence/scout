package mcpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ianclemence/scout/internal/config"
	"github.com/ianclemence/scout/internal/runtime"
	"github.com/ianclemence/scout/internal/store"
)

func testDeps(t *testing.T) Deps {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	core, err := runtime.New(cfg, db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = db.DB.Exec(`INSERT INTO opportunities(id,source,source_opp_id,title,description,status,created_at,updated_at) VALUES('o1','manual','o1','Go API','Build a Go API backend service', 'discovered',?,?)`, now, now)
	return Deps{Core: core}
}

func dial(t *testing.T, h http.Handler) *mcp.ClientSession {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
	sess, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: srv.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	return sess
}

func TestMCPStatusTool(t *testing.T) {
	sess := dial(t, Handler(testDeps(t)))
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tl := range tools.Tools {
		names[tl.Name] = true
	}
	for _, want := range []string{"get_status", "search_opportunities", "get_opportunity", "analyze_opportunity", "match_opportunity", "prepare_proposal", "list_applications", "list_pending_approvals", "approve_action", "reject_action", "get_pipeline", "get_profile", "discover_opportunities"} {
		if !names[want] {
			t.Fatalf("missing tool %s", want)
		}
	}
	res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_status", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Content) == 0 {
		t.Fatal("empty status result")
	}
}

func TestMCPMatchRoundtrip(t *testing.T) {
	deps := testDeps(t)
	sess := dial(t, Handler(deps))
	res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: "match_opportunity", Arguments: map[string]any{"id": "o1"}})
	if err != nil || len(res.Content) == 0 {
		t.Fatalf("match failed: %v", err)
	}
	res, err = sess.CallTool(context.Background(), &mcp.CallToolParams{Name: "search_opportunities", Arguments: map[string]any{"query": "Go API"}})
	if err != nil || len(res.Content) == 0 {
		t.Fatalf("search failed: %v", err)
	}
}

func TestInMemoryServerSession(t *testing.T) {
	deps := testDeps(t)
	srv := New(deps)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx := context.Background()
	sess2, err := srv.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sess2.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	sess, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "get_status", Arguments: map[string]any{}})
	if err != nil || len(res.Content) == 0 {
		t.Fatalf("in-memory call failed: %v", err)
	}
}
