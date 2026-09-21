// Package upwork adapts the official Upwork MCP server to Scout's domain.
// It never hard-codes tool names: capabilities are discovered via ListTools
// and mapped to domain.Capability. No scraping, no private endpoints.
package upwork

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/ianclemence/scout/internal/domain"
	"github.com/ianclemence/scout/internal/mcpclient"
)

const Endpoint = "https://mcp.upwork.com/mcp"

// Discover maps remote tool names to Scout capabilities by keyword matching.
func Discover(tools []mcpclient.ToolInfo) []domain.Capability {
	caps := map[domain.Capability]bool{}
	nameOf := func(t mcpclient.ToolInfo) string { return strings.ToLower(t.Name + " " + t.Description) }
	for _, t := range tools {
		n := nameOf(t)
		switch {
		case containsAny(n, []string{"search job", "find job", "job search", "search_job"}):
			caps[domain.CapSearchOpportunities] = true
		case containsAny(n, []string{"job detail", "get job", "read job", "job_detail"}):
			caps[domain.CapReadOpportunity] = true
		case containsAny(n, []string{"freelancer", "talent", "client"}):
			caps[domain.CapSearchClients] = true
		case containsAny(n, []string{"message", "thread", "conversation"}):
			caps[domain.CapReadMessages] = true
		case containsAny(n, []string{"proposal", "draft"}):
			caps[domain.CapDraftApplication] = true
		case containsAny(n, []string{"submit", "apply"}):
			caps[domain.CapSubmitApplication] = true
		case containsAny(n, []string{"contract"}):
			caps[domain.CapReadContract] = true
		case containsAny(n, []string{"earning", "balance", "transaction"}):
			caps[domain.CapReadEarnings] = true
		}
	}
	var out []domain.Capability
	for c := range caps {
		out = append(out, c)
	}
	return out
}

func containsAny(hay string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(hay, strings.ToLower(n)) {
			return true
		}
	}
	return false
}

// NormalizeJob maps an arbitrary Upwork job payload into a domain.Opportunity.
// Unknown fields are kept in RawSnapshot; important fields are normalized.
func NormalizeJob(raw json.RawMessage) (*domain.Opportunity, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	str := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := m[k].(string); ok && v != "" {
				return v
			}
		}
		return ""
	}
	num := func(keys ...string) float64 {
		for _, k := range keys {
			switch v := m[k].(type) {
			case float64:
				return v
			case int:
				return float64(v)
			}
		}
		return 0
	}
	o := &domain.Opportunity{
		Source:      "upwork",
		SourceOppID: str("id", "job_id", "uid"),
		Title:       str("title", "name"),
		Description: str("description", "details"),
		Category:    str("category"),
		BudgetMin:   num("budget_min", "amount_min"),
		BudgetMax:   num("budget_max", "amount", "budget"),
		BudgetType:  str("budget_type", "type"),
		RawSnapshot: string(raw),
		Status:      "discovered",
	}
	if id, ok := m["id"].(string); ok {
		o.CanonicalURL = "https://www.upwork.com/jobs/" + id
	}
	return o, nil
}

var _ = context.Background
