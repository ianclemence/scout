package match

import (
	"strings"
	"testing"

	"github.com/ianclemence/scout/pkg/domain"
)

func testProfile() *domain.ProfessionalProfile {
	return &domain.ProfessionalProfile{
		Skills: []string{"go", "laravel", "react"}, Technologies: []string{"go", "postgres"},
		MinProjectBudget: 500, MinHourlyRate: 30, MaxConnectsPerApp: 16,
		ExcludedWork: []string{"wordpress"},
	}
}

func TestDeterministicFilterBudget(t *testing.T) {
	p := testProfile()
	o := &domain.Opportunity{BudgetType: "fixed", BudgetMax: 100}
	if r := DeterministicFilter(p, o); r.Pass {
		t.Fatal("expected budget gate to reject")
	}
	o2 := &domain.Opportunity{BudgetType: "fixed", BudgetMax: 2000}
	if r := DeterministicFilter(p, o2); !r.Pass {
		t.Fatal("expected pass")
	}
}

func TestDeterministicFilterConnectsAndExcluded(t *testing.T) {
	p := testProfile()
	o := &domain.Opportunity{Title: "WP site", Description: "wordpress work", ConnectsCost: 30}
	if r := DeterministicFilter(p, o); r.Pass {
		t.Fatal("expected connects/excluded rejection")
	}
}

func TestFingerprintStable(t *testing.T) {
	a := Fingerprint("upwork", "1", "Title", "Desc")
	b := Fingerprint("upwork", "1", "Title", "Desc")
	c := Fingerprint("upwork", "2", "Title", "Desc")
	if a != b || a == c {
		t.Fatal("fingerprint not stable/unique")
	}
}

func TestHeuristicEvaluateStructured(t *testing.T) {
	p := testProfile()
	o := &domain.Opportunity{Title: "Go API backend", Description: strings.Repeat("Need go postgres api backend. ", 40), BudgetType: "fixed", BudgetMax: 2000}
	ev := HeuristicEvaluate(p, o)
	if len(ev.Dimensions) < 4 {
		t.Fatal("expected dimensions, not a single score")
	}
	if ev.Recommendation == "" {
		t.Fatal("expected recommendation")
	}
}

func TestDetectRisks(t *testing.T) {
	o := &domain.Opportunity{Title: "Quick task", Description: "Contact me on telegram for unpaid test, work outside upwork."}
	risks := DetectRisks(o)
	if len(risks) < 3 {
		t.Fatalf("expected >=3 risks, got %v", risks)
	}
}
