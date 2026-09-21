// Command scout — terminal-native AI work acquisition agent.
// Bare `scout` enters the interactive session; subcommands are scriptable.
package main

import (
	"fmt"
	"os"

	"github.com/ianclemence/scout/pkg/changelog"
	"github.com/ianclemence/scout/pkg/config"
	"github.com/ianclemence/scout/pkg/runtime"
	"github.com/ianclemence/scout/pkg/secret"
	"github.com/ianclemence/scout/pkg/store"
	"github.com/ianclemence/scout/pkg/upwork"
	"github.com/ianclemence/scout/pkg/version"
	"github.com/ianclemence/scout/pkg/workspace"
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
	case "logout":
		err = withCore(func(c *runtime.Core) error { return logoutCmd(c, rest) })
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
	case "changelog", "changes", "whatsnew":
		fmt.Println(changelog.Raw())
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
  scout discover [query]       search connected sources and store new work
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
  scout integrations [list|test|login|add|token|enable|disable|remove]
                         work sources and MCP connectors (alias: sources)
  scout sessions [list]        persistent sessions
  scout skills [query]         agent skill registry
  scout tools                  tool registry with permission classes
  scout run discovery          planned discovery run (drafts only)
  scout config                 effective config (secrets redacted)
  scout doctor                 diagnostics for Raspberry Pi troubleshooting
  scout backup <file>          backup database
  scout update [--self|--models|--all] [--check|--notes] [--force] [--dry-run]
               [--channel release|dev] [--version V]
                         install a verified release (skip when current);
                         --channel dev builds the local checkout
  scout changelog            release notes for this install
  scout restore <file>         restore database backup
  scout mcp [stdio|serve]      Scout MCP server for other MCP clients
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
	if err := workspace.Init(cfg.DataDir); err != nil {
		return err
	}
	fmt.Println("initialized", cfg.DataDir)
	return nil
}

func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// ---------- one-shot commands ----------
