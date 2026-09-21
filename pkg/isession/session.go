package isession

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chzyer/readline"
	"golang.org/x/term"

	"github.com/ianclemence/scout/pkg/csession"
	"github.com/ianclemence/scout/pkg/domain"
	"github.com/ianclemence/scout/pkg/llm"
	"github.com/ianclemence/scout/pkg/runtime"
	"github.com/ianclemence/scout/pkg/version"
)

var (
	errQuit        = fmt.Errorf("quit")
	errClearScreen = fmt.Errorf("clear")
)

func Version() string { return version.Version }

func ctxBg() context.Context { return context.Background() }

// ReplState holds interactive-only state (never persisted except history file).
type ReplState struct {
	Core     *runtime.Core
	Sess     *csession.Session
	LastOpps []domain.Opportunity
	History  []llm.Message // in-memory conversation context for the agent
	// ScopedModels is the session-local set/order of models available for
	// cycling. Loaded from app_settings at startup; changed by /scoped-models.
	ScopedModels ScopedModels
	// OpenScopedModels, when set (TUI), opens the interactive scoped-model
	// selector instead of the line-mode fallback.
	OpenScopedModels func()
	// OpenModelSelector, when set (TUI), opens the interactive model selector
	// instead of the line-mode fallback. The argument pre-fills the search.
	OpenModelSelector func(search string)
	// OpenThinking, when set (TUI), opens the interactive reasoning selector.
	OpenThinking func()
	// OpenSessions, when set (TUI), opens the interactive session picker.
	OpenSessions func()
	// OpenApprovals, when set (TUI), opens the interactive approval picker.
	OpenApprovals func()
	// OpenSources, when set (TUI), opens the interactive work-source manager.
	OpenSources func()
	// Out receives command output. Defaults to stdout printing.
	Out func(format string, a ...any)
	// Width is the terminal width for command output (0 = unknown).
	Width int
	// SwitchSession swaps the live session (TUI sets this).
	SwitchSession func(s *csession.Session) error
}

func (r *ReplState) ctx() *SessionCtx {
	out := r.Out
	if out == nil {
		out = func(f string, a ...any) { fmt.Printf(f, a...) }
	}
	return &SessionCtx{Core: r.Core, Session: r.Sess,
		Out:           out,
		ResolveOpp:    r.resolveOpp,
		Width:         r.Width,
		SwitchSession: r.SwitchSession,
		SetLastOpps: func(opps []domain.Opportunity) {
			r.LastOpps = opps
		},
		ScopedModels:      func() ScopedModels { return r.ScopedModels },
		SetScopedModels:   r.setScopedModels,
		OpenScopedModels:  r.OpenScopedModels,
		OpenModelSelector: r.OpenModelSelector,
		OpenThinking:      r.OpenThinking,
		OpenSessions:      r.OpenSessions,
		OpenApprovals:     r.OpenApprovals,
		OpenSources:       r.OpenSources,
	}
}

// SetScopedModels updates the in-memory selection and persists it. It is the
// exported entry point used by the TUI selector.
func (r *ReplState) SetScopedModels(ids []string) error {
	if err := SaveScopedModels(r.Core, ids); err != nil {
		return err
	}
	r.ScopedModels.Set(ids)
	return nil
}

// setScopedModels is the command-layer alias.
func (r *ReplState) setScopedModels(ids []string) error { return r.SetScopedModels(ids) }

// Dispatch runs a slash command line (without leading "/") against the state.
// It is shared by the line-mode loop and the full-screen TUI.
func Dispatch(st *ReplState, line string) error {
	return runSlash(st, line)
}

// resolveOpp accepts full id, id prefix, or 1-based index into last listing.
func (r *ReplState) resolveOpp(ref string) (*domain.Opportunity, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("usage: /opportunity <id>")
	}
	if n, err := strconv.Atoi(ref); err == nil {
		if n >= 1 && n <= len(r.LastOpps) {
			o := r.LastOpps[n-1]
			return r.Core.GetOpportunity(o.ID)
		}
		return nil, fmt.Errorf("no opportunity #%d in last listing", n)
	}
	var id string
	err := r.Core.DB.DB.QueryRow(`SELECT id FROM opportunities WHERE id=? OR id LIKE ? ORDER BY updated_at DESC LIMIT 1`, ref, ref+"%").Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("opportunity %q not found", ref)
	}
	return r.Core.GetOpportunity(id)
}

// Run starts the interactive session. Bare `scout` enters here.
func Run(core *runtime.Core, sess *csession.Session) error {
	st := &ReplState{Core: core, Sess: sess, ScopedModels: LoadScopedModels(core)}
	// Restore recent context for continuity.
	if msgs, err := csession.LoadMessages(core.DB, sess.ID, 20); err == nil {
		for _, m := range msgs {
			st.History = append(st.History, llm.Message{Role: m.Role, Content: m.Content})
		}
	}
	rl, err := readline.NewEx(&readline.Config{
		Prompt:            "scout› ",
		HistoryFile:       core.Cfg.DataDir + "/history",
		HistoryLimit:      500,
		AutoComplete:      &cmdCompleter{},
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
		HistorySearchFold: true,
	})
	if err != nil {
		return err
	}
	defer rl.Close()

	fmt.Printf("Scout %s · %s/%s · session %s\n", version.Version, sess.Provider, sess.Model, sess.ID[:12])
	fmt.Printf("Type /help for commands, or just ask. Ctrl-C interrupts · Ctrl-D exits.\n")
	for {
		line, err := rl.Readline()
		if err == readline.ErrInterrupt {
			continue
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "/") {
			if err := runSlash(st, strings.TrimPrefix(line, "/")); err == errQuit {
				return nil
			} else if err == errClearScreen {
				fmt.Print("\033[H\033[2J")
			} else if err != nil {
				fmt.Printf("error: %s\n", err)
			}
			continue
		}
		turnCtx, cancel := signalContext()
		handleAgentTurn(st, line, turnCtx)
		cancel()
	}
}

func runSlash(st *ReplState, line string) error {
	name := line
	args := ""
	if i := strings.Index(line, " "); i >= 0 {
		name, args = line[:i], strings.TrimSpace(line[i+1:])
	}
	cmd := FindCommand(name)
	if cmd == nil {
		return fmt.Errorf("unknown command /%s — try /help", name)
	}
	return cmd.Handler(st.ctx(), args)
}

// signalContext cancels on SIGINT (Ctrl-C) so long operations interrupt cleanly.
func signalContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	go func() {
		select {
		case <-ch:
			fmt.Printf("\n(interrupted)\n")
			cancel()
		case <-ctx.Done():
		}
		signal.Stop(ch)
	}()
	return ctx, cancel
}

// ActivityLabel maps a tool name to a short, human activity phrase used in
// the live status line ("Searching work…", "Drafting a proposal…"). It
// returns "" when no tool is active. One definition shared by the TUI and
// line mode keeps the wording consistent.
func ActivityLabel(name string) string {
	switch name {
	case "":
		return ""
	case "search_opportunities", "search_opportunity_history", "search_application_history",
		"discover_opportunities", "run_discovery", "find_duplicate_opportunity", "check_opportunity_status":
		return "Searching work"
	case "analyze_opportunity", "get_opportunity":
		return "Analyzing fit"
	case "prepare_proposal", "draft_cover_letter", "validate_application":
		return "Drafting a proposal"
	case "prepare_follow_up":
		return "Drafting a follow-up"
	case "answer_screening_questions":
		return "Answering questions"
	case "web_search":
		return "Searching the web"
	case "fetch_web_content", "github_repo_info", "github_repo_readme", "research_company":
		return "Researching"
	case "parse_document":
		return "Reading a document"
	case "get_profile", "get_user_cv", "get_user_experience", "get_user_portfolio",
		"get_user_skills", "get_user_preferences", "list_evidence", "search_user_evidence",
		"get_portfolio_evidence", "search_learned_preferences", "verify_claim":
		return "Reviewing your profile"
	case "save_opportunity", "record_application", "record_application_status",
		"record_opportunity_outcome", "record_learned_observation", "add_feedback",
		"update_user_profile":
		return "Saving"
	case "source_health", "get_source_capabilities", "list_sources":
		return "Checking sources"
	case "list_pending_approvals", "request_approval":
		return "Filing an approval"
	case "submit_application":
		return "Submitting"
	case "send_message":
		return "Sending a message"
	case "list_messages", "list_applications", "list_proposals", "get_pipeline":
		return "Reading the pipeline"
	case "load_skill":
		return "Loading a skill"
	}
	return "Working"
}

// handleAgentTurn runs one conversational agent turn with streaming render.
func handleAgentTurn(st *ReplState, input string, ctx context.Context) {
	eng := st.Core.EngineFor(st.Sess.Provider, st.Sess.Model)
	if eng.LLM == nil {
		fmt.Printf("No model configured. /login <provider> or /model ollama/<model>.\n")
		return
	}
	st.History = append(st.History, llm.Message{Role: "user", Content: input})
	msgs := append([]llm.Message{}, st.History...)
	var assistant strings.Builder
	// Line mode renders the same product-language activity the TUI shows
	// ("Working", "Searching work…"). Raw tool names, arguments, and tool
	// output are never printed into the conversation — only a short activity
	// line, overwritten in place — so model-loop internals cannot leak.
	lastActivity := ""
	_, err := st.Core.RunAgent(ctx, eng, msgs, st.Sess.Thinking, func(ev runtime.Event) {
		switch ev.Type {
		case "token":
			assistant.WriteString(ev.Text)
			fmt.Print(ev.Text)
		case "tool_start":
			if a := ActivityLabel(ev.Name); a != "" && a != lastActivity {
				lastActivity = a
				fmt.Printf("\r\033[K◐ %s…", a)
			}
		case "tool_end":
			// deliberately silent: tool results are internals, not chat.
		case "error":
			fmt.Printf("\r\033[K")
			fmt.Printf("error: %s\n", ev.Err)
		}
	})
	if lastActivity != "" {
		fmt.Print("\r\033[K")
	}
	fmt.Printf("\n")
	final := assistant.String()
	if err != nil && final == "" {
		st.History = st.History[:len(st.History)-1]
		return
	}
	st.History = append(st.History, llm.Message{Role: "assistant", Content: final})
	_ = csession.AppendMessages(st.Core.DB, st.Sess.ID, []csession.Message{
		{Role: "user", Content: input},
		{Role: "assistant", Content: final},
	})
	csession.Touch(st.Core.DB, st.Sess.ID, st.Sess.Provider, st.Sess.Model)
	if len(st.History) > 40 {
		st.History = st.History[len(st.History)-40:]
	}
}

type cmdCompleter struct{}

func (c *cmdCompleter) Do(line []rune, pos int) ([][]rune, int) {
	prefix := string(line[:pos])
	if !strings.HasPrefix(prefix, "/") || strings.Contains(prefix, " ") {
		return nil, 0
	}
	var out [][]rune
	for _, cmd := range Registry() {
		if strings.HasPrefix("/"+cmd.Name, prefix) {
			out = append(out, []rune("/" + cmd.Name)[len(prefix):])
		}
	}
	sort.Strings([]string{})
	return out, len(prefix)
}

// promptPassword reads a secret without echoing.
func promptPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

var _ = io.EOF
var _ = sort.Strings

// readLineCooked reads one line while the terminal is in cooked mode
// (used for short interactive prompts inside command handlers).
func readLineCooked() (string, error) {
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// probeOllama reports whether the local Ollama endpoint answers.
func probeOllama(host string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimSuffix(host, "/")+"/api/tags", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 500
}
