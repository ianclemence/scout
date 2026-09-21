// Package match implements the matching engine.
// Deterministic filters first; LLM analysis only on candidates.
// Output is structured dimensions, never a single magic score.
package match

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/ianclemence/scout/internal/domain"
)

type FilterResult struct {
	Pass   bool
	Reason string
}

// DeterministicFilter applies cheap, explainable gates.
func DeterministicFilter(p *domain.ProfessionalProfile, o *domain.Opportunity) FilterResult {
	if p.MinProjectBudget > 0 && o.BudgetType == "fixed" && o.BudgetMax > 0 && o.BudgetMax < p.MinProjectBudget {
		return FilterResult{false, fmt.Sprintf("budget max %.0f below minimum %.0f", o.BudgetMax, p.MinProjectBudget)}
	}
	if p.MinHourlyRate > 0 && o.BudgetType == "hourly" && o.HourlyRateMax > 0 && o.HourlyRateMax < p.MinHourlyRate {
		return FilterResult{false, "hourly max below minimum"}
	}
	if p.MaxConnectsPerApp > 0 && o.ConnectsCost > p.MaxConnectsPerApp {
		return FilterResult{false, fmt.Sprintf("connects cost %d exceeds max %d", o.ConnectsCost, p.MaxConnectsPerApp)}
	}
	lowTitle := strings.ToLower(o.Title + " " + o.Description)
	for _, ex := range p.ExcludedWork {
		ex = strings.ToLower(strings.TrimSpace(ex))
		if ex != "" && strings.Contains(lowTitle, ex) {
			return FilterResult{false, "excluded work: " + ex}
		}
	}
	return FilterResult{true, "passed deterministic gates"}
}

// Fingerprint gives opportunities a stable identity for dedup.
func Fingerprint(source, sourceID, title, desc string) string {
	h := sha256.Sum256([]byte(source + "|" + sourceID + "|" + title + "|" + desc[:min(500, len(desc))]))
	return fmt.Sprintf("%x", h)[:32]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// HeuristicEvaluate builds a structured evaluation without an LLM.
// The agent layer may enrich this with model reasoning; this is the floor.
func HeuristicEvaluate(p *domain.ProfessionalProfile, o *domain.Opportunity) *domain.MatchEvaluation {
	skillHits := 0
	lowDesc := strings.ToLower(o.Title + " " + o.Description + " " + strings.Join(o.Skills, " "))
	profileTerms := append(append([]string{}, p.Skills...), p.Technologies...)
	var matched []string
	for _, s := range profileTerms {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" && strings.Contains(lowDesc, s) {
			skillHits++
			matched = append(matched, s)
		}
	}
	skillRating := "weak"
	if skillHits >= 3 {
		skillRating = "strong"
	} else if skillHits == 2 {
		skillRating = "good"
	} else if skillHits == 1 {
		skillRating = "moderate"
	}
	risks := DetectRisks(o)
	rec := "review"
	if skillRating == "weak" || len(risks) >= 3 {
		rec = "ignore"
	} else if skillRating == "strong" && len(risks) == 0 {
		rec = "apply"
	}
	budget := "acceptable"
	if p.MinProjectBudget > 0 && o.BudgetMax > 0 && o.BudgetMax < p.MinProjectBudget {
		budget = "unacceptable"
		rec = "ignore"
	}
	return &domain.MatchEvaluation{
		ID:            fmt.Sprintf("ev-%d", time.Now().UnixNano()),
		OpportunityID: o.ID,
		Dimensions: []domain.MatchDimension{
			{Name: "skills", Rating: skillRating, Detail: fmt.Sprintf("%d profile terms matched: %s", skillHits, strings.Join(matched, ", "))},
			{Name: "budget", Rating: budget, Detail: budgetDetail(o, p)},
			{Name: "scope_clarity", Rating: scopeRating(o), Detail: "heuristic: description length + question marks"},
			{Name: "risks", Rating: riskRating(risks), Detail: fmt.Sprintf("%d signals", len(risks))},
		},
		Risks:          risks,
		Recommendation: rec,
		Reason:         fmt.Sprintf("skills=%s risks=%d", skillRating, len(risks)),
		CreatedAt:      time.Now().UTC(),
	}
}

func budgetDetail(o *domain.Opportunity, p *domain.ProfessionalProfile) string {
	if o.BudgetType == "hourly" {
		return fmt.Sprintf("hourly %.0f-%.0f vs min %.0f", o.HourlyRateMin, o.HourlyRateMax, p.MinHourlyRate)
	}
	return fmt.Sprintf("fixed %.0f-%.0f vs min %.0f", o.BudgetMin, o.BudgetMax, p.MinProjectBudget)
}

func scopeRating(o *domain.Opportunity) string {
	l := len(o.Description)
	switch {
	case l > 1500:
		return "good"
	case l > 500:
		return "moderate"
	default:
		return "weak"
	}
}

func riskRating(r []string) string {
	if len(r) == 0 {
		return "strong"
	}
	if len(r) <= 1 {
		return "good"
	}
	if len(r) == 2 {
		return "moderate"
	}
	return "weak"
}

// DetectRisks returns evidence-based, non-accusatory signals.
func DetectRisks(o *domain.Opportunity) []string {
	var risks []string
	low := strings.ToLower(o.Description + " " + o.Title)
	add := func(sub, label string) {
		if strings.Contains(low, sub) && !strings.Contains(low, "no "+sub) {
			risks = append(risks, label)
		}
	}
	add("unpaid test", "mentions unpaid test work")
	add("free trial", "mentions free trial work")
	add("telegram", "asks to move comms to Telegram")
	add("whatsapp", "asks to move comms to WhatsApp")
	add("outside upwork", "asks to work outside the platform")
	add("pay a fee", "asks freelancer to pay a fee")
	add("giveaway", "contest/giveaway language")
	if o.BudgetMax > 0 && o.BudgetMax < 20 && o.BudgetType == "fixed" {
		risks = append(risks, "fixed budget under $20")
	}
	if len(o.Description) < 150 {
		risks = append(risks, "very short description — scope unclear")
	}
	return risks
}
