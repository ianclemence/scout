package sources

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ianclemence/scout/pkg/mcpclient"
)

// fakeCaller is a scripted mcpCaller for adapter tests.
type fakeCaller struct {
	tools     []mcpclient.ToolInfo
	responses map[string]string
	lastArgs  map[string]map[string]any
}

func (f *fakeCaller) ListTools(context.Context) ([]mcpclient.ToolInfo, error) {
	return f.tools, nil
}

func (f *fakeCaller) CallTool(_ context.Context, name string, args map[string]any) (string, error) {
	if f.lastArgs == nil {
		f.lastArgs = map[string]map[string]any{}
	}
	f.lastArgs[name] = args
	return f.responses[name], nil
}

func upworkFake() *fakeCaller {
	return &fakeCaller{
		tools: []mcpclient.ToolInfo{
			{Name: "upwork__list_accounts"},
			{Name: "upwork__find_jobs"},
			{Name: "upwork__manage_proposals"},
			{Name: "upwork__confirm_preview"},
			{Name: "upwork__send_message"},
		},
		responses: map[string]string{
			"upwork__list_accounts": `{"accounts":[{"name":"Ian","org_uid":"ORG123","role":"TALENT"}]}`,
			"upwork__find_jobs": `{"jobs":[{"id":"2095","title":"Senior Go Backend Developer",
				"description":"<untrusted_participant_content>\nBuild a Go service.\n</untrusted_participant_content>",
				"budget":"30.00–50.00/hr","job_type":"hourly","skills":["Golang","MongoDB"],
				"url":"https://www.upwork.com/jobs/~022095","published_date":"2026-09-21T17:06:24.941Z",
				"experience_level":"expert","client":{"country":"Canada"}}]}`,
		},
	}
}

func TestUpworkSearchNormalizesAndResolvesOrg(t *testing.T) {
	f := upworkFake()
	u := NewUpworkAdapter("src-upwork", "Upwork", f)

	opps, err := u.Search(context.Background(), SearchFilter{Title: "Golang", JobType: "hourly", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(opps) != 1 {
		t.Fatalf("expected 1 opportunity, got %d", len(opps))
	}
	o := opps[0]
	if o.Title != "Senior Go Backend Developer" || o.Source != "src-upwork" || o.SourceOppID != "2095" {
		t.Fatalf("unexpected opportunity: %+v", o)
	}
	if o.BudgetType != "hourly" || o.BudgetMin != 30 || o.BudgetMax != 50 {
		t.Fatalf("budget parse failed: %s %.0f-%.0f", o.BudgetType, o.BudgetMin, o.BudgetMax)
	}
	if strings.Contains(o.Description, "untrusted_participant_content") {
		t.Fatalf("untrusted tags not stripped: %q", o.Description)
	}
	if o.Fingerprint == "" || o.CanonicalURL == "" {
		t.Fatalf("missing fingerprint/url: %+v", o)
	}

	// The search call must carry action + org_uid and nest filters under params.
	args := f.lastArgs["upwork__find_jobs"]
	if args["action"] != "search" || args["org_uid"] != "ORG123" {
		t.Fatalf("search args wrong: %v", args)
	}
	params, _ := args["params"].(map[string]any)
	if params["title"] != "Golang" || params["job_type"] != "hourly" {
		t.Fatalf("params wrong: %v", params)
	}
	if n, _ := params["limit"].(int); n != 5 {
		t.Fatalf("limit not forwarded: %v", params["limit"])
	}
}

func TestUpworkSubmitUsesPreviewConfirm(t *testing.T) {
	f := upworkFake()
	f.responses["upwork__manage_proposals"] = `{"preview_id":"prev-1"}`
	f.responses["upwork__confirm_preview"] = `{"status":"ok"}`
	u := NewUpworkAdapter("src-upwork", "Upwork", f)

	out, err := u.SubmitApplication(context.Background(), map[string]any{
		"job_id": "2095", "cover_letter": "Hello", "charged_amount": 45.0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ok") {
		t.Fatalf("submit output = %q", out)
	}
	create := f.lastArgs["upwork__manage_proposals"]
	if create["action"] != "create" {
		t.Fatalf("create args: %v", create)
	}
	cp, _ := create["params"].(map[string]any)
	// The live contract names the job param job_reference and requires a
	// numeric charged_amount.
	if cp["job_reference"] != "2095" || cp["cover_letter"] != "Hello" {
		t.Fatalf("create params: %v", cp)
	}
	if amt, _ := cp["charged_amount"].(float64); amt != 45 {
		t.Fatalf("charged_amount must be a number: %v", cp["charged_amount"])
	}
	confirm := f.lastArgs["upwork__confirm_preview"]
	if confirm["action"] != "confirm" {
		t.Fatalf("confirm args: %v", confirm)
	}
	cp2, _ := confirm["params"].(map[string]any)
	if cp2["type"] != "proposal" || cp2["preview_id"] != "prev-1" {
		t.Fatalf("confirm params: %v", cp2)
	}
}

// TestUpworkSubmitSurfacesPolicyGate ensures a policy-acknowledgment response
// is surfaced to the user rather than confirmed blindly.
func TestUpworkSubmitSurfacesPolicyGate(t *testing.T) {
	f := upworkFake()
	f.responses["upwork__manage_proposals"] = `Please confirm: I understand Upwork's policies. To proceed the user must acknowledge the policy.`
	u := NewUpworkAdapter("src-upwork", "Upwork", f)

	out, err := u.SubmitApplication(context.Background(), map[string]any{
		"job_reference": "2095", "cover_letter": "Hi", "charged_amount": 30.0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "acknowledge") {
		t.Fatalf("policy gate must be surfaced: %q", out)
	}
	if _, ok := f.lastArgs["upwork__confirm_preview"]; ok {
		t.Fatal("a policy-gated draft must never be confirmed")
	}
}

// TestUpworkSubmitRequiresNumber ensures a string/missing bid is refused.
func TestUpworkSubmitRequiresNumber(t *testing.T) {
	f := upworkFake()
	u := NewUpworkAdapter("src-upwork", "Upwork", f)
	for _, args := range []map[string]any{
		{"job_reference": "2095", "cover_letter": "Hi"},
		{"job_reference": "2095", "cover_letter": "Hi", "charged_amount": "45"},
	} {
		if _, err := u.SubmitApplication(context.Background(), args); err == nil {
			t.Fatalf("expected a numeric charged_amount error for %v", args)
		}
	}
}

func TestParseUpworkBudget(t *testing.T) {
	cases := []struct {
		in       string
		jobType  string
		wantType string
		min, max float64
	}{
		{"30.00–50.00/hr", "hourly", "hourly", 30, 50},
		{"11.00–11.00/hr", "hourly", "hourly", 11, 11},
		{"12,000.00", "fixed", "fixed", 12000, 12000},
		{"", "", "unknown", 0, 0},
		{"", "hourly", "hourly", 0, 0},
	}
	for _, c := range cases {
		gotType, gotMin, gotMax := parseUpworkBudget(c.in, c.jobType)
		if gotType != c.wantType || gotMin != c.min || gotMax != c.max {
			t.Fatalf("parseUpworkBudget(%q,%q) = (%s,%.0f,%.0f), want (%s,%.0f,%.0f)",
				c.in, c.jobType, gotType, gotMin, gotMax, c.wantType, c.min, c.max)
		}
	}
}

func TestUpworkGetDetail(t *testing.T) {
	f := upworkFake()
	detail := map[string]any{
		"data": map[string]any{"marketplaceJobPosting": map[string]any{
			"content": map[string]any{"title": "Go Dev", "description": "<untrusted_participant_content>Do Go work</untrusted_participant_content>"},
			"contractTerms": map[string]any{
				"contractType":        "HOURLY",
				"hourlyContractTerms": map[string]any{"hourlyBudgetMin": float64(40), "hourlyBudgetMax": float64(60)},
			},
			"classification": map[string]any{"category": map[string]any{"preferredLabel": "Web, Mobile & Software Dev"}},
			"url":            "https://www.upwork.com/jobs/~x",
		}},
	}
	b, _ := json.Marshal(detail)
	f.responses["upwork__find_jobs"] = string(b)
	u := NewUpworkAdapter("src-upwork", "Upwork", f)
	o, err := u.Get(context.Background(), "2095")
	if err != nil {
		t.Fatal(err)
	}
	if o.Title != "Go Dev" || o.BudgetType != "hourly" || o.BudgetMin != 40 || o.BudgetMax != 60 {
		t.Fatalf("detail parse failed: %+v", o)
	}
	if strings.Contains(o.Description, "untrusted") {
		t.Fatalf("detail untrusted tags not stripped: %q", o.Description)
	}
}

func TestNewAdapterForSelectsUpworkDialect(t *testing.T) {
	up := NewAdapterFor("src-upwork", "Upwork", "https://mcp.upwork.com/mcp", upworkFake())
	if _, ok := up.(*UpworkAdapter); !ok {
		t.Fatalf("upwork endpoint should use UpworkAdapter, got %T", up)
	}
	gen := NewAdapterFor("src-acme", "Acme", "https://mcp.acme.com/mcp", upworkFake())
	if _, ok := gen.(*MCPAdapter); !ok {
		t.Fatalf("other endpoints should use MCPAdapter, got %T", gen)
	}
}
