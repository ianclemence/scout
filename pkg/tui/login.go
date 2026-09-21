package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ianclemence/scout/pkg/runtime"
)

// This file implements Scout's interactive login/logout flows:
//
//	/login               -> provider selector (searchable) -> key dialog
//	/login <provider>    -> straight to that provider's key dialog
//	/logout              -> stored-credential selector
//
// The provider selector shows live configuration status, supports type-to-
// filter search, and navigation with arrows. The key dialog shows a titled box
// with a masked input. All rendering uses Scout's palette.

type loginStage int

const (
	loginStageProvider loginStage = iota
	loginStageKey
	loginStageLogout
)

// authProvider is a provider entry in the provider selector.
type authProvider struct {
	id     string
	name   string
	status string // human-readable configuration status
	ok     bool   // true when configured
}

// loginFlowUI is the whole staged login/logout experience.
type loginFlowUI struct {
	stage loginStage

	providers []authProvider
	filtered  []authProvider
	cur       int
	search    string

	// pending provider for the key dialog
	provider string
	input    textinput.Model
	errMsg   string

	// logout mode
	modeLogout bool
}

func newLoginFlow() *loginFlowUI {
	return &loginFlowUI{stage: loginStageProvider}
}

func (f *loginFlowUI) openProviderStage(core *runtime.Core, initialSearch string) {
	f.stage = loginStageProvider
	f.search = initialSearch
	f.providers = loginProviders(core)
	f.rebuild()
}

func (f *loginFlowUI) openLogoutStage(core *runtime.Core) {
	f.stage = loginStageLogout
	f.modeLogout = true
	f.search = ""
	f.providers = logoutProviders(core)
	f.rebuild()
}

func (f *loginFlowUI) openKeyStage(provider, name string) {
	if name == "" {
		name = providerDisplay(provider)
	}
	ti := textinput.New()
	ti.Prompt = ""
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '•'
	ti.CharLimit = 300
	ti.Focus()
	f.stage = loginStageKey
	f.provider = provider
	f.input = ti
	f.errMsg = ""
}

func (f *loginFlowUI) rebuild() {
	q := strings.TrimSpace(f.search)
	if q == "" {
		f.filtered = f.providers
	} else {
		f.filtered = fuzzyFilter(f.providers, q, func(p authProvider) string {
			return p.name + " " + p.id + " " + p.status
		})
	}
	if f.cur >= len(f.filtered) {
		f.cur = maxInt(0, len(f.filtered)-1)
	}
	if f.cur < 0 {
		f.cur = 0
	}
}

// keyResult reports what the flow wants the model layer to do after a keypress.
type keyResult struct {
	done     bool        // flow finished/cancelled; close it
	submit   bool        // enter on the key dialog: save the key
	provider string      // provider whose key dialog was submitted/opened
	logout   string      // provider to log out
	moveTo   *loginStage // stage to move to (nil = stay)
}

func stage(l loginStage) *loginStage { return &l }

// handleKey advances the flow from a raw terminal key message, so the key
// dialog can forward unhandled keys to its text input. Storage side effects
// are performed by the caller (which owns the Core).
func (f *loginFlowUI) handleKey(msg tea.KeyMsg) keyResult {
	key := msg.String()
	switch f.stage {
	case loginStageProvider, loginStageLogout:
		switch key {
		case "esc", "ctrl+c":
			return keyResult{done: true}
		case "up":
			if f.cur > 0 {
				f.cur--
			}
		case "down":
			if f.cur < len(f.filtered)-1 {
				f.cur++
			}
		case "enter", "tab":
			if len(f.filtered) == 0 {
				return keyResult{}
			}
			p := f.filtered[f.cur]
			if f.stage == loginStageLogout {
				return keyResult{logout: p.id}
			}
			return keyResult{provider: p.id, moveTo: stage(loginStageKey)}
		case "backspace":
			if f.search != "" {
				r := []rune(f.search)
				f.search = string(r[:len(r)-1])
				f.rebuild()
			}
		default:
			if isPrintableKey(key) {
				f.search += printableText(key)
				f.rebuild()
			}
		}
		return keyResult{}
	case loginStageKey:
		switch key {
		case "esc", "ctrl+c":
			return keyResult{done: true}
		case "enter":
			return keyResult{submit: true, provider: f.provider}
		}
		var cmd tea.Cmd
		f.input, cmd = f.input.Update(msg)
		_ = cmd
		return keyResult{}
	}
	return keyResult{}
}

// view renders the active stage using Scout's palette.
func (f *loginFlowUI) view(width int) string {
	rule := stylePromptBar.Render(strings.Repeat("─", width))
	var b strings.Builder
	b.WriteString(rule + "\n")
	switch f.stage {
	case loginStageProvider:
		b.WriteString(" " + styleModalTitle.Render("Select provider to configure:") + "\n\n")
		b.WriteString(searchField(f.search, "type to filter…", "  ") + "\n\n")
		b.WriteString(f.providerList())
		b.WriteString("\n" + styleFooterHint.Render("  ↑↓ pick · enter select · esc cancel"))
	case loginStageLogout:
		b.WriteString(" " + styleModalTitle.Render("Select provider to logout:") + "\n\n")
		b.WriteString(searchField(f.search, "type to filter…", "  ") + "\n\n")
		b.WriteString(f.providerList())
		b.WriteString("\n" + styleFooterHint.Render("  ↑↓ pick · enter remove · esc cancel"))
	case loginStageKey:
		b.WriteString(" " + styleModalTitle.Render("Login to "+providerDisplay(f.provider)) + "\n\n")
		b.WriteString(" " + styleAssistant.Render("Enter "+providerDisplay(f.provider)+" API key") + "\n")
		b.WriteString(" " + f.input.View())
		if f.errMsg != "" {
			b.WriteString("\n " + styleError.Render(f.errMsg))
		}
		b.WriteString("\n\n" + styleFooterHint.Render("  esc to cancel · enter to submit"))
	}
	b.WriteString("\n" + rule)
	return b.String()
}

func (f *loginFlowUI) providerList() string {
	if len(f.filtered) == 0 {
		return stylePaletteNoMatch.Render("  No matching providers") + "\n"
	}
	var b strings.Builder
	maxRows := 8
	off := scrollOffset(f.cur, len(f.filtered), maxRows)
	end := minInt(off+maxRows, len(f.filtered))
	for i := off; i < end; i++ {
		p := f.filtered[i]
		status := styleModelScopeInactive.Render(" • " + p.status)
		if p.ok {
			status = styleModelEnabled.Render(" ✓ " + p.status)
		}
		if i == f.cur {
			b.WriteString(stylePaletteSel.Render("→ "+p.name) + status + "\n")
		} else {
			b.WriteString("  " + styleAssistant.Render(p.name) + status + "\n")
		}
	}
	if off > 0 || end < len(f.filtered) {
		b.WriteString(stylePaletteScroll.Render(fmt.Sprintf("  (%d/%d)", f.cur+1, len(f.filtered))) + "\n")
	}
	return b.String()
}

// ---------- provider catalogs ----------

// loginProviders builds the API-key provider catalog with live status.
func loginProviders(core *runtime.Core) []authProvider {
	var out []authProvider
	for _, ps := range core.ProviderStatus(bg()) {
		if ps.Provider == "ollama" {
			continue // local, no credential
		}
		status := "unconfigured"
		ok := ps.Configured
		switch {
		case ps.Configured && strings.Contains(ps.Detail, "environment"):
			status = strings.TrimPrefix(ps.Detail, "key in ")
		case ps.Configured:
			status = "configured"
		}
		out = append(out, authProvider{
			id:     ps.Provider,
			name:   providerDisplay(ps.Provider),
			status: status,
			ok:     ok,
		})
	}
	return out
}

// logoutProviders lists providers with stored credentials only.
func logoutProviders(core *runtime.Core) []authProvider {
	var out []authProvider
	for _, p := range core.StoredProviders() {
		out = append(out, authProvider{
			id:     p,
			name:   providerDisplay(p),
			status: "stored key",
			ok:     true,
		})
	}
	return out
}
