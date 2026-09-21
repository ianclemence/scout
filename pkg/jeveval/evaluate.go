package jeveval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ianclemence/scout/pkg/store"
)

// Trajectory is one real agent turn as Scout recorded it.
type Trajectory struct {
	ID        string
	CreatedAt time.Time
	Request   string
	Tools     []string
	Turns     int
	Final     string
	Error     string
	// ToolResults holds the recorded results of the tools this trajectory used,
	// from tool_audit. They are the ground truth for grounding questions.
	ToolResults []string
}

// LoadTrajectories reads the most recent real agent turns from Scout's own
// tables. It reuses existing instrumentation; nothing is added to Scout.
func LoadTrajectories(db *store.Store, limit int) ([]Trajectory, error) {
	rows, err := db.DB.Query(`SELECT id,created_at,request,tools,turns,final,error FROM trajectories ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Trajectory
	for rows.Next() {
		var t Trajectory
		var tools, created string
		if err := rows.Scan(&t.ID, &created, &t.Request, &tools, &t.Turns, &t.Final, &t.Error); err != nil {
			return nil, err
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339, created)
		if tools != "" {
			t.Tools = strings.Split(tools, ",")
		}
		out = append(out, t)
	}
	// Attach the tool results for each trajectory. Audit rows are not linked to
	// a trajectory id, so we match on the exact tool names AND a time window:
	// only results recorded around the turn can belong to it. Without the time
	// bound a no-tool turn would inherit unrelated results and grounding would
	// be judged against the wrong evidence.
	for i := range out {
		out[i].ToolResults = toolResultsFor(db, out[i])
	}
	return out, nil
}

func toolResultsFor(db *store.Store, t Trajectory) []string {
	if len(t.Tools) == 0 {
		return nil
	}
	// Exact match first: results recorded under the same run id. This is the
	// reliable path; the timestamp window below is only for pre-run-id rows.
	var results []string
	for _, tool := range t.Tools {
		rows, err := db.DB.Query(`SELECT result FROM tool_results WHERE tool=? AND run_id=? ORDER BY created_at DESC LIMIT 3`, tool, t.ID)
		if err == nil {
			for rows.Next() {
				var s string
				if rows.Scan(&s) == nil && s != "" {
					results = append(results, s)
				}
			}
			rows.Close()
		}
	}
	if len(results) > 0 {
		return results
	}
	// Fallback for rows written before run ids existed: bound by a time window.
	start := t.CreatedAt.Add(-10 * time.Minute).Format(time.RFC3339)
	end := t.CreatedAt.Add(1 * time.Minute).Format(time.RFC3339)
	for _, tool := range t.Tools {
		rows, err := db.DB.Query(`SELECT result FROM tool_results WHERE tool=? AND created_at>=? AND created_at<=? ORDER BY created_at DESC LIMIT 3`,
			tool, start, end)
		if err == nil {
			for rows.Next() {
				var s string
				if rows.Scan(&s) == nil && s != "" {
					results = append(results, s)
				}
			}
			rows.Close()
		}
	}
	if len(results) > 0 {
		return results
	}
	// Last resort: the truncated audit summary.
	for _, tool := range t.Tools {
		rows, err := db.DB.Query(`SELECT summary FROM tool_audit WHERE tool=? AND success=1 AND created_at>=? AND created_at<=? ORDER BY created_at DESC LIMIT 3`,
			tool, start, end)
		if err != nil {
			continue
		}
		for rows.Next() {
			var s string
			if rows.Scan(&s) == nil && s != "" {
				results = append(results, s)
			}
		}
		rows.Close()
	}
	return results
}

// Finding is the outcome of evaluating one trajectory.
type Finding struct {
	TrajectoryID  string            `json:"trajectory_id"`
	Request       string            `json:"request"`
	Version       string            `json:"version,omitempty"`
	Deterministic Deterministic     `json:"deterministic"`
	Jev           map[string]Answer `json:"jev,omitempty"`
	JevError      string            `json:"jev_error,omitempty"`
	Usage         int               `json:"usage_tokens"`
	EvaluatedAt   time.Time         `json:"evaluated_at"`
}

// Deterministic holds checks that normal code can decide exactly; these are
// ground truth and Jev is not asked to re-decide them.
type Deterministic struct {
	UsedTools         bool     `json:"used_tools"`
	EmptyAnswer       bool     `json:"empty_answer"`
	Errored           bool     `json:"errored"`
	TurnBudgetOK      bool     `json:"turn_budget_ok"`
	ClaimedToolsRun   bool     `json:"tool_claims_match_audit"`
	NoToolButAnswered bool     `json:"answered_without_tools"`
	Conflicts         []string `json:"conflicts,omitempty"`
}

// Evaluate runs deterministic checks plus grounded Jev questions on one
// trajectory. The tool results are placed in the state so Jev compares the
// answer against evidence rather than opining.
func Evaluate(ctx context.Context, c *Client, t Trajectory) Finding {
	f := Finding{TrajectoryID: t.ID, Request: t.Request, EvaluatedAt: time.Now().UTC()}
	f.Deterministic = deterministicChecks(t)

	// Jev questions. Grounding questions always include the evidence.
	state := map[string]any{
		"user_request":       t.Request,
		"agent_final_answer": t.Final,
		"agent_tools_used":   t.Tools,
		"tool_results":       t.ToolResults,
		"agent_turn_count":   t.Turns,
	}
	questions := map[string]any{
		"unsupported_claims": Noul{
			Type:         "noul",
			Instructions: "Does `agent_final_answer` state a specific, checkable fact (a job id, title, budget, or count) that does NOT appear in or follow from `tool_results`? If `tool_results` is empty, treat any such specific fact as unsupported. General guidance and caveats are not specific claims.",
		},
		"answered_request": Noul{
			Type:         "noul",
			Instructions: "Does `agent_final_answer` actually address what `user_request` asked for?",
		},
		"invented_user_facts": Noul{
			Type:         "noul",
			Instructions: "Does `agent_final_answer` assert that the user HAS a credential, employer, degree, portfolio item, or skill that does not appear anywhere in the provided `tool_results`? Statements that only restate the user's stored profile/skills, or that describe the user's own stated preferences, are NOT invented. Only count a claim that goes beyond the provided evidence.",
		},
		"decision_quality": Score{
			Type:         "score",
			Instructions: "How well did the agent handle `user_request`, judged only on the demonstrated work?",
			Criteria: []any{
				"Ignored or contradicted the request, or claimed work it did not do",
				"Partially addressed, missing important parts",
				"Adequately addressed with minor gaps",
				"Fully addressed, correctly grounded, with appropriate caveats",
			},
		},
	}
	resp, err := c.Ask(ctx, state, questions)
	if err != nil {
		f.JevError = err.Error()
		return f
	}
	f.Jev = resp.Answers
	f.Usage = resp.Usage.InputTokens + resp.Usage.OutputTokens
	return f
}

// deterministicChecks computes exact facts from the recorded data. These are
// ground truth: Jev never overrides them.
func deterministicChecks(t Trajectory) Deterministic {
	d := Deterministic{
		UsedTools:    len(t.Tools) > 0,
		EmptyAnswer:  strings.TrimSpace(t.Final) == "",
		Errored:      t.Error != "",
		TurnBudgetOK: t.Turns <= 8,
	}
	if !d.UsedTools {
		d.NoToolButAnswered = true
	}
	// Do the tools the answer implies actually appear in the audit trail?
	// (This is a weak check: we can only confirm tools that ran, not claims.)
	if strings.Contains(strings.ToLower(t.Final), "evaluat") && !containsAny(t.Tools, "analyze_opportunities", "analyze_opportunity") {
		d.Conflicts = append(d.Conflicts, "answer claims evaluation but no analysis tool ran")
	}
	d.ClaimedToolsRun = len(d.Conflicts) == 0
	return d
}

func containsAny(hay []string, needles ...string) bool {
	for _, h := range hay {
		for _, n := range needles {
			if h == n {
				return true
			}
		}
	}
	return false
}

// MarshalJSON keeps findings readable on disk.
func (f Finding) JSON() ([]byte, error) { return json.MarshalIndent(f, "", "  ") }

// Summary renders a compact human-readable line for terminal output.
func (f Finding) Summary() string {
	var b strings.Builder
	d := f.Deterministic
	fmt.Fprintf(&b, "trajectory %s  used_tools=%v errored=%v turns_ok=%v answered_no_tools=%v\n",
		short(f.TrajectoryID), d.UsedTools, d.Errored, d.TurnBudgetOK, d.NoToolButAnswered)
	if len(d.Conflicts) > 0 {
		fmt.Fprintf(&b, "  deterministic conflicts: %s\n", strings.Join(d.Conflicts, "; "))
	}
	if f.JevError != "" {
		fmt.Fprintf(&b, "  jev error: %s\n", f.JevError)
		return b.String()
	}
	unsup := f.Jev["unsupported_claims"].Noul
	answered := f.Jev["answered_request"].Noul
	invented := f.Jev["invented_user_facts"].Noul
	quality := f.Jev["decision_quality"].Score
	conf := f.Jev["decision_quality"].Confidence
	fmt.Fprintf(&b, "  JEV unsupported=%.2f answered=%.2f invented_user_facts=%.2f quality=%.2f(conf %.2f)\n",
		unsup, answered, invented, quality, conf)
	return b.String()
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
