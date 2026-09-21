package upwork

import (
	"encoding/json"
	"testing"

	"github.com/ianclemence/scout/internal/domain"
	"github.com/ianclemence/scout/internal/mcpclient"
)

func TestDiscoverCapabilities(t *testing.T) {
	tools := []mcpclient.ToolInfo{
		{Name: "search_jobs", Description: "search jobs by skill"},
		{Name: "get_job_details", Description: "read a job posting"},
		{Name: "submit_proposal", Description: "submit a proposal draft"},
		{Name: "mystery_tool", Description: "does something unrelated"},
	}
	caps := Discover(tools)
	has := map[domain.Capability]bool{}
	for _, c := range caps {
		has[c] = true
	}
	if !has[domain.CapSearchOpportunities] || !has[domain.CapReadOpportunity] {
		t.Fatalf("missing expected caps: %v", caps)
	}
}

func TestDiscoverEmpty(t *testing.T) {
	if caps := Discover(nil); len(caps) != 0 {
		t.Fatal("expected no caps")
	}
}

func TestNormalizeJob(t *testing.T) {
	raw := json.RawMessage(`{"id":"abc123","title":"Go API","description":"Build it","budget":1500}`)
	o, err := NormalizeJob(raw)
	if err != nil {
		t.Fatal(err)
	}
	if o.Source != "upwork" || o.SourceOppID != "abc123" || o.Title != "Go API" {
		t.Fatalf("bad normalization: %+v", o)
	}
	if o.CanonicalURL == "" || o.Status != "discovered" {
		t.Fatal("missing url/status")
	}
}
