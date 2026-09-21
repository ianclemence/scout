// Package isession implements the interactive Scout terminal session:
// prompt loop, slash commands, streaming render, inline approvals.
package isession

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"
	"time"

	"github.com/ianclemence/scout/pkg/config"
	"github.com/ianclemence/scout/pkg/csession"
	"github.com/ianclemence/scout/pkg/domain"
	"github.com/ianclemence/scout/pkg/llm"
	"github.com/ianclemence/scout/pkg/mcpauth"
	"github.com/ianclemence/scout/pkg/profile"
	"github.com/ianclemence/scout/pkg/runtime"
	"github.com/ianclemence/scout/pkg/sources"
)

// Command is a slash command with Scout-specific utility.
type Command struct {
	Name        string
	Description string
	ArgHint     string
	Handler     func(ctx *SessionCtx, args string) error
	// Group organizes the command in /help and the palette. Every command
	// belongs to exactly one group; the help output is grouped and ordered.
	Group string
	// Aliases are alternate names that resolve to this command.
	Aliases []string
}

// Command groups, in the order they appear in /help. Order mirrors the
// product: do the work, decide, understand yourself, connect, configure.
const (
	GroupWork    = "Work"
	GroupDecide  = "Decide"
	GroupYou     = "You"
	GroupConnect = "Connect"
	GroupSession = "Session"
)

// groupOrder is the canonical rendering order for command groups.
var groupOrder = []string{GroupWork, GroupDecide, GroupYou, GroupConnect, GroupSession}

// SessionCtx carries per-session state for command handlers.
type SessionCtx struct {
	Core       *runtime.Core
	Session    *csession.Session
	Out        func(format string, a ...any)
	ResolveOpp func(ref string) (*domain.Opportunity, error)
	// SetLastOpps records the last listing for index-based selection.
	SetLastOpps func(opps []domain.Opportunity)
	// Width is the terminal width for wrapping/tabular output.
	// Zero means unknown (one-shot CLI): do not wrap, print full rows.
	Width int
	// SwitchSession, when set (TUI), switches the live session in place.
	SwitchSession func(s *csession.Session) error
	// OpenModelSelector, when set (TUI), opens the interactive model selector.
	OpenModelSelector func(search string)
	// OpenThinking, when set (TUI), opens the interactive reasoning-level
	// selector. Line mode falls back to a printed list and a numeric prompt.
	OpenThinking func()
	// OpenSessions, when set (TUI), opens the interactive session picker.
	OpenSessions func()
	// OpenApprovals, when set (TUI), opens the interactive approval picker.
	OpenApprovals func()
	// OpenSources, when set (TUI), opens the interactive work-source manager.
	OpenSources func()
}

func (s *SessionCtx) Printf(format string, a ...any) { s.Out(format, a...) }

// Registry returns all commands in the canonical order: grouped by job, then
// alphabetical within a group. This single list drives the /help output, the
// TUI command palette, and the line-mode completer.
func Registry() []*Command {
	cmds := []*Command{
		// Work — find, evaluate, draft, track.
		{Name: "discover", Group: GroupWork, Description: "Search connected sources and store new work", Handler: cmdDiscover},
		{Name: "opportunities", Group: GroupWork, Description: "List stored opportunities", ArgHint: "<query>", Aliases: []string{"opps"}, Handler: cmdOpps},
		{Name: "opportunity", Group: GroupWork, Description: "Posting, evaluation, and proposal", ArgHint: "<id>", Handler: cmdOpp},
		{Name: "analyze", Group: GroupWork, Description: "Structured fit for an opportunity", ArgHint: "<id>", Handler: cmdAnalyze},
		{Name: "proposal", Group: GroupWork, Description: "Draft a grounded proposal (never sends)", ArgHint: "<id>", Handler: cmdProposal},
		{Name: "applications", Group: GroupWork, Description: "Applications and pipeline counts", Aliases: []string{"apps"}, Handler: cmdApplications},
		{Name: "feedback", Group: GroupWork, Description: "Record an explicit preference signal", ArgHint: "<id> <signal>", Handler: cmdFeedback},
		// Decide — the trust boundary.
		{Name: "approvals", Group: GroupDecide, Description: "Review and decide pending actions", Handler: cmdApprovals},
		// You — the source of truth.
		{Name: "profile", Group: GroupYou, Description: "Who Scout thinks you are; `/profile evidence` for the CV", ArgHint: "<query|import <path>|evidence>", Handler: cmdProfile},
		// Connect — sources, providers, models.
		{Name: "sources", Group: GroupConnect, Description: "Work sources & MCP connectors", Aliases: []string{"integrations"}, Handler: cmdSources},
		{Name: "login", Group: GroupConnect, Description: "Connect a provider", ArgHint: "<provider>", Handler: cmdLogin},
		{Name: "logout", Group: GroupConnect, Description: "Remove a stored provider credential", Handler: cmdLogout},
		{Name: "model", Group: GroupConnect, Description: "Select conversation model", ArgHint: "<provider/model>", Handler: cmdModel},
		{Name: "thinking", Group: GroupConnect, Description: "Set reasoning level", ArgHint: "<level>", Handler: cmdThinking},
		// Session — lifecycle and transcript.
		{Name: "help", Group: GroupSession, Description: "Show commands and keys", Handler: cmdHelp},
		{Name: "status", Group: GroupSession, Description: "Provider, model, profile, pending approvals, counts", Handler: cmdStatus},
		{Name: "sessions", Group: GroupSession, Description: "List or switch sessions", Aliases: []string{"resume"}, Handler: cmdSessions},
		{Name: "new", Group: GroupSession, Description: "Start a new session", Handler: cmdNew},
		{Name: "name", Group: GroupSession, Description: "Rename the session", ArgHint: "<name>", Handler: cmdName},
		{Name: "export", Group: GroupSession, Description: "Export the transcript to markdown", ArgHint: "<path>", Handler: cmdExport},
		{Name: "compact", Group: GroupSession, Description: "Summarize and trim session context", Handler: cmdCompact},
		{Name: "doctor", Group: GroupSession, Description: "Diagnostics (DB, providers, Ollama, disk)", Handler: cmdDoctor},
		{Name: "quit", Group: GroupSession, Description: "Exit Scout", Aliases: []string{"exit"}, Handler: cmdQuit},
	}
	// Stable order: canonical group order, then command order within a group.
	rank := map[string]int{}
	for i, g := range groupOrder {
		rank[g] = i
	}
	sort.SliceStable(cmds, func(i, j int) bool {
		ri, rj := rank[cmds[i].Group], rank[cmds[j].Group]
		if ri != rj {
			return ri < rj
		}
		return cmds[i].Name < cmds[j].Name
	})
	return cmds
}

func FindCommand(name string) *Command {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, c := range Registry() {
		if c.Name == name {
			return c
		}
		for _, a := range c.Aliases {
			if a == name {
				return c
			}
		}
	}
	return nil
}

func cmdHelp(ctx *SessionCtx, args string) error {
	ctx.Printf("Scout commands\n")
	last := ""
	for _, c := range Registry() {
		if c.Group != last {
			ctx.Printf("\n%s\n", c.Group)
			last = c.Group
		}
		ctx.Printf("  /%-14s %s\n", c.Name, c.Description)
	}
	ctx.Printf("\nAnything else is a request to the agent. Ctrl-C interrupts · Ctrl-D exits.\n")
	ctx.Printf("Keys: type / for the command palette · Ctrl+L model · Esc interrupt/quit.\n")
	return nil
}

func cmdStatus(ctx *SessionCtx, args string) error {
	var opps, pending, apps int
	_ = ctx.Core.DB.DB.QueryRow(`SELECT COUNT(*) FROM opportunities`).Scan(&opps)
	_ = ctx.Core.DB.DB.QueryRow(`SELECT COUNT(*) FROM pending_actions WHERE status IN ('draft','pending_approval')`).Scan(&pending)
	_ = ctx.Core.DB.DB.QueryRow(`SELECT COUNT(*) FROM applications`).Scan(&apps)
	p, _ := ctx.Core.Profile()
	ctx.Printf("scout %s · session %s (%s)\n", Version(), ctx.Session.ID[:12], ctx.Session.Name)
	think := ctx.Session.Thinking
	if think == "" {
		think = "provider default"
	}
	ctx.Printf("provider %s · model %s · thinking %s\n", ctx.Session.Provider, ctx.Session.Model, think)
	ctx.Printf("profile %s · %d skills\n", p.DisplayName, len(p.Skills))
	ctx.Printf("opportunities %d · pending approvals %d · applications %d\n", opps, pending, apps)
	return nil
}

func cmdProfile(ctx *SessionCtx, args string) error {
	fields := strings.Fields(args)
	if len(fields) >= 2 && fields[0] == "import" {
		raw, err := os.ReadFile(fields[1])
		if err != nil {
			return err
		}
		p, _, err := profile.ImportDocument(ctx.Core.DB, filepath.Base(fields[1]), raw)
		if err != nil {
			return err
		}
		ctx.Printf("Imported CV for %s — %d skills detected, resume stored. Review with /profile and /profile evidence.\n", p.DisplayName, len(p.Skills))
		return nil
	}
	if len(fields) >= 1 && (fields[0] == "evidence" || fields[0] == "cv") {
		return profileEvidence(ctx)
	}
	p, err := ctx.Core.Profile()
	if err != nil {
		return err
	}
	ctx.Printf("Profile: %s — %s\n", p.DisplayName, p.Title)
	ctx.Printf("Skills: %s\n", strings.Join(p.Skills, ", "))
	ctx.Printf("Min budget %.0f · min hourly %.0f · max connects/app %d\n", p.MinProjectBudget, p.MinHourlyRate, p.MaxConnectsPerApp)
	if len(p.ExcludedWork) > 0 {
		ctx.Printf("Excluded: %s\n", strings.Join(p.ExcludedWork, ", "))
	}
	ev, _ := ctx.Core.Evidence(5)
	ctx.Printf("Resume items: %d (latest: ", len(ev))
	for i, e := range ev {
		if i > 0 {
			ctx.Printf(", ")
		}
		ctx.Printf("%s:%s", e.Kind, e.Reference)
	}
	ctx.Printf(")\nImport or update from a file: /profile import <path>. Full evidence: /profile evidence.\n")
	return nil
}

// profileEvidence lists the resume content and citable items (the former /cv).
func profileEvidence(ctx *SessionCtx) error {
	ev, err := ctx.Core.Evidence(20)
	if err != nil {
		return err
	}
	if len(ev) == 0 {
		ctx.Printf("No resume content yet. Import a CV: /profile import <file>\n")
		return nil
	}
	ctx.Printf("Resume content and supporting items (%d):\n", len(ev))
	for _, e := range ev {
		ctx.Printf("  %-12s %-20s %s\n", e.Kind, e.Reference, truncate80(e.Content))
	}
	return nil
}

func cmdOpps(ctx *SessionCtx, args string) error {
	f := runtime.OpportunityFilter{Limit: 20}
	parts := strings.Fields(args)
	for _, p := range parts {
		if strings.HasPrefix(p, "--status=") {
			f.Status = strings.TrimPrefix(p, "--status=")
		} else if p == "--status" {
			f.Status = "review"
		} else {
			f.Query += p + " "
		}
	}
	f.Query = strings.TrimSpace(f.Query)
	opps, err := ctx.Core.ListOpportunities(f)
	if err != nil {
		return err
	}
	if ctx.SetLastOpps != nil {
		ctx.SetLastOpps(opps)
	}
	if len(opps) == 0 {
		ctx.Printf("No opportunities. Add one: scout opportunity add --title ... (or ask me to help draft from a posting).\n")
		return nil
	}
	ctx.Printf("OPPORTUNITIES (%d)\n", len(opps))
	w := wOf(ctx)
	for i, o := range opps {
		ctx.Printf("%s\n", cell(fmt.Sprintf("%2d  %s", i+1, o.Title), w))
		ctx.Printf("    %s\n", cell(shortID(o.ID)+" · "+o.Source+" · "+o.Status, w))
	}
	return nil
}

func cmdOpp(ctx *SessionCtx, args string) error {
	id := firstField(args)
	if id == "" {
		return fmt.Errorf("usage: /opportunity <id>")
	}
	o, err := ctx.ResolveOpp(id)
	if err != nil {
		return err
	}
	ctx.Printf("= %s =\n[%s] %s\nBudget %s %.0f–%.0f · credits %d\n\n%s\n", o.Title, o.Source, o.Status,
		o.BudgetType, o.BudgetMin, o.BudgetMax, o.ConnectsCost, o.Description)
	ev, err := ctx.Core.LatestEvaluation(o.ID)
	if err != nil {
		ctx.Printf("\nNot analyzed yet. Run /analyze %s\n", shortID(o.ID))
		return nil
	}
	ctx.Printf("\nMATCH: %s — %s\n", ev.Recommendation, ev.Reason)
	for _, d := range ev.Dimensions {
		ctx.Printf("  %-12s %-12s %s\n", d.Name, d.Rating, d.Detail)
	}
	if len(ev.Risks) > 0 {
		ctx.Printf("Risks: %s\n", strings.Join(ev.Risks, "; "))
	}
	if pr, err := ctx.Core.LatestProposal(o.ID); err == nil {
		ctx.Printf("\nProposal (%s):\n%s\n", pr.Status, pr.CoverLetter)
	}
	return nil
}

func cmdDiscover(ctx *SessionCtx, args string) error {
	ctx.Printf("Searching connected sources…\n")
	runCtx, cancel := context.WithTimeout(ctxBg(), 45*time.Second)
	defer cancel()
	res, err := ctx.Core.DiscoverSources(runCtx, sources.SearchFilter{Query: strings.TrimSpace(args), Limit: 20})
	if err != nil {
		return err
	}
	if len(res.Sources) == 0 {
		ctx.Printf("No connected sources yet. Add one: /sources add Upwork https://mcp.upwork.com/mcp\n")
		return nil
	}
	ctx.Printf("Searched %d source(s): %d found, %d stored.\n", len(res.Sources), res.Found, res.Stored)
	for _, w := range res.Warnings {
		ctx.Printf("  ! %s\n", w)
	}
	if res.Stored > 0 {
		ctx.Printf("Review them with /opportunities.\n")
	}
	return nil
}

func cmdAnalyze(ctx *SessionCtx, args string) error {
	id := firstField(args)
	o, err := ctx.ResolveOpp(id)
	if err != nil {
		return err
	}
	ctx.Printf("Analyzing %s…\n", o.Title)
	ev, f, err := ctx.Core.Analyze(ctxBg(), o.ID, ctx.Core.EngineForRole(config.RoleWorker))
	if err != nil {
		return err
	}
	ctx.Printf("Filter: pass=%v (%s)\nRecommendation: %s — %s\n", f.Pass, f.Reason, ev.Recommendation, ev.Reason)
	for _, d := range ev.Dimensions {
		ctx.Printf("  %-12s %-12s %s\n", d.Name, d.Rating, d.Detail)
	}
	return nil
}

func cmdProposal(ctx *SessionCtx, args string) error {
	id := firstField(args)
	o, err := ctx.ResolveOpp(id)
	if err != nil {
		return err
	}
	ctx.Printf("Drafting proposal for %s…\n", o.Title)
	pr, err := ctx.Core.DraftProposal(ctxBg(), o.ID, ctx.Core.EngineForRole(config.RoleWorker))
	if err != nil {
		return err
	}
	ctx.Printf("\nPROPOSAL DRAFT\n\n%s\n\nBased on: %s\nRate %.0f %s\n[approve: /approvals once you request submission]\n",
		pr.CoverLetter, strings.Join(pr.EvidenceIDs, ", "), pr.Rate, pr.RateType)
	return nil
}

func cmdApprovals(ctx *SessionCtx, args string) error {
	parts := strings.Fields(args)
	if len(parts) == 2 && (parts[0] == "approve" || parts[0] == "reject") {
		status := map[string]string{"approve": "approved", "reject": "rejected"}[parts[0]]
		if err := ctx.Core.SetApprovalStatus(parts[1], status); err != nil {
			return err
		}
		ctx.Printf("%s → %s.\n", parts[1], status)
		ctx.Printf("This records your decision. Scout submits externally only when a connected source can execute it; otherwise ask the agent to run it and it will report what happened.\n")
		return nil
	}
	acts, err := ctx.Core.PendingApprovals()
	if err != nil {
		return err
	}
	if len(acts) == 0 {
		ctx.Printf("Nothing awaiting approval.\n")
		return nil
	}
	w := wOf(ctx)
	for _, a := range acts {
		ctx.Printf("\nACTION REQUIRES APPROVAL\n")
		ctx.Printf("  %s\n", cell(a.ActionType+" → "+a.Target+"  [risk "+a.RiskLevel+"]", w))
		for _, ln := range wrapLines(a.Payload, w-4) {
			ctx.Printf("  %s\n", ln)
		}
		ctx.Printf("  /approvals approve %s · /approvals reject %s\n", shortID(a.ID), shortID(a.ID))
	}
	ctx.Printf("\nApproving records your decision. External submission happens only through a connected source that can execute it; otherwise ask the agent to run the approved action.\n")
	return nil
}

func cmdApplications(ctx *SessionCtx, args string) error {
	m, _ := ctx.Core.Pipeline()
	apps, err := ctx.Core.ListApplications(30)
	if err != nil {
		return err
	}
	if len(apps) == 0 && len(m) == 0 {
		ctx.Printf("No applications yet.\n")
		return nil
	}
	if len(m) > 0 {
		ctx.Printf("Pipeline:\n")
		for s, n := range m {
			ctx.Printf("  %-12s %d\n", s, n)
		}
		ctx.Printf("\n")
	}
	w := wOf(ctx)
	for _, a := range apps {
		ctx.Printf("%s\n", cell(padRight(shortID(a.OpportunityID), 14)+padRight(a.Stage, 12)+a.Source, w))
	}
	return nil
}

// feedbackSignals is the set of preference signals Scout records. Keeping it
// closed means preference data stays queryable and consistent.
var feedbackSignals = []string{"good_match", "bad_match", "too_low_budget", "unclear_scope", "bad_client", "already_applied"}

var validFeedback = func() map[string]bool {
	m := map[string]bool{}
	for _, s := range feedbackSignals {
		m[s] = true
	}
	return m
}()

func cmdFeedback(ctx *SessionCtx, args string) error {
	parts := strings.Fields(args)
	if len(parts) < 2 {
		return fmt.Errorf("usage: /feedback <opp-id> <signal> [note]  (signals: %s)", strings.Join(feedbackSignals, ", "))
	}
	signal := strings.ToLower(parts[1])
	if !validFeedback[signal] {
		return fmt.Errorf("unknown signal %q — use one of: %s", parts[1], strings.Join(feedbackSignals, ", "))
	}
	o, err := ctx.ResolveOpp(parts[0])
	if err != nil {
		return err
	}
	note := ""
	if len(parts) > 2 {
		note = strings.Join(parts[2:], " ")
	}
	if err := ctx.Core.AddFeedback(o.ID, signal, note); err != nil {
		return err
	}
	ctx.Printf("Feedback recorded (%s). It becomes explicit preference data, not hidden model behavior.\n", signal)
	return nil
}

// thinkingLevels is the ordered set of reasoning levels Scout accepts,
// aligned with the provider drivers in pkg/llm.
var thinkingLevels = llm.ThinkLevels

var validThinking = func() map[string]bool {
	m := map[string]bool{}
	for _, l := range thinkingLevels {
		m[l] = true
	}
	return m
}()

// cmdThinking: bare /thinking opens an interactive selector (TUI) or a
// numbered prompt (line mode); an argument selects directly and is validated
// against the available levels, listing them on an unknown value.
func cmdThinking(ctx *SessionCtx, args string) error {
	level := strings.ToLower(firstField(args))
	if level != "" {
		if !validThinking[level] {
			return fmt.Errorf("unknown thinking level %q — available: %s", level, strings.Join(thinkingLevels, ", "))
		}
		return applyThinking(ctx, level)
	}
	if ctx.OpenThinking != nil {
		ctx.OpenThinking()
		return nil
	}
	// Line mode: numbered selector.
	ctx.Printf("Thinking level (current: %s):\n", displayThinking(ctx.Session.Thinking))
	for i, l := range thinkingLevels {
		mark := "  "
		if l == ctx.Session.Thinking {
			mark = "✓ "
		}
		ctx.Printf("  %d  %s%-8s %s\n", i+1, mark, l, llm.ThinkDescription(ctx.Session.Provider, l))
	}
	ctx.Printf("Choice: ")
	choice, err := readLineCooked()
	if err != nil || strings.TrimSpace(choice) == "" {
		return nil
	}
	var n int
	if _, err := fmt.Sscanf(choice, "%d", &n); err == nil && n >= 1 && n <= len(thinkingLevels) {
		return applyThinking(ctx, thinkingLevels[n-1])
	}
	return cmdThinking(ctx, choice)
}

// applyThinking sets and persists the session reasoning level.
func applyThinking(ctx *SessionCtx, level string) error {
	ctx.Session.Thinking = level
	csession.SetThinking(ctx.Core.DB, ctx.Session.ID, level)
	ctx.Printf("Reasoning level → %s (mapped to %s capabilities).\n", level, ctx.Session.Provider)
	return nil
}

// displayThinking renders an empty level as the provider default.
func displayThinking(level string) string {
	if level == "" {
		return "provider default"
	}
	return level
}

func cmdSources(ctx *SessionCtx, args string) error {
	parts := strings.Fields(args)
	sub := "list"
	if len(parts) > 0 {
		sub = parts[0]
	}
	switch sub {
	case "list", "ls":
		return printConnections(ctx)
	case "test":
		ref := ""
		if len(parts) > 1 {
			ref = parts[1]
		}
		return cmdSourcesTest(ctx, ref)
	case "add":
		if len(parts) < 3 {
			return fmt.Errorf("usage: /sources add <name> <https-url>  (or /sources add <name> --command \"prog args\")")
		}
		name := parts[1]
		if parts[2] == "--command" {
			cmdline := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(args[strings.Index(args, "--command")+len("--command"):]), "\""))
			cmdline = strings.Trim(cmdline, "\"")
			if err := ctx.Core.AddStdioConnection(name, cmdline); err != nil {
				return err
			}
			ctx.Printf("Added stdio connector %s. Test it: /sources test %s\n", name, name)
			return nil
		}
		if err := ctx.Core.AddMCPConnection(name, parts[2]); err != nil {
			return err
		}
		ctx.Printf("Added connector %s. Authenticate with /sources token %s, then /sources test %s.\n", name, name, name)
		return nil
	case "token":
		if len(parts) < 2 {
			return fmt.Errorf("usage: /sources token <name>")
		}
		return cmdSourcesToken(ctx, parts[1])
	case "enable", "disable":
		if len(parts) < 2 {
			return fmt.Errorf("usage: /sources %s <name>", sub)
		}
		if err := ctx.Core.SetConnectionEnabled(parts[1], sub == "enable"); err != nil {
			return err
		}
		ctx.Printf("%s %sd.\n", parts[1], sub)
		return nil
	case "remove", "rm":
		if len(parts) < 2 {
			return fmt.Errorf("usage: /sources remove <name>")
		}
		if err := ctx.Core.RemoveConnection(parts[1]); err != nil {
			return err
		}
		ctx.Printf("Removed %s.\n", parts[1])
		return nil
	case "login":
		if len(parts) < 2 {
			return fmt.Errorf("usage: /sources login <name>")
		}
		return cmdSourcesLogin(ctx, parts[1])
	default:
		return fmt.Errorf("usage: /sources [list|test <name>|add <name> <url>|login <name>|token <name>|enable|disable|remove <name>]")
	}
}

// cmdSourcesLogin runs the MCP OAuth 2.1 flow for a remote connector in line
// mode: discovery, dynamic client registration, a loopback callback, and PKCE.
// It completes on the callback or a pasted redirect URL/code.
func cmdSourcesLogin(ctx *SessionCtx, ref string) error {
	flow, name, err := ctx.Core.BeginMCPLogin(ctxBg(), ref)
	if err != nil {
		return err
	}
	defer flow.Close()
	url := flow.AuthorizeURL()
	ctx.Printf("Authorize %s by opening this URL in a browser:\n\n  %s\n\n", name, url)
	_ = openBrowserLine(url)
	ctx.Printf("Waiting for authorization… (or paste the redirect URL / code and press enter)\n")

	type outcome struct {
		cred *mcpauth.Credential
		err  error
	}
	resCh := make(chan outcome, 1)
	go func() {
		cred, werr := flow.Wait(ctxBg())
		resCh <- outcome{cred: cred, err: werr}
	}()
	inputCh := make(chan string, 1)
	go func() {
		line, _ := readLineCooked()
		inputCh <- line
	}()

	var r outcome
	select {
	case r = <-resCh:
	case line := <-inputCh:
		if strings.TrimSpace(line) != "" && !flow.Submit(line) {
			return fmt.Errorf("could not parse an authorization code from the input")
		}
		r = <-resCh
	}
	if r.err != nil {
		return r.err
	}
	if err := ctx.Core.SaveMCPCredential(name, r.cred); err != nil {
		return err
	}
	ctx.Printf("Signed in to %s. Credential stored (encrypted, never displayed).\n", name)
	return nil
}

// openBrowserLine launches the platform browser for an authorization URL,
// ignoring failure (the URL is printed for manual use).
func openBrowserLine(u string) error {
	switch goruntime.GOOS {
	case "darwin":
		return exec.Command("open", u).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	default:
		return exec.Command("xdg-open", u).Start()
	}
}

// printConnections renders the configured connector surface — the terminal
// answer to "what MCP is configured".
func printConnections(ctx *SessionCtx) error {
	conns, err := ctx.Core.Connections()
	if err != nil {
		return err
	}
	if len(conns) == 0 {
		ctx.Printf("No work sources configured.\nAdd one: /sources add Upwork https://mcp.upwork.com/mcp\n")
		return nil
	}
	ctx.Printf("WORK SOURCES (%d)\n", len(conns))
	for _, conn := range conns {
		state := "disabled"
		if conn.Enabled {
			state = "enabled"
			if conn.Status != "" {
				state = conn.Status
			}
		}
		auth := conn.Auth
		switch auth {
		case "token_stored":
			auth = "token stored (untested)"
		case "unauthenticated":
			auth = "not authenticated"
		case "authenticated":
			auth = "authenticated"
		}
		ctx.Printf("  %s · %s · %s\n", conn.Name, conn.Kind, state)
		target := conn.Endpoint
		if conn.Kind == "mcp-stdio" {
			target = conn.Command
		}
		if target != "" {
			ctx.Printf("    %s\n", target)
		}
		ctx.Printf("    auth: %s · capabilities: %s\n", auth, runtime.CapabilityLabels(conn.Capabilities))
		if conn.Detail != "" && conn.Status != "" && conn.Status != "configured" {
			ctx.Printf("    %s\n", conn.Detail)
		}
	}
	ctx.Printf("\nSign in: /sources login <name> · test: /sources test <name> · paste a token: /sources token <name>\n")
	return nil
}

// cmdSourcesTest probes one source (or all when ref is empty) with a bounded
// timeout and reports capabilities.
func cmdSourcesTest(ctx *SessionCtx, ref string) error {
	if ref == "" {
		ctx.Printf("Probing all enabled sources…\n")
		conns, err := ctx.Core.ProbeAll(ctxBg(), 8*time.Second)
		if err != nil {
			return err
		}
		for _, conn := range conns {
			if !conn.Enabled {
				continue
			}
			ctx.Printf("  %s: %s · auth %s · %s\n", conn.Name, conn.Status, conn.Auth, runtime.CapabilityLabels(conn.Capabilities))
			if conn.Detail != "" {
				ctx.Printf("    %s\n", conn.Detail)
			}
		}
		return nil
	}
	conn, err := ctx.Core.ProbeConnection(ctxBg(), ref, 12*time.Second)
	if err != nil {
		return err
	}
	ctx.Printf("%s: %s · auth %s · %d tools · %s\n", conn.Name, conn.Status, conn.Auth, conn.ToolCount, runtime.CapabilityLabels(conn.Capabilities))
	if conn.Detail != "" {
		ctx.Printf("  %s\n", conn.Detail)
	}
	return nil
}

// cmdSourcesToken stores an MCP token. In the TUI this runs through the
// line-mode readline path only when invoked from the CLI fallback; the TUI
// uses its own masked prompt via the login flow.
func cmdSourcesToken(ctx *SessionCtx, name string) error {
	conn, err := ctx.Core.FindConnection(name)
	if err != nil {
		return err
	}
	ctx.Printf("Storing a token for %s requires a masked prompt. Run in your shell: scout integrations token %s\n", conn.Name, conn.Name)
	return nil
}

func cmdSessions(ctx *SessionCtx, args string) error {
	// With an argument, this is "resume": resolve and switch in place.
	if ref := firstField(args); ref != "" {
		return cmdResume(ctx, ref)
	}
	list, err := csession.List(ctx.Core.DB)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		ctx.Printf("No sessions yet.\n")
		return nil
	}
	w := wOf(ctx)
	for i, s := range list {
		mark := ""
		if s.ID == ctx.Session.ID {
			mark = "  ← current"
		}
		ctx.Printf("%s\n", cell(fmt.Sprintf("%2d  ", i+1)+padRight(shortID(s.ID), 14)+padRight(s.Name, 18)+s.Provider+"/"+s.Model+mark, w))
	}
	return nil
}

func cmdDoctor(ctx *SessionCtx, args string) error {
	ctx.Printf("Scout doctor:\n")
	allOK := true
	for _, ch := range ctx.Core.Doctor(ctxBg()) {
		mark := "OK"
		if !ch.OK {
			mark = "!!"
			allOK = false
		}
		ctx.Printf("  [%s] %s %s\n", mark, ch.Name, ch.Detail)
	}
	if !allOK {
		ctx.Printf("Fix flagged items, then re-run /doctor.\n")
	}
	return nil
}

func cmdQuit(ctx *SessionCtx, args string) error { return errQuit }

// helpers shared with session.go
// wOf is widthOf for terse call sites.
func wOf(ctx *SessionCtx) int { return widthOf(ctx) }

// widthOf returns the render width, or 0 when unknown (no wrapping).
func widthOf(ctx *SessionCtx) int {
	if ctx.Width >= 20 {
		return ctx.Width
	}
	return 0
}

// cell cuts s to at most w display cells (ANSI-aware). w<=0 passes through.
func cell(s string, w int) string {
	if w <= 0 || displayWidth(s) <= w {
		return s
	}
	runes := []rune(stripANSI(s))
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi) / 2
		if displayWidth(string(runes[:mid])) < w-1 {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo > 1 {
		return string(runes[:lo-1]) + "…"
	}
	return "…"
}

// padRight pads s to exactly w display cells. w<=0 passes through.
func padRight(s string, w int) string {
	if w <= 0 {
		return s
	}
	if n := w - displayWidth(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// wrapLines word-wraps to w display cells. w<=0 returns lines unchanged.
func wrapLines(s string, w int) []string {
	if w < 20 {
		return strings.Split(s, "\n")
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		if displayWidth(para) <= w {
			out = append(out, para)
			continue
		}
		var cur strings.Builder
		curW := 0
		for _, word := range strings.Fields(para) {
			ww := displayWidth(word)
			if curW == 0 {
				cur.WriteString(word)
				curW = ww
				continue
			}
			if curW+1+ww > w {
				out = append(out, cur.String())
				cur.Reset()
				cur.WriteString(word)
				curW = ww
				continue
			}
			cur.WriteString(" " + word)
			curW += 1 + ww
		}
		out = append(out, cur.String())
	}
	return out
}

// displayWidth counts display cells without pulling in a TUI dependency.
func displayWidth(s string) int {
	w := 0
	for _, r := range stripANSI(s) {
		if r == '\n' {
			continue
		}
		w += runeWidth(r)
	}
	return w
}

func runeWidth(r rune) int {
	switch {
	case r < 32 || (r >= 0x7f && r < 0xa0):
		return 0
	case r >= 0x1100 && (r <= 0x115f || r == 0x2329 || r == 0x232a ||
		(r >= 0x2e80 && r <= 0xa4cf && r != 0x303f) ||
		(r >= 0xac00 && r <= 0xd7a3) ||
		(r >= 0xf900 && r <= 0xfaff) ||
		(r >= 0xfe30 && r <= 0xfe4f) ||
		(r >= 0xff00 && r <= 0xff60) ||
		(r >= 0xffe0 && r <= 0xffe6)):
		return 2
	}
	return 1
}

func stripANSI(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && ((s[j] >= '0' && s[j] <= '9') || s[j] == ';' || s[j] == '?' || s[j] == '!') {
				j++
			}
			if j < len(s) {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func firstField(s string) string {
	f := strings.Fields(s)
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func truncate80(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}

var _ = profile.Load
