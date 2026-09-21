package tui

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// authDialog replaces the composer during login, mirroring Pi's login
// dialog: titled box, masked key prompt, esc cancels, enter submits.
type authDialog struct {
	provider string
	input    textinput.Model
	err      string
}

func newAuthDialog(provider string) authDialog {
	ti := textinput.New()
	ti.Prompt = ""
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '•'
	ti.CharLimit = 300
	ti.Focus()
	return authDialog{provider: provider, input: ti}
}

func (m *model) openLoginFlow(arg string) {
	if p := strings.ToLower(strings.TrimSpace(arg)); p != "" {
		if !validLoginProvider(p) {
			m.println(entry{kind: eErr, text: "Unknown provider. Choose: openai, anthropic, deepseek, moonshot.", at: time.Now()})
			return
		}
		d := newAuthDialog(p)
		m.auth = &d
		return
	}
	// No provider: pick from all key providers with live state.
	items := []selItem{}
	for _, p := range []string{"openai", "anthropic", "deepseek", "moonshot"} {
		items = append(items, selItem{label: p, detail: loginProviderState(m, p), value: p})
	}
	m.sel = &selector{title: "Login — choose provider", items: items}
	m.selMode = "login"
}

func validLoginProvider(p string) bool {
	switch p {
	case "openai", "anthropic", "deepseek", "moonshot":
		return true
	}
	return false
}

func loginProviderState(m *model, p string) string {
	if s, err := m.st.Core.LoadSecret("llm:" + p); err == nil && s != "" {
		return "key stored"
	}
	env := map[string]string{"openai": "OPENAI_API_KEY", "anthropic": "ANTHROPIC_API_KEY", "deepseek": "DEEPSEEK_API_KEY", "moonshot": "MOONSHOT_API_KEY"}[p]
	if env != "" && os.Getenv(env) != "" {
		return "key in env (" + env + ")"
	}
	return "no key"
}

func (m *model) openLogoutFlow() {
	stored := m.st.Core.StoredProviders()
	if len(stored) == 0 {
		m.println(entry{kind: eNotice, text: "No stored credentials to remove. /logout only removes credentials saved by /login; environment variables are unchanged.", at: time.Now()})
		return
	}
	items := []selItem{}
	for _, p := range stored {
		items = append(items, selItem{label: p, detail: "stored key — enter removes", value: p})
	}
	m.sel = &selector{title: "Logout — remove stored key", items: items}
	m.selMode = "logout"
}

// submitAuthDialog saves the key and verifies it where cheap.
func (m *model) submitAuthDialog() tea.Cmd {
	a := m.auth
	m.auth = nil
	key := strings.TrimSpace(a.input.Value())
	if key == "" {
		return tea.Println(renderEntryStatic(entry{kind: eErr, text: "Login cancelled — empty key."}))
	}
	if err := m.st.Core.SaveSecret("llm:"+a.provider, key); err != nil {
		return tea.Println(renderEntryStatic(entry{kind: eErr, text: "Failed to save API key for " + a.provider + ": " + err.Error()}))
	}
	msg := "Saved API key for " + a.provider + "."
	if v := verifyKey(m, a.provider, key); v != "" {
		msg += " " + v
	}
	return tea.Println(renderEntryStatic(entry{kind: eNotice, text: msg}))
}

// endpointFor resolves the chat-compatible base URL for verification.
func endpointFor(m *model, provider string) string {
	if m.st.Core == nil {
		return ""
	}
	reg := m.st.Core.Registry()
	if reg.Endpoint != nil {
		return reg.Endpoint(provider)
	}
	return ""
}

// verifyKey performs a cheap live check where the provider allows one
// (/models for OpenAI-compatible APIs). Anthropic exposes no key-check
// endpoint, so its keys save unverified.
func verifyKey(m *model, provider, key string) string {
	ep := ""
	switch provider {
	case "openai", "deepseek", "moonshot":
		ep = endpointFor(m, provider)
	}
	if ep == "" {
		return "(key saved; restart-free and ready)"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", ep+"/models", nil)
	if err != nil {
		return "(saved, verification skipped)"
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "(saved, verification skipped: unreachable)"
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Sprintf("(saved, but verification failed: HTTP %d — check the key with /login %s)", resp.StatusCode, provider)
	}
	return "(verified against the provider)"
}
