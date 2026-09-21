package tui

import (
	"strings"
	"testing"

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
	if !strings.Contains(out, "|") {
		t.Fatalf("too-narrow table should fall back to raw markdown:\n%s", out)
	}
}
