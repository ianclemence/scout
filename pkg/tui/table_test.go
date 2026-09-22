package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

const sampleTable = `| # | Title | Rec | Notes |
|---|-------|-----|-------|
| 1 | Full Stack Engineer — Music-Release Ingestion Pipeline + Curation Dashboard | apply | Strong fit; scope is large and open-ended |
| 2 | Senior Full-Stack Developer for SaaS | apply | $8-20/hr is low for senior scope |`

// TestTableRendersBoxGrid ensures a markdown table becomes a box-drawn grid
// with a top border, a separator between every row, and a bottom border — the
// opencode/Ghost table language.
func TestTableRendersBoxGrid(t *testing.T) {
	out := RenderMarkdownWidth(sampleTable, 80)
	if !strings.Contains(out, "┌") || !strings.Contains(out, "┐") {
		t.Fatalf("missing top border:\n%s", out)
	}
	if !strings.Contains(out, "└") || !strings.Contains(out, "┘") {
		t.Fatalf("missing bottom border:\n%s", out)
	}
	if strings.Count(out, "├") < 2 {
		t.Fatalf("expected a separator after the header and between rows:\n%s", out)
	}
	if strings.Contains(out, "|---|") {
		t.Fatalf("delimiter row should be concealed:\n%s", out)
	}
}

// TestTableFitsTerminalWidth ensures the grid never exceeds the requested
// content width, at several terminal sizes.
func TestTableFitsTerminalWidth(t *testing.T) {
	for _, w := range []int{80, 70, 60, 50} {
		out := RenderMarkdownWidth(sampleTable, w)
		for _, ln := range strings.Split(out, "\n") {
			if lipgloss.Width(ln) > w {
				t.Fatalf("width %d: line exceeds content width (%d):\n%s", w, lipgloss.Width(ln), out)
			}
		}
	}
}

// TestTableColumnsAlign ensures every grid line shares the same width, so the
// right border is flush. Byte-length padding would break on wide characters
// (e.g. the em-dash in the sample).
func TestTableColumnsAlign(t *testing.T) {
	out := RenderMarkdownWidth(sampleTable, 70)
	widths := map[int]bool{}
	for _, ln := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(ln), "│") || strings.HasPrefix(strings.TrimSpace(ln), "┌") ||
			strings.HasPrefix(strings.TrimSpace(ln), "├") || strings.HasPrefix(strings.TrimSpace(ln), "└") {
			widths[lipgloss.Width(ln)] = true
		}
	}
	if len(widths) != 1 {
		t.Fatalf("grid lines are not flush; widths seen: %v\n%s", widths, out)
	}
}

// TestTableFallsBackWhenTooNarrow ensures a table that cannot form a stable
// grid is shown as raw markdown rather than a broken one.
func TestTableFallsBackWhenTooNarrow(t *testing.T) {
	out := RenderMarkdownWidth(sampleTable, 20)
	if strings.Contains(out, "┌") {
		t.Fatalf("too-narrow table should not attempt a grid:\n%s", out)
	}
	lines := strings.Split(out, "\n")
	for _, ln := range lines {
		if strings.Contains(ln, "|") {
			t.Fatalf("too-narrow table must never show raw pipes:\n%s", out)
		}
		if lipgloss.Width(ln) > 20 {
			t.Fatalf("stacked line exceeds width:\n%s", out)
		}
	}
	for _, want := range []string{"Title:", "Rec:", "Full Stack", "Music-Release"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stacked fallback must keep labels and content, want %q:\n%s", want, out)
		}
	}
}

// breakWord prefers URL/slug boundaries so long links and hyphenated terms
// stay readable instead of chopping mid-word ("digital-s"/"ignage").
func TestBreakWordPrefersBoundaries(t *testing.T) {
	got := breakWord("Queue/appointment/kiosk/SMS/digital-signage", 20)
	want := []string{"Queue/appointment/", "kiosk/SMS/digital-", "signage"}
	if len(got) != len(want) {
		t.Fatalf("breakWord = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("breakWord = %q, want %q", got, want)
		}
	}
}

// breakWord never splits a multibyte rune: byte slicing corrupts UTF-8 and
// breaks grid alignment.
func TestBreakWordRuneSafe(t *testing.T) {
	for _, p := range breakWord("日本語テスト日本語テスト日本語テスト", 10) {
		if !utf8.ValidString(p) {
			t.Fatalf("piece is not valid UTF-8: %q", p)
		}
		if lipgloss.Width(p) > 10 {
			t.Fatalf("piece exceeds width: %q", p)
		}
	}
}

// A table cell with a long hyphenated value must stay grid-aligned: every
// rendered line the same visible width, no mid-word chop.
func TestTableLongHyphenatedCellAligned(t *testing.T) {
	out := RenderMarkdownWidth("| | Music pipeline | Queue SaaS |\n|---|---|---|\n| Core work | Ingestion, normalization | Queue/appointment/kiosk/SMS/digital-signage product |", 78)
	lines := strings.Split(out, "\n")
	w := lipgloss.Width(lines[0])
	for _, ln := range lines {
		if lipgloss.Width(ln) != w {
			t.Fatalf("misaligned row (want width %d): %q\nfull:\n%s", w, ln, out)
		}
	}
	if strings.Contains(out, "digital-s\n") || strings.Contains(out, "digital-s ") {
		t.Fatalf("hyphenated word chopped mid-word:\n%s", out)
	}
}
