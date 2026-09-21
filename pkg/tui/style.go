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
	// A restored-conversation divider: quiet, so it reads as a boundary, not
	// as content.
	styleDayDivider = lipgloss.NewStyle().Foreground(cFaint)

	styleModelLocal = lipgloss.NewStyle().Foreground(cGreen).Bold(true)
	styleModelCloud = lipgloss.NewStyle().Foreground(cBlue).Bold(true)
	styleModelPod   = lipgloss.NewStyle().Foreground(cMuted).Bold(true)

	// Model selector (/model).
	styleModelScopeTitle    = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	styleModelScopeHint     = lipgloss.NewStyle().Foreground(cMuted)
	styleModelScopeFooter   = lipgloss.NewStyle().Foreground(cFaint)
	styleModelScopeActive   = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	styleModelScopeInactive = lipgloss.NewStyle().Foreground(cMuted)
	styleModelScopeWarn     = lipgloss.NewStyle().Foreground(cGold)
	styleModelSearch        = lipgloss.NewStyle().Foreground(cInk)
	styleModelEnabled       = lipgloss.NewStyle().Foreground(cAccent)
	styleModelCurrent       = lipgloss.NewStyle().Foreground(cAccent)
	styleModelUnavail       = lipgloss.NewStyle().Foreground(cMuted).Strikethrough(true)

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

	styleWelcomeCmd  = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	styleWelcomeDesc = lipgloss.NewStyle().Foreground(cMuted)

	// Markdown roles, mapped to Scout brand tokens (accent = identity,
	// gold = headings/emphasis, green = code, muted = quotes/meta). This
	// keeps model responses on-palette rather than importing foreign hexes.
	styleMDHead      = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	styleMDHead1     = lipgloss.NewStyle().Foreground(cAccent).Bold(true).Underline(true)
	styleMDStrong    = lipgloss.NewStyle().Foreground(cGold).Bold(true)
	styleMDEmph      = lipgloss.NewStyle().Foreground(cGold).Italic(true)
	styleMDQuote     = lipgloss.NewStyle().Foreground(cMuted).Italic(true)
	styleMDQuoteMark = lipgloss.NewStyle().Foreground(cAccent)
	styleMDCode      = lipgloss.NewStyle().Foreground(cGreen)
	styleMDCodeBlock = lipgloss.NewStyle().Foreground(cGreen)
	styleMDList      = lipgloss.NewStyle().Foreground(cAccent)
	styleMDEnum      = lipgloss.NewStyle().Foreground(cAccent)
	styleMDCheck     = lipgloss.NewStyle().Foreground(cGreen)
	styleMDUncheck   = lipgloss.NewStyle().Foreground(cMuted)
	styleMDLinkText  = lipgloss.NewStyle().Foreground(cBlue).Underline(true)
	styleMDTableHead = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	// Table grid: dim box borders, bold header, plain body cells — the
	// opencode/Ghost box-drawn table language.
	styleMDTableBorder = lipgloss.NewStyle().Foreground(cFaint)
	styleMDTableRow    = lipgloss.NewStyle().Foreground(cInk)

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
