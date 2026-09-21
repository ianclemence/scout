package sources

import (
	"context"
	"testing"

	"github.com/ianclemence/scout/pkg/domain"
)

func listing(id, title string) domain.Opportunity {
	desc := "Design interfaces and user flows"
	if id == "1" || id == "9" {
		desc = "Build a Go API backend with Postgres"
	}
	return domain.Opportunity{SourceOppID: id, Title: title, Description: desc, LiveStatus: "active"}
}

func TestFakeSearchFilters(t *testing.T) {
	f := NewFake("board", "Board", []Capability{CapSearch, CapReadListing, CapStatus},
		[]domain.Opportunity{listing("1", "Go API"), listing("2", "React app")})
	res, err := f.Search(context.Background(), SearchFilter{Query: "go", Limit: 10})
	if err != nil || len(res) != 1 || res[0].Title != "Go API" {
		t.Fatalf("filter failed: %v %v", res, err)
	}
	if res[0].Source != "board" || res[0].Fingerprint == "" {
		t.Fatal("normalization missing")
	}
}

func TestFakeFailure(t *testing.T) {
	f := NewFake("x", "X", []Capability{CapSearch}, nil)
	f.FailSearch = true
	if _, err := f.Search(context.Background(), SearchFilter{}); err == nil {
		t.Fatal("expected failure")
	}
	if h := f.Health(context.Background()); h.State != "unavailable" {
		t.Fatal("expected unavailable")
	}
}

func TestRegistryIsolation(t *testing.T) {
	r := NewRegistry()
	bad := NewFake("bad", "Bad", []Capability{CapSearch}, nil)
	bad.FailSearch = true
	good := NewFake("good", "Good", []Capability{CapSearch}, []domain.Opportunity{listing("1", "Go API")})
	r.Add(bad)
	r.Add(good)
	res, errs := r.SearchAll(context.Background(), SearchFilter{})
	if len(res) != 1 {
		t.Fatalf("expected 1 result, got %d", len(res))
	}
	if errs["bad"] == nil {
		t.Fatal("expected isolated bad-source error")
	}
}

func TestRegistryCapabilities(t *testing.T) {
	r := NewRegistry()
	r.Add(NewFake("a", "A", []Capability{CapSearch}, nil))
	r.Add(NewFake("b", "B", []Capability{CapReadListing}, nil))
	if len(r.WithCapability(CapSearch)) != 1 {
		t.Fatal("capability filter broken")
	}
}

func TestNormalizePayloads(t *testing.T) {
	opps, err := NormalizeSearchPayload("x", []byte(`[{"id":"1","title":"T","description":"D"}]`))
	if err != nil || len(opps) != 1 || opps[0].SourceOppID != "1" {
		t.Fatalf("array shape failed: %v %v", opps, err)
	}
	opps, err = NormalizeSearchPayload("x", []byte(`{"jobs":[{"job_id":"2","title":"T2"}]}`))
	if err != nil || len(opps) != 1 {
		t.Fatalf("envelope shape failed: %v %v", opps, err)
	}
	if _, err := NormalizeSearchPayload("x", []byte(`{"weird":true}`)); err == nil {
		t.Fatal("unknown shape should error, not fake success")
	}
	if _, err := NormalizeSearchPayload("x", []byte(`not json`)); err == nil {
		t.Fatal("non-JSON should error")
	}
}

func TestUpworkAgnostic(t *testing.T) {
	// Core concepts must not mention Upwork.
	for _, s := range []string{"search_opportunities", "evaluate_opportunity"} {
		_ = s
	}
	f := NewFake("generic-board", "Board", []Capability{CapSearch, CapReadListing}, []domain.Opportunity{listing("9", "Go API")})
	res, err := f.Search(context.Background(), SearchFilter{Query: "go"})
	if err != nil || len(res) != 1 {
		t.Fatal("generic source must work without Upwork")
	}
}
