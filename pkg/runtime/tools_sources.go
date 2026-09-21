package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ianclemence/scout/pkg/sources"
)

func strList(args map[string]any, k string) []string {
	switch v := args[k].(type) {
	case []any:
		var out []string
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		if v != "" {
			return strings.Split(v, ",")
		}
	}
	return nil
}

func sourceTools(c *Core) []*Tool {
	str := func(args map[string]any, k string) string {
		if v, ok := args[k].(string); ok {
			return v
		}
		return ""
	}
	num := func(args map[string]any, k string, d int) int {
		if v, ok := args[k].(float64); ok {
			return int(v)
		}
		return d
	}
	return []*Tool{
		{Name: "discover_opportunities", Permission: PermRead, ReadOnly: true,
			Description: "Search one or all connected sources. Returns normalized opportunities; per-source failures are isolated, never fatal.",
			ArgsHint:    `{"query": "go api", "skills": ["go"], "sources": ["upwork"], "limit": 20}`,
			ArgsSchema:  map[string]string{"query": "string", "skills": "string[]", "sources": "string[]", "location": "string", "remote": "string", "min_budget": "number", "limit": "number"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				reg := c.SourceRegistry()
				f := sources.SearchFilter{
					Query: str(args, "query"), Skills: strList(args, "skills"),
					Location: str(args, "location"), Remote: str(args, "remote"),
					MinBudget: numf(args, "min_budget"), Limit: num(args, "limit", 20),
				}
				var out []map[string]any
				var warnings []string
				targets := reg.WithCapability(sources.CapSearch)
				if only, ok := args["sources"].([]any); ok && len(only) > 0 {
					targets = nil
					for _, o := range only {
						if id, ok := o.(string); ok {
							if s, found := reg.Get(id); found {
								targets = append(targets, s)
							} else {
								warnings = append(warnings, "unknown source "+id)
							}
						}
					}
				}
				seen := map[string]bool{}
				for _, s := range targets {
					res, err := s.Search(ctx, f)
					if err != nil {
						warnings = append(warnings, s.ID()+": "+err.Error())
						continue
					}
					for _, o := range res {
						if seen[o.Fingerprint] {
							continue
						}
						seen[o.Fingerprint] = true
						out = append(out, map[string]any{
							"source": o.Source, "source_id": o.SourceOppID, "title": o.Title,
							"company": o.Company, "budget_type": o.BudgetType,
							"budget_min": o.BudgetMin, "budget_max": o.BudgetMax,
							"remote": o.RemoteStatus, "location": o.Location,
							"skills": o.Skills, "url": o.CanonicalURL,
						})
					}
				}
				b, _ := json.Marshal(ToolResult{Success: true, Data: out, Warnings: warnings,
					Meta: map[string]string{"deduplicated": "true"}})
				return string(b), nil
			}},
		{Name: "get_source_capabilities", Permission: PermRead, ReadOnly: true,
			Description: "What a connected source actually supports (never assume).",
			ArgsHint:    `{"source": "upwork"}`,
			ArgsSchema:  map[string]string{"source": "string"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				s, ok := c.SourceRegistry().Get(str(args, "source"))
				if !ok {
					return "", fmt.Errorf("unknown source %q", str(args, "source"))
				}
				return okResult(map[string]any{"source": s.ID(), "capabilities": s.Capabilities()}), nil
			}},
		{Name: "source_health", Permission: PermRead, ReadOnly: true,
			Description: "Connectivity/auth state of one or all sources.",
			ArgsHint:    `{"source": "upwork"}`,
			ArgsSchema:  map[string]string{"source": "string"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				reg := c.SourceRegistry()
				if id := str(args, "source"); id != "" {
					s, ok := reg.Get(id)
					if !ok {
						return "", fmt.Errorf("unknown source %q", id)
					}
					return okResult(map[string]any{"source": id, "health": s.Health(ctx)}), nil
				}
				out := map[string]any{}
				for _, s := range reg.All() {
					out[s.ID()] = s.Health(ctx)
				}
				return okResult(out), nil
			}},
		{Name: "check_opportunity_status", Permission: PermRead, ReadOnly: true,
			Description: "Whether an opportunity is still actionable (active/closed/expired/unknown).",
			ArgsHint:    `{"source": "upwork", "source_id": "..."}`,
			ArgsSchema:  map[string]string{"source": "string", "source_id": "string"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				s, ok := c.SourceRegistry().Get(str(args, "source"))
				if !ok {
					return "", fmt.Errorf("unknown source %q", str(args, "source"))
				}
				if !s.Has(sources.CapStatus) && !s.Has(sources.CapReadListing) {
					return okResult(map[string]any{"status": "unknown", "reason": "source does not expose status"}), nil
				}
				st, err := s.Status(ctx, str(args, "source_id"))
				if err != nil {
					return okResult(map[string]any{"status": "unknown", "reason": err.Error()}), nil
				}
				return okResult(map[string]any{"status": st}), nil
			}},
		{Name: "list_sources", Permission: PermRead, ReadOnly: true,
			Description: "Configured opportunity sources and their status.",
			ArgsHint:    `{}`,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				srcs, err := c.ListSources()
				if err != nil {
					return "", err
				}
				return okResult(srcs), nil
			}},
	}
}

func numf(args map[string]any, k string) float64 {
	if v, ok := args[k].(float64); ok {
		return v
	}
	return 0
}
