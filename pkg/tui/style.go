package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Scout palette. Same semantic scheme as the reference design:
// brand accent = selection/identity, gold = approvals only,
// green = success/local, red = errors, dim = hints/meta.
var (
	cInk    = lipgloss.Color("#d8d4cc")
	cMuted  = lipgloss.Color("#8a857c")
	cFaint  = lipgloss.Color("#5c574f")
	cAccent = lipgloss.Color("#7aa2f7") // Scout blue (identity difference)
	cTool   = lipgloss.Color("#6f9c86")
	cErr    = lipgloss.Color("#c86a5c")
	cGold   = lipgloss.Color("#e8c06a")
	cGreen  = lipgloss.Color("#7fb08a")
	cBlue   = lipgloss.Color("#7fa8c9")
	cViolet = lipgloss.Color("#7aa2f7")
	cCodeBg = lipgloss.Color("#201c18")
	cSelBg  = lipgloss.Color("#2a251f")

	styleUser       = lipgloss.NewStyle().Foreground(cInk).Bold(true)
	styleAssistant  = lipgloss.NewStyle().Foreground(cInk)
	styleTool       = lipgloss.NewStyle().Foreground(cTool)
	styleToolActive = lipgloss.NewStyle().Foreground(cGreen)
	styleNotice     = lipgloss.NewStyle().Foreground(cMuted)
	styleError      = lipgloss.NewStyle().Foreground(cErr)
	styleWorking    = lipgloss.NewStyle().Foreground(cAccent)
	styleApproval   = lipgloss.NewStyle().Foreground(cGold).Bold(true)

	styleUserName      = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	styleAssistantName = lipgloss.NewStyle().Foreground(cMuted).Bold(true)
	styleAssistantMeta = lipgloss.NewStyle().Foreground(cFaint)
	styleErrorCard     = lipgloss.NewStyle().Foreground(cErr).Bold(true)
	// User bubble: one accent bar, plain text, transparent ground.
	styleUserBar   = lipgloss.NewStyle().Foreground(cAccent)
	styleUserText  = lipgloss.NewStyle().Foreground(cInk)
	styleUserPanel = lipgloss.NewStyle().PaddingLeft(1)

	// Palette: bare rows, → pointer + accent on selection, dim description.
	stylePaletteSel     = lipgloss.NewStyle().Foreground(cViolet).Bold(true)
	stylePaletteDesc    = lipgloss.NewStyle().Foreground(cMuted)
	stylePaletteNoMatch = lipgloss.NewStyle().Foreground(cMuted)
	stylePaletteScroll  = lipgloss.NewStyle().Foreground(cFaint)

	styleFooter     = lipgloss.NewStyle().Foreground(cFaint)
	styleFooterHint = lipgloss.NewStyle().Foreground(cFaint).Italic(true)

	styleModelLocal = lipgloss.NewStyle().Foreground(cGreen).Bold(true)
	styleModelCloud = lipgloss.NewStyle().Foreground(cBlue).Bold(true)
	styleModelPod   = lipgloss.NewStyle().Foreground(cMuted).Bold(true)

	// Composer rules keep the idle bar color; only status is accented.
	stylePromptBar     = lipgloss.NewStyle().Foreground(lipgloss.Color("#6e648a"))
	styleApprovalBar   = lipgloss.NewStyle().Foreground(cGold)
	styleModalTitle    = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	styleApprovalTitle = lipgloss.NewStyle().Foreground(cGold).Bold(true)
	styleApprovalKeys  = lipgloss.NewStyle().Foreground(lipgloss.Color("#efe9dc"))
	styleApprovalSel   = lipgloss.NewStyle().Foreground(lipgloss.Color("#1b1815")).Background(cGold).Bold(true)
	styleRiskHigh      = lipgloss.NewStyle().Foreground(lipgloss.Color("#1b1815")).Background(lipgloss.Color("#c86a5c")).Bold(true)
	styleRiskMid       = lipgloss.NewStyle().Foreground(lipgloss.Color("#1b1815")).Background(cGold).Bold(true)
	styleRiskLow       = lipgloss.NewStyle().Foreground(lipgloss.Color("#1b1815")).Background(cGreen).Bold(true)
	styleRiskDefault   = lipgloss.NewStyle().Foreground(cMuted).Background(cSelBg)

	styleWelcomeTitle = lipgloss.NewStyle().Foreground(lipgloss.Color("#efe9dc")).Bold(true)
	styleScoutArt     = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	styleWelcomeCmds  = lipgloss.NewStyle().Foreground(cMuted)
	styleDayDivider   = lipgloss.NewStyle().Foreground(cFaint)

	// Markdown roles.
	styleMDHead      = lipgloss.NewStyle().Foreground(lipgloss.Color("#9d7cd8")).Bold(true)
	styleMDHead1     = lipgloss.NewStyle().Foreground(lipgloss.Color("#9d7cd8")).Bold(true).Underline(true)
	styleMDStrong    = lipgloss.NewStyle().Foreground(lipgloss.Color("#f5a742")).Bold(true)
	styleMDEmph      = lipgloss.NewStyle().Foreground(lipgloss.Color("#e5c07b")).Italic(true)
	styleMDQuote     = lipgloss.NewStyle().Foreground(lipgloss.Color("#e5c07b")).Italic(true)
	styleMDQuoteMark = lipgloss.NewStyle().Foreground(cMuted)
	styleMDCode      = lipgloss.NewStyle().Foreground(lipgloss.Color("#7fd88f"))
	styleMDCodeBlock = lipgloss.NewStyle().Foreground(cInk)
	styleMDList      = lipgloss.NewStyle().Foreground(lipgloss.Color("#fab283"))
	styleMDEnum      = lipgloss.NewStyle().Foreground(lipgloss.Color("#56b6c2"))
	styleMDCheck     = lipgloss.NewStyle().Foreground(lipgloss.Color("#7fd88f"))
	styleMDUncheck   = lipgloss.NewStyle().Foreground(cMuted)
	styleMDLinkText  = lipgloss.NewStyle().Foreground(lipgloss.Color("#56b6c2")).Underline(true)

	// Legacy aliases kept for compat within package.
	styleUserLabel   = styleUserName
	styleScout       = styleAssistantName
	styleErr         = styleError
	styleDim         = styleFooter
	styleCode        = styleMDCode
	styleBold        = lipgloss.NewStyle().Bold(true)
	styleHeading     = styleMDHead
	styleFooterOld   = styleFooter
	styleBox         = lipgloss.NewStyle()
	styleBoxActive   = lipgloss.NewStyle()
	styleSelect      = stylePaletteSel
	styleRiskHighOld = styleRiskHigh
)

var (
	_ = styleUser
	_ = styleAssistant
	_ = styleToolActive
	_ = styleWorking
	_ = styleApproval
	_ = styleUserLabel
	_ = styleScout
	_ = styleErr
	_ = styleDim
	_ = styleCode
	_ = styleBold
	_ = styleHeading
	_ = styleFooterOld
	_ = styleBox
	_ = styleBoxActive
	_ = styleSelect
	_ = styleRiskHighOld
)
