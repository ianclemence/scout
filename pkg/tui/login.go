package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ianclemence/scout/pkg/runtime"
)

// This file implements Scout's interactive login/logout flows, modeled on the
// same staged structure used by mature terminal agents:
//
//	/login               -> authentication-method selector
//	/login <provider>    -> straight to the provider's key dialog
//	  method "api key"   -> provider selector (searchable) -> key dialog
//	  method "account"   -> provider selector (OAuth-style; none today)
//	/logout              -> stored-credential selector
//
// The provider selector shows live configuration status, supports type-to-
// filter search, and navigation with arrows. The key dialog shows a titled
// box with a masked input. All rendering uses Scout's palette.

type loginStage int

const (
	loginStageMethod loginStage = iota
	loginStageProvider
	loginStageKey
	loginStageLogout
)

// loginMethod is one authentication method in the first-stage selector.
type loginMethod struct {
	label    string
	authType string // "api_key" or "account"
}

// authProvider is a provider entry in the provider selector.
type authProvider struct {
	id       string
	name     string
	authType string
	status   string // human-readable configuration status
	ok       bool   // true when configured
}

// loginFlowUI is the whole staged login/logout experience.
type loginFlowUI struct {
	stage   loginStage
	methods []loginMethod
	mCur    int

	providers []authProvider
	filtered  []authProvider
	cur       int
	search    string

	// pending provider for the key dialog
	provider string
	input    textinput.Model
	errMsg   string

	// which auth type the provider list is filtered to ("" = all)
	authFilter string
	// logout mode: whether the method stage was shown (for cancel back-stack)
	modeLogout bool
}

func newLoginFlow() *loginFlowUI {
	return &loginFlowUI{
		stage: loginStageMethod,
		methods: []loginMethod{
			{label: "Sign in with an API key", authType: "api_key"},
		},
	}
}

func (f *loginFlowUI) openProviderStage(core *runtime.Core, authType string, initialSearch string) {
	f.stage = loginStageProvider
	f.authFilter = authType
	f.search = initialSearch
	f.providers = loginProviders(core, authType)
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
	q := strings.ToLower(strings.TrimSpace(f.search))
	if q == "" {
		f.filtered = f.providers
	} else {
		var out []authProvider
		for _, p := range f.providers {
			hay := strings.ToLower(p.name + " " + p.id + " " + p.status)
			if strings.Contains(hay, q) {
				out = append(out, p)
			}
		}
		f.filtered = out
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
	method   string      // selected method authType (from the method stage)
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
	case loginStageMethod:
		switch key {
		case "esc", "ctrl+c":
			return keyResult{done: true}
		case "up":
			if f.mCur > 0 {
				f.mCur--
			}
		case "down":
			if f.mCur < len(f.methods)-1 {
				f.mCur++
			}
		case "enter", "tab":
			if len(f.methods) == 0 {
				return keyResult{done: true}
			}
			return keyResult{method: f.methods[f.mCur].authType, moveTo: stage(loginStageProvider)}
		}
		return keyResult{}
	case loginStageProvider, loginStageLogout:
		switch key {
		case "esc", "ctrl+c":
			if f.stage == loginStageProvider {
				return keyResult{moveTo: stage(loginStageMethod)}
			}
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
	case loginStageMethod:
		b.WriteString(" " + styleModalTitle.Render("Select authentication method") + "\n\n")
		for i, mth := range f.methods {
			line := "  " + mth.label
			if i == f.mCur {
				line = stylePaletteSel.Render("→ " + mth.label)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n" + styleFooterHint.Render("  ↑↓ pick · enter select · esc cancel"))
	case loginStageProvider:
		b.WriteString(" " + styleModalTitle.Render("Select provider to configure:") + "\n")
		b.WriteString(styleModelSearch.Render("  /"+f.search+"_") + "\n")
		b.WriteString(f.providerList())
		b.WriteString(styleFooterHint.Render("\n  ↑↓ pick · type to filter · enter select · esc back"))
	case loginStageLogout:
		b.WriteString(" " + styleModalTitle.Render("Select provider to logout:") + "\n")
		b.WriteString(styleModelSearch.Render("  /"+f.search+"_") + "\n")
		b.WriteString(f.providerList())
		b.WriteString(styleFooterHint.Render("\n  ↑↓ pick · type to filter · enter remove · esc cancel"))
	case loginStageKey:
		b.WriteString(" " + styleModalTitle.Render("Login to "+providerDisplay(f.provider)) + "\n\n")
		b.WriteString(" " + styleAssistant.Render("Enter "+providerDisplay(f.provider)+" API key") + "\n")
		b.WriteString(" " + f.input.View())
		if f.errMsg != "" {
			b.WriteString("\n " + styleError.Render(f.errMsg))
		}
		b.WriteString("\n\n" + styleFooterHint.Render(" (esc to cancel, enter to submit)"))
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

// loginProviders builds the provider catalog with live status, optionally
// filtered to one auth type.
func loginProviders(core *runtime.Core, authType string) []authProvider {
	var out []authProvider
	ctx := bg()
	for _, ps := range core.ProviderStatus(ctx) {
		if ps.Provider == "ollama" {
			continue // local, no credential
		}
		status := "unconfigured"
		ok := ps.Configured
		switch {
		case ps.Configured && strings.Contains(ps.Detail, "environment"):
			status = strings.TrimPrefix(strings.TrimPrefix(ps.Detail, "key in "), "")
		case ps.Configured:
			status = "configured"
		}
		if authType != "" && authType != "api_key" {
			continue
		}
		out = append(out, authProvider{
			id:       ps.Provider,
			name:     providerDisplay(ps.Provider),
			authType: "api_key",
			status:   status,
			ok:       ok,
		})
	}
	return out
}

// logoutProviders lists providers with stored credentials only.
func logoutProviders(core *runtime.Core) []authProvider {
	var out []authProvider
	for _, p := range core.StoredProviders() {
		out = append(out, authProvider{
			id:       p,
			name:     providerDisplay(p),
			authType: "api_key",
			status:   "stored key",
			ok:       true,
		})
	}
	return out
}
