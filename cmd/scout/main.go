// Command scout — terminal-native AI work acquisition agent.
// Bare `scout` enters the interactive session; subcommands are scriptable.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ianclemence/scout/internal/config"
	"github.com/ianclemence/scout/internal/csession"
	"github.com/ianclemence/scout/internal/isession"
	"github.com/ianclemence/scout/internal/llm"
	"github.com/ianclemence/scout/internal/mcpclient"
	"github.com/ianclemence/scout/internal/mcpserver"
	"github.com/ianclemence/scout/internal/profile"
	"github.com/ianclemence/scout/internal/runtime"
	"github.com/ianclemence/scout/internal/secret"
	"github.com/ianclemence/scout/internal/skills"
	"github.com/ianclemence/scout/internal/store"
	"github.com/ianclemence/scout/internal/tui"
	"github.com/ianclemence/scout/internal/upwork"
	"github.com/ianclemence/scout/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		must(runInteractive(""))
		return
	}
	cmd, rest := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "init":
		err = initCmd()
	case "status":
		err = withCore(func(c *runtime.Core) error { return statusCmd(c) })
	case "discover":
		err = withCore(func(c *runtime.Core) error { return discoverCmd(c, rest) })
	case "opportunities", "opps":
		err = withCore(func(c *runtime.Core) error { return oppsCmd(c, rest) })
	case "opportunity", "opp":
		err = withCore(func(c *runtime.Core) error { return oppCmd(c, rest) })
	case "analyze":
		err = withCore(func(c *runtime.Core) error { return analyzeCmd(c, rest) })
	case "proposal":
		err = withCore(func(c *runtime.Core) error { return proposalCmd(c, rest) })
	case "approvals":
		err = withCore(func(c *runtime.Core) error { return approvalsCmd(c, rest) })
	case "applications", "apps", "pipeline":
		err = withCore(func(c *runtime.Core) error { return appsCmd(c) })
	case "inbox", "messages":
		err = withCore(func(c *runtime.Core) error { return inboxCmd(c) })
	case "profile":
		err = withCore(func(c *runtime.Core) error { return profileCmd(c, rest) })
	case "providers", "models":
		err = withCore(func(c *runtime.Core) error { return modelsCmd(c, rest) })
	case "login":
		err = withCore(func(c *runtime.Core) error { return loginCmd(c, rest) })
	case "integrations", "sources":
		err = withCore(func(c *runtime.Core) error { return integrationsCmd(c, rest) })
	case "sessions":
		err = withCore(func(c *runtime.Core) error { return sessionsCmd(c, rest) })
	case "skills":
		err = withCore(func(c *runtime.Core) error { return skillsCmd(c, rest) })
	case "tools":
		err = withCore(func(c *runtime.Core) error { return toolsCmd(c) })
	case "resume":
		must(runInteractive(firstArg(rest)))
		return
	case "ask":
		err = withCore(func(c *runtime.Core) error { return askCmd(c, rest) })
	case "run":
		err = withCore(func(c *runtime.Core) error { return runCmd(c, rest) })
	case "config":
		err = configCmd()
	case "doctor":
		err = withCore(func(c *runtime.Core) error { return doctorCmd(c) })
	case "backup":
		err = withCore(func(c *runtime.Core) error { return backupCmd(c, rest) })
	case "update":
		err = updateCmd(rest)
	case "restore":
		err = restoreCmd(rest)
	case "mcp":
		err = withCore(func(c *runtime.Core) error { return mcpCmd(c, rest) })
	case "version", "--version", "-v":
		fmt.Println("scout", version.Version)
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q — scout help\n", cmd)
		os.Exit(2)
	}
	must(err)
}

func usage() {
	fmt.Println(`scout — terminal-native AI work acquisition agent

  scout                        interactive session (resume with: scout resume <id>)
  scout ask "question"         one-shot agent turn (scriptable, --json for JSON)
  scout status                 counts + session-relevant state
  scout discover [--dry-run]   discovery summary (no external writes)
  scout opportunities [query]  list opportunities
  scout opportunity show <id>  full detail + evaluation + proposal
  scout opportunity add --title T --description-file F [--skills s]
  scout analyze <id>           filter + match evaluation
  scout proposal <id>          draft proposal (no external writes)
  scout approvals [list|approve <id>|reject <id>]
  scout applications           pipeline applications
  scout inbox                  stored messages
  scout profile show|import <file>
  scout providers              provider availability
  scout models                 model roles
  scout login <provider>       store API key (masked prompt)
  scout integrations [list|add|test]
  scout sessions [list]        persistent sessions
  scout skills [query]         agent skill registry
  scout tools                  tool registry with permission classes
  scout run discovery          planned discovery run (drafts only)
  scout config                 effective config (secrets redacted)
  scout doctor                 diagnostics for Raspberry Pi troubleshooting
  scout backup <file>          backup database
  scout update [--dry-run] [--force]  pull, rebuild, reinstall, restart service
  scout restore <file>         restore database backup
  scout mcp [stdio|serve]      Scout MCP server for OpenCode/Codex/Claude
  scout version`)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func secretMasterKey(cfg config.Config) ([]byte, error) {
	return secret.MasterKey(cfg.DataDir, cfg.ScoutEnvKey)
}

func firstArg(a []string) string {
	if len(a) > 0 {
		return a[0]
	}
	return ""
}

func openCore() (*runtime.Core, error) {
	cfg := config.Load()
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	return runtime.New(cfg, db)
}

func withCore(fn func(*runtime.Core) error) error {
	c, err := openCore()
	if err != nil {
		return err
	}
	defer c.DB.Close()
	return fn(c)
}

// ---------- interactive ----------

func runInteractive(resumeRef string) error {
	c, err := openCore()
	if err != nil {
		return err
	}
	defer c.DB.Close()
	var sess *csession.Session
	if resumeRef != "" {
		sess, err = c.ResolveSession(resumeRef)
		if err != nil {
			return err
		}
		fmt.Printf("Resumed session %s (%s).\n", sess.ID[:12], sess.Name)
	} else {
		sess, err = csession.Create(c.DB, "interactive", c.Cfg.Models["conversation"].Provider, c.Cfg.Models["conversation"].Model)
		if err != nil {
			return err
		}
	}
	st := &isession.ReplState{Core: c, Sess: sess}
	if msgs, err := csession.LoadMessages(c.DB, sess.ID, 20); err == nil {
		for _, m := range msgs {
			st.History = append(st.History, llm.Message{Role: m.Role, Content: m.Content})
		}
	}
	if isTerminal() {
		return tui.Run(st)
	}
	return isession.Run(c, sess)
}

func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// ---------- one-shot commands ----------

func initCmd() error {
	cfg := config.Load()
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return err
	}
	c, err := openCore()
	if err != nil {
		return err
	}
	defer c.DB.Close()
	if _, err := secretMasterKey(cfg); err != nil {
		return err
	}
	_, _ = c.DB.DB.Exec(`INSERT OR IGNORE INTO sources(id,name,kind,endpoint,enabled,capabilities) VALUES('src-upwork','Upwork','mcp',?,1,'')`, upwork.Endpoint)
	fmt.Println("initialized", cfg.DataDir)
	return nil
}

func statusCmd(c *runtime.Core) error {
	var o, p, a int
	_ = c.DB.DB.QueryRow(`SELECT COUNT(*) FROM opportunities`).Scan(&o)
	_ = c.DB.DB.QueryRow(`SELECT COUNT(*) FROM pending_actions WHERE status IN ('draft','pending_approval')`).Scan(&p)
	_ = c.DB.DB.QueryRow(`SELECT COUNT(*) FROM applications`).Scan(&a)
	pr, _ := c.Profile()
	fmt.Printf("scout %s · profile %s (%d skills) · opportunities=%d pending=%d applications=%d\n",
		version.Version, pr.DisplayName, len(pr.Skills), o, p, a)
	return nil
}

func discoverCmd(c *runtime.Core, args []string) error {
	dry := true
	for _, a := range args {
		if a == "--live" {
			dry = false
		}
	}
	s, err := c.RunDiscovery(dry)
	if err != nil {
		return err
	}
	fmt.Printf("discovered=%d candidates=%d dry_run=%v (no external writes)\n", s.Total, s.Candidates, s.DryRun)
	return nil
}

func oppsCmd(c *runtime.Core, args []string) error {
	opps, err := c.ListOpportunities(runtime.OpportunityFilter{Query: strings.Join(args, " "), Limit: 50})
	if err != nil {
		return err
	}
	for _, o := range opps {
		fmt.Printf("%s\t[%s] %s (%s)\n", o.ID, o.Source, o.Title, o.Status)
	}
	return nil
}

func resolveID(c *runtime.Core, ref string) (string, error) {
	var id string
	err := c.DB.DB.QueryRow(`SELECT id FROM opportunities WHERE id=? OR id LIKE ? ORDER BY updated_at DESC LIMIT 1`, ref, ref+"%").Scan(&id)
	if err != nil {
		return "", fmt.Errorf("opportunity %q not found", ref)
	}
	return id, nil
}

func oppCmd(c *runtime.Core, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: scout opportunity <show|add> ...")
	}
	switch args[0] {
	case "show":
		if len(args) < 2 {
			return fmt.Errorf("usage: scout opportunity show <id>")
		}
		id, err := resolveID(c, args[1])
		if err != nil {
			return err
		}
		o, err := c.GetOpportunity(id)
		if err != nil {
			return err
		}
		fmt.Printf("[%s] %s (%s)\nbudget %s %.0f–%.0f · credits %d\n\n%s\n", o.Source, o.Title, o.Status,
			o.BudgetType, o.BudgetMin, o.BudgetMax, o.ConnectsCost, o.Description)
		if ev, err := c.LatestEvaluation(id); err == nil {
			b, _ := json.MarshalIndent(ev, "", "  ")
			fmt.Printf("\n--- evaluation ---\n%s\n", b)
		}
		if pr, err := c.LatestProposal(id); err == nil {
			fmt.Printf("\n--- proposal (%s) ---\n%s\n", pr.Status, pr.CoverLetter)
		}
		return nil
	case "add":
		var title, descFile, skills string
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--title":
				i++
				if i < len(args) {
					title = args[i]
				}
			case "--description-file":
				i++
				if i < len(args) {
					descFile = args[i]
				}
			case "--skills":
				i++
				if i < len(args) {
					skills = args[i]
				}
			}
		}
		if descFile == "" {
			return fmt.Errorf("usage: scout opportunity add --title T --description-file F [--skills s]")
		}
		raw, err := os.ReadFile(descFile)
		if err != nil {
			return err
		}
		o, err := c.AddOpportunity(title, string(raw), skills)
		if err != nil {
			return err
		}
		fmt.Printf("added %s\n", o.ID)
		return nil
	default:
		return fmt.Errorf("usage: scout opportunity <show|add> ...")
	}
}

func analyzeCmd(c *runtime.Core, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: scout analyze <opp-id>")
	}
	id, err := resolveID(c, args[0])
	if err != nil {
		return err
	}
	ev, f, err := c.Analyze(context.Background(), id, c.EngineForRole("analysis"))
	if err != nil {
		return err
	}
	fmt.Printf("filter: pass=%v reason=%s\n", f.Pass, f.Reason)
	b, _ := json.MarshalIndent(ev, "", "  ")
	fmt.Println(string(b))
	return nil
}

func proposalCmd(c *runtime.Core, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: scout proposal <opp-id>")
	}
	id, err := resolveID(c, args[0])
	if err != nil {
		return err
	}
	pr, err := c.DraftProposal(context.Background(), id, c.EngineForRole("proposal"))
	if err != nil {
		return err
	}
	fmt.Println(pr.CoverLetter)
	fmt.Printf("\n[questions: %s]\n(draft %s saved, no external writes)\n", strings.Join(pr.Questions, " | "), pr.ID)
	return nil
}

func approvalsCmd(c *runtime.Core, args []string) error {
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "list", "pending":
		acts, err := c.PendingApprovals()
		if err != nil {
			return err
		}
		for _, a := range acts {
			fmt.Printf("%s\t%s\t%s\t%s\trisk=%s\n", a.ID, a.Status, a.ActionType, a.Target, a.RiskLevel)
		}
	case "approve", "reject":
		if len(args) < 2 {
			return fmt.Errorf("usage: scout approvals %s <id>", sub)
		}
		status := "approved"
		if sub == "reject" {
			status = "rejected"
		}
		return c.SetApprovalStatus(args[1], status)
	default:
		return fmt.Errorf("usage: scout approvals [list|approve|reject]")
	}
	return nil
}

func appsCmd(c *runtime.Core) error {
	apps, err := c.ListApplications(100)
	if err != nil {
		return err
	}
	for _, a := range apps {
		fmt.Printf("%s\t%s\t%s\t%s\tconnects=%d\n", a.ID, a.OpportunityID, a.Source, a.Stage, a.CostConnects)
	}
	pipe, _ := c.Pipeline()
	if len(pipe) > 0 {
		fmt.Printf("pipeline: %v\n", pipe)
	}
	return nil
}

func inboxCmd(c *runtime.Core) error {
	tool := c.FindTool("list_messages")
	out, err := tool.Handler(context.Background(), map[string]any{})
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}

func profileCmd(c *runtime.Core, args []string) error {
	if len(args) == 0 || args[0] == "show" {
		p, err := c.Profile()
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(p, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	if args[0] == "import" && len(args) == 2 {
		raw, err := os.ReadFile(args[1])
		if err != nil {
			return err
		}
		p, ev, err := profile.ImportDocument(c.DB, filepath.Base(args[1]), raw)
		if err != nil {
			return err
		}
		fmt.Printf("imported %s: %d skills, %d evidence\n", p.DisplayName, len(p.Skills), len(ev))
		return nil
	}
	return fmt.Errorf("usage: scout profile [show|import <file>]")
}

func providersCmd(c *runtime.Core) error {
	fmt.Printf("%-16s %-10s %-6s %s\n", "PROVIDER", "CONFIGURED", "MODELS", "DETAIL / ROLES")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, p := range c.ProviderStatus(ctx) {
		mark := "no"
		if p.Configured {
			mark = "yes"
		}
		roles := ""
		if len(p.Roles) > 0 {
			roles = " [" + strings.Join(p.Roles, ",") + "]"
		}
		fmt.Printf("%-16s %-10s %-6d %s%s\n", p.Provider, mark, p.Models, p.Detail, roles)
	}
	fmt.Printf("\nroles:\n")
	for _, role := range []string{"screening", "analysis", "proposal", "conversation", "deep_analysis"} {
		r := c.Cfg.Models[role]
		fmt.Printf("  %-13s %s/%s\n", role, r.Provider, r.Model)
	}
	return nil
}

// modelsCmd lists the registry catalog; `scout models refresh [provider]`.
func modelsCmd(c *runtime.Core, args []string) error {
	if len(args) > 0 && args[0] == "refresh" {
		prov := ""
		if len(args) > 1 {
			prov = args[1]
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		n, err := c.Registry().Refresh(ctx, prov)
		fmt.Printf("refreshed %d models\n", n)
		return err
	}
	if len(args) == 0 {
		return providersCmd(c)
	}
	return fmt.Errorf("usage: scout models [refresh [provider]]")
}

func loginCmd(c *runtime.Core, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: scout login <openai|anthropic|deepseek|moonshot>")
	}
	p := strings.ToLower(args[0])
	switch p {
	case "openai", "anthropic", "deepseek", "moonshot":
	default:
		return fmt.Errorf("unknown provider %q", p)
	}
	fmt.Printf("%s API key: ", p)
	key, err := readPassword()
	if err != nil || strings.TrimSpace(key) == "" {
		return fmt.Errorf("no key entered")
	}
	if err := c.SaveSecret("llm:"+p, strings.TrimSpace(key)); err != nil {
		return err
	}
	fmt.Println("stored (encrypted).")
	return nil
}

func integrationsCmd(c *runtime.Core, args []string) error {
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "list":
		srcs, err := c.ListSources()
		if err != nil {
			return err
		}
		for _, s := range srcs {
			fmt.Printf("%s\t%s\t%s\tenabled=%v\tcaps=%v\n", s.Name, s.Kind, s.Endpoint, s.Enabled, s.Capabilities)
		}
	case "add":
		if len(args) < 3 {
			return fmt.Errorf("usage: scout integrations add <name> <endpoint> | scout integrations add <name> --command \"prog args...\"")
		}
		if len(args) >= 3 && args[1] == "--command" {
			return fmt.Errorf("usage: scout integrations add <name> --command \"prog args...\"")
		}
		if args[2] == "--command" {
			if len(args) < 4 {
				return fmt.Errorf("usage: scout integrations add <name> --command \"prog args...\"")
			}
			_, err := c.DB.DB.Exec(`INSERT OR REPLACE INTO sources(id,name,kind,endpoint,command,enabled,capabilities) VALUES(?,?,?,?,?,1,'')`,
				"src-"+strings.ToLower(strings.ReplaceAll(args[1], " ", "-")), args[1], "mcp-stdio", "", args[3])
			fmt.Println("added stdio source", args[1])
			return err
		}
		if !strings.HasPrefix(args[2], "https://") && !strings.HasPrefix(args[2], "http://localhost") && !strings.HasPrefix(args[2], "http://127.0.0.1") {
			return fmt.Errorf("endpoint must be https (or localhost http)")
		}
		_, err := c.DB.DB.Exec(`INSERT OR REPLACE INTO sources(id,name,kind,endpoint,enabled,capabilities) VALUES(?,?,?,?,1,'')`,
			"src-"+strings.ToLower(strings.ReplaceAll(args[1], " ", "-")), args[1], "mcp", args[2])
		fmt.Println("added", args[1])
		return err
	case "test":
		name := "Upwork"
		if len(args) > 1 {
			name = args[1]
		}
		var endpoint, kind, command string
		if err := c.DB.DB.QueryRow(`SELECT endpoint,kind,COALESCE(command,'') FROM sources WHERE name=?`, name).Scan(&endpoint, &kind, &command); err != nil {
			return fmt.Errorf("source %q not found", name)
		}
		tok, _ := c.LoadSecret("mcp:" + name)
		conn := &mcpclient.Connector{ID: name, Endpoint: endpoint, Token: tok}
		if kind == "mcp-stdio" {
			if command == "" {
				return fmt.Errorf("stdio source %q has no command", name)
			}
			conn.Command = strings.Fields(command)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		tools, err := conn.ListTools(ctx)
		if err != nil {
			return fmt.Errorf("capability discovery failed: %w", err)
		}
		caps := upwork.Discover(tools)
		fmt.Printf("%s: %d tools, capabilities=%v\n", name, len(tools), caps)
		for _, t := range tools {
			fmt.Printf("  - %s\n", t.Name)
		}
		cb, _ := json.Marshal(caps)
		_, _ = c.DB.DB.Exec(`UPDATE sources SET capabilities=? WHERE name=?`, string(cb), name)
	case "token":
		return integrationsTokenCmd(c, args[1:])
	default:
		return fmt.Errorf("usage: scout integrations [list|add|test|token]")
	}
	return nil
}

// integrationsTokenCmd stores an MCP access token (masked prompt, encrypted).
func integrationsTokenCmd(c *runtime.Core, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: scout integrations token <name>")
	}
	fmt.Printf("Access token for %s: ", args[0])
	key, err := readPassword()
	if err != nil || key == "" {
		return fmt.Errorf("no token entered")
	}
	if err := c.SaveSecret("mcp:"+args[0], key); err != nil {
		return err
	}
	fmt.Println("token stored (encrypted, never displayed).")
	return nil
}

func sessionsCmd(c *runtime.Core, args []string) error {
	list, err := csession.List(c.DB)
	if err != nil {
		return err
	}
	asJSON := len(args) > 0 && args[0] == "--json"
	if asJSON {
		b, _ := json.Marshal(list)
		fmt.Println(string(b))
		return nil
	}
	for _, s := range list {
		fmt.Printf("%s\t%s\t%s/%s\t%s\n", s.ID, s.Name, s.Provider, s.Model, s.UpdatedAt.Format(time.RFC3339))
	}
	return nil
}

func skillsCmd(c *runtime.Core, args []string) error {
	_ = c
	reg, err := skills.Load()
	if err != nil {
		return err
	}
	if len(args) > 0 {
		for _, s := range reg.Select(strings.Join(args, " "), 5) {
			fmt.Printf("%s\n", s.Name)
		}
		return nil
	}
	for _, s := range reg.List() {
		fmt.Printf("%-28s %s\n", s.Name, strings.Join(s.Triggers, ", "))
	}
	return nil
}

func toolsCmd(c *runtime.Core) error {
	for _, t := range c.Tools() {
		fmt.Printf("%-26s %-14s %s\n", t.Name, t.Permission, t.Description)
	}
	return nil
}

// askCmd runs one agent turn non-interactively; --json emits the final text as JSON.
func askCmd(c *runtime.Core, args []string) error {
	asJSON := false
	var q []string
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		} else {
			q = append(q, a)
		}
	}
	if len(q) == 0 {
		return fmt.Errorf("usage: scout ask [--json] \"question\"")
	}
	sess, err := csession.Create(c.DB, "ask", c.Cfg.Models["conversation"].Provider, c.Cfg.Models["conversation"].Model)
	if err != nil {
		return err
	}
	eng := c.EngineFor(sess.Provider, sess.Model)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	final, err := c.RunAgent(ctx, eng, []llm.Message{{Role: "user", Content: strings.Join(q, " ")}}, "", func(ev runtime.Event) {
		switch ev.Type {
		case "tool_start":
			fmt.Fprintf(os.Stderr, "◐ %s %s\n", ev.Name, ev.Args)
		case "tool_end":
			if ev.Err != nil {
				fmt.Fprintf(os.Stderr, "✗ %s: %s\n", ev.Name, ev.Err)
			} else {
				fmt.Fprintf(os.Stderr, "✓ %s\n", ev.Text)
			}
		case "error":
			fmt.Fprintf(os.Stderr, "error: %s\n", ev.Err)
		default:
			if ev.Type == "token" && !asJSON {
				fmt.Print(ev.Text)
			}
		}
	})
	if err != nil {
		return err
	}
	_ = csession.AppendMessages(c.DB, sess.ID, []csession.Message{{Role: "user", Content: strings.Join(q, " ")}, {Role: "assistant", Content: final}})
	if asJSON {
		b, _ := json.Marshal(map[string]string{"session": sess.ID, "response": final})
		fmt.Println(string(b))
	} else {
		fmt.Println()
	}
	return nil
}

func runCmd(c *runtime.Core, args []string) error {
	dry := true
	for _, a := range args[1:] {
		if a == "--live" {
			dry = false
		}
	}
	_ = args
	s, err := c.RunDiscovery(dry)
	if err != nil {
		return err
	}
	fmt.Printf("discovered=%d candidates=%d dry_run=%v (no external writes)\n", s.Total, s.Candidates, s.DryRun)
	return nil
}

func configCmd() error {
	cfg := config.Load()
	fmt.Printf("data_dir=%s\ndb=%s\nollama=%s\ndry_run=%v\nmodels=%v\n",
		cfg.DataDir, cfg.DBPath, cfg.OllamaHost, cfg.DryRun, cfg.Models)
	fmt.Println("(secrets redacted)")
	return nil
}

func doctorCmd(c *runtime.Core) error {
	fmt.Println("scout doctor")
	cfg := c.Cfg
	check := func(name string, ok bool, detail string) {
		s := "OK"
		if !ok {
			s = "MISSING"
		}
		fmt.Printf("  [%s] %s %s\n", s, name, detail)
	}
	_, err := os.Stat(cfg.DataDir)
	check("data-dir", err == nil, cfg.DataDir)
	check("sqlite", c.DB.DB.Ping() == nil, cfg.DBPath)
	check("ollama", ollamaUp(cfg.OllamaHost), cfg.OllamaHost)
	check("openai-key", hasKey("OPENAI_API_KEY", c, "llm:openai"), "env or stored")
	check("anthropic-key", hasKey("ANTHROPIC_API_KEY", c, "llm:anthropic"), "env or stored")
	check("deepseek-key", hasKey("DEEPSEEK_API_KEY", c, "llm:deepseek"), "env or stored")
	check("moonshot-key", hasKey("MOONSHOT_API_KEY", c, "llm:moonshot"), "env or stored")
	return nil
}

func hasKey(env string, c *runtime.Core, secretKey string) bool {
	if os.Getenv(env) != "" {
		return true
	}
	s, err := c.LoadSecret(secretKey)
	return err == nil && s != ""
}

func ollamaUp(host string) bool {
	cl := http.Client{Timeout: 5 * time.Second}
	r, err := cl.Get(strings.TrimSuffix(host, "/") + "/api/tags")
	if err != nil {
		return false
	}
	defer r.Body.Close()
	return r.StatusCode < 500
}

func backupCmd(c *runtime.Core, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: scout backup <file>")
	}
	if _, err := c.DB.DB.Exec(`VACUUM INTO ?`, args[0]); err != nil {
		return err
	}
	fmt.Println("backup written to", args[0], "(includes encrypted secrets table — keep private)")
	return nil
}

func restoreCmd(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: scout restore <file>")
	}
	cfg := config.Load()
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
	if _, err := copyFile(dst, src); err != nil {
		return err
	}
	fmt.Println("restored to", cfg.DBPath)
	return nil
}

func mcpCmd(c *runtime.Core, args []string) error {
	sub := "stdio"
	if len(args) > 0 {
		sub = args[0]
	}
	deps := mcpserver.Deps{Core: c}
	switch sub {
	case "stdio":
		return mcpserver.RunStdio(deps)
	case "serve":
		addr := c.Cfg.Addr
		if len(args) > 1 {
			addr = args[1]
		}
		fmt.Fprintf(os.Stderr, "scout MCP (Streamable HTTP) on http://%s/mcp\n", addr)
		return http.ListenAndServe(addr, mcpserver.Handler(deps))
	default:
		return fmt.Errorf("usage: scout mcp [stdio|serve]")
	}
}
