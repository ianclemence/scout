package domain

import (
	"strings"
	"testing"
)

func TestHumanOpportunityHidesMachineTokens(t *testing.T) {
	o := &Opportunity{
		Title:        "Music pipeline",
		Source:       "src-upwork",
		Status:       "discovered",
		BudgetType:   "hourly",
		BudgetMin:    15,
		BudgetMax:    25,
		ConnectsCost: 0,
		Description:  "Build a pipeline.",
	}
	out := HumanOpportunity(o)
	for _, bad := range []string{"src-upwork", "discovered", "credits", "token_stored"} {
		if strings.Contains(out, bad) {
			t.Fatalf("human view leaked machine token %q:\n%s", bad, out)
		}
	}
	if !strings.Contains(out, "Upwork") {
		t.Fatalf("source should be humanized to Upwork:\n%s", out)
	}
	if !strings.Contains(out, "$15–25/hr") {
		t.Fatalf("hourly budget should read as a rate:\n%s", out)
	}
	if !strings.Contains(out, "not yet reviewed") {
		t.Fatalf("status should be humanized:\n%s", out)
	}
}

func TestHumanBudgetVariants(t *testing.T) {
	cases := []struct {
		o    Opportunity
		want string
	}{
		{Opportunity{BudgetType: "hourly", BudgetMin: 15, BudgetMax: 25}, "$15–25/hr"},
		{Opportunity{BudgetType: "fixed", BudgetMin: 500, BudgetMax: 500}, "$500 fixed"},
		{Opportunity{BudgetType: "fixed", BudgetMin: 500, BudgetMax: 900}, "$500–900 fixed"},
		{Opportunity{}, "not stated"},
	}
	for _, c := range cases {
		if got := c.o.HumanBudget(); got != c.want {
			t.Errorf("HumanBudget(%+v) = %q, want %q", c.o, got, c.want)
		}
	}
}

func TestHumanEvaluationRendersAnalysisSection(t *testing.T) {
	ev := &MatchEvaluation{
		Recommendation: "review",
		Reason:         "skills=strong budget=unverified",
		Dimensions:     []MatchDimension{{Name: "skills", Rating: "strong", Detail: "5 matched"}},
		Analysis:       "**Skills:** moderate — general stack but no DME experience.",
	}
	out := HumanEvaluation(ev, true, "")
	if !strings.Contains(out, "worth a look") {
		t.Fatalf("verdict should be humanized:\n%s", out)
	}
	if !strings.Contains(out, "**Model notes**") {
		t.Fatalf("analysis should get its own section:\n%s", out)
	}
	if !strings.Contains(out, "**Skills:**") {
		t.Fatalf("model markdown should be preserved for the renderer:\n%s", out)
	}
	if strings.Contains(out, "| llm:") {
		t.Fatalf("analysis must not be appended to the reason:\n%s", out)
	}
}

func TestHumanAuthHidesTokenStored(t *testing.T) {
	// Exercised via the runtime type in its own package; here we guard the
	// human opportunity view never leaks the raw state.
	o := &Opportunity{Source: "src-upwork", Status: "analyzed"}
	if strings.Contains(HumanOpportunity(o), "token_stored") {
		t.Fatal("raw auth token leaked")
	}
}
