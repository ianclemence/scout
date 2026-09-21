package eval

import (
	"strings"

	"github.com/ianclemence/scout/pkg/domain"
	"github.com/ianclemence/scout/pkg/preference"
)

// DefaultCases builds the decision suite. It adapts to the user's real profile,
// so the same suite runs hermetically in tests and against a live install.
func DefaultCases(p *domain.ProfessionalProfile) []Case {
	if p == nil || len(p.Skills) < 2 {
		p = fixtureProfile()
	}
	// Work on a copy and ensure the suite exercises the budget floor even when
	// the real profile has no floor configured.
	cp := *p
	p = &cp
	if p.MinProjectBudget <= 0 {
		p.MinProjectBudget = 500
	}
	if p.MinHourlyRate <= 0 {
		p.MinHourlyRate = 30
	}
	skills := make([]string, len(p.Skills))
	copy(skills, p.Skills)
	// Prefer distinctive skills (2-letter tokens like "ai" are ambiguous).
	var usable []string
	for _, s := range skills {
		if len([]rune(s)) >= 3 {
			usable = append(usable, s)
		}
	}
	if len(usable) < 3 {
		usable = skills
	}
	strong := take(usable, 5)
	primary := "developer"
	if len(strong) > 0 {
		primary = strong[0]
	}
	floor := p.MinProjectBudget
	if floor <= 0 {
		floor = 500
	}
	pass, fail := true, false

	cases := []Case{
		{
			Name:    "strong match",
			Profile: p,
			Opportunity: domain.Opportunity{
				Title:         "Senior " + primary + " Engineer",
				Description:   "We are hiring a senior engineer for a long-term product role with a clearly defined scope, existing documentation, and a well-specified first milestone. The stack is " + strings.Join(strong, ", ") + ". You will own delivery end to end and work with a small team.",
				Skills:        strong,
				BudgetType:    "hourly",
				BudgetMin:     50,
				BudgetMax:     90,
				HourlyRateMin: 50,
				HourlyRateMax: 90,
			},
			WantFilterPass: &pass,
			WantRecommend:  "apply",
			WantSkills:     "strong",
		},
		{
			Name:    "weak match",
			Profile: p,
			Opportunity: domain.Opportunity{
				Title:       "SAP ABAP developer for warehouse operations",
				Description: "We need SAP ABAP, warehouse operations, and cold calling support. Long description so scope is not the issue here at all whatsoever.",
				Skills:      []string{"SAP ABAP", "Warehouse Operations", "Cold Calling"},
				BudgetType:  "hourly", HourlyRateMin: 40, HourlyRateMax: 60,
			},
			WantFilterPass: &pass,
			WantRecommend:  "ignore",
			WantSkills:     "weak",
		},
		{
			Name:    "partial match",
			Profile: p,
			Opportunity: domain.Opportunity{
				Title:       "Engineer with " + strings.Join(take(usable, 2), " and "),
				Description: "We need help with " + strings.Join(take(usable, 2), " and ") + ". This posting is long enough to describe a clear scope, a defined first milestone, existing documentation, and concrete deliverables you can own from start to finish.",
				Skills:      take(usable, 2),
				BudgetType:  "hourly", HourlyRateMin: 45, HourlyRateMax: 70,
			},
			WantFilterPass: &pass,
			WantRecommend:  "review",
			WantSkills:     "good",
		},
		{
			Name:    "missing information",
			Profile: p,
			Opportunity: domain.Opportunity{
				Title:       primary + " needed",
				Description: "Need " + primary + ".",
				Skills:      []string{primary},
			},
			WantFilterPass: &pass,
			WantRisk:       []string{"very short description"},
		},
		{
			Name:    "suspicious opportunity",
			Profile: p,
			Opportunity: domain.Opportunity{
				Title:       "Quick " + primary + " task — unpaid test first",
				Description: "There is an unpaid test before hiring. Please move comms to Telegram and we will proceed. Long enough description to avoid the short-text risk signal entirely.",
				Skills:      []string{primary},
			},
			WantFilterPass: &pass,
			WantRisk:       []string{"unpaid test", "Telegram"},
		},
		{
			Name:    "keyword trap",
			Profile: p,
			Opportunity: domain.Opportunity{
				Title:       "Google Ads and Google Analytics specialist",
				Description: "Manage Google Ads and Google Analytics campaigns. This is a marketing role, unrelated to the user's engineering skills, with a long enough description to avoid short-text signals.",
				Skills:      []string{"Google Ads", "Google Analytics"},
				BudgetType:  "hourly", HourlyRateMin: 20, HourlyRateMax: 40,
			},
			WantFilterPass: &pass,
			WantSkills:     "weak",
		},
		{
			Name:    "budget below floor",
			Profile: p,
			Opportunity: domain.Opportunity{
				Title:       "Small " + primary + " fix",
				Description: "A small fix that should be quick. Description is long enough for scope to be clear and deliverables to be defined for milestone one.",
				Skills:      strong,
				BudgetType:  "fixed",
				BudgetMax:   floor - 1,
			},
			WantFilterPass: &fail,
		},
		{
			Name:    "learned preference downgrades",
			Profile: p,
			Opportunity: domain.Opportunity{
				Title:       "Senior " + primary + " Engineer (WordPress migration)",
				Description: "Long-term role with a clearly defined scope, existing documentation, and a well-specified first milestone. Stack: " + strings.Join(strong, ", ") + ", plus a WordPress migration. You will own delivery end to end.",
				Skills:      append(append([]string{}, strong...), "WordPress"),
				BudgetType:  "hourly", HourlyRateMin: 50, HourlyRateMax: 90,
			},
			Feedback: []preference.Sample{
				{Terms: []string{"wordpress"}, Positive: false},
				{Terms: []string{"wordpress"}, Positive: false},
			},
			WantFilterPass: &pass,
			WantRecommend:  "review",
		},
	}
	return cases
}

// TrajectoryCases scripts agent turns to assert process behavior.
func TrajectoryCases() []TrajectoryCase {
	return []TrajectoryCase{
		{
			Name: "discover then answer",
			Turns: []string{
				"```tool\n{\"name\": \"list_sources\", \"arguments\": {}}\n```",
				"I checked the configured sources.",
			},
			WantTools:    []string{"list_sources"},
			MaxToolCalls: 2,
			WantFinal:    "sources",
		},
		{
			Name: "runaway is bounded",
			Turns: []string{
				"```tool\n{\"name\": \"get_profile\", \"arguments\": {}}\n```",
			},
			MaxToolCalls: 3,
		},
		{
			Name: "unknown tool recovers",
			Turns: []string{
				"```tool\n{\"name\": \"does_not_exist\", \"arguments\": {}}\n```",
				"Recovered and answered without the tool.",
			},
			WantFinal: "Recovered",
		},
	}
}

func fixtureProfile() *domain.ProfessionalProfile {
	return &domain.ProfessionalProfile{
		DisplayName: "Test User",
		Skills:      []string{"go", "typescript", "react", "postgresql", "docker"},
	}
}

func take(xs []string, n int) []string {
	if len(xs) <= n {
		return xs
	}
	return xs[:n]
}
