package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Search fields across the TUI (model picker, list pickers, login/logout
// provider lists, and the command palette) share one rendering so the user can
// always tell where typing goes. The pattern matches the pi coding agent's
// input: a visible prompt, a placeholder shown dim when empty, and a cursor
// block marking the insertion point — so the field is never a blank mystery.

// searchPrompt is the leading glyph that marks an editable search field.
const searchPrompt = "› "

var styleSearchPrompt = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
var styleSearchCursor = lipgloss.NewStyle().Reverse(true)

// searchField renders a one-line search input. When the query is empty a dim
// placeholder is shown with the cursor over its first character; when the user
// has typed, the value is shown with a cursor block at the end. indent is
// prepended for dialogs that align their content.
func searchField(query, placeholder, indent string) string {
	var b strings.Builder
	b.WriteString(indent)
	b.WriteString(styleSearchPrompt.Render(searchPrompt))
	if query == "" {
		if placeholder == "" {
			b.WriteString(styleSearchCursor.Render(" "))
			return b.String()
		}
		r := []rune(placeholder)
		// Cursor sits on the first placeholder rune (reverse video); the rest
		// is dim, so the field reads as focused and empty.
		b.WriteString(styleSearchCursor.Render(string(r[0])))
		if len(r) > 1 {
			b.WriteString(styleSearchPlaceholder.Render(string(r[1:])))
		}
		return b.String()
	}
	b.WriteString(styleModelSearch.Render(query))
	b.WriteString(styleSearchCursor.Render(" "))
	return b.String()
}
