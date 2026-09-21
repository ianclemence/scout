package agent

import (
	"strings"
	"testing"

	"github.com/ianclemence/scout/internal/domain"
)

func TestFallbackProposalNoLLM(t *testing.T) {
	e := &Engine{}
	p := &domain.ProfessionalProfile{DisplayName: "Jane", Title: "Go dev", Skills: []string{"go", "postgres"}}
	o := &domain.Opportunity{Title: "API job", Description: "Build a Go API"}
	cover, used, qs, err := e.DraftProposal(p, o, []domain.Evidence{{ID: "e1"}}, "concise")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cover, "API job") {
		t.Fatal("proposal should reference the job")
	}
	if strings.Contains(strings.ToLower(cover), "experienced developer") && strings.Contains(cover, "love to work") {
		t.Fatal("proposal must not be generic")
	}
	if len(used) != 1 || len(qs) != 2 {
		t.Fatal("expected evidence refs + questions")
	}
}

func TestEnrichNilLLMReturnsBase(t *testing.T) {
	e := &Engine{}
	base := &domain.MatchEvaluation{Recommendation: "review"}
	if out := e.EnrichEvaluation(&domain.ProfessionalProfile{}, &domain.Opportunity{}, base, nil); out != base {
		t.Fatal("expected base returned")
	}
}

func TestSystemPromptMarksUntrusted(t *testing.T) {
	if !strings.Contains(SystemPrompt, "UNTRUSTED") {
		t.Fatal("system prompt must treat marketplace content as untrusted")
	}
}
