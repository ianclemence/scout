package tui

import (
	"context"
	"os/exec"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ianclemence/scout/pkg/mcpauth"
)

// mcpLoginResultMsg carries the outcome of a background MCP OAuth flow.
type mcpLoginResultMsg struct {
	name string
	cred *mcpauth.Credential
	err  error
}

// mcpLoginUI is the modal shown while an MCP OAuth flow runs. The callback
// completes automatically when the browser is on this machine; otherwise the
// user pastes the redirect URL or code into the field.
type mcpLoginUI struct {
	name   string
	url    string
	input  textinput.Model
	errMsg string
}

func newMCPLoginUI(name, authorizeURL string) *mcpLoginUI {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 2000
	ti.Focus()
	return &mcpLoginUI{name: name, url: authorizeURL, input: ti}
}

func (u *mcpLoginUI) view(width int) string {
	if width < 20 {
		width = 20
	}
	rule := stylePromptBar.Render(strings.Repeat("─", width))
	var b strings.Builder
	b.WriteString(rule + "\n")
	b.WriteString(" " + styleModalTitle.Render("Sign in to "+u.name) + "\n\n")
	b.WriteString(" " + styleAssistant.Render("Open this URL in a browser to authorize:") + "\n")
	for _, ln := range wrapANSI(u.url, width-4) {
		b.WriteString(" " + styleNotice.Render(ln) + "\n")
	}
	b.WriteString("\n " + styleAssistant.Render("Waiting for authorization…") + "\n")
	b.WriteString(" " + styleFooterHint.Render("If the browser is on another machine, paste the final redirect URL or code:") + "\n")
	b.WriteString(" " + u.input.View())
	if u.errMsg != "" {
		b.WriteString("\n " + styleError.Render(u.errMsg))
	}
	b.WriteString("\n\n" + styleFooterHint.Render("  esc to cancel · enter to submit pasted code"))
	b.WriteString("\n" + rule)
	return b.String()
}

// startMCPLogin begins an OAuth flow for a connector and opens the modal.
func (m *model) startMCPLogin(name string) (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	flow, canonical, err := m.st.Core.BeginMCPLogin(ctx, name)
	if err != nil {
		cancel()
		m.println(entry{kind: eErr, text: "Sign-in failed: " + err.Error(), at: time.Now()})
		return m, m.flushCmds()
	}
	m.mcpFlow = flow
	m.mcpFlowCancel = cancel
	m.mcpLogin = newMCPLoginUI(canonical, flow.AuthorizeURL())
	_ = openBrowser(flow.AuthorizeURL())
	return m, waitMCPLoginCmd(flow, canonical, ctx)
}

// waitMCPLoginCmd waits for the callback or a pasted code and exchanges it.
func waitMCPLoginCmd(flow *mcpauth.Flow, name string, ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		cred, err := flow.Wait(ctx)
		flow.Close()
		return mcpLoginResultMsg{name: name, cred: cred, err: err}
	}
}

func (m *model) handleMCPLoginKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		if m.mcpFlow != nil {
			m.mcpFlow.Close()
		}
		if m.mcpFlowCancel != nil {
			m.mcpFlowCancel()
		}
		m.mcpLogin = nil
		m.mcpFlow = nil
		m.mcpFlowCancel = nil
		return m, nil
	case "enter":
		if v := strings.TrimSpace(m.mcpLogin.input.Value()); v != "" && m.mcpFlow != nil {
			if !m.mcpFlow.Submit(v) {
				m.mcpLogin.errMsg = "That doesn't look like a redirect URL or authorization code."
			}
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.mcpLogin.input, cmd = m.mcpLogin.input.Update(msg)
	_ = cmd
	return m, nil
}

func (m *model) finishMCPLogin(msg mcpLoginResultMsg) (tea.Model, tea.Cmd) {
	if m.mcpFlowCancel != nil {
		m.mcpFlowCancel()
	}
	if m.mcpLogin == nil {
		return m, nil
	}
	if msg.err != nil {
		m.mcpLogin.errMsg = msg.err.Error()
		m.mcpFlow = nil
		return m, nil
	}
	if err := m.st.Core.SaveMCPCredential(msg.name, msg.cred); err != nil {
		m.mcpLogin.errMsg = "Failed to save sign-in: " + err.Error()
		return m, nil
	}
	m.mcpLogin = nil
	m.mcpFlow = nil
	m.mcpFlowCancel = nil
	m.refreshConnHint()
	return m, tea.Println(renderEntryStatic(entry{kind: eNotice, text: "Signed in to " + msg.name + ". Credential stored (encrypted)."}))
}

// openBrowser launches the platform browser for an authorization URL. Failure
// is non-fatal: the URL is shown in the modal for manual use.
func openBrowser(url string) error {
	switch goruntime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
