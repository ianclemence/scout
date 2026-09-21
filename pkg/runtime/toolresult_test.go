package runtime

import (
	"context"
	"testing"
)

// TestToolResultsBounded verifies the full-result store does not grow without
// bound: retention keeps at most toolResultRetention rows.
func TestToolResultsBounded(t *testing.T) {
	c := testCore(t)
	tool := c.FindTool("get_profile")
	if tool == nil {
		t.Fatal("get_profile missing")
	}
	ctx := context.Background()
	for i := 0; i < toolResultRetention+40; i++ {
		if _, err := c.Execute(ctx, tool, map[string]any{}); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := c.DB.DB.QueryRow(`SELECT COUNT(*) FROM tool_results`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n > toolResultRetention {
		t.Fatalf("tool_results grew to %d, above retention %d", n, toolResultRetention)
	}
	if n == 0 {
		t.Fatal("expected some stored results")
	}
}

// TestToolResultRecorded verifies a successful tool call stores its full result.
func TestToolResultRecorded(t *testing.T) {
	c := testCore(t)
	tool := c.FindTool("get_profile")
	if _, err := c.Execute(context.Background(), tool, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	var n int
	c.DB.DB.QueryRow(`SELECT COUNT(*) FROM tool_results WHERE tool='get_profile'`).Scan(&n)
	if n == 0 {
		t.Fatal("a successful tool call must store its full result for evaluation")
	}
}
