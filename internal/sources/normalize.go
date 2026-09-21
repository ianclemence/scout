package sources

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ianclemence/scout/internal/domain"
	imat "github.com/ianclemence/scout/internal/match"
)

// NormalizeSearchPayload maps common MCP search payload shapes into
// normalized opportunities. Unknown shapes are errors, not guesses.
func NormalizeSearchPayload(source string, raw []byte) ([]domain.Opportunity, error) {
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return nil, fmt.Errorf("empty payload")
	}
	// Try: array of job objects.
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err == nil {
		var out []domain.Opportunity
		for _, m := range arr {
			out = append(out, *normalizeMap(source, m))
		}
		return out, nil
	}
	// Try: {jobs:[...]} / {results:[...]} / {opportunities:[...]} / {data:[...]}.
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("not JSON: %s", truncate(raw, 120))
	}
	for _, k := range []string{"jobs", "results", "opportunities", "data", "listings"} {
		if v, ok := obj[k]; ok {
			b, _ := json.Marshal(v)
			var arr2 []map[string]any
			if json.Unmarshal(b, &arr2) == nil {
				var out []domain.Opportunity
				for _, m := range arr2 {
					out = append(out, *normalizeMap(source, m))
				}
				return out, nil
			}
		}
	}
	return nil, fmt.Errorf("unrecognized search payload shape")
}

// NormalizeListingPayload maps one listing payload.
func NormalizeListingPayload(source, sourceID string, raw []byte) (*domain.Opportunity, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("not JSON: %s", truncate(raw, 120))
	}
	// Unwrap single-key envelopes.
	if len(m) == 1 {
		for _, v := range m {
			if inner, ok := v.(map[string]any); ok {
				m = inner
			}
		}
	}
	o := normalizeMap(source, m)
	if o.SourceOppID == "" {
		o.SourceOppID = sourceID
	}
	return o, nil
}

func normalizeMap(source string, m map[string]any) *domain.Opportunity {
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
	strs := func(keys ...string) []string {
		for _, k := range keys {
			switch v := m[k].(type) {
			case []string:
				return v
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
					return []string{v}
				}
			}
		}
		return nil
	}
	o := &domain.Opportunity{
		Source:       source,
		SourceOppID:  str("id", "job_id", "uid", "key"),
		Title:        str("title", "name", "headline"),
		Company:      str("company", "client", "client_name", "organization"),
		Description:  str("description", "details", "body", "summary"),
		Category:     str("category"),
		BudgetMin:    num("budget_min", "amount_min", "salary_min"),
		BudgetMax:    num("budget_max", "amount", "budget", "salary_max"),
		BudgetType:   str("budget_type", "type", "compensation_type"),
		Currency:     str("currency"),
		Location:     str("location", "country", "region"),
		RemoteStatus: str("remote", "remote_status", "workplace"),
		Skills:       strs("skills", "tags", "technologies"),
		SourceURL:    str("url", "link", "canonical_url"),
		Status:       "discovered",
		LiveStatus:   "unknown",
		RawSnapshot:  truncate(rawOf(m), 4000),
	}
	o.CanonicalURL = o.SourceURL
	o.Fingerprint = imat.Fingerprint(source, o.SourceOppID, o.Title, o.Description)
	return Normalize(source, o)
}

func rawOf(m map[string]any) []byte {
	b, _ := json.Marshal(m)
	return b
}

func truncate(b []byte, n int) string {
	s := string(b)
	if len(s) <= n {
		return s
	}
	return s[:n]
}
