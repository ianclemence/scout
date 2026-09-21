package jeveval

import (
	"strings"
	"testing"
	"time"

	"github.com/ianclemence/scout/pkg/store"
)

// TestLoadTrajectoriesSkipsNoiseAndTagsResults verifies the evaluator reads real
// turns and attaches only results that belong to that run (by run id).
func TestLoadTrajectoriesSkipsNoiseAndTagsResults(t *testing.T) {
	db, err := store.Open(t.TempDir() + "/e.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// A real run with a full tool result tagged by run id.
	if _, err := db.DB.Exec(`INSERT INTO trajectories(id,created_at,request,tools,turns,final,error) VALUES(?,?,?,?,?,?,?)`,
		"run-1", time.Now().UTC().Format(time.RFC3339), "How many opportunities are stored?", "analyze_opportunities", 2, "There are 50 stored opportunities.", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.Exec(`INSERT INTO tool_results(id,created_at,tool,result,run_id) VALUES(?,?,?,?,?)`,
		"tr-1", time.Now().UTC().Format(time.RFC3339), "analyze_opportunities", `{"evaluated":50}`, "run-1"); err != nil {
		t.Fatal(err)
	}
	// A result for a DIFFERENT run must not be attached.
	db.DB.Exec(`INSERT INTO tool_results(id,created_at,tool,result,run_id) VALUES(?,?,?,?,?)`,
		"tr-2", time.Now().UTC().Format(time.RFC3339), "analyze_opportunities", `{"evaluated":999}`, "run-other")

	trajs, err := LoadTrajectories(db, 10)
	if err != nil {
		t.Fatal(err)
	}
	var found *Trajectory
	for i := range trajs {
		if trajs[i].ID == "run-1" {
			found = &trajs[i]
		}
	}
	if found == nil {
		t.Fatal("run-1 not loaded")
	}
	if len(found.ToolResults) != 1 {
		t.Fatalf("expected exactly the run's own result, got %d", len(found.ToolResults))
	}
	if !strings.Contains(found.ToolResults[0], "50") || strings.Contains(found.ToolResults[0], "999") {
		t.Fatalf("wrong result attached: %q", found.ToolResults[0])
	}
}

// TestDeterministicChecksAreExact ensures the code-decided facts are correct.
func TestDeterministicChecksAreExact(t *testing.T) {
	d := deterministicChecks(Trajectory{Tools: []string{"search_opportunities"}, Turns: 2, Final: "done"})
	if !d.UsedTools || d.EmptyAnswer || d.Errored || !d.TurnBudgetOK {
		t.Fatalf("unexpected deterministic result: %+v", d)
	}
	// Over budget.
	if d2 := deterministicChecks(Trajectory{Turns: 9, Final: "x"}); d2.TurnBudgetOK {
		t.Fatal("turn budget over 8 must fail")
	}
	// Claims evaluation but no analysis tool ran.
	d3 := deterministicChecks(Trajectory{Tools: []string{"get_profile"}, Turns: 1, Final: "I evaluated all opportunities"})
	if len(d3.Conflicts) == 0 {
		t.Fatal("expected a claim/audit conflict")
	}
}
