// Package httpapi serves the web UI, JSON API, health endpoints, and mounts
// the Scout MCP Streamable HTTP endpoint. Local auth: first-run token in
// data dir + session cookie (bcrypt-hashed password optional).
package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/ianclemence/scout/internal/agent"
	"github.com/ianclemence/scout/internal/approve"
	"github.com/ianclemence/scout/internal/config"
	"github.com/ianclemence/scout/internal/domain"
	imat "github.com/ianclemence/scout/internal/match"
	"github.com/ianclemence/scout/internal/mcpserver"
	"github.com/ianclemence/scout/internal/profile"
	"github.com/ianclemence/scout/internal/secret"
	"github.com/ianclemence/scout/internal/store"
	"github.com/ianclemence/scout/internal/version"
)

type Server struct {
	cfg config.Config
	db  *store.Store
	tpl *template.Template
	mux *http.ServeMux
	eng *agent.Engine
	key []byte // master key for secrets
}

func New(cfg config.Config, db *store.Store, eng *agent.Engine, tpl *template.Template, key []byte) *Server {
	s := &Server{cfg: cfg, db: db, tpl: tpl, mux: http.NewServeMux(), eng: eng, key: key}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"scout":` + jsonStr(version.Version) + `}`))
	})
	s.mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.db.DB.Ping(); err != nil {
			http.Error(w, "db not ready", 503)
			return
		}
		w.Write([]byte("ready"))
	})
	// Scout MCP over Streamable HTTP.
	s.mux.Handle("/mcp", mcpserver.Handler(mcpserver.Deps{Store: s.db}))

	s.mux.HandleFunc("/api/status", s.requireAuth(s.apiStatus))
	s.mux.HandleFunc("/api/opportunities", s.requireAuth(s.apiOpportunities))
	s.mux.HandleFunc("/api/approvals", s.requireAuth(s.apiApprovals))

	s.mux.HandleFunc("/", s.requireAuth(s.handleHome))
	s.mux.HandleFunc("/o/", s.requireAuth(s.handleOpportunity))
	s.mux.HandleFunc("/opportunities/add", s.requireAuth(s.handleAddOpp))
	s.mux.HandleFunc("/approvals", s.requireAuth(s.handleApprovals))
	s.mux.HandleFunc("/approvals/", s.requireAuth(s.handleApprovalAction))
	s.mux.HandleFunc("/applications", s.requireAuth(s.handleApplications))
	s.mux.HandleFunc("/profile", s.requireAuth(s.handleProfile))
	s.mux.HandleFunc("/profile/import", s.requireAuth(s.handleProfileImport))
	s.mux.HandleFunc("/integrations", s.requireAuth(s.handleIntegrations))
	s.mux.HandleFunc("/integrations/add", s.requireAuth(s.handleIntegrationsAdd))
	s.mux.HandleFunc("/status", s.requireAuth(s.handleStatus))
	s.mux.HandleFunc("/setup", s.handleSetup)
}

// ---------- auth ----------

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.AuthDisabled && isLocalhost(r) {
			next(w, r)
			return
		}
		if s.hasPassword() {
			c, err := r.Cookie("scout_session")
			if err != nil || !s.validSession(c.Value) {
				http.Redirect(w, r, "/setup", http.StatusSeeOther)
				return
			}
			next(w, r)
			return
		}
		// First run: require setup token for non-localhost, allow localhost.
		if !isLocalhost(r) {
			http.Error(w, "first-run setup required from localhost", 403)
			return
		}
		next(w, r)
	}
}

func isLocalhost(r *http.Request) bool {
	h := r.Host
	return strings.HasPrefix(h, "127.0.0.1") || strings.HasPrefix(h, "localhost") || strings.HasPrefix(h, "[::1]")
}

func (s *Server) hasPassword() bool {
	var n int
	_ = s.db.DB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n > 0
}

func (s *Server) validSession(tok string) bool {
	var v string
	err := s.db.DB.QueryRow(`SELECT value FROM app_settings WHERE key='session:'||?`, hash(tok)).Scan(&v)
	return err == nil && v == "1"
}

func hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// ---------- pages ----------

type pageData struct {
	Version       string
	Page          string
	Flash         string
	Query         string
	Filter        string
	Stats         struct{ Opportunities, Pending, Applications int }
	PendingItems  []domain.PendingAction
	Opportunities []oppRow
	Opp           *oppRow
	Eval          *domain.MatchEvaluation
	Proposal      *domain.Proposal
	Applications  []domain.Application
	Profile       *domain.ProfessionalProfile
	SkillsStr     string
	ExcludedStr   string
	Sources       []srcRow
	PendingCount  int
	StatusJSON    string
}

type oppRow struct {
	ID           string
	Source       string
	Title        string
	Description  string
	Status       string
	BudgetType   string
	BudgetMin    float64
	BudgetMax    float64
	ConnectsCost int
}

type srcRow struct {
	Name, Kind, Endpoint, Capabilities string
	Enabled                            bool
}

func (s *Server) render(w http.ResponseWriter, d pageData) {
	d.Version = version.Version
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tpl.Execute(w, d); err != nil {
		http.Error(w, "template error", 500)
	}
}

func (s *Server) stats() (opps, pending, apps int) {
	_ = s.db.DB.QueryRow(`SELECT COUNT(*) FROM opportunities`).Scan(&opps)
	_ = s.db.DB.QueryRow(`SELECT COUNT(*) FROM pending_actions WHERE status IN ('draft','pending_approval')`).Scan(&pending)
	_ = s.db.DB.QueryRow(`SELECT COUNT(*) FROM applications`).Scan(&apps)
	return
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	d := pageData{Page: "home", Query: r.URL.Query().Get("q"), Filter: r.URL.Query().Get("filter")}
	o, p, a := s.stats()
	d.Stats.Opportunities, d.Stats.Pending, d.Stats.Applications = o, p, a
	d.PendingCount = p
	items, _ := approve.List(s.db, true)
	d.PendingItems = items
	q := "%" + d.Query + "%"
	fq := ""
	args := []any{q, q}
	if d.Filter != "" {
		fq = "AND status=?"
		args = append(args, d.Filter)
	}
	rows, _ := s.db.DB.Query(`SELECT id,source,title,status FROM opportunities WHERE (title LIKE ? OR description LIKE ?) `+fq+` ORDER BY updated_at DESC LIMIT 100`, args...)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var x oppRow
			rows.Scan(&x.ID, &x.Source, &x.Title, &x.Status)
			d.Opportunities = append(d.Opportunities, x)
		}
	}
	s.render(w, d)
}

func (s *Server) handleOpportunity(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/o/")
	if strings.HasSuffix(rest, "/proposal") && r.Method == "POST" {
		s.generateProposal(w, r, strings.TrimSuffix(rest, "/proposal"))
		return
	}
	if strings.HasSuffix(rest, "/request-approval") && r.Method == "POST" {
		s.requestApproval(w, r, strings.TrimSuffix(rest, "/request-approval"))
		return
	}
	id := strings.SplitN(rest, "/", 2)[0]
	var x oppRow
	err := s.db.DB.QueryRow(`SELECT id,source,title,description,status,budget_type,COALESCE(budget_min,0),COALESCE(budget_max,0),COALESCE(connects_cost,0) FROM opportunities WHERE id=?`, id).
		Scan(&x.ID, &x.Source, &x.Title, &x.Description, &x.Status, &x.BudgetType, &x.BudgetMin, &x.BudgetMax, &x.ConnectsCost)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	d := pageData{Page: "opportunity", Opp: &x}
	var evalData string
	_ = s.db.DB.QueryRow(`SELECT data FROM evaluations WHERE opportunity_id=? ORDER BY created_at DESC LIMIT 1`, id).Scan(&evalData)
	if evalData != "" {
		var ev domain.MatchEvaluation
		if json.Unmarshal([]byte(evalData), &ev) == nil {
			d.Eval = &ev
		}
	}
	var pr domain.Proposal
	var evIDs, qs string
	err = s.db.DB.QueryRow(`SELECT id,cover_letter,rate,rate_type,duration_est,evidence_ids,questions,status FROM proposals WHERE opportunity_id=? ORDER BY created_at DESC LIMIT 1`, id).
		Scan(&pr.ID, &pr.CoverLetter, &pr.Rate, &pr.RateType, &pr.DurationEst, &evIDs, &qs, &pr.Status)
	if err == nil {
		pr.OpportunityID = id
		d.Proposal = &pr
	}
	s.render(w, d)
}

func (s *Server) handleAddOpp(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	desc := r.FormValue("description")
	skills := r.FormValue("skills")
	if title == "" || desc == "" {
		http.Error(w, "title and description required", 400)
		return
	}
	id := newID("opp")
	fp := imat.Fingerprint("manual", id, title, desc)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.DB.Exec(`INSERT INTO opportunities(id,source,source_opp_id,title,description,skills,budget_type,fingerprint,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		id, "manual", id, title, desc, skills, "unknown", fp, "discovered", now, now)
	if err != nil {
		http.Error(w, "save failed", 500)
		return
	}
	s.analyze(id)
	http.Redirect(w, r, "/o/"+id, http.StatusSeeOther)
}

func (s *Server) analyze(oppID string) {
	var x oppRow
	var skills string
	err := s.db.DB.QueryRow(`SELECT id,source,title,description,status,budget_type,COALESCE(budget_min,0),COALESCE(budget_max,0),COALESCE(connects_cost,0),skills FROM opportunities WHERE id=?`, oppID).
		Scan(&x.ID, &x.Source, &x.Title, &x.Description, &x.Status, &x.BudgetType, &x.BudgetMin, &x.BudgetMax, &x.ConnectsCost, &skills)
	if err != nil {
		return
	}
	p, _ := profile.Load(s.db)
	o := &domain.Opportunity{ID: x.ID, Source: x.Source, Title: x.Title, Description: x.Description, BudgetType: x.BudgetType, BudgetMin: x.BudgetMin, BudgetMax: x.BudgetMax, ConnectsCost: x.ConnectsCost}
	ev := imat.HeuristicEvaluate(p, o)
	if s.eng != nil {
		evs, _ := profile.ListEvidence(s.db)
		ev = s.eng.EnrichEvaluation(p, o, ev, evs)
	}
	b, _ := json.Marshal(ev)
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = s.db.DB.Exec(`INSERT INTO evaluations(id,opportunity_id,data,created_at) VALUES(?,?,?,?)`, newID("ev"), oppID, string(b), now)
	_, _ = s.db.DB.Exec(`UPDATE opportunities SET status='analyzed', updated_at=? WHERE id=?`, now, oppID)
	_, _ = s.db.DB.Exec(`INSERT INTO events(id,kind,detail,created_at) VALUES(?,?,?,?)`, newID("evt"), "analyzed", oppID+":"+ev.Recommendation, now)
}

func (s *Server) generateProposal(w http.ResponseWriter, r *http.Request, oppID string) {
	var x oppRow
	err := s.db.DB.QueryRow(`SELECT id,source,title,description,status FROM opportunities WHERE id=?`, oppID).Scan(&x.ID, &x.Source, &x.Title, &x.Description, &x.Status)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	p, _ := profile.Load(s.db)
	evs, _ := profile.ListEvidence(s.db)
	o := &domain.Opportunity{ID: x.ID, Source: x.Source, Title: x.Title, Description: x.Description}
	var cover string
	var used []string
	var qs []string
	if s.eng != nil {
		cover, used, qs, _ = s.eng.DraftProposal(p, o, evs, p.ProposalStyle)
	} else {
		cover = "No LLM configured."
	}
	evIDs, _ := json.Marshal(used)
	qsB, _ := json.Marshal(qs)
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = s.db.DB.Exec(`INSERT INTO proposals(id,opportunity_id,cover_letter,rate,rate_type,evidence_ids,questions,status,created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		newID("prop"), oppID, cover, p.MinHourlyRate, "hourly", string(evIDs), string(qsB), "draft", now)
	_, _ = s.db.DB.Exec(`UPDATE opportunities SET status='review', updated_at=? WHERE id=?`, now, oppID)
	http.Redirect(w, r, "/o/"+oppID, http.StatusSeeOther)
}

func (s *Server) requestApproval(w http.ResponseWriter, r *http.Request, oppID string) {
	var cover string
	_ = s.db.DB.QueryRow(`SELECT cover_letter FROM proposals WHERE opportunity_id=? ORDER BY created_at DESC LIMIT 1`, oppID).Scan(&cover)
	if cover == "" {
		http.Error(w, "no proposal draft", 400)
		return
	}
	if s.cfg.DryRun {
		http.Error(w, "dry-run mode: external writes disabled", 403)
		return
	}
	_, err := approve.Create(s.db, "manual", "submit_proposal", oppID, truncate(cover, 2000), "high")
	if err != nil {
		http.Error(w, "approval create failed", 500)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = s.db.DB.Exec(`UPDATE opportunities SET status='review', updated_at=? WHERE id=?`, now, oppID)
	http.Redirect(w, r, "/approvals", http.StatusSeeOther)
}

func (s *Server) handleApprovals(w http.ResponseWriter, r *http.Request) {
	d := pageData{Page: "approvals"}
	items, _ := approve.List(s.db, true)
	d.PendingItems = items
	_, p, _ := s.stats()
	d.PendingCount = p
	s.render(w, d)
}

func (s *Server) handleApprovalAction(w http.ResponseWriter, r *http.Request) {
	// /approvals/{id}/approve or /reject
	rest := strings.TrimPrefix(r.URL.Path, "/approvals/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || r.Method != "POST" {
		http.NotFound(w, r)
		return
	}
	id, act := parts[0], parts[1]
	switch act {
	case "approve":
		// Approval records intent; execution happens via explicit run step.
		_ = approve.SetStatus(s.db, id, "approved")
	case "reject":
		_ = approve.SetStatus(s.db, id, "rejected")
	default:
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/approvals", http.StatusSeeOther)
}

func (s *Server) handleApplications(w http.ResponseWriter, r *http.Request) {
	d := pageData{Page: "applications"}
	rows, _ := s.db.DB.Query(`SELECT id,opportunity_id,source,stage,cost_connects,submitted_at FROM applications ORDER BY submitted_at DESC LIMIT 200`)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var a domain.Application
			var ts string
			rows.Scan(&a.ID, &a.OpportunityID, &a.Source, &a.Stage, &a.CostConnects, &ts)
			a.SubmittedAt, _ = time.Parse(time.RFC3339, ts)
			d.Applications = append(d.Applications, a)
		}
	}
	s.render(w, d)
}

func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		p, _ := profile.Load(s.db)
		p.DisplayName = r.FormValue("display_name")
		p.Title = r.FormValue("title")
		p.Summary = r.FormValue("summary")
		p.Skills = splitCSV(r.FormValue("skills"))
		p.Technologies = p.Skills
		p.ExcludedWork = splitCSV(r.FormValue("excluded"))
		p.ProposalStyle = r.FormValue("proposal_style")
		p.MinProjectBudget, _ = strconv.ParseFloat(r.FormValue("min_budget"), 64)
		p.MinHourlyRate, _ = strconv.ParseFloat(r.FormValue("min_hourly"), 64)
		p.MaxConnectsPerApp, _ = strconv.Atoi(r.FormValue("max_connects"))
		if p.MaxAppsPerDay == 0 {
			p.MaxAppsPerDay = 10
		}
		_ = profile.Save(s.db, p)
		http.Redirect(w, r, "/profile", http.StatusSeeOther)
		return
	}
	p, _ := profile.Load(s.db)
	d := pageData{Page: "profile", Profile: p, SkillsStr: strings.Join(p.Skills, ", "), ExcludedStr: strings.Join(p.ExcludedWork, ", ")}
	s.render(w, d)
}

func (s *Server) handleProfileImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/profile", http.StatusSeeOther)
		return
	}
	f, hdr, err := r.FormFile("cv")
	if err != nil {
		http.Error(w, "no file", 400)
		return
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 5<<20))
	if err != nil {
		http.Error(w, "read failed", 500)
		return
	}
	name := hdr.Filename
	if name == "" {
		name = "upload"
	}
	// SSRF/path safety: never trust filename for paths; store content only.
	if _, _, err := profile.ImportDocument(s.db, filepath.Base(name), raw); err != nil {
		http.Error(w, "import failed", 500)
		return
	}
	http.Redirect(w, r, "/profile", http.StatusSeeOther)
}

func (s *Server) handleIntegrations(w http.ResponseWriter, r *http.Request) {
	d := pageData{Page: "integrations"}
	rows, _ := s.db.DB.Query(`SELECT name,kind,endpoint,enabled,capabilities FROM sources ORDER BY name`)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var x srcRow
			var en int
			rows.Scan(&x.Name, &x.Kind, &x.Endpoint, &en, &x.Capabilities)
			x.Enabled = en == 1
			d.Sources = append(d.Sources, x)
		}
	}
	s.render(w, d)
}

func (s *Server) handleIntegrationsAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/integrations", http.StatusSeeOther)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	endpoint := strings.TrimSpace(r.FormValue("endpoint"))
	token := r.FormValue("token")
	if name == "" || endpoint == "" {
		http.Error(w, "name and endpoint required", 400)
		return
	}
	if !strings.HasPrefix(endpoint, "https://") && !strings.HasPrefix(endpoint, "http://localhost") && !strings.HasPrefix(endpoint, "http://127.0.0.1") {
		http.Error(w, "endpoint must be https (or localhost http)", 400)
		return
	}
	en := 1
	_, _ = s.db.DB.Exec(`INSERT OR REPLACE INTO sources(id,name,kind,endpoint,enabled,capabilities) VALUES(?,?,?,?,?,?)`,
		"src-"+strings.ToLower(strings.ReplaceAll(name, " ", "-")), name, "mcp", endpoint, en, "")
	if token != "" {
		// Store encrypted via secrets table by caller-provided key helper.
		_ = saveSecret(s.db, s.cfg, "mcp:"+name, token)
	}
	http.Redirect(w, r, "/integrations", http.StatusSeeOther)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	o, p, a := s.stats()
	b, _ := json.Marshal(map[string]any{"version": version.Version, "opportunities": o, "pending": p, "applications": a, "dry_run": s.cfg.DryRun})
	s.render(w, pageData{Page: "status", StatusJSON: string(b)})
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		pw := r.FormValue("password")
		if s.hasPassword() {
			// Login.
			var h string
			if err := s.db.DB.QueryRow(`SELECT password_hash FROM users WHERE id='admin'`).Scan(&h); err != nil {
				http.Error(w, "no admin set up", 400)
				return
			}
			if bcrypt.CompareHashAndPassword([]byte(h), []byte(pw)) != nil {
				http.Error(w, "invalid password", 401)
				return
			}
			tok := randomToken()
			_, _ = s.db.DB.Exec(`INSERT OR REPLACE INTO app_settings(key,value) VALUES(?,?)`, "session:"+hash(tok), "1")
			http.SetCookie(w, &http.Cookie{Name: "scout_session", Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		if len(pw) < 12 {
			http.Error(w, "password must be at least 12 chars", 400)
			return
		}
		h, _ := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
		_, _ = s.db.DB.Exec(`INSERT OR REPLACE INTO users(id,password_hash,created_at) VALUES('admin',?,?)`, string(h), time.Now().UTC().Format(time.RFC3339))
		tok := randomToken()
		_, _ = s.db.DB.Exec(`INSERT OR REPLACE INTO app_settings(key,value) VALUES(?,?)`, "session:"+hash(tok), "1")
		http.SetCookie(w, &http.Cookie{Name: "scout_session", Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<html><body style="font-family:system-ui;max-width:480px;margin:40px auto"><h1>Scout setup</h1><form method="POST"><label>Admin password (12+ chars)<br><input type="password" name="password" required minlength="12"></label><br><br><button>Set up</button></form></body></html>`))
}

// ---------- JSON API ----------

func (s *Server) apiStatus(w http.ResponseWriter, r *http.Request) {
	o, p, a := s.stats()
	writeJSON(w, map[string]any{"version": version.Version, "opportunities": o, "pending": p, "applications": a})
}

func (s *Server) apiOpportunities(w http.ResponseWriter, r *http.Request) {
	rows, _ := s.db.DB.Query(`SELECT id,source,title,status FROM opportunities ORDER BY updated_at DESC LIMIT 100`)
	var out []map[string]string
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var id, src, title, st string
			rows.Scan(&id, &src, &title, &st)
			out = append(out, map[string]string{"id": id, "source": src, "title": title, "status": st})
		}
	}
	writeJSON(w, out)
}

func (s *Server) apiApprovals(w http.ResponseWriter, r *http.Request) {
	items, _ := approve.List(s.db, true)
	writeJSON(w, items)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
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

func newID(prefix string) string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%s-%s%d", prefix, hex.EncodeToString(b[:]), time.Now().UnixNano()%1000)
}

func randomToken() string {
	var b [24]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// saveSecret encrypts a credential into the secrets table. Never logged.
func saveSecret(db *store.Store, cfg config.Config, key, plaintext string) error {
	mk, err := secret.MasterKey(cfg.DataDir, cfg.ScoutEnvKey)
	if err != nil {
		return err
	}
	ct, err := secret.Encrypt(mk, []byte(plaintext))
	if err != nil {
		return err
	}
	_, err = db.DB.Exec(`INSERT OR REPLACE INTO secrets(key,value,updated_at) VALUES(?,?,?)`, key, ct, time.Now().UTC().Format(time.RFC3339))
	return err
}
