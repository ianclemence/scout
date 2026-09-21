// Command scout is the single binary: web UI, CLI, and MCP server.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ianclemence/scout/internal/agent"
	"github.com/ianclemence/scout/internal/approve"
	"github.com/ianclemence/scout/internal/config"
	"github.com/ianclemence/scout/internal/domain"
	"github.com/ianclemence/scout/internal/httpapi"
	"github.com/ianclemence/scout/internal/llm"
	imat "github.com/ianclemence/scout/internal/match"
	"github.com/ianclemence/scout/internal/mcpclient"
	"github.com/ianclemence/scout/internal/mcpserver"
	"github.com/ianclemence/scout/internal/profile"
	"github.com/ianclemence/scout/internal/secret"
	"github.com/ianclemence/scout/internal/store"
	"github.com/ianclemence/scout/internal/upwork"
	"github.com/ianclemence/scout/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "init":
		must(initCmd())
	case "serve":
		must(serveCmd(os.Args[2:]))
	case "status":
		must(statusCmd())
	case "profile":
		must(profileCmd(os.Args[2:]))
	case "integrations":
		must(integrationsCmd(os.Args[2:]))
	case "opportunities", "opps":
		must(oppsCmd(os.Args[2:]))
	case "analyze":
		must(analyzeCmd(os.Args[2:]))
	case "proposal":
		must(proposalCmd(os.Args[2:]))
	case "applications", "apps":
		must(appsCmd())
	case "approvals":
		must(approvalsCmd(os.Args[2:]))
	case "run":
		must(runCmd(os.Args[2:]))
	case "config":
		must(configCmd())
	case "doctor":
		must(doctorCmd())
	case "backup":
		must(backupCmd(os.Args[2:]))
	case "restore":
		must(restoreCmd(os.Args[2:]))
	case "mcp":
		must(mcpCmd(os.Args[2:]))
	case "version", "--version", "-v":
		fmt.Println("scout", version.Version)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Println(`scout — self-hosted AI work acquisition agent

  scout init                          prepare data dir + database
  scout serve [--addr 127.0.0.1:3210] run web UI + API + Scout MCP (HTTP)
  scout status                        local counts
  scout profile show                  show profile summary
  scout profile import <cv.txt|md|pdf> import CV into profile + evidence
  scout integrations [list|add|test]  manage MCP connectors
  scout opportunities [list|show ID]  list / show opportunities
  scout analyze <opp-id>              run match evaluation
  scout proposal <opp-id>             draft a proposal (no external writes)
  scout applications                  show pipeline
  scout approvals [list|approve|reject] human approval queue
  scout run discovery [--dry-run]     discovery placeholder (manual sources)
  scout config                        show effective config (secrets redacted)
  scout doctor                        environment + dependency checks
  scout backup <file>                 backup database (secrets excluded by default)
  scout restore <file>                restore database backup
  scout mcp [stdio|serve]             run Scout MCP server (stdio default)
  scout version                       print version`)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func openStore() (*store.Store, config.Config, error) {
	cfg := config.Default()
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return nil, cfg, err
	}
	return db, cfg, nil
}

func engineFor(cfg config.Config, db *store.Store, role string) *agent.Engine {
	r, ok := cfg.Models[role]
	if !ok {
		r = cfg.Models["analysis"]
	}
	p, err := providerFromEnv(db, cfg, r.Provider, r.Model)
	if err != nil || p == nil {
		return &agent.Engine{}
	}
	return &agent.Engine{LLM: p}
}

func providerFromEnv(db *store.Store, cfg config.Config, provider, model string) (llm.Provider, error) {
	lcfg := llm.Config{Provider: provider, Model: model, Endpoint: cfg.OllamaHost}
	switch provider {
	case "openai":
		lcfg.APIKey = os.Getenv("OPENAI_API_KEY")
		lcfg.Endpoint = "https://api.openai.com/v1"
	case "deepseek":
		lcfg.APIKey = os.Getenv("DEEPSEEK_API_KEY")
		lcfg.Endpoint = "https://api.deepseek.io"
		if model == "" {
			lcfg.Model = "deepseek-chat"
		}
	case "anthropic":
		lcfg.APIKey = os.Getenv("ANTHROPIC_API_KEY")
	case "openai_compatible":
		lcfg.Endpoint = os.Getenv("OPENAI_COMPAT_ENDPOINT")
		lcfg.APIKey = os.Getenv("OPENAI_COMPAT_KEY")
	case "ollama":
		lcfg.Endpoint = cfg.OllamaHost
	}
	if provider == "openai" || provider == "anthropic" || provider == "deepseek" {
		if sec, err := loadSecret(db, cfg, "llm:"+provider); err == nil && sec != "" {
			lcfg.APIKey = sec
		}
	}
	if (provider == "openai" || provider == "anthropic" || provider == "deepseek") && lcfg.APIKey == "" {
		return nil, fmt.Errorf("no API key for %s", provider)
	}
	return llm.New(lcfg)
}

func loadSecret(db *store.Store, cfg config.Config, key string) (string, error) {
	var blob []byte
	if err := db.DB.QueryRow(`SELECT value FROM secrets WHERE key=?`, key).Scan(&blob); err != nil {
		return "", err
	}
	mk, err := secret.MasterKey(cfg.DataDir, cfg.ScoutEnvKey)
	if err != nil {
		return "", err
	}
	pt, err := secret.Decrypt(mk, blob)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

func initCmd() error {
	cfg := config.Default()
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return err
	}
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := secret.MasterKey(cfg.DataDir, cfg.ScoutEnvKey); err != nil {
		return err
	}
	_, _ = db.DB.Exec(`INSERT OR IGNORE INTO sources(id,name,kind,endpoint,enabled,capabilities) VALUES('src-upwork','Upwork','mcp',?,1,'')`, upwork.Endpoint)
	fmt.Println("initialized", cfg.DataDir)
	return nil
}

func serveCmd(args []string) error {
	addr := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--addr" && i+1 < len(args) {
			addr = args[i+1]
		}
	}
	db, cfg, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	if addr == "" {
		addr = cfg.Addr
	}
	tpl := loadTemplates()
	eng := engineFor(cfg, db, "proposal")
	mk, _ := secret.MasterKey(cfg.DataDir, cfg.ScoutEnvKey)
	srv := httpapi.New(cfg, db, eng, tpl, mk)
	mux := http.NewServeMux()
	staticDir := findWebStatic()
	if staticDir != "" {
		mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))
	}
	mux.Handle("/", srv)
	fmt.Printf("scout %s listening on http://%s\n", version.Version, addr)
	return http.ListenAndServe(addr, mux)
}

func statusCmd() error {
	db, _, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	var o, p, a int
	_ = db.DB.QueryRow(`SELECT COUNT(*) FROM opportunities`).Scan(&o)
	_ = db.DB.QueryRow(`SELECT COUNT(*) FROM pending_actions WHERE status IN ('draft','pending_approval')`).Scan(&p)
	_ = db.DB.QueryRow(`SELECT COUNT(*) FROM applications`).Scan(&a)
	fmt.Printf("scout %s opportunities=%d pending=%d applications=%d\n", version.Version, o, p, a)
	return nil
}

func profileCmd(args []string) error {
	db, _, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	if len(args) == 0 || args[0] == "show" {
		p, _ := profile.Load(db)
		b, _ := json.MarshalIndent(p, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	if args[0] == "import" && len(args) == 2 {
		raw, err := os.ReadFile(args[1])
		if err != nil {
			return err
		}
		p, ev, err := profile.ImportDocument(db, filepath.Base(args[1]), raw)
		if err != nil {
			return err
		}
		fmt.Printf("imported %s: %d skills, %d evidence\n", p.DisplayName, len(p.Skills), len(ev))
		return nil
	}
	return fmt.Errorf("usage: scout profile [show|import <file>]")
}

func integrationsCmd(args []string) error {
	db, cfg, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "list":
		rows, _ := db.DB.Query(`SELECT name,kind,endpoint,enabled FROM sources`)
		defer rows.Close()
		for rows.Next() {
			var n, k, e string
			var en int
			rows.Scan(&n, &k, &e, &en)
			fmt.Printf("%s\t%s\t%s\tenabled=%d\n", n, k, e, en)
		}
	case "add":
		// scout integrations add <name> <endpoint>
		if len(args) < 3 {
			return fmt.Errorf("usage: scout integrations add <name> <endpoint>")
		}
		_, err = db.DB.Exec(`INSERT OR REPLACE INTO sources(id,name,kind,endpoint,enabled,capabilities) VALUES(?,?,?,?,1,'')`,
			"src-"+strings.ToLower(args[1]), args[1], "mcp", args[2])
		fmt.Println("added", args[1])
		return err
	case "test":
		name := "Upwork"
		if len(args) > 1 {
			name = args[1]
		}
		var endpoint string
		err := db.DB.QueryRow(`SELECT endpoint FROM sources WHERE name=?`, name).Scan(&endpoint)
		if err != nil {
			return fmt.Errorf("source %q not found", name)
		}
		tok, _ := loadSecret(db, cfg, "mcp:"+name)
		c := &mcpclient.Connector{ID: name, Endpoint: endpoint, Token: tok}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		tools, err := c.ListTools(ctx)
		if err != nil {
			return fmt.Errorf("capability discovery failed: %w", err)
		}
		caps := upwork.Discover(tools)
		fmt.Printf("%s: %d tools, capabilities=%v\n", name, len(tools), caps)
		for _, t := range tools {
			fmt.Printf("  - %s\n", t.Name)
		}
		// Persist discovered capabilities.
		cb, _ := json.Marshal(caps)
		_, _ = db.DB.Exec(`UPDATE sources SET capabilities=? WHERE name=?`, string(cb), name)
	default:
		return fmt.Errorf("usage: scout integrations [list|add|test]")
	}
	return nil
}

func oppsCmd(args []string) error {
	db, _, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	if len(args) >= 2 && args[0] == "show" {
		var t, d, st, src string
		err := db.DB.QueryRow(`SELECT title,description,status,source FROM opportunities WHERE id=?`, args[1]).Scan(&t, &d, &st, &src)
		if err == sql.ErrNoRows {
			return fmt.Errorf("not found")
		}
		fmt.Printf("[%s] %s (%s)\n\n%s\n", src, t, st, d)
		var ev string
		_ = db.DB.QueryRow(`SELECT data FROM evaluations WHERE opportunity_id=? ORDER BY created_at DESC LIMIT 1`, args[1]).Scan(&ev)
		if ev != "" {
			fmt.Printf("\n--- evaluation ---\n%s\n", ev)
		}
		return err
	}
	rows, _ := db.DB.Query(`SELECT id,source,title,status FROM opportunities ORDER BY updated_at DESC LIMIT 100`)
	defer rows.Close()
	for rows.Next() {
		var id, src, t, st string
		rows.Scan(&id, &src, &t, &st)
		fmt.Printf("%s\t[%s] %s (%s)\n", id, src, t, st)
	}
	return nil
}

func analyzeCmd(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: scout analyze <opp-id>")
	}
	db, cfg, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	var x struct {
		id, src, title, desc, bt string
		bmin, bmax               float64
		conn                     int
	}
	err = db.DB.QueryRow(`SELECT id,source,title,description,budget_type,COALESCE(budget_min,0),COALESCE(budget_max,0),COALESCE(connects_cost,0) FROM opportunities WHERE id=?`, args[0]).
		Scan(&x.id, &x.src, &x.title, &x.desc, &x.bt, &x.bmin, &x.bmax, &x.conn)
	if err != nil {
		return fmt.Errorf("opportunity %s: %w", args[0], err)
	}
	p, _ := profile.Load(db)
	o := &domain.Opportunity{ID: x.id, Source: x.src, Title: x.title, Description: x.desc, BudgetType: x.bt, BudgetMin: x.bmin, BudgetMax: x.bmax, ConnectsCost: x.conn}
	f := imat.DeterministicFilter(p, o)
	fmt.Printf("filter: pass=%v reason=%s\n", f.Pass, f.Reason)
	ev := imat.HeuristicEvaluate(p, o)
	eng := engineFor(cfg, db, "analysis")
	evs, _ := profile.ListEvidence(db)
	ev = eng.EnrichEvaluation(p, o, ev, evs)
	b, _ := json.MarshalIndent(ev, "", "  ")
	fmt.Println(string(b))
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = db.DB.Exec(`INSERT INTO evaluations(id,opportunity_id,data,created_at) VALUES(?,?,?,?)`, fmt.Sprintf("ev-%d", time.Now().UnixNano()), x.id, string(b), now)
	_, _ = db.DB.Exec(`UPDATE opportunities SET status='analyzed', updated_at=? WHERE id=?`, now, x.id)
	return nil
}

func proposalCmd(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: scout proposal <opp-id>")
	}
	db, cfg, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	var title, desc, src string
	if err := db.DB.QueryRow(`SELECT title,description,source FROM opportunities WHERE id=?`, args[0]).Scan(&title, &desc, &src); err != nil {
		return fmt.Errorf("not found")
	}
	p, _ := profile.Load(db)
	evs, _ := profile.ListEvidence(db)
	o := &domain.Opportunity{ID: args[0], Source: src, Title: title, Description: desc}
	eng := engineFor(cfg, db, "proposal")
	cover, used, qs, _ := eng.DraftProposal(p, o, evs, p.ProposalStyle)
	evIDs, _ := json.Marshal(used)
	qsB, _ := json.Marshal(qs)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = db.DB.Exec(`INSERT INTO proposals(id,opportunity_id,cover_letter,rate,rate_type,evidence_ids,questions,status,created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		fmt.Sprintf("prop-%d", time.Now().UnixNano()), args[0], cover, p.MinHourlyRate, "hourly", string(evIDs), string(qsB), "draft", now)
	if err != nil {
		return err
	}
	_, _ = db.DB.Exec(`UPDATE opportunities SET status='review', updated_at=? WHERE id=?`, now, args[0])
	fmt.Println(cover)
	fmt.Printf("\n[questions: %s]\n(draft saved, no external writes)\n", strings.Join(qs, " | "))
	return nil
}

func appsCmd() error {
	db, _, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	rows, _ := db.DB.Query(`SELECT id,opportunity_id,source,stage,cost_connects FROM applications ORDER BY submitted_at DESC LIMIT 100`)
	defer rows.Close()
	for rows.Next() {
		var id, oid, src, stage string
		var cc int
		rows.Scan(&id, &oid, &src, &stage, &cc)
		fmt.Printf("%s\t%s\t%s\t%s\tconnects=%d\n", id, oid, src, stage, cc)
	}
	return nil
}

func approvalsCmd(args []string) error {
	db, _, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "list":
		items, _ := approve.List(db, len(args) > 1 && args[1] == "pending")
		for _, a := range items {
			fmt.Printf("%s\t%s\t%s\t%s\trisk=%s\n", a.ID, a.Status, a.ActionType, a.Target, a.RiskLevel)
		}
	case "approve":
		if len(args) < 2 {
			return fmt.Errorf("usage: scout approvals approve <id>")
		}
		return approve.SetStatus(db, args[1], "approved")
	case "reject":
		if len(args) < 2 {
			return fmt.Errorf("usage: scout approvals reject <id>")
		}
		return approve.SetStatus(db, args[1], "rejected")
	default:
		return fmt.Errorf("usage: scout approvals [list|approve|reject]")
	}
	return nil
}

func runCmd(args []string) error {
	dry := false
	for _, a := range args {
		if a == "--dry-run" {
			dry = true
		}
	}
	db, cfg, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	if cfg.DryRun {
		dry = true
	}
	id := fmt.Sprintf("run-%d", time.Now().UnixNano())
	now := time.Now().UTC()
	_, _ = db.DB.Exec(`INSERT INTO agent_runs(id,kind,status,summary,dry_run,started_at,ended_at) VALUES(?,?,'running','',?, ?,?)`,
		id, "discovery", boolToInt(dry), now.Format(time.RFC3339), now.Format(time.RFC3339))
	var total, cands int
	_ = db.DB.QueryRow(`SELECT COUNT(*) FROM opportunities`).Scan(&total)
	type cand struct {
		oid, t, d, bt string
		bmin, bmax    float64
		cc            int
	}
	var candsRows []cand
	func() {
		rows, err := db.DB.Query(`SELECT id,title,description,budget_type,COALESCE(budget_min,0),COALESCE(budget_max,0),COALESCE(connects_cost,0) FROM opportunities WHERE status IN ('discovered','analyzed')`)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var c cand
			if err := rows.Scan(&c.oid, &c.t, &c.d, &c.bt, &c.bmin, &c.bmax, &c.cc); err == nil {
				candsRows = append(candsRows, c)
			}
		}
	}()
	p, _ := profile.Load(db)
	for _, c := range candsRows {
		o := &domain.Opportunity{ID: c.oid, Title: c.t, Description: c.d, BudgetType: c.bt, BudgetMin: c.bmin, BudgetMax: c.bmax, ConnectsCost: c.cc}
		if f := imat.DeterministicFilter(p, o); f.Pass {
			cands++
		}
	}
	summary := fmt.Sprintf("discovered=%d candidates=%d dry_run=%v (no external writes)", total, cands, dry)
	_, _ = db.DB.Exec(`UPDATE agent_runs SET status='done', summary=?, ended_at=? WHERE id=?`, summary, time.Now().UTC().Format(time.RFC3339), id)
	fmt.Println(summary)
	return nil
}

func configCmd() error {
	cfg := config.Default()
	fmt.Printf("data_dir=%s\ndb=%s\naddr=%s\nollama=%s\ndry_run=%v\nmodels=%v\n",
		cfg.DataDir, cfg.DBPath, cfg.Addr, cfg.OllamaHost, cfg.DryRun, cfg.Models)
	fmt.Println("(secrets redacted; set OPENAI_API_KEY / ANTHROPIC_API_KEY / OLLAMA_HOST / SCOUT_MASTER_KEY as env)")
	return nil
}

func doctorCmd() error {
	fmt.Println("scout doctor")
	cfg := config.Default()
	check := func(name string, ok bool, detail string) {
		s := "OK"
		if !ok {
			s = "MISSING"
		}
		fmt.Printf("  [%s] %s %s\n", s, name, detail)
	}
	_, err := os.Stat(cfg.DataDir)
	check("data-dir", err == nil, cfg.DataDir)
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		check("sqlite", false, err.Error())
	} else {
		check("sqlite", true, cfg.DBPath)
		db.Close()
	}
	check("ollama", ollamaUp(cfg.OllamaHost), cfg.OllamaHost)
	check("openai-key", os.Getenv("OPENAI_API_KEY") != "", "env OPENAI_API_KEY")
	check("anthropic-key", os.Getenv("ANTHROPIC_API_KEY") != "", "env ANTHROPIC_API_KEY")
	check("deepseek-key", os.Getenv("DEEPSEEK_API_KEY") != "", "env DEEPSEEK_API_KEY")
	return nil
}

func ollamaUp(host string) bool {
	c := http.Client{Timeout: 5 * time.Second}
	r, err := c.Get(strings.TrimSuffix(host, "/") + "/api/tags")
	if err != nil {
		return false
	}
	defer r.Body.Close()
	return r.StatusCode < 500
}

func backupCmd(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: scout backup <file>")
	}
	_, cfg, err := openStore()
	if err != nil {
		return err
	}
	// SQLite backup via VACUUM INTO (excludes WAL artifacts).
	db2, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db2.Close()
	if _, err := db2.DB.Exec(`VACUUM INTO ?`, args[0]); err != nil {
		return err
	}
	fmt.Println("backup written to", args[0], "(secrets table included — keep private; see README)")
	return nil
}

func restoreCmd(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: scout restore <file>")
	}
	cfg := config.Default()
	src, err := os.Open(args[0])
	if err != nil {
		return err
	}
	defer src.Close()
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o700); err != nil {
		return err
	}
	dst, err := os.OpenFile(cfg.DBPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	if err == nil {
		fmt.Println("restored to", cfg.DBPath)
	}
	return err
}

func mcpCmd(args []string) error {
	sub := "stdio"
	if len(args) > 0 {
		sub = args[0]
	}
	db, cfg, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	deps := mcpserver.Deps{Store: db}
	switch sub {
	case "stdio":
		return mcpserver.RunStdio(deps)
	case "serve":
		addr := cfg.Addr
		if len(args) > 1 {
			addr = args[1]
		}
		fmt.Fprintf(os.Stderr, "scout MCP (Streamable HTTP) on http://%s/mcp\n", addr)
		return http.ListenAndServe(addr, mcpserver.Handler(deps))
	default:
		return fmt.Errorf("usage: scout mcp [stdio|serve]")
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func loadTemplates() *template.Template {
	for _, p := range []string{
		"web/templates/app.html",
		filepath.Join(exeDir(), "web/templates/app.html"),
		"/home/ianclemence/scout/web/templates/app.html",
	} {
		if _, err := os.Stat(p); err == nil {
			return template.Must(template.ParseFiles(p))
		}
	}
	return template.Must(template.New("app").Parse(`<html><body>templates missing</body></html>`))
}

func findWebStatic() string {
	for _, p := range []string{
		"web/static",
		filepath.Join(exeDir(), "web/static"),
		"/home/ianclemence/scout/web/static",
	} {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
	}
	return ""
}

func exeDir() string {
	ex, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(ex)
}
