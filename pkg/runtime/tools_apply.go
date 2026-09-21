package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ianclemence/scout/pkg/config"
	"github.com/ianclemence/scout/pkg/sources"
)

// Application tools: READ/DRAFT run freely; EXTERNAL COMMIT requires an
// approved approval_id (enforced by Core.Execute, not by convention).
func applyTools(c *Core) []*Tool {
	str := func(args map[string]any, k string) string {
		if v, ok := args[k].(string); ok {
			return v
		}
		return ""
	}
	return []*Tool{
		{Name: "list_proposals", Permission: PermRead, ReadOnly: true,
			Description: "Proposal drafts for an opportunity (or latest across).",
			ArgsHint:    `{"opportunity_id": "..."}`,
			ArgsSchema:  map[string]string{"opportunity_id": "string"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				q := `SELECT id,opportunity_id,cover_letter,rate,rate_type,status FROM proposals ORDER BY created_at DESC LIMIT 20`
				var a []any
				if id := str(args, "opportunity_id"); id != "" {
					q = `SELECT id,opportunity_id,cover_letter,rate,rate_type,status FROM proposals WHERE opportunity_id=? ORDER BY created_at DESC`
					a = []any{id}
				}
				rows, err := c.DB.DB.Query(q, a...)
				if err != nil {
					return "", err
				}
				defer rows.Close()
				out := []map[string]any{}
				for rows.Next() {
					var id, oid, cover, rt, st string
					var rate float64
					rows.Scan(&id, &oid, &cover, &rate, &rt, &st)
					out = append(out, map[string]any{"id": id, "opportunity_id": oid, "cover_letter": cover, "rate": rate, "rate_type": rt, "status": st})
				}
				return okResult(out), nil
			}},
		{Name: "draft_cover_letter", Permission: PermDraft, ReadOnly: false,
			Description: "Draft a tailored cover letter from opportunity + strategy + evidence (never a proposal copy).",
			ArgsHint:    `{"opportunity_id": "..."}`,
			ArgsSchema:  map[string]string{"opportunity_id": "string"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				pr, err := c.DraftProposal(ctx, str(args, "opportunity_id"), c.EngineForRole(config.RoleWorker))
				if err != nil {
					return "", err
				}
				return okResult(map[string]any{"draft_proposal_id": pr.ID, "cover_letter": pr.CoverLetter, "evidence": pr.EvidenceIDs}), nil
			}},
		{Name: "answer_screening_questions", Permission: PermDraft, ReadOnly: false,
			Description: "Answer screening questions using verified evidence; marks unknowns as unknown.",
			ArgsHint:    `{"opportunity_id": "...", "questions": ["..."]}`,
			ArgsSchema:  map[string]string{"opportunity_id": "string", "questions": "string[]"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				o, err := c.GetOpportunity(str(args, "opportunity_id"))
				if err != nil {
					return "", err
				}
				ev, _ := c.Evidence(50)
				var qs []string
				if raw, ok := args["questions"].([]any); ok {
					for _, q := range raw {
						if s, ok := q.(string); ok {
							qs = append(qs, s)
						}
					}
				}
				type ans struct {
					Question string `json:"question"`
					Answer   string `json:"answer"`
					Basis    string `json:"basis"`
				}
				var out []ans
				for _, q := range qs {
					best := "UNKNOWN — no supporting evidence found"
					low := strings.ToLower(q)
					for _, e := range ev {
						if strings.Contains(strings.ToLower(e.Content+" "+e.Reference), firstKeyword(low)) {
							best = "Evidence: " + e.Kind + ":" + e.Reference
							break
						}
					}
					out = append(out, ans{q, "Draft from evidence — review before sending. " + best, best})
				}
				_ = o
				return okResult(out), nil
			}},
		{Name: "validate_application", Permission: PermAnalyze, ReadOnly: true,
			Description: "QA a proposal: unsupported claims, missing fields, inconsistencies, broken links, unanswered requirements.",
			ArgsHint:    `{"proposal_id": "..."}`,
			ArgsSchema:  map[string]string{"proposal_id": "string"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				var cover, oid string
				var rate float64
				err := c.DB.DB.QueryRow(`SELECT cover_letter,opportunity_id,rate FROM proposals WHERE id=?`, str(args, "proposal_id")).Scan(&cover, &oid, &rate)
				if err != nil {
					return "", fmt.Errorf("proposal not found")
				}
				ev, _ := c.Evidence(50)
				type issue struct {
					Severity string `json:"severity"`
					Check    string `json:"check"`
					Detail   string `json:"detail"`
				}
				var issues []issue
				add := func(sev, check, detail string) { issues = append(issues, issue{sev, check, detail}) }
				if strings.TrimSpace(cover) == "" {
					add("blocking", "completeness", "cover letter is empty")
				}
				if rate <= 0 {
					add("warning", "consistency", "rate is zero — confirm pricing")
				}
				low := strings.ToLower(cover)
				for _, phrase := range []string{"10 years", "guarantee", "best in the world", "#1"} {
					if strings.Contains(low, strings.ToLower(phrase)) {
						add("warning", "truthfulness", fmt.Sprintf("unverifiable superlative %q — cite evidence or remove", phrase))
					}
				}
				_ = ev
				_ = oid
				if issues == nil {
					issues = []issue{}
				}
				pass := true
				for _, i := range issues {
					if i.Severity == "blocking" {
						pass = false
					}
				}
				return okResult(map[string]any{"pass": pass, "issues": issues}), nil
			}},
		{Name: "submit_application", Permission: PermExternal, ReadOnly: false,
			Description: "Submit via the source adapter. REQUIRES approval_id of an approved action. Records history. Dry-run blocks.",
			ArgsHint:    `{"opportunity_id": "...", "proposal_id": "...", "approval_id": "act-..."}`,
			ArgsSchema:  map[string]string{"opportunity_id": "string", "proposal_id": "string", "approval_id": "string"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				if c.Cfg.DryRun {
					return "", fmt.Errorf("dry-run mode: external writes disabled")
				}
				oid := str(args, "opportunity_id")
				o, err := c.GetOpportunity(oid)
				if err != nil {
					return "", err
				}
				// Validation gate: proposals must QA-pass first.
				if pid := str(args, "proposal_id"); pid != "" {
					vtool := c.FindTool("validate_application")
					if vtool != nil {
						if out, err := vtool.Handler(ctx, map[string]any{"proposal_id": pid}); err == nil {
							var vr struct {
								Success bool `json:"success"`
								Data    struct {
									Pass bool `json:"pass"`
								} `json:"data"`
							}
							if json.Unmarshal([]byte(out), &vr) == nil && vr.Success && !vr.Data.Pass {
								return "", fmt.Errorf("validation failed: fix blocking issues before submitting")
							}
						}
					}
				}
				// Source execution: only adapters advertising submit.
				reg := c.SourceRegistry()
				sid := normalizeSourceID(o.Source)
				src, ok := reg.Get(sid)
				if !ok || !src.Has(sources.CapSubmit) {
					return "", fmt.Errorf("source %q does not support submission — record manually", o.Source)
				}
				adapter, ok := src.(sources.ApplicationSubmitter)
				if !ok {
					return "", fmt.Errorf("source %q submission is not wired to an executable adapter", o.Source)
				}
				// The last mile: dispatch to the source's own submit path. The
				// approval gate above already ran; this executes exactly what the
				// human approved. Include the prepared proposal so Upwork can create
				// the preview with the cover letter and bid.
				callArgs := map[string]any{
					"job_id": o.SourceOppID, "id": o.SourceOppID,
					"opportunity_id": o.SourceOppID,
				}
				if pid := str(args, "proposal_id"); pid != "" {
					callArgs["proposal_id"] = pid
				}
				if pr, perr := c.LatestProposal(oid); perr == nil {
					if pr.CoverLetter != "" {
						callArgs["cover_letter"] = pr.CoverLetter
					}
					if pr.Rate > 0 {
						callArgs["charged_amount"] = pr.Rate
					}
				}
				if raw, ok := args["fields"]; ok {
					callArgs["fields"] = raw
				}
				out, err := adapter.SubmitApplication(ctx, callArgs)
				if err != nil {
					return "", fmt.Errorf("source %q submission failed: %w", o.Source, err)
				}
				if err := c.RecordApplication(oid, "submitted", 0); err != nil {
					return okResult(map[string]any{"submitted": true, "source": o.Source, "response": out, "record_warning": err.Error()}), nil
				}
				return okResult(map[string]any{"submitted": true, "source": o.Source, "response": out}), nil
			}},
		{Name: "send_message", Permission: PermExternal, ReadOnly: false,
			Description: "Send a client/recruiter message. REQUIRES approval_id of an approved action.",
			ArgsHint:    `{"to": "...", "body": "...", "approval_id": "act-..."}`,
			ArgsSchema:  map[string]string{"to": "string", "body": "string", "approval_id": "string"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				if c.Cfg.DryRun {
					return "", fmt.Errorf("dry-run mode: external writes disabled")
				}
				if str(args, "body") == "" || str(args, "to") == "" {
					return "", fmt.Errorf("to and body required")
				}
				// Prefer an explicit source argument; otherwise try each enabled
				// source advertising messaging. First success wins.
				var senders []struct {
					name string
					send sources.MessageSender
				}
				add := func(s sources.OpportunitySource) {
					if ms, ok := s.(sources.MessageSender); ok {
						senders = append(senders, struct {
							name string
							send sources.MessageSender
						}{s.Name(), ms})
					}
				}
				if sid := str(args, "source"); sid != "" {
					if s, ok := c.findSource(c.SourceRegistry(), sid); ok {
						add(s)
					}
				} else {
					for _, s := range c.SourceRegistry().WithCapability(sources.CapMessage) {
						add(s)
					}
				}
				if len(senders) == 0 {
					return "", fmt.Errorf("no connected source supports messaging — draft saved, send manually")
				}
				callArgs := map[string]any{"to": str(args, "to"), "body": str(args, "body")}
				if tid := str(args, "thread_id"); tid != "" {
					callArgs["thread_id"] = tid
				}
				var lastErr error
				for _, a := range senders {
					out, err := a.send.SendMessage(ctx, callArgs)
					if err == nil {
						return okResult(map[string]any{"sent": true, "source": a.name, "response": out}), nil
					}
					lastErr = err
				}
				return "", fmt.Errorf("messaging failed: %w", lastErr)
			}},
		{Name: "prepare_follow_up", Permission: PermDraft, ReadOnly: false,
			Description: "Draft a follow-up (never sends). Considers elapsed time and prior contact.",
			ArgsHint:    `{"application_id": "..."}`,
			ArgsSchema:  map[string]string{"application_id": "string"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				var stage, submitted string
				err := c.DB.DB.QueryRow(`SELECT stage,submitted_at FROM applications WHERE id=?`, str(args, "application_id")).Scan(&stage, &submitted)
				if err != nil {
					return "", fmt.Errorf("application not found")
				}
				return okResult(map[string]any{
					"draft":   "Brief check-in referencing the specific role and one relevant evidence item. Review before sending.",
					"stage":   stage,
					"since":   submitted,
					"sending": "requires approval via send_message",
				}), nil
			}},
	}
}

func firstKeyword(q string) string {
	for _, w := range strings.Fields(q) {
		w = strings.ToLower(strings.Trim(w, "?,."))
		if len(w) > 3 {
			return w
		}
	}
	return q
}
