package tui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RenderMarkdown renders assistant text at the given content width:
// headings, bold, italic, inline code, fenced code, lists, ordered lists,
// task items, quotes, links, tables. Prose is word-wrapped to width (never
// truncated); fenced code and tables keep their own layout.
func RenderMarkdownWidth(s string, width int) string {
	if width < 20 {
		width = 80
	}
	lines := strings.Split(s, "\n")
	var b strings.Builder
	inFence := false
	i := 0
	for i < len(lines) {
		line := strings.TrimRight(lines[i], " \t")
		ts := strings.TrimSpace(line)
		if strings.HasPrefix(ts, "```") {
			inFence = !inFence
			// The fence markers are syntax, not content: conceal them, keeping a
			// single blank line so the code block still reads as distinct from
			// surrounding prose. Do not stack blanks if one is already present.
			out := b.String()
			if !strings.HasSuffix(out, "\n\n") && out != "" {
				b.WriteString("\n")
			}
			i++
			continue
		}
		if inFence {
			for _, wl := range wrap(line, width) {
				b.WriteString(styleMDCodeBlock.Render(wl) + "\n")
			}
			i++
			continue
		}
		if isTableRow(ts) && i+1 < len(lines) && isTableDelimiter(strings.TrimSpace(lines[i+1])) {
			j := i
			var block []string
			for j < len(lines) && isTableRow(strings.TrimSpace(lines[j])) {
				block = append(block, lines[j])
				j++
			}
			for _, rl := range renderTable(block, width) {
				b.WriteString(rl + "\n")
			}
			i = j
			continue
		}
		switch {
		case strings.HasPrefix(ts, "### "):
			b.WriteString(inlineStyledBlock(strings.TrimPrefix(ts, "### "), styleMDHead, width) + "\n")
		case strings.HasPrefix(ts, "## "):
			b.WriteString(inlineStyledBlock(strings.TrimPrefix(ts, "## "), styleMDHead, width) + "\n")
		case strings.HasPrefix(ts, "# "):
			b.WriteString(inlineStyledBlock(strings.TrimPrefix(ts, "# "), styleMDHead1, width) + "\n")
		case strings.HasPrefix(ts, "> "):
			b.WriteString(wrapPrefixed(inline(strings.TrimPrefix(ts, "> ")), styleMDQuoteMark.Render("│ "), width, styleMDQuote) + "\n")
		case strings.HasPrefix(ts, "- [x] ") || strings.HasPrefix(ts, "- [X] "):
			b.WriteString(wrapPrefixed(inline(ts[6:]), styleMDCheck.Render("✓ "), width, lipgloss.NewStyle()) + "\n")
		case strings.HasPrefix(ts, "- [ ] "):
			b.WriteString(wrapPrefixed(inline(ts[6:]), styleMDUncheck.Render("○ "), width, lipgloss.NewStyle()) + "\n")
		case strings.HasPrefix(ts, "- ") || strings.HasPrefix(ts, "* "):
			b.WriteString(wrapPrefixed(inline(ts[2:]), styleMDList.Render("• "), width, lipgloss.NewStyle()) + "\n")
		case isOrderedList(ts):
			dot := strings.Index(ts, ".")
			b.WriteString(wrapPrefixed(inline(strings.TrimSpace(ts[dot+1:])), styleMDEnum.Render(ts[:dot+1])+" ", width, lipgloss.NewStyle()) + "\n")
		default:
			// Plain prose: wrap the raw line, then apply inline styling per
			// wrapped line so bold/code survive the wrap.
			b.WriteString(inlineStyledBlock(line, lipgloss.NewStyle(), width) + "\n")
		}
		i++
	}
	// Collapse any stacked blank lines the block elements introduced, so
	// concealing fences never leaves a double gap.
	return collapseBlankRuns(strings.TrimRight(b.String(), "\n"))
}

// collapseBlankRuns reduces any run of two or more consecutive blank lines to a
// single blank line, preserving paragraph separation without double gaps.
func collapseBlankRuns(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, ln)
	}
	return strings.Join(out, "\n")
}

// RenderMarkdown renders assistant text wrapping at a sane default width.
// Prefer RenderMarkdownWidth from the renderer, which knows the terminal.
func RenderMarkdown(s string) string { return RenderMarkdownWidth(s, 80) }

// inlineStyledBlock wraps raw markdown source to width, applies inline
// formatting per wrapped line, and styles each line with blockStyle. Wrapping
// the source (not the styled output) keeps bold/code intact and avoids
// splitting ANSI escapes.
func inlineStyledBlock(src string, blockStyle lipgloss.Style, width int) string {
	lines := wrap(src, width)
	var out []string
	for _, ln := range lines {
		out = append(out, blockStyle.Render(inline(ln)))
	}
	return strings.Join(out, "\n")
}

// wrapPrefixed wraps already-inline-rendered text to width, prefixing the
// first line with prefix and indenting continuation lines to match. The
// lineStyle wraps the wrapped segments of the running text (commentary), not
// the prefix.
func wrapPrefixed(text, prefix string, width int, lineStyle lipgloss.Style) string {
	indent := strings.Repeat(" ", lipgloss.Width(prefix))
	avail := width - lipgloss.Width(prefix)
	if avail < 10 {
		avail = 10
	}
	lines := wrapANSI(text, avail)
	if len(lines) == 0 {
		return prefix
	}
	var b strings.Builder
	b.WriteString(prefix + lineStyle.Render(lines[0]))
	for _, ln := range lines[1:] {
		b.WriteString("\n" + indent + lineStyle.Render(ln))
	}
	return b.String()
}

func isOrderedList(s string) bool {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return i > 0 && i < len(s) && s[i] == '.' && i+1 < len(s) && s[i+1] == ' '
}

// inline handles **bold**, *italic*, `code`, and [text](url).
func inline(s string) string {
	s = renderLinks(s)
	s = renderSpan(s, "**", func(t ...string) string { return styleMDStrong.Render(t[0]) })
	s = renderEmphasis(s)
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '`' {
			end := strings.IndexByte(s[i+1:], '`')
			if end < 0 {
				b.WriteString(s[i:])
				break
			}
			b.WriteString(styleMDCode.Render(s[i+1 : i+1+end]))
			i += 1 + end + 1
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func renderSpan(s, delim string, fn func(...string) string) string {
	var b strings.Builder
	for {
		a := strings.Index(s, delim)
		if a < 0 {
			b.WriteString(s)
			break
		}
		c := strings.Index(s[a+len(delim):], delim)
		if c < 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:a])
		b.WriteString(fn(s[a+len(delim) : a+len(delim)+c]))
		s = s[a+len(delim)+c+len(delim):]
	}
	return b.String()
}

func renderEmphasis(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '*' && !(i+1 < len(s) && s[i+1] == '*') {
			end := -1
			for j := i + 1; j < len(s); j++ {
				if s[j] == '*' && !(j+1 < len(s) && s[j+1] == '*') {
					end = j
					break
				}
			}
			if end < 0 {
				b.WriteString(s[i:])
				break
			}
			b.WriteString(styleMDEmph.Render(s[i+1 : end]))
			i = end + 1
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func renderLinks(s string) string {
	var b strings.Builder
	for {
		a := strings.IndexByte(s, '[')
		if a < 0 {
			b.WriteString(s)
			break
		}
		m := strings.Index(s[a:], "](")
		if m < 0 {
			b.WriteString(s)
			break
		}
		e := strings.IndexByte(s[a+m+2:], ')')
		if e < 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:a])
		b.WriteString(styleMDLinkText.Render(s[a+1 : a+m]))
		s = s[a+m+2+e+1:]
	}
	return b.String()
}

// ---- tables ----
// A markdown table renders as a box-drawn grid, matching the opencode CLI and
// Ghost: `┌─┬─┐` / `├─┼─┤` / `└─┴─┘`, a bold header row, a separator between
// every row, dim borders, and inline styling inside cells. Columns size to
// their natural width and shrink to fit the available space, wrapping long
// cells; if even a single unbroken word cannot fit, the raw markdown is
// returned rather than a broken grid.

// tableMaxUnbrokenWord caps how wide a single unbroken word may force a
// column, so one long URL or identifier cannot starve the rest of the grid.
const tableMaxUnbrokenWord = 30

// mdLinkRe matches an inline link, capturing its label, so table cells can be
// measured and painted on their visible text rather than their markdown.
var mdLinkRe = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)

// isTableRow reports whether a line is a pipe-delimited table row. A single
// pipe is enough (a two-column table without outer pipes).
func isTableRow(s string) bool {
	return strings.Contains(s, "|")
}

// isTableDelimiter reports whether a line is a markdown table delimiter
// (e.g. `| --- | :--: |`), allowing alignment colons.
func isTableDelimiter(s string) bool {
	if !isTableRow(s) {
		return false
	}
	cells := splitTableRow(s)
	if len(cells) < 2 {
		return false
	}
	for _, c := range cells {
		c = strings.TrimSpace(c)
		if c == "" {
			return false
		}
		for _, r := range c {
			if r != '-' && r != ':' {
				return false
			}
		}
		if !strings.Contains(c, "-") {
			return false
		}
	}
	return true
}

// splitTableRow splits a pipe row into trimmed cells. The leading and trailing
// pipes are optional and dropped.
func splitTableRow(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "|")
	s = strings.TrimSuffix(s, "|")
	parts := strings.Split(s, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// renderTable renders a parsed markdown table into box-drawn lines at the
// given content width.
func renderTable(block []string, width int) []string {
	if len(block) < 2 {
		return wrap(strings.Join(block, "\n"), width)
	}
	header := splitTableRow(block[0])
	numCols := len(header)
	if numCols == 0 {
		return wrap(strings.Join(block, "\n"), width)
	}
	var rows [][]string
	for _, ln := range block[2:] {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		row := splitTableRow(ln)
		// Normalize to the header's column count.
		for len(row) < numCols {
			row = append(row, "")
		}
		if len(row) > numCols {
			row = row[:numCols]
		}
		rows = append(rows, row)
	}

	// Border overhead: "│ " + (n-1)*" │ " + " │" = 3n + 1.
	availableForCells := width - (3*numCols + 1)
	if availableForCells < numCols {
		return wrap(strings.Join(block, "\n"), width)
	}

	// Natural (unwrapped) and minimum (longest word) column widths.
	natural := make([]int, numCols)
	minWord := make([]int, numCols)
	measure := func(cells []string) {
		for i, c := range cells {
			plain := stripInline(c)
			if w := lipgloss.Width(plain); w > natural[i] {
				natural[i] = w
			}
			if w := longestWordWidth(plain, tableMaxUnbrokenWord); w > minWord[i] {
				minWord[i] = w
			}
		}
	}
	measure(header)
	for _, r := range rows {
		measure(r)
	}
	for i := range minWord {
		if minWord[i] < 1 {
			minWord[i] = 1
		}
	}

	widths := fitColumns(natural, minWord, availableForCells)
	// A word wider than its column would burst the grid: show raw markdown.
	for i, w := range widths {
		if minWord[i] > w {
			return wrap(strings.Join(block, "\n"), width)
		}
	}

	var out []string
	borderLine := func(left, mid, right string) string {
		var sb strings.Builder
		sb.WriteString(left)
		for i, w := range widths {
			if i > 0 {
				sb.WriteString(mid)
			}
			sb.WriteString(strings.Repeat("─", w))
		}
		sb.WriteString(right)
		return styleMDTableBorder.Render(sb.String())
	}

	out = append(out, borderLine("┌─", "─┬─", "─┐"))
	out = append(out, renderTableRowPadded(header, widths, styleMDTableHead)...)
	out = append(out, borderLine("├─", "─┼─", "─┤"))
	for ri, row := range rows {
		out = append(out, renderTableRowPadded(row, widths, styleMDTableRow)...)
		if ri < len(rows)-1 {
			out = append(out, borderLine("├─", "─┼─", "─┤"))
		}
	}
	out = append(out, borderLine("└─", "─┴─", "─┘"))
	return out
}

// fitColumns sizes columns to fit availableForCells: natural widths when they
// fit, otherwise each column keeps at least its longest word and the remaining
// space is distributed proportionally (the opencode/Ghost algorithm).
func fitColumns(natural, minWord []int, availableForCells int) []int {
	n := len(natural)
	totalNatural, minCells := 0, 0
	for i := range natural {
		totalNatural += natural[i]
		minCells += minWord[i]
	}
	if totalNatural <= availableForCells {
		out := make([]int, n)
		copy(out, natural)
		return out
	}
	base := make([]int, n)
	if minCells <= availableForCells {
		copy(base, minWord)
	} else {
		for i := range base {
			base[i] = 1
		}
		extra := availableForCells - n
		if extra > 0 {
			totalWeight := 0
			for _, w := range minWord {
				if w-1 > 0 {
					totalWeight += w - 1
				}
			}
			for i, w := range minWord {
				if totalWeight > 0 && w-1 > 0 {
					base[i] += (w - 1) * extra / totalWeight
				}
			}
		}
	}
	// Grow toward natural widths with the leftover space.
	allocated := 0
	for _, w := range base {
		allocated += w
	}
	remaining := availableForCells - allocated
	for remaining > 0 {
		grew := false
		for i := 0; i < n && remaining > 0; i++ {
			if base[i] < natural[i] {
				base[i]++
				remaining--
				grew = true
			}
		}
		if !grew {
			break
		}
	}
	return base
}

// renderTableRowPadded wraps each cell to its column width, pads it, and joins
// the cells with the box's vertical separators. Header cells take the header
// style; body cells take the row style.
func renderTableRowPadded(cells []string, widths []int, style lipgloss.Style) []string {
	wrapped := make([][]string, len(cells))
	height := 1
	for i := range cells {
		wrapped[i] = wrap(stripInline(cells[i]), maxInt(1, widths[i]))
		if len(wrapped[i]) > height {
			height = len(wrapped[i])
		}
	}
	var out []string
	for line := 0; line < height; line++ {
		var sb strings.Builder
		sb.WriteString(styleMDTableBorder.Render("│"))
		for i := range cells {
			piece := ""
			if line < len(wrapped[i]) {
				piece = wrapped[i][line]
			}
			pad := widths[i] - lipgloss.Width(piece)
			if pad < 0 {
				pad = 0
			}
			sb.WriteString(" ")
			sb.WriteString(style.Render(piece + strings.Repeat(" ", pad)))
			sb.WriteString(" ")
			sb.WriteString(styleMDTableBorder.Render("│"))
		}
		out = append(out, sb.String())
	}
	return out
}

// stripInline removes markdown inline markers so widths are measured on the
// visible text. Cells render plainly; the grid supplies the structure.
func stripInline(s string) string {
	s = mdLinkRe.ReplaceAllString(s, "$1")
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "`", "")
	s = strings.ReplaceAll(s, "~~", "")
	s = strings.Trim(s, "*_")
	return s
}

// longestWordWidth is the widest single word in s, capped at max.
func longestWordWidth(s string, max int) int {
	best := 0
	for _, word := range strings.Fields(s) {
		w := lipgloss.Width(word)
		if w > best {
			best = w
		}
	}
	if best > max {
		best = max
	}
	return best
}
