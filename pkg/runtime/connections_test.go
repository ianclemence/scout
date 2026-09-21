package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/ianclemence/scout/pkg/mcpclient"
)

// TestConnectionsListsConfiguredSources covers the terminal answer to
// "what MCP is configured": a configured source appears with its kind,
// endpoint, enabled state, auth state, and stored-token flag.
func TestConnectionsListsConfiguredSources(t *testing.T) {
	c := testCore(t)
	if err := c.AddMCPConnection("Upwork", "https://mcp.upwork.com/mcp"); err != nil {
		t.Fatal(err)
	}
	conns, err := c.Connections()
	if err != nil {
		t.Fatal(err)
	}
	if len(conns) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(conns))
	}
	got := conns[0]
	if got.Name != "Upwork" || got.Kind != "mcp" || got.Endpoint != "https://mcp.upwork.com/mcp" {
		t.Fatalf("unexpected connection: %+v", got)
	}
	if !got.Enabled {
		t.Fatal("connection should be enabled by default")
	}
	if got.HasToken || got.Auth != "unauthenticated" {
		t.Fatalf("expected unauthenticated with no token, got auth=%q hasToken=%v", got.Auth, got.HasToken)
	}

	// Storing a token must flip the auth state without a network probe.
	if err := c.StoreConnectionToken("upwork", "tok"); err != nil {
		t.Fatal(err)
	}
	conns, _ = c.Connections()
	if !conns[0].HasToken || conns[0].Auth != "token_stored" {
		t.Fatalf("expected token_stored, got %+v", conns[0])
	}
}

// TestConnectionRejectsInsecureEndpoint locks the transport policy: remote
// connectors must be https (localhost http is the only exception).
func TestConnectionRejectsInsecureEndpoint(t *testing.T) {
	c := testCore(t)
	if err := c.AddMCPConnection("Evil", "http://example.com/mcp"); err == nil {
		t.Fatal("http remote endpoint must be rejected")
	}
	if err := c.AddMCPConnection("Local", "http://localhost:8080/mcp"); err != nil {
		t.Fatalf("localhost http should be allowed: %v", err)
	}
}

// TestFindConnectionByPrefix lets users type `upwork` instead of `src-upwork`.
func TestFindConnectionByPrefix(t *testing.T) {
	c := testCore(t)
	if err := c.AddMCPConnection("Upwork", "https://mcp.upwork.com/mcp"); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"Upwork", "upwork", "src-upwork", "UPWO"} {
		conn, err := c.FindConnection(ref)
		if err != nil {
			t.Fatalf("FindConnection(%q): %v", ref, err)
		}
		if conn.Name != "Upwork" {
			t.Fatalf("FindConnection(%q) = %q", ref, conn.Name)
		}
	}
	if _, err := c.FindConnection("nonexistent"); err == nil {
		t.Fatal("unknown source should error")
	}
}

// TestSetAndRemoveConnection verifies enable/disable and removal (including
// the stored token, so a removed source cannot leave a secret behind).
func TestSetAndRemoveConnection(t *testing.T) {
	c := testCore(t)
	if err := c.AddMCPConnection("Upwork", "https://mcp.upwork.com/mcp"); err != nil {
		t.Fatal(err)
	}
	if err := c.StoreConnectionToken("Upwork", "tok"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetConnectionEnabled("Upwork", false); err != nil {
		t.Fatal(err)
	}
	conns, _ := c.Connections()
	if conns[0].Enabled {
		t.Fatal("expected disabled")
	}
	if err := c.RemoveConnection("Upwork"); err != nil {
		t.Fatal(err)
	}
	conns, _ = c.Connections()
	if len(conns) != 0 {
		t.Fatalf("expected no connections, got %d", len(conns))
	}
	if _, err := c.LoadSecret("mcp:Upwork"); err == nil {
		t.Fatal("removing a connection must delete its stored token")
	}
}

// TestProbeConnectionUnreachableIsBounded proves a dead endpoint returns an
// unavailable status within the timeout instead of hanging the turn.
func TestProbeConnectionUnreachableIsBounded(t *testing.T) {
	c := testCore(t)
	// Reserved TEST-NET-1 address; connection will fail fast or time out.
	if err := c.AddMCPConnection("Dead", "https://192.0.2.1/mcp"); err != nil {
		t.Fatal(err)
	}
	done := make(chan *Connection, 1)
	go func() {
		conn, err := c.ProbeConnection(context.Background(), "Dead", 2*time.Second)
		if err != nil {
			done <- &Connection{Status: "error:" + err.Error()}
			return
		}
		done <- conn
	}()
	select {
	case conn := <-done:
		if conn.Status == "connected" {
			t.Fatalf("dead endpoint reported connected: %+v", conn)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("ProbeConnection did not return within its bound")
	}
}

// TestDiscoverCapabilitiesMapping locks the tool-name → capability mapping
// used by the CLI, session, and agent alike.
func TestDiscoverCapabilitiesMapping(t *testing.T) {
	caps := discoverCapabilities([]mcpclient.ToolInfo{
		{Name: "search_jobs", Description: "Search job postings"},
		{Name: "submit_proposal", Description: "Submit an application"},
	})
	var hasSearch, hasSubmit bool
	for _, c := range caps {
		switch string(c) {
		case "search_opportunities":
			hasSearch = true
		case "submit_application":
			hasSubmit = true
		}
	}
	if !hasSearch || !hasSubmit {
		t.Fatalf("expected search + submit capabilities, got %v", caps)
	}
}

// TestFindSourceResolvesNameAndPrefix proves discovery accepts "upwork" (name)
// and "Up" (prefix), not only the internal "src-upwork" id — the agent and
// users naturally type the name.
func TestFindSourceResolvesNameAndPrefix(t *testing.T) {
	c := testCore(t)
	if err := c.AddMCPConnection("Upwork", "https://mcp.upwork.com/mcp"); err != nil {
		t.Fatal(err)
	}
	reg := c.SourceRegistry()
	for _, ref := range []string{"src-upwork", "Upwork", "upwork", "UPW"} {
		if _, ok := c.findSource(reg, ref); !ok {
			t.Fatalf("findSource(%q) did not resolve", ref)
		}
	}
	if _, ok := c.findSource(reg, "nope"); ok {
		t.Fatal("findSource should not resolve an unknown source")
	}
}
