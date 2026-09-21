package tui

import (
	"strings"
	"testing"
)

// TestSearchFieldShowsPromptAndCursor ensures a search input is never blank:
// it shows a prompt glyph and, when empty, a dim placeholder with a cursor.
func TestSearchFieldShowsPromptAndCursor(t *testing.T) {
	empty := searchField("", "search models…", "")
	if !strings.Contains(empty, searchPrompt) {
		t.Fatalf("search field must show a prompt: %q", empty)
	}
	if !strings.Contains(empty, "search models…") {
		t.Fatalf("empty search field must show a placeholder: %q", empty)
	}

	typed := searchField("deepseek", "search models…", "")
	if !strings.Contains(typed, "deepseek") {
		t.Fatalf("typed query must be shown: %q", typed)
	}
	if strings.Contains(typed, "search models…") {
		t.Fatalf("placeholder must be hidden once typing: %q", typed)
	}
}

// TestListPickerSearchHasPlaceholder ensures the shared list picker renders an
// editable search row rather than a blank line.
func TestListPickerSearchHasPlaceholder(t *testing.T) {
	u := &listPickerUI{
		title:             "Sessions",
		items:             []pickItem{{label: "one", value: "1"}},
		searchPlaceholder: "type to filter…",
	}
	u.filtered = u.items
	out := u.view(70)
	if !strings.Contains(out, "type to filter…") {
		t.Fatalf("list picker search should show a placeholder:\n%s", out)
	}
	if !strings.Contains(out, searchPrompt) {
		t.Fatalf("list picker search should show a prompt:\n%s", out)
	}
}
