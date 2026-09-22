package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
			Description: "Search one or all connected sources. Returns normalized opportunities with posted date; per-source failures are isolated, never fatal.",
			ArgsHint:    `{"query": "go api", "title": "Golang", "skills": ["go"], "job_type": "hourly", "rate_min": 30, "sources": ["upwork"], "limit": 20}`,
			ArgsSchema:  map[string]string{"query": "string", "title": "string", "skills": "string[]", "sources": "string[]", "location": "string", "remote": "string", "job_type": "string", "min_budget": "number", "rate_min": "number", "rate_max": "number", "limit": "number"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				reg := c.SourceRegistry()
				f := sources.SearchFilter{
					Query: str(args, "query"), Title: str(args, "title"), Skills: strList(args, "skills"),
					Location: str(args, "location"), Remote: str(args, "remote"), JobType: str(args, "job_type"),
					MinBudget: numf(args, "min_budget"), MinRate: numf(args, "rate_min"), MaxRate: numf(args, "rate_max"),
					Limit: num(args, "limit", 20),
				}
				var out []map[string]any
				var warnings []string
				var targets []sources.OpportunitySource
				if only, ok := args["sources"].([]any); ok && len(only) > 0 {
					// Accept a source id ("src-upwork"), name ("Upwork"), or
					// case-insensitive prefix ("upwork") — never silently skip.
					for _, o := range only {
						ref, _ := o.(string)
						if ref == "" {
							continue
						}
						if s, found := c.findSource(reg, ref); found {
							targets = append(targets, s)
						} else {
							warnings = append(warnings, "unknown source "+ref)
						}
					}
				} else {
					// All connected external sources. Capabilities are discovered
					// lazily by each adapter, so do not pre-filter on them here:
					// filtering would skip every MCP source until first probe.
					for _, s := range reg.All() {
						if s.ID() == "local" {
							continue
						}
						targets = append(targets, s)
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
							"posted_at": formatStoredTime(o.PostedAt),
						})
					}
				}
				b, _ := json.Marshal(ToolResult{Success: true, Data: out, Warnings: warnings,
					Meta: map[string]string{"deduplicated": "true"}})
				return string(b), nil
			}},
		{Name: "get_source_capabilities", Permission: PermRead, ReadOnly: true,
			Description: "What a connected source actually supports (never assume). Probes once if unknown.",
			ArgsHint:    `{"source": "upwork"}`,
			ArgsSchema:  map[string]string{"source": "string"},
			Timeout:     10 * time.Second,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				conn, err := c.ProbeConnection(ctx, str(args, "source"), 8*time.Second)
				if err != nil {
					return "", err
				}
				return okResult(map[string]any{
					"source": conn.Name, "id": conn.ID, "kind": conn.Kind,
					"status": conn.Status, "auth": conn.Auth,
					"capabilities": conn.Capabilities, "tool_count": conn.ToolCount,
					"detail": conn.Detail,
				}), nil
			}},
		{Name: "source_health", Permission: PermRead, ReadOnly: true,
			Description: "Connectivity/auth state of one or all sources.",
			ArgsHint:    `{"source": "upwork"}`,
			ArgsSchema:  map[string]string{"source": "string"},
			// Bounded: a health probe must never stall a turn on a dead or
			// slow MCP endpoint.
			Timeout: 20 * time.Second,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				if id := str(args, "source"); id != "" {
					conn, err := c.ProbeConnection(ctx, id, 8*time.Second)
					if err != nil {
						return "", err
					}
					return okResult(map[string]any{
						"source": conn.Name, "status": conn.Status, "auth": conn.Auth,
						"detail": conn.Detail, "tool_count": conn.ToolCount,
					}), nil
				}
				conns, err := c.ProbeAll(ctx, 8*time.Second)
				if err != nil {
					return "", err
				}
				out := []map[string]any{}
				for _, conn := range conns {
					if !conn.Enabled {
						continue
					}
					out = append(out, map[string]any{
						"source": conn.Name, "status": conn.Status, "auth": conn.Auth,
						"detail": conn.Detail, "tool_count": conn.ToolCount,
					})
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
			Description: "Configured opportunity sources and their status: kind, endpoint, auth state, and discovered capabilities.",
			ArgsHint:    `{}`,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				conns, err := c.Connections()
				if err != nil {
					return "", err
				}
				if len(conns) == 0 {
					return okResult([]map[string]any{}), nil
				}
				return okResult(conns), nil
			}},
	}
}

func numf(args map[string]any, k string) float64 {
	if v, ok := args[k].(float64); ok {
		return v
	}
	return 0
}
