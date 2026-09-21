package tui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ianclemence/scout/pkg/config"
	"github.com/ianclemence/scout/pkg/csession"
	"github.com/ianclemence/scout/pkg/isession"
	"github.com/ianclemence/scout/pkg/runtime"
	"github.com/ianclemence/scout/pkg/store"
)

func timeNow() time.Time { return time.Now() }

func TestRenderMarkdown(t *testing.T) {
	out := RenderMarkdown("# Title\n\nSome **bold** and `code` text.\n\n- item one\n- item two\n\n```go\nfmt.Println()\n```\n\n> quoted")
	if !strings.Contains(out, "Title") || !strings.Contains(out, "bold") || !strings.Contains(out, "code") {
		t.Fatalf("lost content: %q", out)
	}
	if strings.Contains(out, "**") || strings.Contains(out, "# Title") {
		t.Fatalf("markup not rendered: %q", out)
	}
	if RenderMarkdown("plain") != "plain" {
		t.Fatal("plain text altered")
	}
}

func TestRenderEntryKinds(t *testing.T) {
	m := &model{width: 80, st: &isession.ReplState{Sess: &csession.Session{Provider: "ollama", Model: "qwen3:0.6b"}}}
	render := func(e entry) string { return m.renderEntry(e) }
	for _, e := range []entry{
		{kind: eUser, text: "hi"},
		{kind: eScout, text: "hello **there**"},
		{kind: eTool, text: "search"},
		{kind: eNotice, text: "note"},
		{kind: eApproval, text: "approve me"},
		{kind: eErr, text: "boom"},
	} {
		if s := render(e); s == "" {
			t.Fatalf("empty render for kind %d", e.kind)
		}
	}
	if s := render(entry{kind: eApproval, text: "x"}); !strings.Contains(s, "◆") {
		t.Fatal("approval must be prominent")
	}
	if s := render(entry{kind: eScout, text: "hi"}); !strings.Contains(s, "👷 Scout") {
		t.Fatal("assistant label must carry the builder mark")
	}
}

func TestTruncateWrap(t *testing.T) {
	if truncate("abcdef", 4) != "abc…" {
		t.Fatal("truncate wrong")
	}
	if len(wrap("abcdefghij", 4)) != 3 {
		t.Fatal("wrap wrong")
	}
}

func keyRunes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// TestPaletteHasNoGroupTags locks the Pi-style command palette: each row is
// the command name plus a description (argument hint folded in), with no
// [Group] tag next to the command.
func TestPaletteHasNoGroupTags(t *testing.T) {
	m := testModel()
	m.openPalette("")
	if len(m.sel.items) == 0 {
		t.Fatal("palette should list commands")
	}
	for _, it := range m.sel.items {
		if !strings.HasPrefix(it.label, "/") {
			t.Fatalf("label should be a slash command, got %q", it.label)
		}
		if strings.ContainsAny(it.label, "[]") {
			t.Fatalf("label must not carry a group tag: %q", it.label)
		}
		for _, tag := range []string{"[Work]", "[Decide]", "[You]", "[Connect]", "[Session]"} {
			if strings.Contains(it.detail, tag) {
				t.Fatalf("description must not carry a group tag: %q", it.detail)
			}
		}
	}
	// The argument hint folds into the description with an em dash.
	found := false
	for _, it := range m.sel.items {
		if it.label == "/analyze" {
			found = true
			if !strings.Contains(it.detail, "<id>") || !strings.Contains(it.detail, "—") {
				t.Fatalf("argument hint should fold into the description: %q", it.detail)
			}
		}
	}
	if !found {
		t.Fatal("expected /analyze in the palette")
	}
}

func TestPaletteSlashFlow(t *testing.T) {
	m := testModel()
	m.width, m.height, m.ready = 80, 24, true
	// Type "/": slash stays visible, palette opens unfiltered.
	nm, _ := m.handleKey(keyRunes("/"))
	m = nm.(*model)
	if m.ta.Value() != "/" {
		t.Fatalf("slash must stay visible, got %q", m.ta.Value())
	}
	if m.sel == nil || m.selMode != "palette" {
		t.Fatal("palette should open")
	}
	if len(m.sel.items) == 0 {
		t.Fatal("unfiltered palette should list commands")
	}
	// Type "mod": palette filters.
	for _, r := range []string{"m", "o", "d"} {
		nm, _ := m.handleKey(keyRunes(r))
		m = nm.(*model)
	}
	if m.palFilter != "mod" {
		t.Fatalf("filter = %q", m.palFilter)
	}
	for _, it := range m.sel.items {
		name := strings.ToLower(strings.TrimPrefix(it.label, "/"))
		desc := strings.ToLower(it.detail)
		if !strings.HasPrefix(name, "mod") && !strings.Contains(desc, "mod") {
			t.Fatalf("palette item does not match filter: %q / %q", it.label, it.detail)
		}
	}
	// Backspace all the way: palette dismisses, composer keeps "".
	for i := 0; i < 4; i++ {
		nm, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
		m = nm.(*model)
	}
	if m.sel != nil {
		t.Fatal("palette should dismiss when slash erased")
	}
	if m.ta.Value() != "" {
		t.Fatalf("composer should be empty, got %q", m.ta.Value())
	}
}

func testModel() *model {
	st := &isession.ReplState{Sess: &csession.Session{Provider: "ollama", Model: "qwen3:0.6b"}}
	m := initialModel(st)
	m.width, m.height, m.ready = 80, 24, true
	return m
}

func TestComposerRules(t *testing.T) {
	m := testModel()
	idle := m.composerTopRule()
	if !strings.Contains(idle, "──") {
		t.Fatal("idle composer must be a plain rule")
	}
	m.working = true
	m.turnFrom = timeNow()
	w := m.composerTopRule()
	if !strings.Contains(w, "Working") && !strings.Contains(w, "──") {
		t.Fatalf("working rule must carry status: %q", w)
	}
}

func TestWelcomeCard(t *testing.T) {
	m := testModel()
	card := m.welcomeCard()
	for _, want := range []string{"S C O U T", "Find work worth doing.", "/help", "/model"} {
		if !strings.Contains(card, want) {
			t.Fatalf("welcome card missing %q", want)
		}
	}
}

func TestApprovalCard(t *testing.T) {
	m := testModel()
	m.approval = &pendingApproval{id: "a1", title: "submit_proposal → opp-1", risk: "high"}
	card := m.approvalCard()
	for _, want := range []string{"Approval required", "high", "[1] approve", "[2] reject"} {
		if !strings.Contains(card, want) {
			t.Fatalf("approval card missing %q", want)
		}
	}
}

func TestFooter(t *testing.T) {
	m := testModel()
	if s := m.footerStats(); !strings.Contains(s, "ollama/qwen3:0.6b") || !strings.Contains(s, "local") {
		t.Fatalf("footer must show locality + model: %q", s)
	}
	if s := m.footerKeys(); !strings.Contains(s, "/ commands") {
		t.Fatalf("footer keys missing: %q", s)
	}
	// Idle keys are justified: the command hint leads, the exit hint trails.
	m.width = 80
	keys := m.footerKeys()
	if !strings.HasPrefix(keys, "/ commands") || !strings.HasSuffix(keys, "esc quit") {
		t.Fatalf("idle footer must justify / commands … esc quit: %q", keys)
	}
	if !strings.Contains(keys, strings.Repeat(" ", 20)) {
		t.Fatalf("idle footer must separate the ends with padding: %q", keys)
	}
}

func TestAutoName(t *testing.T) {
	if got := autoName("  Find me Go backend work please  "); got != "Find me Go backend work please" {
		t.Fatalf("bad name: %q", got)
	}
	if got := autoName(strings.Repeat("x", 100)); len([]rune(got)) > 41 {
		t.Fatalf("name not capped: %q", got)
	}
}

func TestSwitchSession(t *testing.T) {
	core := testCore(t)
	a, _ := csession.Create(core.DB, "aaa", "ollama", "m")
	b, _ := csession.Create(core.DB, "bbb", "ollama", "m")
	_ = csession.AppendMessages(core.DB, b.ID, []csession.Message{{Role: "user", Content: "hi b"}})
	st := &isession.ReplState{Core: core, Sess: a}
	m := initialModel(st)
	if err := m.switchSession(b); err != nil {
		t.Fatal(err)
	}
	if m.st.Sess.ID != b.ID || len(m.st.History) != 1 || m.st.History[0].Content != "hi b" {
		t.Fatal("session switch must swap identity + history")
	}
}

func testCore(t *testing.T) *runtime.Core {
	t.Helper()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	db, err := store.Open(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	c, err := runtime.New(cfg, db)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAuthDialogFlow(t *testing.T) {
	core := testCore(t)
	st := &isession.ReplState{Core: core, Sess: &csession.Session{Provider: "ollama", Model: "qwen3:0.6b"}}
	m := initialModel(st)
	m.width, m.ready = 80, true
	// /login deepseek opens the masked key stage, not cooked output.
	nm, _ := m.runCommand("login deepseek")
	m = nm.(*model)
	if m.login == nil || m.login.stage != loginStageKey || m.login.provider != "deepseek" {
		t.Fatal("login key stage should open")
	}
	if card := m.login.view(80); !strings.Contains(card, "Login to DeepSeek") || !strings.Contains(card, "esc to cancel") {
		t.Fatalf("bad dialog card: %q", card)
	}
	// Esc cancels without storing.
	nm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(*model)
	if m.login != nil {
		t.Fatal("esc should close the flow")
	}
	if _, err := core.LoadSecret("llm:deepseek"); err == nil {
		t.Fatal("cancelled login must not store")
	}
	// Unknown provider rejected.
	nm, _ = m.runCommand("login nope")
	m = nm.(*model)
	if m.login != nil {
		t.Fatal("unknown provider must not open the flow")
	}
}

func TestLoginStagedFlow(t *testing.T) {
	core := testCore(t)
	st := &isession.ReplState{Core: core, Sess: &csession.Session{}}
	m := initialModel(st)
	m.width, m.ready = 80, true

	// /login with no argument starts at the method stage.
	nm, _ := m.runCommand("login")
	m = nm.(*model)
	if m.login == nil || m.login.stage != loginStageMethod {
		t.Fatal("bare /login should open the method selector")
	}
	if v := m.login.view(80); !strings.Contains(v, "Select authentication method") {
		t.Fatalf("method stage missing: %q", v)
	}
	// Both methods are offered, account first (matching the reference UI).
	if len(m.login.methods) != 2 || m.login.methods[0].authType != "account" || m.login.methods[1].authType != "api_key" {
		t.Fatalf("method list wrong: %+v", m.login.methods)
	}
	// Account sign-in has a provider (Anthropic): Enter moves to the account
	// provider list rather than dead-ending on the method stage.
	nm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(*model)
	if m.login == nil || m.login.stage != loginStageProvider ||
		len(m.login.filtered) != 1 || m.login.filtered[0].id != "anthropic" || m.login.filtered[0].authType != "account" {
		t.Fatalf("account sign-in should list Anthropic: %+v", m.login)
	}
	// Esc returns to the method stage; choose API key and continue.
	nm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(*model)
	if m.login == nil || m.login.stage != loginStageMethod {
		t.Fatalf("esc should return to the method stage: %+v", m.login)
	}
	// Move down to "Sign in with an API key" and Enter: the provider stage
	// opens and lists providers.
	nm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	m = nm.(*model)
	nm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(*model)
	if m.login.stage != loginStageProvider || len(m.login.providers) == 0 {
		t.Fatalf("provider stage not reached: %+v", m.login)
	}
	// Filtering narrows the list.
	for _, r := range "deepseek" {
		m.login.search += string(r)
		m.login.rebuild()
	}
	if len(m.login.filtered) != 1 || m.login.filtered[0].id != "deepseek" {
		t.Fatalf("filter failed: %+v", m.login.filtered)
	}
	// Esc from the provider stage returns to the method stage.
	nm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(*model)
	if m.login == nil || m.login.stage != loginStageMethod {
		t.Fatalf("esc should return to method stage: %+v", m.login)
	}
}

func TestLogoutListsStoredOnly(t *testing.T) {
	core := testCore(t)
	st := &isession.ReplState{Core: core, Sess: &csession.Session{}}
	m := initialModel(st)
	m.width, m.ready = 80, true
	if err := core.SaveSecret("llm:openai", "k"); err != nil {
		t.Fatal(err)
	}
	nm, _ := m.runCommand("logout")
	m = nm.(*model)
	if m.login == nil || m.login.stage != loginStageLogout || len(m.login.filtered) != 1 {
		t.Fatalf("logout should list stored only: %+v", m.login)
	}
	nm, _ = m.removeStoredKey("openai")
	m = nm.(*model)
	if _, err := core.LoadSecret("llm:openai"); err == nil {
		t.Fatal("key should be removed")
	}
}
