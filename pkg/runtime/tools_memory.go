package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ianclemence/scout/pkg/domain"
	imat "github.com/ianclemence/scout/pkg/match"
)

func memoryTools(c *Core) []*Tool {
	str := func(args map[string]any, k string) string {
		if v, ok := args[k].(string); ok {
			return v
		}
		return ""
	}
	return []*Tool{
		{Name: "save_opportunity", Permission: PermMutateLocal, ReadOnly: false,
			Description: "Persist a normalized opportunity (deduplicates by fingerprint).",
			ArgsHint:    `{"source": "...", "source_id": "...", "title": "...", "description": "..."}`,
			ArgsSchema:  map[string]string{"source": "string", "source_id": "string", "title": "string", "description": "string", "url": "string", "company": "string"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				title, desc := str(args, "title"), str(args, "description")
				if title == "" || desc == "" {
					return "", fmt.Errorf("title and description required")
				}
				src := str(args, "source")
				if src == "" {
					src = "manual"
				}
				fp := imat.Fingerprint(src, str(args, "source_id"), title, desc)
				var existing string
				_ = c.DB.DB.QueryRow(`SELECT id FROM opportunities WHERE fingerprint=?`, fp).Scan(&existing)
				if existing != "" {
					return okResult(map[string]any{"id": existing, "duplicate": true}), nil
				}
				id := newID("opp")
				now := now()
				_, err := c.DB.DB.Exec(`INSERT INTO opportunities(id,source,source_opp_id,title,company,description,canonical_url,source_url,fingerprint,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
					id, src, str(args, "source_id"), title, str(args, "company"), desc,
					str(args, "url"), str(args, "url"), fp, "discovered", now, now)
				if err != nil {
					return "", err
				}
				return okResult(map[string]any{"id": id, "duplicate": false}), nil
			}},
		{Name: "find_duplicate_opportunity", Permission: PermRead, ReadOnly: true,
			Description: "Check whether a listing duplicates a known opportunity (fingerprint + fuzzy signals).",
			ArgsHint:    `{"title": "...", "description": "...", "source": "..."}`,
			ArgsSchema:  map[string]string{"title": "string", "description": "string", "source": "string"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				title, desc := str(args, "title"), str(args, "description")
				fp := imat.Fingerprint(str(args, "source"), "", title, desc)
				var id, stitle string
				if err := c.DB.DB.QueryRow(`SELECT id,title FROM opportunities WHERE fingerprint=?`, fp).Scan(&id, &stitle); err == nil {
					return okResult(map[string]any{"duplicate": true, "id": id, "title": stitle, "signal": "fingerprint"}), nil
				}
				// Fuzzy: same normalized title.
				norm := strings.ToLower(strings.TrimSpace(title))
				rows, err := c.DB.DB.Query(`SELECT id,title FROM opportunities`)
				if err != nil {
					return "", err
				}
				defer rows.Close()
				for rows.Next() {
					var oid, ot string
					rows.Scan(&oid, &ot)
					if strings.ToLower(strings.TrimSpace(ot)) == norm && norm != "" {
						return okResult(map[string]any{"duplicate": true, "id": oid, "signal": "title"}), nil
					}
				}
				return okResult(map[string]any{"duplicate": false}), nil
			}},
		{Name: "search_opportunity_history", Permission: PermRead, ReadOnly: true,
			Description: "Search previously discovered opportunities.",
			ArgsHint:    `{"query": "go", "limit": 20}`,
			ArgsSchema:  map[string]string{"query": "string", "limit": "number"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				opps, err := c.ListOpportunities(OpportunityFilter{Query: str(args, "query"), Limit: int(numf(args, "limit"))})
				if err != nil {
					return "", err
				}
				if opps == nil {
					opps = []domain.Opportunity{}
				}
				return okResult(opps), nil
			}},
		{Name: "record_application", Permission: PermMutateLocal, ReadOnly: false,
			Description: "Record an application attempt and stage (prepared/approved/submitted/viewed/responded/interview/rejected/withdrawn/offer/won/closed).",
			ArgsHint:    `{"opportunity_id": "...", "stage": "submitted"}`,
			ArgsSchema:  map[string]string{"opportunity_id": "string", "stage": "string", "connects": "number"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				stage := str(args, "stage")
				if stage == "" {
					stage = "prepared"
				}
				valid := map[string]bool{"prepared": true, "approved": true, "submitted": true, "viewed": true, "responded": true, "interview": true, "rejected": true, "withdrawn": true, "offer": true, "won": true, "closed": true, "unknown": true}
				if !valid[stage] {
					return "", fmt.Errorf("invalid stage %q", stage)
				}
				if err := c.RecordApplication(str(args, "opportunity_id"), stage, int(numf(args, "connects"))); err != nil {
					return "", err
				}
				return okResult(map[string]any{"stage": stage}), nil
			}},
		{Name: "record_application_status", Permission: PermMutateLocal, ReadOnly: false,
			Description: "Update an application's stage.",
			ArgsHint:    `{"application_id": "...", "stage": "interview"}`,
			ArgsSchema:  map[string]string{"application_id": "string", "stage": "string"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				r, err := c.DB.DB.Exec(`UPDATE applications SET stage=?, updated_at=? WHERE id=?`, str(args, "stage"), now(), str(args, "application_id"))
				if err != nil {
					return "", err
				}
				if n, _ := r.RowsAffected(); n == 0 {
					return "", fmt.Errorf("application not found")
				}
				return okResult(map[string]any{"ok": true}), nil
			}},
		{Name: "record_opportunity_outcome", Permission: PermMutateLocal, ReadOnly: false,
			Description: "Store the eventual outcome (rejected/interview/offer/won/dismissed + note). Feeds learning, never rewrites explicit preferences.",
			ArgsHint:    `{"opportunity_id": "...", "outcome": "rejected", "note": "..."}`,
			ArgsSchema:  map[string]string{"opportunity_id": "string", "outcome": "string", "note": "string"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				_, err := c.DB.DB.Exec(`INSERT INTO events(id,kind,detail,created_at) VALUES(?,?,?,?)`,
					newID("evt"), "outcome:"+str(args, "outcome"), str(args, "opportunity_id")+" "+str(args, "note"), now())
				return okResult(map[string]any{"ok": true}), err
			}},
		{Name: "search_application_history", Permission: PermRead, ReadOnly: true,
			Description: "Retrieve prior applications, optionally filtered by stage.",
			ArgsHint:    `{"stage": "submitted"}`,
			ArgsSchema:  map[string]string{"stage": "string"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				apps, err := c.ListApplications(100)
				if err != nil {
					return "", err
				}
				if st := str(args, "stage"); st != "" {
					var f []domain.Application
					for _, a := range apps {
						if a.Stage == st {
							f = append(f, a)
						}
					}
					apps = f
				}
				if apps == nil {
					apps = []domain.Application{}
				}
				return okResult(apps), nil
			}},
		{Name: "record_learned_observation", Permission: PermMutateLocal, ReadOnly: false,
			Description: "Store an inferred pattern (NOT a hard constraint). Explicit preferences always win.",
			ArgsHint:    `{"pattern": "rejects wordpress-heavy work", "signal": "repeated dismissal"}`,
			ArgsSchema:  map[string]string{"pattern": "string", "signal": "string"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				if str(args, "pattern") == "" {
					return "", fmt.Errorf("pattern required")
				}
				_, err := c.DB.DB.Exec(`INSERT INTO learned_observations(id,pattern,signal,created_at) VALUES(?,?,?,?)`,
					newID("learn"), str(args, "pattern"), str(args, "signal"), time.Now().UTC().Format(time.RFC3339))
				return okResult(map[string]any{"ok": true}), err
			}},
		{Name: "search_learned_preferences", Permission: PermRead, ReadOnly: true,
			Description: "Retrieve learned observations (advisory only).",
			ArgsHint:    `{}`,
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				rows, err := c.DB.DB.Query(`SELECT pattern,signal,created_at FROM learned_observations ORDER BY created_at DESC LIMIT 50`)
				if err != nil {
					return "", err
				}
				defer rows.Close()
				out := []map[string]string{}
				for rows.Next() {
					var p, s, t string
					rows.Scan(&p, &s, &t)
					out = append(out, map[string]string{"pattern": p, "signal": s, "at": t})
				}
				return okResult(out), nil
			}},
	}
}
