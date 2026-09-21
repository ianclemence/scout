package tui

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ianclemence/scout/pkg/oauth"
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
		if m.oauthFlow != nil {
			m.oauthFlow.Close()
			m.oauthFlow = nil
		}
		m.login = nil
		return m, nil
	case res.logout != "":
		m.login = nil
		return m.removeStoredKey(res.logout)
	case res.manual != "":
		if m.oauthFlow == nil || !m.oauthFlow.Submit(res.manual) {
			if m.login != nil {
				m.login.errMsg = "That doesn't look like a redirect URL or authorization code."
			}
		}
		return m, nil
	case res.moveTo != nil && *res.moveTo == loginStageOAuth && res.provider != "":
		return m.startOAuthLogin(res.provider)
	case res.moveTo != nil && *res.moveTo == loginStageProvider && res.method != "":
		// Account sign-in with no account provider would be an empty list;
		// guard so the UI never dead-ends.
		if res.method == "account" && !hasAccountProviders(m.st.Core) {
			m.login = newLoginFlow()
			return m, tea.Println(styleNotice.Render("No account sign-in providers are available yet — choose \"Sign in with an API key\"."))
		}
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

// startOAuthLogin begins an account (subscription) sign-in. It opens the
// authorization URL in the browser and runs the PKCE callback/exchange in the
// background; the dialog also accepts a pasted redirect URL for remote
// browsers.
func (m *model) startOAuthLogin(provider string) (tea.Model, tea.Cmd) {
	if provider != "anthropic" {
		m.login.errMsg = "Account sign-in is not available for " + provider + " yet."
		return m, nil
	}
	flow, err := oauth.NewFlow()
	if err != nil {
		m.login.errMsg = "Could not start sign-in: " + err.Error()
		return m, nil
	}
	// Best-effort loopback listener; manual paste still works if it fails.
	_ = flow.Start()
	m.oauthFlow = flow
	url := flow.AuthorizeURL()
	m.login.openOAuthStage(provider, url)
	_ = openBrowser(url)
	return m, waitOAuthCmd(flow, provider)
}

// waitOAuthCmd waits for the callback (or a pasted code) and exchanges it.
func waitOAuthCmd(flow *oauth.Flow, provider string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		code, err := flow.Wait(ctx)
		if err != nil {
			flow.Close()
			return oauthResultMsg{provider: provider, err: err}
		}
		cred, err := flow.Exchange(ctx, code)
		flow.Close()
		return oauthResultMsg{provider: provider, cred: cred, err: err}
	}
}

// finishOAuthLogin stores a completed account sign-in, or shows the error in
// the dialog so the user can retry or paste a code.
func (m *model) finishOAuthLogin(msg oauthResultMsg) (tea.Model, tea.Cmd) {
	if m.login == nil {
		return m, nil
	}
	if msg.err != nil {
		m.login.errMsg = msg.err.Error()
		return m, nil
	}
	if err := m.st.Core.SaveSecret("llm:"+msg.provider, msg.cred.Encode()); err != nil {
		m.login.errMsg = "Failed to save sign-in: " + err.Error()
		return m, nil
	}
	m.oauthFlow = nil
	m.login = nil
	return m, tea.Println(renderEntryStatic(entry{kind: eNotice, text: "Signed in to " + providerDisplay(msg.provider) + " with an account. Scout will use it for requests."}))
}

// openBrowser launches the platform browser for an authorization URL. Failure
// is non-fatal: the URL is shown in the dialog for manual use.
func openBrowser(url string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler"}
	default:
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
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
