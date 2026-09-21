package tui

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// openLoginFlow starts the staged login experience.
//
//	/login              -> authentication-method selector
//	/login <provider>   -> straight to that provider's key dialog
func (m *model) openLoginFlow(arg string) {
	if p := strings.ToLower(strings.TrimSpace(arg)); p != "" {
		if !validLoginProvider(p) {
			m.println(entry{kind: eErr, text: "Unknown provider. Choose: openai, anthropic, deepseek, moonshot.", at: time.Now()})
			return
		}
		f := newLoginFlow()
		f.openKeyStage(p, providerDisplay(p))
		m.login = f
		return
	}
	m.login = newLoginFlow()
}

// openLogoutFlow starts the stored-credential selector.
func (m *model) openLogoutFlow() {
	if len(m.st.Core.StoredProviders()) == 0 {
		m.println(entry{kind: eNotice, text: "No stored credentials to remove. /logout only removes credentials saved by /login; environment variables are unchanged.", at: time.Now()})
		return
	}
	f := newLoginFlow()
	f.openLogoutStage(m.st.Core)
	m.login = f
}

func validLoginProvider(p string) bool {
	switch p {
	case "openai", "anthropic", "deepseek", "moonshot":
		return true
	}
	return false
}

// handleLoginKey routes a key to the active login flow and applies its actions.
func (m *model) handleLoginKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	res := m.login.handleKey(msg)

	switch {
	case res.done:
		m.login = nil
		return m, nil
	case res.logout != "":
		m.login = nil
		return m.removeStoredKey(res.logout)
	case res.moveTo != nil && *res.moveTo == loginStageProvider && res.method != "":
		m.login.openProviderStage(m.st.Core, res.method, "")
		return m, nil
	case res.moveTo != nil && *res.moveTo == loginStageMethod:
		m.login = newLoginFlow()
		return m, nil
	case res.moveTo != nil && *res.moveTo == loginStageKey && res.provider != "":
		m.login.openKeyStage(res.provider, providerDisplay(res.provider))
		return m, nil
	case res.submit:
		return m, m.submitAuthDialog()
	}
	return m, nil
}

// submitAuthDialog saves the entered key and verifies it where cheap.
func (m *model) submitAuthDialog() tea.Cmd {
	f := m.login
	if f == nil {
		return nil
	}
	key := strings.TrimSpace(f.input.Value())
	if key == "" {
		f.errMsg = "Empty key — try again."
		return nil
	}
	if err := m.st.Core.SaveSecret("llm:"+f.provider, key); err != nil {
		f.errMsg = "Failed to save: " + err.Error()
		return nil
	}
	m.login = nil
	msg := "Saved API key for " + providerDisplay(f.provider) + "."
	if v := verifyKey(m, f.provider, key); v != "" {
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
		return "(key saved; ready to use)"
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
		return fmt.Sprintf("(saved, but verification failed: HTTP %d — check the key)", resp.StatusCode)
	}
	return "(verified against the provider)"
}
