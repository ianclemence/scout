package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/ianclemence/scout/pkg/agent"
	"github.com/ianclemence/scout/pkg/llm"
)

func TestToolCatalogForScopesAndCaps(t *testing.T) {
	c := testCore(t)
	full := c.FullToolCatalog()
	scoped := c.ToolCatalogFor("find React Native developer jobs on Upwork under my rate")
	if len(scoped) >= len(full) {
		t.Fatalf("scoped catalog (%d) should be smaller than full (%d)", len(scoped), len(full))
	}
	for _, want := range []string{"load_skill", "search_opportunities", "list_tools", "analyze_opportunity", "request_approval"} {
		if !strings.Contains(scoped, "- "+want+":") {
			t.Fatalf("scoped catalog missing core tool %s:\n%s", want, scoped)
		}
	}
	// list_tools must expose the full catalog on demand.
	if !strings.Contains(full, "- parse_document:") {
		t.Fatal("full catalog should contain all tools")
	}
	// An empty request falls back to the full catalog.
	if c.ToolCatalogFor("") != full {
		t.Fatal("empty request should return the full catalog")
	}
}

func TestRunAgentRecordsTrajectory(t *testing.T) {
	c := testCore(t)
	fake := &fakeProvider{turns: []string{
		"```tool\n{\"name\": \"list_sources\", \"arguments\": {}}\n```\n",
		"Done.",
	}}
	if _, err := c.RunAgent(context.Background(), &agent.Engine{LLM: fake},
		[]llm.Message{{Role: "user", Content: "what mcp is configured"}}, "", func(Event) {}); err != nil {
		t.Fatal(err)
	}
	var req, tools string
	var turns int
	if err := c.DB.DB.QueryRow(`SELECT request,tools,turns FROM trajectories ORDER BY created_at DESC LIMIT 1`).Scan(&req, &tools, &turns); err != nil {
		t.Fatal("no trajectory recorded:", err)
	}
	if req != "what mcp is configured" {
		t.Fatalf("trajectory request = %q", req)
	}
	if !strings.Contains(tools, "list_sources") {
		t.Fatalf("trajectory tools = %q", tools)
	}
	if turns < 2 {
		t.Fatalf("trajectory turns = %d", turns)
	}
}
