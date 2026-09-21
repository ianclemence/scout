package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ianclemence/scout/pkg/csession"
	"github.com/ianclemence/scout/pkg/isession"
)

// TestLoginViewRendersScoutPalette drives the staged login flow through Update
// and asserts the rendered dock contains the expected Scout-styled content.
func TestLoginViewRendersScoutPalette(t *testing.T) {
	core := testCore(t)
	st := &isession.ReplState{Core: core, Sess: &csession.Session{Provider: "ollama", Model: "qwen3:0.6b"}}
	m := initialModel(st)
	m.width, m.height, m.ready = 100, 40, true

	// /login -> method stage.
	m.ta.SetValue("/login")
	nm, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(*model)
	if m.login == nil {
		t.Fatal("login flow did not open")
	}
	v := m.View()
	if !strings.Contains(v, "Select authentication method") || !strings.Contains(v, "Sign in with an API key") {
		t.Fatalf("method stage not rendered:\n%s", v)
	}

	// Enter -> provider stage.
	nm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(*model)
	v = m.View()
	if !strings.Contains(v, "Select provider to configure") {
		t.Fatalf("provider stage not rendered:\n%s", v)
	}
	if !strings.Contains(v, "OpenAI") || !strings.Contains(v, "DeepSeek") {
		t.Fatalf("provider list missing entries:\n%s", v)
	}

	// Choose DeepSeek (filter then enter) -> key stage.
	for _, r := range "deepseek" {
		nm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(*model)
	}
	nm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(*model)
	v = m.View()
	if !strings.Contains(v, "Login to DeepSeek") || !strings.Contains(v, "API key") {
		t.Fatalf("key stage not rendered:\n%s", v)
	}

	// Type a key and submit; it must be stored encrypted, never echoed.
	for _, r := range "secret-key" {
		nm, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(*model)
	}
	if strings.Contains(m.View(), "secret-key") {
		t.Fatalf("key must not be echoed in plain text:\n%s", m.View())
	}
	nm, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(*model)
	if m.login != nil {
		t.Fatal("flow should close after submit")
	}
	if cmd == nil {
		t.Fatal("submit should emit a confirmation print command")
	}
	if got, err := core.LoadSecret("llm:deepseek"); err != nil || got != "secret-key" {
		t.Fatalf("key not stored correctly: %q err=%v", got, err)
	}
}

// TestLogoutViewRenders ensures the logout selector renders stored providers.
func TestLogoutViewRenders(t *testing.T) {
	core := testCore(t)
	if err := core.SaveSecret("llm:openai", "k"); err != nil {
		t.Fatal(err)
	}
	st := &isession.ReplState{Core: core, Sess: &csession.Session{}}
	m := initialModel(st)
	m.width, m.height, m.ready = 100, 40, true
	m.ta.SetValue("/logout")
	nm, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(*model)
	v := m.View()
	if !strings.Contains(v, "Select provider to logout") || !strings.Contains(v, "OpenAI") {
		t.Fatalf("logout selector not rendered:\n%s", v)
	}
}
