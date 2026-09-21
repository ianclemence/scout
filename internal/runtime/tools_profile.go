package runtime

import (
	"context"
	"fmt"
	"strings"
)

func profileTools(c *Core) []*Tool {
	str := func(args map[string]any, k string) string {
		if v, ok := args[k].(string); ok {
			return v
		}
		return ""
	}
	return []*Tool{
		{Name: "get_user_skills", Permission: PermRead, ReadOnly: true,
			Description: "Skills, technologies, and proficiency signals from the profile.",
			ArgsHint:    `{}`,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				p, err := c.Profile()
				if err != nil {
					return "", err
				}
				return okResult(map[string]any{"skills": p.Skills, "technologies": p.Technologies}), nil
			}},
		{Name: "get_user_experience", Permission: PermRead, ReadOnly: true,
			Description: "Professional history: employment, projects, education.",
			ArgsHint:    `{}`,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				p, err := c.Profile()
				if err != nil {
					return "", err
				}
				return okResult(map[string]any{"experience": p.Experience, "education": p.Education}), nil
			}},
		{Name: "get_user_cv", Permission: PermRead, ReadOnly: true,
			Description: "Current CV/resume content (evidence text).",
			ArgsHint:    `{}`,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				ev, err := c.Evidence(20)
				if err != nil {
					return "", err
				}
				var out []map[string]string
				for _, e := range ev {
					if e.Kind == "cv_section" {
						out = append(out, map[string]string{"reference": e.Reference, "content": truncate(e.Content, 4000)})
					}
				}
				return okResult(out), nil
			}},
		{Name: "get_user_portfolio", Permission: PermRead, ReadOnly: true,
			Description: "Portfolio projects with technologies and URLs.",
			ArgsHint:    `{}`,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				p, err := c.Profile()
				if err != nil {
					return "", err
				}
				return okResult(map[string]any{"portfolio_urls": p.PortfolioURLs, "github": p.GitHubURL, "website": p.WebsiteURL}), nil
			}},
		{Name: "search_user_evidence", Permission: PermRead, ReadOnly: true,
			Description: "Find supporting evidence across profile, CV, and portfolio for given keywords.",
			ArgsHint:    `{"keywords": ["laravel", "api"]}`,
			ArgsSchema:  map[string]string{"keywords": "string[]"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				ev, err := c.Evidence(50)
				if err != nil {
					return "", err
				}
				kws := strList(args, "keywords")
				var out []map[string]string
				for _, e := range ev {
					low := strings.ToLower(e.Content + " " + e.Reference)
					hits := 0
					for _, k := range kws {
						if k != "" && strings.Contains(low, strings.ToLower(k)) {
							hits++
						}
					}
					if hits > 0 || len(kws) == 0 {
						out = append(out, map[string]string{"id": e.ID, "kind": e.Kind, "reference": e.Reference, "excerpt": truncate(e.Content, 400)})
						if len(out) >= 10 {
							break
						}
					}
				}
				return okResult(out), nil
			}},
		{Name: "get_portfolio_evidence", Permission: PermAnalyze, ReadOnly: true,
			Description: "Given an opportunity, map requirements to credible portfolio evidence (supported/partial/unsupported).",
			ArgsHint:    `{"opportunity_id": "..."}`,
			ArgsSchema:  map[string]string{"opportunity_id": "string"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				o, err := c.GetOpportunity(str(args, "opportunity_id"))
				if err != nil {
					return "", err
				}
				ev, _ := c.Evidence(50)
				terms := append(append([]string{}, o.Skills...), o.Technologies...)
				type m struct {
					Requirement string `json:"requirement"`
					Status      string `json:"status"`
					Evidence    string `json:"evidence,omitempty"`
				}
				var out []m
				for _, t := range terms {
					t = strings.TrimSpace(t)
					if t == "" {
						continue
					}
					best, hits := "", 0
					for _, e := range ev {
						if strings.Contains(strings.ToLower(e.Content+" "+e.Reference), strings.ToLower(t)) {
							hits++
							if best == "" {
								best = e.Kind + ":" + e.Reference
							}
						}
					}
					status := "unsupported"
					if hits >= 2 {
						status = "supported"
					} else if hits == 1 {
						status = "partially_supported"
					}
					out = append(out, m{t, status, best})
				}
				return okResult(out), nil
			}},
		{Name: "get_user_preferences", Permission: PermRead, ReadOnly: true,
			Description: "Compensation floors, engagement preferences, exclusions, availability, constraints.",
			ArgsHint:    `{}`,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				p, err := c.Profile()
				if err != nil {
					return "", err
				}
				return okResult(map[string]any{
					"min_hourly": p.MinHourlyRate, "min_budget": p.MinProjectBudget,
					"max_connects": p.MaxConnectsPerApp, "max_apps_day": p.MaxAppsPerDay,
					"preferred_job_types": p.PreferredJobTypes, "excluded_work": p.ExcludedWork,
					"preferred_countries": p.PreferredCountries, "availability": p.Availability,
					"proposal_style": p.ProposalStyle,
				}), nil
			}},
		{Name: "update_user_profile", Permission: PermMutateLocal, ReadOnly: false,
			Description: "Update profile fields ONLY with user-authorized values. Never silently rewrite.",
			ArgsHint:    `{"display_name": "...", "skills": ["..."]}`,
			ArgsSchema:  map[string]string{"display_name": "string", "title": "string", "skills": "string[]", "min_budget": "number", "min_hourly": "number"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				if _, ok := args["confirmed"]; !ok {
					return "", fmt.Errorf("refusing: pass confirmed:true — profile updates require explicit user authorization")
				}
				p, err := c.Profile()
				if err != nil {
					return "", err
				}
				if v := str(args, "display_name"); v != "" {
					p.DisplayName = v
				}
				if v := str(args, "title"); v != "" {
					p.Title = v
				}
				if sk := strList(args, "skills"); len(sk) > 0 {
					p.Skills = sk
					p.Technologies = sk
				}
				if v, ok := args["min_budget"].(float64); ok {
					p.MinProjectBudget = v
				}
				if v, ok := args["min_hourly"].(float64); ok {
					p.MinHourlyRate = v
				}
				if err := saveProfile(c, p); err != nil {
					return "", err
				}
				return okResult(map[string]any{"ok": true}), nil
			}},
	}
}
