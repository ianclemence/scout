package isession

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/ianclemence/scout/pkg/csession"
	"github.com/ianclemence/scout/pkg/llm"
	"github.com/ianclemence/scout/pkg/registry"
)

// loginProviderIDs is the credential-bearing provider catalog, in stable order.
var loginProviderIDs = []string{"openai", "anthropic", "deepseek", "moonshot"}

// cmdLogin implements the line-mode login flow, mirroring the TUI's staged
// experience: with a provider argument it prompts for that key directly;
// otherwise it presents the provider selector first.
func cmdLogin(ctx *SessionCtx, args string) error {
	p := strings.ToLower(firstField(args))
	if p == "" {
		// Stage 1: authentication method, mirroring the TUI and the
		// reference agents.
		ctx.Printf("Select authentication method:\n")
		ctx.Printf("  1  Sign in with an account\n")
		ctx.Printf("  2  Sign in with an API key\n")
		ctx.Printf("Choice: ")
		method, err := readLineCooked()
		if err != nil || strings.TrimSpace(method) == "" {
			return nil
		}
		if strings.TrimSpace(method) == "1" {
			ctx.Printf("No account sign-in providers are available yet — choose \"Sign in with an API key\".\n")
			return nil
		}
		// Stage 2: provider.
		ctx.Printf("Select provider to configure (number):\n")
		for i, id := range loginProviderIDs {
			ctx.Printf("  %2d  %-10s %s\n", i+1, id, loginProviderState(ctx, id))
		}
		ctx.Printf("Choice: ")
		choice, err := readLineCooked()
		if err != nil || strings.TrimSpace(choice) == "" {
			return nil
		}
		var n int
		if _, err := fmt.Sscanf(choice, "%d", &n); err != nil || n < 1 || n > len(loginProviderIDs) {
			p = strings.ToLower(strings.TrimSpace(choice))
		} else {
			p = loginProviderIDs[n-1]
		}
	}
	valid := false
	for _, id := range loginProviderIDs {
		if id == p {
			valid = true
		}
	}
	if !valid {
		return fmt.Errorf("usage: /login <openai|anthropic|deepseek|moonshot>")
	}
	key, err := promptPassword(fmt.Sprintf("%s API key: ", p))
	if err != nil || key == "" {
		return fmt.Errorf("no key entered")
	}
	if err := ctx.Core.SaveSecret("llm:"+p, key); err != nil {
		return err
	}
	ctx.Printf("%s key stored (encrypted). Never displayed again.\n", p)
	return nil
}

func cmdLogout(ctx *SessionCtx, args string) error {
	p := strings.ToLower(firstField(args))
	if p == "" {
		stored := ctx.Core.StoredProviders()
		if len(stored) == 0 {
			ctx.Printf("No stored credentials to remove. /logout only removes credentials saved by /login; environment variables are unchanged.\n")
			return nil
		}
		ctx.Printf("Select provider to remove (number):\n")
		for i, id := range stored {
			ctx.Printf("  %2d  %s\n", i+1, id)
		}
		ctx.Printf("Choice: ")
		choice, err := readLineCooked()
		if err != nil || strings.TrimSpace(choice) == "" {
			return nil
		}
		var n int
		if _, err := fmt.Sscanf(choice, "%d", &n); err != nil || n < 1 || n > len(stored) {
			p = strings.ToLower(strings.TrimSpace(choice))
		} else {
			p = stored[n-1]
		}
	}
	valid := false
	for _, id := range loginProviderIDs {
		if id == p {
			valid = true
		}
	}
	if !valid {
		return fmt.Errorf("usage: /logout <openai|anthropic|deepseek|moonshot>")
	}
	_, err := ctx.Core.DB.DB.Exec(`DELETE FROM secrets WHERE key=?`, "llm:"+p)
	if err != nil {
		return err
	}
	ctx.Printf("%s stored key removed. (Environment variable, if set, still applies.)\n", p)
	return nil
}

// loginProviderState describes a provider's credential status for the selector.
func loginProviderState(ctx *SessionCtx, p string) string {
	if s, err := ctx.Core.LoadSecret("llm:" + p); err == nil && s != "" {
		return "configured (stored key)"
	}
	env := map[string]string{
		"openai": "OPENAI_API_KEY", "anthropic": "ANTHROPIC_API_KEY",
		"deepseek": "DEEPSEEK_API_KEY", "moonshot": "MOONSHOT_API_KEY",
	}[p]
	if env != "" && os.Getenv(env) != "" {
		return "configured (env: " + env + ")"
	}
	return "unconfigured"
}

// cmdModel: with an exact "provider/model" argument it
// switches immediately; otherwise it opens the searchable model selector
// (pre-filled with the argument as the search term). Scoped models, when set,
// restrict what the selector offers. The TUI provides OpenModelSelector; the
// line-mode fallback is a numbered prompt over the same model set.
func cmdModel(ctx *SessionCtx, args string) error {
	q := strings.TrimSpace(args)
	// An exact provider/model reference switches immediately. Anything else
	// opens the selector.
	if q != "" {
		if prov, model := splitModelRef(q); prov != "" && model != "" {
			if exactModelExists(ctx, prov, model) {
				return switchSessionModel(ctx, prov, model)
			}
		}
	}
	if ctx.OpenModelSelector != nil {
		ctx.OpenModelSelector(q)
		return nil
	}

	models := selectorModels(ctx)
	opts := fuzzyFilterModels(models, q)
	if len(opts) == 0 {
		return fmt.Errorf("no models match %q", q)
	}
	ctx.Printf("Select model (number, or /model provider/model):\n")
	for i, m := range opts {
		mark := ""
		if m.Provider == ctx.Session.Provider && m.ID == ctx.Session.Model {
			mark = "  ← current"
		}
		ctx.Printf("  %2d  %-28s %s%s\n", i+1, m.Provider+"/"+m.ID, modelNote(m), mark)
	}
	ctx.Printf("Choice: ")
	choice, err := readLineCooked()
	if err != nil || choice == "" {
		return nil
	}
	var n int
	if _, err := fmt.Sscanf(choice, "%d", &n); err != nil || n < 1 || n > len(opts) {
		// Maybe they typed a ref instead of a number.
		if prov, model := splitModelRef(choice); prov != "" && model != "" {
			return switchSessionModel(ctx, prov, model)
		}
		return fmt.Errorf("invalid choice %q", choice)
	}
	m := opts[n-1]
	return switchSessionModel(ctx, m.Provider, m.ID)
}

// cmdScopedModels is an interactive enable/disable +
// reorder list for models used when cycling with Ctrl+P. Changes are
// session-local; the selector's Ctrl+S persists them to settings. The TUI
// provides the full interactive component; line mode uses a numbered toggle.
func cmdScopedModels(ctx *SessionCtx, args string) error {
	if ctx.OpenScopedModels != nil {
		ctx.OpenScopedModels()
		return nil
	}
	return scopedModelsLineUI(ctx, args)
}

// switchSessionModel sets the session conversation model and records it.
func switchSessionModel(ctx *SessionCtx, prov, model string) error {
	ctx.Session.Provider, ctx.Session.Model = prov, model
	csession.Touch(ctx.Core.DB, ctx.Session.ID, prov, model)
	ctx.Printf("Session model → %s/%s\n", prov, model)
	return nil
}

// exactModelExists reports whether provider/model is a known registry model.
// Resolution uses the full catalog (AllModels), so an explicitly named model
// can still be selected even when its provider is not yet configured.
func exactModelExists(ctx *SessionCtx, prov, model string) bool {
	for _, m := range AllModels(ctx.Core) {
		if m.Provider == prov && m.ID == model {
			return true
		}
	}
	return false
}

// selectorModels returns the model set the /model selector should offer: the
// configured-provider catalog, further narrowed by the scoped set when active.
func selectorModels(ctx *SessionCtx) []registry.ModelInfo {
	models := AvailableModels(ctx.Core)
	if ctx.ScopedModels != nil {
		return FilterScoped(models, ctx.ScopedModels())
	}
	return models
}

// fuzzyFilterModels filters by a case-insensitive substring match across
// provider, id, and display name. An empty query returns models unchanged.
func fuzzyFilterModels(models []registry.ModelInfo, q string) []registry.ModelInfo {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return models
	}
	var out []registry.ModelInfo
	for _, m := range models {
		hay := strings.ToLower(m.Provider + "/" + m.ID + " " + m.DisplayName)
		if strings.Contains(hay, q) {
			out = append(out, m)
		}
	}
	return out
}

func modelNote(m registry.ModelInfo) string {
	switch {
	case m.Provider == "ollama" || m.Source == "ollama":
		return "local"
	case m.Reasoning != "" && m.Reasoning != "unknown":
		return "reasoning:" + m.Reasoning
	default:
		return m.Source
	}
}

// scopedModelsLineUI is the non-TTY fallback for /scoped-models: it prints the
// current enable/order state and accepts toggles and reorders by number. It
// lists the full catalog so any model can be enabled in advance.
func scopedModelsLineUI(ctx *SessionCtx, args string) error {
	models := AllModels(ctx.Core)
	sc := ScopedModels{}
	if ctx.ScopedModels != nil {
		sc = ctx.ScopedModels()
	}

	switch strings.ToLower(firstField(args)) {
	case "all":
		if err := ctx.SetScopedModels(nil); err != nil {
			return err
		}
		ctx.Printf("Scoped models → all enabled.\n")
		return nil
	case "clear":
		if err := ctx.SetScopedModels([]string{}); err != nil {
			return err
		}
		ctx.Printf("Scoped models → none enabled (cycling disabled).\n")
		return nil
	case "":
		// fall through to listing
	default:
		return fmt.Errorf("usage: /scoped-models [all|clear]")
	}

	ctx.Printf("Scoped models (%s; Ctrl+P cycles enabled):\n", scopedCount(sc, models))
	ordered := orderedModelIDs(sc, models)
	byID := map[string]registry.ModelInfo{}
	for _, m := range models {
		byID[m.Provider+"/"+m.ID] = m
	}
	for i, id := range ordered {
		mark := "○"
		if sc.IsEnabled(id) {
			mark = "✓"
		}
		cur := ""
		if m, ok := byID[id]; ok && m.Provider == ctx.Session.Provider && m.ID == ctx.Session.Model {
			cur = "  ← current"
		}
		ctx.Printf("  %2d  %s %s%s\n", i+1, mark, id, cur)
	}
	ctx.Printf("Toggle by number (space-separated), or /scoped-models all|clear: ")
	choice, err := readLineCooked()
	if err != nil || strings.TrimSpace(choice) == "" {
		return nil
	}
	ids := sc.IDs()
	if ids == nil {
		ids = ordered
	}
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}
	for _, f := range strings.Fields(choice) {
		var n int
		if _, err := fmt.Sscanf(f, "%d", &n); err != nil || n < 1 || n > len(ordered) {
			continue
		}
		id := ordered[n-1]
		if sc.AllEnabled() || set[id] {
			delete(set, id)
		} else {
			set[id] = true
		}
	}
	var next []string
	for _, id := range ordered {
		if set[id] {
			next = append(next, id)
		}
	}
	if len(next) == 0 {
		next = []string{}
	}
	if err := ctx.SetScopedModels(next); err != nil {
		return err
	}
	ctx.Printf("Scoped models updated (%d enabled).\n", len(next))
	return nil
}

func scopedCount(sc ScopedModels, models []registry.ModelInfo) string {
	if sc.AllEnabled() {
		return fmt.Sprintf("all %d enabled", len(models))
	}
	enabled := 0
	for _, id := range sc.ids {
		for _, m := range models {
			if m.Provider+"/"+m.ID == id {
				enabled++
			}
		}
	}
	return fmt.Sprintf("%d/%d enabled", enabled, len(models))
}

// orderedModelIDs returns the explicit scoped order followed by the remaining
// available models.
func orderedModelIDs(sc ScopedModels, models []registry.ModelInfo) []string {
	all := make([]string, 0, len(models))
	for _, m := range models {
		all = append(all, m.Provider+"/"+m.ID)
	}
	if sc.AllEnabled() {
		return all
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(all))
	for _, id := range sc.ids {
		out = append(out, id)
		seen[id] = true
	}
	for _, id := range all {
		if !seen[id] {
			out = append(out, id)
		}
	}
	return out
}

func splitModelRef(s string) (string, string) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

func cmdNew(ctx *SessionCtx, args string) error {
	name := strings.TrimSpace(args)
	if name == "" {
		name = "session"
	}
	s, err := csession.Create(ctx.Core.DB, name, ctx.Session.Provider, ctx.Session.Model)
	if err != nil {
		return err
	}
	if ctx.SwitchSession != nil {
		if err := ctx.SwitchSession(s); err != nil {
			return err
		}
		ctx.Printf("Switched to new session %s (%s).\n", s.ID[:12], name)
		return nil
	}
	ctx.Printf("Started session %s (%s). Resume with /resume %s\n", s.ID[:12], name, s.ID[:12])
	return nil
}

func cmdResume(ctx *SessionCtx, args string) error {
	ref := firstField(args)
	if ref == "" {
		return fmt.Errorf("usage: /resume <id|name>")
	}
	s, err := csession.Resolve(ctx.Core.DB, ref)
	if err != nil {
		return err
	}
	if ctx.SwitchSession != nil {
		if err := ctx.SwitchSession(s); err != nil {
			return err
		}
		ctx.Printf("Switched to session %s (%s).\n", s.ID[:12], s.Name)
		return nil
	}
	ctx.Printf("Resume session %s (%s): exit and run `scout resume %s`.\n", s.ID[:12], s.Name, s.ID[:12])
	return nil
}

func cmdName(ctx *SessionCtx, args string) error {
	name := strings.TrimSpace(args)
	if name == "" {
		ctx.Printf("Session name: %s (rename with /name <name>).\n", ctx.Session.Name)
		return nil
	}
	if err := csession.Rename(ctx.Core.DB, ctx.Session.ID, name); err != nil {
		return err
	}
	ctx.Session.Name = name
	ctx.Printf("Session renamed to %s.\n", name)
	return nil
}

func cmdExport(ctx *SessionCtx, args string) error {
	path := strings.TrimSpace(args)
	if path == "" {
		return fmt.Errorf("usage: /export <path>  (writes transcript markdown)")
	}
	msgs, err := csession.LoadMessages(ctx.Core.DB, ctx.Session.ID, 500)
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# Scout session: " + ctx.Session.Name + "\n\n")
	for _, m := range msgs {
		who := "You"
		if m.Role == "assistant" {
			who = "Scout"
		}
		b.WriteString("## " + who + "\n\n" + m.Content + "\n\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return err
	}
	ctx.Printf("Exported %d messages to %s.\n", len(msgs), path)
	return nil
}

func cmdCopy(ctx *SessionCtx, args string) error {
	msgs, err := csession.LoadMessages(ctx.Core.DB, ctx.Session.ID, 20)
	if err != nil {
		return err
	}
	last := ""
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "assistant" && strings.TrimSpace(msgs[i].Content) != "" {
			last = msgs[i].Content
			break
		}
	}
	if last == "" {
		return fmt.Errorf("no assistant message to copy yet")
	}
	if err := copyToClipboard(last); err != nil {
		ctx.Printf("Clipboard unavailable (%s). Last answer printed below:\n\n%s\n", err, last)
		return nil
	}
	ctx.Printf("Copied last answer (%d chars).\n", len(last))
	return nil
}

func copyToClipboard(s string) error {
	var cmd *exec.Cmd
	switch {
	case hasBin("wl-copy"):
		cmd = exec.Command("wl-copy")
	case hasBin("xclip"):
		cmd = exec.Command("xclip", "-selection", "clipboard")
	case hasBin("xsel"):
		cmd = exec.Command("xsel", "-b")
	case hasBin("pbcopy"):
		cmd = exec.Command("pbcopy")
	case hasBin("clip.exe"):
		cmd = exec.Command("clip.exe")
	default:
		return fmt.Errorf("no clipboard tool (wl-copy/xclip/xsel/pbcopy)")
	}
	cmd.Stdin = strings.NewReader(s)
	return cmd.Run()
}

func hasBin(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func cmdKeys(ctx *SessionCtx, args string) error {
	ctx.Printf("Keys:\n")
	ctx.Printf("  enter        send · alt-enter newline in composer\n")
	ctx.Printf("  esc          abort turn · close dialogs · quit when idle\n")
	ctx.Printf("  ctrl+c       abort turn · quit when idle\n")
	ctx.Printf("  ctrl+l       model picker (Tab all/scoped · ctrl+s default)\n")
	ctx.Printf("  ctrl+p       cycle scoped models (shift+ctrl+p previous)\n")
	ctx.Printf("  tab          complete palette selection · scope toggle in /model\n")
	ctx.Printf("  ↑↓           navigate lists · type to filter selectors · history in line mode\n")
	ctx.Printf("  1 / 2        approve / reject on the approval card\n")
	ctx.Printf("  ctrl+a/x     (scoped-models) enable all / clear all\n")
	ctx.Printf("  alt+↑↓       (scoped-models) reorder · ctrl+s saves\n")
	return nil
}

func cmdCompact(ctx *SessionCtx, args string) error {
	msgs, err := csession.LoadMessages(ctx.Core.DB, ctx.Session.ID, 100)
	if err != nil || len(msgs) < 10 {
		ctx.Printf("Nothing to compact yet.\n")
		return nil
	}
	eng := ctx.Core.EngineForRole("conversation")
	if eng.LLM == nil {
		return fmt.Errorf("no model for summarization")
	}
	var sb strings.Builder
	for _, m := range msgs[:len(msgs)-4] {
		sb.WriteString(m.Role + ": " + truncateStr(m.Content, 500) + "\n")
	}
	out, err := eng.LLM.Complete(llm.Request{
		System:      "Summarize this work-acquisition session in 10 terse bullets: profile facts, opportunities reviewed, decisions, pending approvals.",
		Messages:    []llm.Message{{Role: "user", Content: sb.String()}},
		Temperature: 0.2, MaxTokens: 600,
	})
	if err != nil {
		return err
	}
	keep := len(msgs) - 4
	if keep < 0 {
		keep = 0
	}
	if err := csession.ReplaceTail(ctx.Core.DB, ctx.Session.ID, keep, out); err != nil {
		return err
	}
	ctx.Printf("Compacted: kept last 4 messages + summary.\n")
	return nil
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
