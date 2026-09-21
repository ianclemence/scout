package isession

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/ianclemence/scout/pkg/csession"
	"github.com/ianclemence/scout/pkg/llm"
)

func cmdLogin(ctx *SessionCtx, args string) error {
	p := strings.ToLower(firstField(args))
	switch p {
	case "openai", "anthropic", "deepseek", "moonshot":
	default:
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
	switch p {
	case "openai", "anthropic", "deepseek", "moonshot":
	default:
		return fmt.Errorf("usage: /logout <openai|anthropic|deepseek|moonshot>")
	}
	_, err := ctx.Core.DB.DB.Exec(`DELETE FROM secrets WHERE key=?`, "llm:"+p)
	if err != nil {
		return err
	}
	ctx.Printf("%s stored key removed. (Environment variable, if set, still applies.)\n", p)
	return nil
}

// cmdModel switches the session conversation model, with an interactive selector.
func cmdModel(ctx *SessionCtx, args string) error {
	if a := strings.TrimSpace(args); a != "" {
		prov, model := splitModelRef(a)
		if prov == "" || model == "" {
			return fmt.Errorf("usage: /model <provider/model>  (e.g. /model ollama/qwen3:0.6b)")
		}
		ctx.Session.Provider, ctx.Session.Model = prov, model
		csession.Touch(ctx.Core.DB, ctx.Session.ID, prov, model)
		ctx.Printf("Session model → %s/%s\n", prov, model)
		return nil
	}
	options := modelOptions(ctx)
	ctx.Printf("Select conversation model (number):\n")
	for i, o := range options {
		mark := ""
		if o.prov == ctx.Session.Provider && o.model == ctx.Session.Model {
			mark = "  ← current"
		}
		ctx.Printf("  %2d  %-18s %s%s\n", i+1, o.prov+"/"+o.model, o.note, mark)
	}
	ctx.Printf("Choice (or /model provider/model): ")
	choice, err := readLineCooked()
	if err != nil || choice == "" {
		return nil
	}
	var n int
	if _, err := fmt.Sscanf(choice, "%d", &n); err != nil || n < 1 || n > len(options) {
		// Maybe they typed a ref.
		return cmdModel(ctx, choice)
	}
	o := options[n-1]
	ctx.Session.Provider, ctx.Session.Model = o.prov, o.model
	csession.Touch(ctx.Core.DB, ctx.Session.ID, o.prov, o.model)
	ctx.Printf("Session model → %s/%s\n", o.prov, o.model)
	return nil
}

type modelOption struct {
	prov, model, note string
}

func modelOptions(ctx *SessionCtx) []modelOption {
	var out []modelOption
	for _, role := range []string{"screening", "analysis", "proposal", "conversation", "deep_analysis"} {
		r := ctx.Core.Cfg.Models[role]
		out = append(out, modelOption{r.Provider, r.Model, "role:" + role})
	}
	for _, m := range ctx.Core.Registry().List(ctxBg(), "") {
		note := "reasoning:" + m.Reasoning
		if m.Source == "ollama" || m.Provider == "ollama" {
			note = "local"
		}
		out = append(out, modelOption{m.Provider, m.ID, note})
	}
	seen := map[string]bool{}
	var dedup []modelOption
	for _, o := range out {
		if o.prov == "" || o.model == "" {
			continue
		}
		k := o.prov + "/" + o.model
		if !seen[k] {
			seen[k] = true
			dedup = append(dedup, o)
		}
	}
	sort.Slice(dedup, func(i, j int) bool { return dedup[i].prov < dedup[j].prov })
	return dedup
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
	ctx.Printf("  ctrl+l       model picker\n")
	ctx.Printf("  tab          complete palette selection\n")
	ctx.Printf("  ↑↓           navigate lists · history in line mode\n")
	ctx.Printf("  1 / 2        approve / reject on the approval card\n")
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
