// Package isession implements the interactive Scout terminal session:
// prompt loop, slash commands, streaming render, inline approvals.
package isession

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ianclemence/scout/internal/csession"
	"github.com/ianclemence/scout/internal/domain"
	"github.com/ianclemence/scout/internal/profile"
	"github.com/ianclemence/scout/internal/runtime"
	"github.com/ianclemence/scout/internal/skills"
)

// Command is a slash command with Scout-specific utility.
type Command struct {
	Name        string
	Description string
	ArgHint     string
	Handler     func(ctx *SessionCtx, args string) error
}

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
}

func (s *SessionCtx) Printf(format string, a ...any) { s.Out(format, a...) }

// Registry returns all commands in stable order.
func Registry() []*Command {
	cmds := []*Command{
		{Name: "help", Description: "Show commands", Handler: cmdHelp},
		{Name: "status", Description: "Provider, model, profile, pending approvals, counts", Handler: cmdStatus},
		{Name: "profile", Description: "Show profile summary", Handler: cmdProfile},
		{Name: "cv", Description: "Show CV/resume and citable items", Handler: cmdCV},
		{Name: "models", Description: "Show model catalog (registry, cached + discovered)", Handler: cmdModels},
		{Name: "model", Description: "Switch conversation model: /model [provider/model]", ArgHint: "[provider/model]", Handler: cmdModel},
		{Name: "thinking", Description: "Set reasoning level: /thinking <off|low|medium|high|max>", ArgHint: "<level>", Handler: cmdThinking},
		{Name: "providers", Description: "Show provider availability", Handler: cmdProviders},
		{Name: "login", Description: "Store a provider API key (masked): /login <openai|anthropic|deepseek>", ArgHint: "<provider>", Handler: cmdLogin},
		{Name: "logout", Description: "Remove a stored provider key: /logout <provider>", ArgHint: "<provider>", Handler: cmdLogout},
		{Name: "sources", Description: "Work sources and capabilities", Handler: cmdSources},
		{Name: "skills", Description: "List agent skills (workflows)", Handler: cmdSkills},
		{Name: "tools", Description: "List agent tools and permission classes", Handler: cmdTools},
		{Name: "opportunities", Description: "List opportunities: /opportunities [query] [--status s]", ArgHint: "[query]", Handler: cmdOpps},
		{Name: "opportunity", Description: "Show detail + evaluation: /opportunity <id>", ArgHint: "<id>", Handler: cmdOpp},
		{Name: "discover", Description: "Discovery summary over stored opportunities", Handler: cmdDiscover},
		{Name: "analyze", Description: "Analyze fit: /analyze <id>", ArgHint: "<id>", Handler: cmdAnalyze},
		{Name: "proposal", Description: "Draft proposal: /proposal <id>", ArgHint: "<id>", Handler: cmdProposal},
		{Name: "approvals", Description: "Review pending actions: /approvals [approve|reject <id>]", ArgHint: "[approve|reject <id>]", Handler: cmdApprovals},
		{Name: "applications", Description: "List applications", Handler: cmdApplications},
		{Name: "pipeline", Description: "Pipeline counts by stage", Handler: cmdPipeline},
		{Name: "inbox", Description: "Messages needing attention", Handler: cmdInbox},
		{Name: "feedback", Description: "Record feedback: /feedback <opp-id> <signal> [note]", ArgHint: "<opp-id> <signal>", Handler: cmdFeedback},
		{Name: "session", Description: "Current session info", Handler: cmdSession},
		{Name: "sessions", Description: "List sessions", Handler: cmdSessions},
		{Name: "new", Description: "Start a new session", Handler: cmdNew},
		{Name: "name", Description: "Rename the session: /name <name>", ArgHint: "<name>", Handler: cmdName},
		{Name: "export", Description: "Export transcript to markdown: /export <path>", ArgHint: "<path>", Handler: cmdExport},
		{Name: "copy", Description: "Copy last assistant message (clipboard where available)", Handler: cmdCopy},
		{Name: "keys", Description: "Keyboard shortcuts", Handler: cmdKeys},
		{Name: "resume", Description: "Resume a session: /resume <id|name>", ArgHint: "<id|name>", Handler: cmdResume},
		{Name: "clear", Description: "Clear screen (keeps history)", Handler: cmdClear},
		{Name: "compact", Description: "Summarize and trim session context", Handler: cmdCompact},
		{Name: "doctor", Description: "Diagnostics", Handler: cmdDoctor},
		{Name: "quit", Description: "Exit Scout", Handler: cmdQuit},
	}
	sort.Slice(cmds, func(i, j int) bool { return cmds[i].Name < cmds[j].Name })
	return cmds
}

func FindCommand(name string) *Command {
	for _, c := range Registry() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func cmdHelp(ctx *SessionCtx, args string) error {
	ctx.Printf("Scout commands:\n")
	for _, c := range Registry() {
		ctx.Printf("  /%-14s %s\n", c.Name, c.Description)
	}
	ctx.Printf("\nAnything else is a request to the agent. Ctrl-C interrupts, Ctrl-D exits.\n")
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
	if f := strings.Fields(args); len(f) >= 2 && f[0] == "import" {
		raw, err := os.ReadFile(f[1])
		if err != nil {
			return err
		}
		p, ev, err := profile.ImportDocument(ctx.Core.DB, filepath.Base(f[1]), raw)
		if err != nil {
			return err
		}
		ctx.Printf("Imported %s: %d skills, %d evidence items. Review with /profile and /cv.\n", p.DisplayName, len(p.Skills), len(ev))
		return nil
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
	ctx.Printf("Evidence items: %d (latest: ", len(ev))
	for i, e := range ev {
		if i > 0 {
			ctx.Printf(", ")
		}
		ctx.Printf("%s:%s", e.Kind, e.Reference)
	}
	ctx.Printf(")\nImport or update from a file: /profile import <path>.\n")
	return nil
}

func cmdCV(ctx *SessionCtx, args string) error {
	ev, err := ctx.Core.Evidence(20)
	if err != nil {
		return err
	}
	if len(ev) == 0 {
		ctx.Printf("No evidence yet. Import a CV: scout profile import <file>\n")
		return nil
	}
	ctx.Printf("CV — resume content and citable items (%d):\n", len(ev))
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
	s, err := ctx.Core.RunDiscovery(true)
	if err != nil {
		return err
	}
	ctx.Printf("Discovery: %d stored, %d pass filters. (External discovery runs through integrations; see /sources.)\n", s.Total, s.Candidates)
	return nil
}

func cmdAnalyze(ctx *SessionCtx, args string) error {
	id := firstField(args)
	o, err := ctx.ResolveOpp(id)
	if err != nil {
		return err
	}
	ctx.Printf("Analyzing %s…\n", o.Title)
	ev, f, err := ctx.Core.Analyze(ctxBg(), o.ID, ctx.Core.EngineForRole("analysis"))
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
	pr, err := ctx.Core.DraftProposal(ctxBg(), o.ID, ctx.Core.EngineForRole("proposal"))
	if err != nil {
		return err
	}
	ctx.Printf("\nPROPOSAL DRAFT\n\n%s\n\nEvidence: %s\nRate %.0f %s\n[approve: /approvals once you request submission]\n",
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
		ctx.Printf("%s → %s. (External execution happens through the official integration run step.)\n", parts[1], status)
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
	return nil
}

func cmdApplications(ctx *SessionCtx, args string) error {
	apps, err := ctx.Core.ListApplications(30)
	if err != nil {
		return err
	}
	if len(apps) == 0 {
		ctx.Printf("No applications yet.\n")
		return nil
	}
	w := wOf(ctx)
	for _, a := range apps {
		ctx.Printf("%s\n", cell(padRight(shortID(a.OpportunityID), 14)+padRight(a.Stage, 12)+a.Source, w))
	}
	return nil
}

func cmdPipeline(ctx *SessionCtx, args string) error {
	m, err := ctx.Core.Pipeline()
	if err != nil {
		return err
	}
	if len(m) == 0 {
		ctx.Printf("Pipeline empty.\n")
		return nil
	}
	for s, n := range m {
		ctx.Printf("  %-12s %d\n", s, n)
	}
	return nil
}

func cmdInbox(ctx *SessionCtx, args string) error {
	tool := ctx.Core.FindTool("list_messages")
	out, err := tool.Handler(ctxBg(), map[string]any{})
	if err != nil {
		return err
	}
	ctx.Printf("%s\n", out)
	return nil
}

func cmdFeedback(ctx *SessionCtx, args string) error {
	parts := strings.Fields(args)
	if len(parts) < 2 {
		return fmt.Errorf("usage: /feedback <opp-id> <signal> [note]")
	}
	o, err := ctx.ResolveOpp(parts[0])
	if err != nil {
		return err
	}
	note := ""
	if len(parts) > 2 {
		note = strings.Join(parts[2:], " ")
	}
	if err := ctx.Core.AddFeedback(o.ID, parts[1], note); err != nil {
		return err
	}
	ctx.Printf("Feedback recorded (%s). It becomes explicit preference data, not hidden model behavior.\n", parts[1])
	return nil
}

func cmdModels(ctx *SessionCtx, args string) error {
	ctx.Printf("Roles → provider/model (conversation uses session model):\n")
	for _, role := range []string{"screening", "analysis", "proposal", "conversation", "deep_analysis"} {
		r := ctx.Core.Cfg.Models[role]
		mark := ""
		if role == "conversation" {
			mark = fmt.Sprintf("  [session: %s/%s]", ctx.Session.Provider, ctx.Session.Model)
		}
		ctx.Printf("  %-13s %s/%s%s\n", role, r.Provider, r.Model, mark)
	}
	ctx.Printf("\nCatalog (builtin + cached + local Ollama; `scout models refresh` to update):\n")
	for _, m := range ctx.Core.Registry().List(ctxBg(), "") {
		ctx.Printf("  %-22s ctx=%s reasoning=%s tools=%v src=%s\n",
			m.Provider+"/"+m.ID, ctxInt(m.Context), m.Reasoning, m.Tools, m.Source)
	}
	return nil
}

func ctxInt(n int) string {
	if n == 0 {
		return "unknown"
	}
	if n >= 1000 {
		return fmt.Sprintf("%dk", n/1000)
	}
	return fmt.Sprintf("%d", n)
}

func cmdThinking(ctx *SessionCtx, args string) error {
	level := strings.ToLower(firstField(args))
	switch level {
	case "off", "low", "medium", "high", "max", "":
	default:
		return fmt.Errorf("usage: /thinking <off|low|medium|high|max>")
	}
	if level == "" {
		cur := ctx.Session.Thinking
		if cur == "" {
			cur = "provider default"
		}
		ctx.Printf("Reasoning level: %s (provider %s, model %s)\n", cur, ctx.Session.Provider, ctx.Session.Model)
		return nil
	}
	ctx.Session.Thinking = level
	csession.SetThinking(ctx.Core.DB, ctx.Session.ID, level)
	ctx.Printf("Reasoning level → %s (mapped to %s capabilities).\n", level, ctx.Session.Provider)
	return nil
}

func cmdSources(ctx *SessionCtx, args string) error {
	srcs, err := ctx.Core.ListSources()
	if err != nil {
		return err
	}
	w := wOf(ctx)
	for _, s := range srcs {
		en := "off"
		if s.Enabled {
			en = "on"
		}
		ctx.Printf("%s\n", cell(padRight(s.Name, 16)+padRight(s.Kind+" "+en, 14)+s.Endpoint, w))
	}
	return nil
}

func cmdSkills(ctx *SessionCtx, args string) error {
	reg, err := skills.Load()
	if err != nil {
		return err
	}
	if q := strings.TrimSpace(args); q != "" {
		for _, s := range reg.Select(q, 5) {
			ctx.Printf("  %-28s %s\n", s.Name, firstLine(s.Body))
		}
		return nil
	}
	for _, s := range reg.List() {
		ctx.Printf("%s\n", cell(padRight(s.Name, 28)+strings.Join(s.Triggers, ", "), wOf(ctx)))
	}
	return nil
}

func cmdTools(ctx *SessionCtx, args string) error {
	for _, t := range ctx.Core.Tools() {
		ctx.Printf("%s\n", cell(padRight(t.Name, 26)+padRight(string(t.Permission), 15)+t.Description, wOf(ctx)))
	}
	return nil
}

func cmdProviders(ctx *SessionCtx, args string) error {
	w := widthOf(ctx)
	ctx.Printf("%s %s %s %s\n", padRight("Provider", 16), padRight("Cfg", 4), padRight("Models", 7), "Detail / roles")
	for _, p := range ctx.Core.ProviderStatus(ctxBg()) {
		mark := "✗"
		if p.Configured {
			mark = "✓"
		}
		roles := ""
		if len(p.Roles) > 0 {
			roles = " [" + strings.Join(p.Roles, ",") + "]"
		}
		ctx.Printf("%s\n", cell(padRight(p.Provider, 16)+" "+mark+"  "+padRight(itoa(p.Models), 7)+p.Detail+roles, w))
	}
	return nil
}

func cmdSession(ctx *SessionCtx, args string) error {
	msgs, _ := ctx.Core.SessionMessageCount(ctx.Session.ID)
	ctx.Printf("session %s (%s) · %s/%s · %d messages\n", ctx.Session.ID[:12], ctx.Session.Name, ctx.Session.Provider, ctx.Session.Model, msgs)
	return nil
}

func cmdSessions(ctx *SessionCtx, args string) error {
	list, err := csession.List(ctx.Core.DB)
	if err != nil {
		return err
	}
	w := wOf(ctx)
	for _, s := range list {
		mark := ""
		if s.ID == ctx.Session.ID {
			mark = "  ← current"
		}
		ctx.Printf("%s\n", cell(padRight(shortID(s.ID), 14)+padRight(s.Name, 18)+s.Provider+"/"+s.Model+mark, w))
	}
	return nil
}

func cmdClear(ctx *SessionCtx, args string) error { return errClearScreen }

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

func itoa(n int) string {
	if n == 0 {
		return "–"
	}
	return fmt.Sprintf("%d", n)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

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
