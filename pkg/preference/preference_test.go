package preference

import (
	"strings"
	"testing"

	"github.com/ianclemence/scout/pkg/domain"
)

func TestSignalPolarity(t *testing.T) {
	for sig, want := range map[string]int{
		"good_match": 1, "bad_match": -1, "too_low_budget": -1, "unknown": 0, "": 0,
	} {
		if got := SignalPolarity(sig); got != want {
			t.Fatalf("SignalPolarity(%q) = %d, want %d", sig, got, want)
		}
	}
}

func TestTrainAndScore(t *testing.T) {
	m := Train([]Sample{
		{Terms: []string{"react", "typescript", "saas"}, Positive: true},
		{Terms: []string{"react", "typescript"}, Positive: true},
		{Terms: []string{"wordpress", "data entry"}, Positive: false},
		{Terms: []string{"wordpress"}, Positive: false},
		{Terms: []string{"wordpress"}, Positive: false},
	})
	if m.Positive != 2 || m.Negative != 3 {
		t.Fatalf("counts = %d/%d", m.Positive, m.Negative)
	}
	favored, disfavored := m.Top(5)
	if !contains(favored, "react") || !contains(favored, "typescript") {
		t.Fatalf("favored = %v", favored)
	}
	if !contains(disfavored, "wordpress") {
		t.Fatalf("disfavored = %v", disfavored)
	}
	// A react opportunity should score positive; a wordpress one negative.
	good := &domain.Opportunity{Title: "Senior React Developer", Skills: []string{"React", "TypeScript"}}
	bad := &domain.Opportunity{Title: "WordPress site build", Skills: []string{"WordPress"}}
	if m.Score(good) <= 0 {
		t.Fatalf("good score = %v", m.Score(good))
	}
	if m.Score(bad) >= 0 {
		t.Fatalf("bad score = %v", m.Score(bad))
	}
}

func TestTrainNeutralTermIsDropped(t *testing.T) {
	m := Train([]Sample{
		{Terms: []string{"react"}, Positive: true},
		{Terms: []string{"react"}, Positive: false},
	})
	if _, ok := m.Weights["react"]; ok {
		t.Fatal("a term with equal positive/negative evidence should be dropped")
	}
}

func TestContextLine(t *testing.T) {
	if (&Model{}).ContextLine() != "" {
		t.Fatal("empty model must produce no context")
	}
	m := Train([]Sample{{Terms: []string{"react"}, Positive: true}, {Terms: []string{"wordpress"}, Positive: false}})
	line := m.ContextLine()
	if !strings.Contains(line, "favored") || !strings.Contains(line, "react") {
		t.Fatalf("context line = %q", line)
	}
	if !strings.Contains(line, "disfavored") || !strings.Contains(line, "wordpress") {
		t.Fatalf("context line = %q", line)
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
