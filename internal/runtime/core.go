// Package runtime is Scout Core: domain services shared by the CLI,
// the interactive session, and the MCP server. One implementation,
// three interfaces.
package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ianclemence/scout/internal/agent"
	"github.com/ianclemence/scout/internal/approve"
	"github.com/ianclemence/scout/internal/config"
	"github.com/ianclemence/scout/internal/csession"
	"github.com/ianclemence/scout/internal/domain"
	imat "github.com/ianclemence/scout/internal/match"
	"github.com/ianclemence/scout/internal/profile"
	"github.com/ianclemence/scout/internal/secret"
	"github.com/ianclemence/scout/internal/store"
)

// Core bundles dependencies for all Scout operations.
type Core struct {
	Cfg config.Config
	DB  *store.Store
	Key []byte
}

func New(cfg config.Config, db *store.Store) (*Core, error) {
	key, err := secret.MasterKey(cfg.DataDir, cfg.ScoutEnvKey)
	if err != nil {
		return nil, err
	}
	return &Core{Cfg: cfg, DB: db, Key: key}, nil
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func newID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

// ---------- profile ----------

func (c *Core) Profile() (*domain.ProfessionalProfile, error) { return profile.Load(c.DB) }

func (c *Core) Evidence(limit int) ([]domain.Evidence, error) {
	all, err := profile.ListEvidence(c.DB)
	if err != nil {
		return nil, err
	}
	if len(all) > limit {
		return all[:limit], nil
	}
	return all, nil
}

// ---------- opportunities ----------

type OpportunityFilter struct {
	Query  string
	Status string
	Limit  int
}

func (c *Core) ListOpportunities(f OpportunityFilter) ([]domain.Opportunity, error) {
	lim := f.Limit
	if lim <= 0 || lim > 200 {
		lim = 50
	}
	q := `SELECT id,source,title,substr(description,1,2000),status,budget_type,COALESCE(budget_min,0),COALESCE(budget_max,0),COALESCE(connects_cost,0) FROM opportunities WHERE 1=1`
	var args []any
	if f.Query != "" {
		q += ` AND (title LIKE ? OR description LIKE ?)`
		args = append(args, "%"+f.Query+"%", "%"+f.Query+"%")
	}
	if f.Status != "" {
		q += ` AND status=?`
		args = append(args, f.Status)
	}
	q += ` ORDER BY updated_at DESC LIMIT ?`
	args = append(args, lim)
	rows, err := c.DB.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Opportunity
	for rows.Next() {
		var o domain.Opportunity
		rows.Scan(&o.ID, &o.Source, &o.Title, &o.Description, &o.Status, &o.BudgetType, &o.BudgetMin, &o.BudgetMax, &o.ConnectsCost)
		out = append(out, o)
	}
	return out, rows.Err()
}

func (c *Core) GetOpportunity(id string) (*domain.Opportunity, error) {
	var o domain.Opportunity
	var skills, clientID string
	err := c.DB.DB.QueryRow(`SELECT id,source,source_opp_id,title,description,COALESCE(skills,''),COALESCE(category,''),budget_type,COALESCE(budget_min,0),COALESCE(budget_max,0),COALESCE(connects_cost,0),COALESCE(canonical_url,''),status FROM opportunities WHERE id=?`, id).
		Scan(&o.ID, &o.Source, &o.SourceOppID, &o.Title, &o.Description, &skills, &o.Category, &o.BudgetType, &o.BudgetMin, &o.BudgetMax, &o.ConnectsCost, &o.CanonicalURL, &o.Status)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("opportunity %s not found", id)
	}
	if err != nil {
		return nil, err
	}
	o.Skills = splitCSV(skills)
	if clientID != "" {
		var cl domain.Client
		if err := c.DB.DB.QueryRow(`SELECT id,source,display_name,rating,total_hires,total_spend,country FROM clients WHERE id=?`, clientID).
			Scan(&cl.ID, &cl.Source, &cl.DisplayName, &cl.Rating, &cl.TotalHires, &cl.TotalSpend, &cl.Country); err == nil {
			o.Client = &cl
		}
	}
	return &o, nil
}

func (c *Core) AddOpportunity(title, description, skills string) (*domain.Opportunity, error) {
	title = strings.TrimSpace(title)
	if title == "" || strings.TrimSpace(description) == "" {
		return nil, fmt.Errorf("title and description required")
	}
	o := &domain.Opportunity{ID: newID("opp"), Source: "manual", SourceOppID: "",
		Title: title, Description: description, Skills: splitCSV(skills),
		BudgetType: "unknown", Status: "discovered"}
	o.SourceOppID = o.ID
	o.Fingerprint = imat.Fingerprint("manual", o.ID, title, description)
	_, err := c.DB.DB.Exec(`INSERT INTO opportunities(id,source,source_opp_id,title,description,skills,budget_type,fingerprint,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		o.ID, o.Source, o.SourceOppID, o.Title, o.Description, strings.Join(o.Skills, ","),
		o.BudgetType, o.Fingerprint, o.Status, now(), now())
	if err != nil {
		return nil, err
	}
	c.log("opportunity_added", o.ID)
	return o, nil
}

// Analyze runs deterministic filter + heuristic evaluation + optional LLM enrichment.
func (c *Core) Analyze(ctx context.Context, id string, eng *agent.Engine) (*domain.MatchEvaluation, FilterInfo, error) {
	o, err := c.GetOpportunityFull(id)
	if err != nil {
		return nil, FilterInfo{}, err
	}
	p, _ := c.Profile()
	f := imat.DeterministicFilter(p, o)
	ev := imat.HeuristicEvaluate(p, o)
	if eng != nil && eng.LLM != nil {
		evs, _ := c.Evidence(6)
		ev = eng.EnrichEvaluation(p, o, ev, evs)
	}
	b, _ := json.Marshal(ev)
	_, _ = c.DB.DB.Exec(`INSERT INTO evaluations(id,opportunity_id,data,created_at) VALUES(?,?,?,?)`, newID("ev"), id, string(b), now())
	_, _ = c.DB.DB.Exec(`UPDATE opportunities SET status='analyzed', updated_at=? WHERE id=?`, now(), id)
	return ev, FilterInfo{Pass: f.Pass, Reason: f.Reason}, nil
}

type FilterInfo struct {
	Pass   bool
	Reason string
}

func (c *Core) GetOpportunityFull(id string) (*domain.Opportunity, error) {
	return c.GetOpportunity(id)
}

func (c *Core) LatestEvaluation(id string) (*domain.MatchEvaluation, error) {
	var data string
	if err := c.DB.DB.QueryRow(`SELECT data FROM evaluations WHERE opportunity_id=? ORDER BY created_at DESC LIMIT 1`, id).Scan(&data); err != nil {
		return nil, err
	}
	var ev domain.MatchEvaluation
	if err := json.Unmarshal([]byte(data), &ev); err != nil {
		return nil, err
	}
	return &ev, nil
}

// ---------- proposals ----------

func (c *Core) DraftProposal(ctx context.Context, oppID string, eng *agent.Engine) (*domain.Proposal, error) {
	o, err := c.GetOpportunity(oppID)
	if err != nil {
		return nil, err
	}
	p, _ := c.Profile()
	evs, _ := c.Evidence(8)
	var cover string
	var used, qs []string
	if eng != nil {
		cover, used, qs, _ = eng.DraftProposal(p, o, evs, p.ProposalStyle)
	} else {
		cover = "No language model configured."
	}
	evIDs, _ := json.Marshal(used)
	qsB, _ := json.Marshal(qs)
	pr := &domain.Proposal{ID: newID("prop"), OpportunityID: oppID, CoverLetter: cover,
		Rate: p.MinHourlyRate, RateType: "hourly", EvidenceIDs: used, Questions: qs, Status: "draft"}
	_, err = c.DB.DB.Exec(`INSERT INTO proposals(id,opportunity_id,cover_letter,rate,rate_type,evidence_ids,questions,status,created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		pr.ID, oppID, cover, pr.Rate, pr.RateType, string(evIDs), string(qsB), "draft", now())
	if err != nil {
		return nil, err
	}
	_, _ = c.DB.DB.Exec(`UPDATE opportunities SET status='review', updated_at=? WHERE id=?`, now(), oppID)
	c.log("proposal_drafted", oppID)
	return pr, nil
}

func (c *Core) LatestProposal(oppID string) (*domain.Proposal, error) {
	var pr domain.Proposal
	var evIDs, qs string
	err := c.DB.DB.QueryRow(`SELECT id,cover_letter,rate,rate_type,duration_est,evidence_ids,questions,status FROM proposals WHERE opportunity_id=? ORDER BY created_at DESC LIMIT 1`, oppID).
		Scan(&pr.ID, &pr.CoverLetter, &pr.Rate, &pr.RateType, &pr.DurationEst, &evIDs, &qs, &pr.Status)
	if err != nil {
		return nil, err
	}
	pr.OpportunityID = oppID
	json.Unmarshal([]byte(evIDs), &pr.EvidenceIDs)
	json.Unmarshal([]byte(qs), &pr.Questions)
	return &pr, nil
}

// RequestSubmitApproval creates an approval-gated submit action. Never executes.
func (c *Core) RequestSubmitApproval(oppID string) (*domain.PendingAction, error) {
	pr, err := c.LatestProposal(oppID)
	if err != nil {
		return nil, fmt.Errorf("no proposal draft for %s — prepare one first", oppID)
	}
	if c.Cfg.DryRun {
		return nil, fmt.Errorf("dry-run mode: external writes disabled")
	}
	a, err := approve.Create(c.DB, "manual", "submit_proposal", oppID, truncate(pr.CoverLetter, 2000), "high")
	if err != nil {
		return nil, err
	}
	c.log("approval_requested", a.ID)
	return a, nil
}

// ---------- pipeline / applications ----------

func (c *Core) Pipeline() (map[string]int, error) {
	rows, err := c.DB.DB.Query(`SELECT stage, COUNT(*) FROM applications GROUP BY stage`)
	if err != nil {
		return map[string]int{}, err
	}
	defer rows.Close()
	m := map[string]int{}
	for rows.Next() {
		var s string
		var n int
		rows.Scan(&s, &n)
		m[s] = n
	}
	return m, nil
}

func (c *Core) ListApplications(limit int) ([]domain.Application, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := c.DB.DB.Query(`SELECT id,opportunity_id,source,stage,cost_connects,submitted_at FROM applications ORDER BY submitted_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Application{}
	for rows.Next() {
		var a domain.Application
		var tss string
		rows.Scan(&a.ID, &a.OpportunityID, &a.Source, &a.Stage, &a.CostConnects, &tss)
		out = append(out, a)
	}
	return out, nil
}

// ---------- approvals ----------

func (c *Core) PendingApprovals() ([]domain.PendingAction, error) { return approve.List(c.DB, true) }

// ---------- sources ----------

func (c *Core) ListSources() ([]domain.WorkSource, error) {
	rows, err := c.DB.DB.Query(`SELECT id,name,kind,endpoint,enabled,capabilities FROM sources ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.WorkSource
	for rows.Next() {
		var s domain.WorkSource
		var en int
		var caps string
		rows.Scan(&s.ID, &s.Name, &s.Kind, &s.Endpoint, &en, &caps)
		s.Enabled = en == 1
		json.Unmarshal([]byte(caps), &s.Capabilities)
		out = append(out, s)
	}
	return out, nil
}

// ---------- feedback ----------

func (c *Core) AddFeedback(oppID, signal, note string) error {
	_, err := c.DB.DB.Exec(`INSERT INTO feedback(id,opportunity_id,signal,note,created_at) VALUES(?,?,?,?,?)`,
		newID("fb"), oppID, signal, note, now())
	return err
}

// ---------- discovery run ----------

type DiscoverySummary struct {
	Total      int
	Candidates int
	DryRun     bool
}

func (c *Core) RunDiscovery(dry bool) (DiscoverySummary, error) {
	if c.Cfg.DryRun {
		dry = true
	}
	id := newID("run")
	_, _ = c.DB.DB.Exec(`INSERT INTO agent_runs(id,kind,status,summary,dry_run,started_at,ended_at) VALUES(?,?,'running','',?, ?,?)`,
		id, "discovery", boolToInt(dry), now(), now())
	var total int
	_ = c.DB.DB.QueryRow(`SELECT COUNT(*) FROM opportunities`).Scan(&total)
	type row struct {
		id, t, d, bt string
		bmin, bmax   float64
		cc           int
	}
	var rowsData []row
	func() {
		rs, err := c.DB.DB.Query(`SELECT id,title,description,budget_type,COALESCE(budget_min,0),COALESCE(budget_max,0),COALESCE(connects_cost,0) FROM opportunities WHERE status IN ('discovered','analyzed')`)
		if err != nil {
			return
		}
		defer rs.Close()
		for rs.Next() {
			var r row
			if rs.Scan(&r.id, &r.t, &r.d, &r.bt, &r.bmin, &r.bmax, &r.cc) == nil {
				rowsData = append(rowsData, r)
			}
		}
	}()
	p, _ := c.Profile()
	cands := 0
	for _, r := range rowsData {
		o := &domain.Opportunity{ID: r.id, Title: r.t, Description: r.d, BudgetType: r.bt, BudgetMin: r.bmin, BudgetMax: r.bmax, ConnectsCost: r.cc}
		if f := imat.DeterministicFilter(p, o); f.Pass {
			cands++
		}
	}
	summary := fmt.Sprintf("discovered=%d candidates=%d dry_run=%v (no external writes)", total, cands, dry)
	_, _ = c.DB.DB.Exec(`UPDATE agent_runs SET status='done', summary=?, ended_at=? WHERE id=?`, summary, now(), id)
	return DiscoverySummary{Total: total, Candidates: cands, DryRun: dry}, nil
}

// ---------- sessions helper ----------

func (c *Core) ResolveSession(ref string) (*csession.Session, error) {
	return csession.Resolve(c.DB, ref)
}

func (c *Core) SessionMessageCount(sessionID string) (int, error) {
	var n int
	err := c.DB.DB.QueryRow(`SELECT COUNT(*) FROM session_messages WHERE session_id=?`, sessionID).Scan(&n)
	return n, err
}

func (c *Core) SetApprovalStatus(id, status string) error {
	return approve.SetStatus(c.DB, c.resolveActionID(id), status)
}

func (c *Core) resolveActionID(ref string) string {
	var id string
	if err := c.DB.DB.QueryRow(`SELECT id FROM pending_actions WHERE id=? OR id LIKE ? ORDER BY created_at DESC LIMIT 1`, ref, ref+"%").Scan(&id); err == nil {
		return id
	}
	return ref
}

// EngineForRole builds a role-scoped engine (conversation uses session model).
func (c *Core) EngineForRole(role string) *agent.Engine {
	return engineFromEnv(c, role)
}

// EngineFor builds an engine for an explicit provider/model (session switching).
func (c *Core) EngineFor(provider, model string) *agent.Engine {
	saved := c.Cfg.Models["conversation"]
	c.Cfg.Models["conversation"] = config.LLMRole{Provider: provider, Model: model}
	eng := engineFromEnv(c, "conversation")
	c.Cfg.Models["conversation"] = saved
	return eng
}

// ---------- secrets ----------

func (c *Core) SaveSecret(key, plaintext string) error {
	ct, err := secret.Encrypt(c.Key, []byte(plaintext))
	if err != nil {
		return err
	}
	_, err = c.DB.DB.Exec(`INSERT OR REPLACE INTO secrets(key,value,updated_at) VALUES(?,?,?)`, key, ct, now())
	return err
}

func (c *Core) LoadSecret(key string) (string, error) {
	var blob []byte
	if err := c.DB.DB.QueryRow(`SELECT value FROM secrets WHERE key=?`, key).Scan(&blob); err != nil {
		return "", err
	}
	pt, err := secret.Decrypt(c.Key, blob)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

func (c *Core) log(kind, detail string) {
	_, _ = c.DB.DB.Exec(`INSERT INTO events(id,kind,detail,created_at) VALUES(?,?,?,?)`, newID("evt"), kind, detail, now())
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
