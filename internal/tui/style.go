package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Scout theme: calm professional console. Dim chrome, one accent for the
// assistant, distinct user color, red reserved for errors/risk.
var (
	accent    = lipgloss.Color("#7aa2f7")
	userColor = lipgloss.Color("#9ece6a")
	dimColor  = lipgloss.Color("#565f89")
	redColor  = lipgloss.Color("#f7768e")
	yellow    = lipgloss.Color("#e0af68")

	styleUserLabel = lipgloss.NewStyle().Bold(true).Foreground(userColor)
	styleScout     = lipgloss.NewStyle().Bold(true).Foreground(accent)
	styleTool      = lipgloss.NewStyle().Foreground(dimColor)
	styleToolOK    = lipgloss.NewStyle().Foreground(userColor)
	styleErr       = lipgloss.NewStyle().Foreground(redColor)
	styleNotice    = lipgloss.NewStyle().Foreground(yellow)
	styleDim       = lipgloss.NewStyle().Foreground(dimColor)
	styleCode      = lipgloss.NewStyle().Foreground(accent)
	styleBold      = lipgloss.NewStyle().Bold(true)
	styleHeading   = lipgloss.NewStyle().Bold(true).Foreground(accent)
	styleFooter    = lipgloss.NewStyle().Foreground(dimColor)
	styleBox       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(dimColor)
	styleBoxActive = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent)
	styleSelect    = lipgloss.NewStyle().Background(lipgloss.Color("#28344d"))
	styleRiskHigh  = lipgloss.NewStyle().Bold(true).Foreground(redColor)
)
