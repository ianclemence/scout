package tui

import (
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
			b.WriteString(styleFooter.Render("```") + "\n")
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
			for _, rl := range renderTable(block) {
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
	return strings.TrimRight(b.String(), "\n")
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

func isTableRow(s string) bool {
	return strings.HasPrefix(s, "|") && strings.HasSuffix(s, "|") && strings.Count(s, "|") >= 3
}

func isTableDelimiter(s string) bool {
	if !isTableRow(s) {
		return false
	}
	for _, c := range strings.Trim(s, "|") {
		if c != '-' && c != ':' && c != ' ' && c != '|' {
			return false
		}
	}
	return strings.Contains(s, "-")
}

func splitTableRow(s string) []string {
	parts := strings.Split(strings.Trim(s, "|"), "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func renderTable(block []string) []string {
	if len(block) < 2 {
		return block
	}
	header := splitTableRow(block[0])
	var rows [][]string
	for _, ln := range block[2:] {
		rows = append(rows, splitTableRow(ln))
	}
	widths := make([]int, len(header))
	for i, h := range header {
		if len(h) > widths[i] {
			widths[i] = len(h)
		}
	}
	for _, r := range rows {
		for i := range header {
			if i < len(r) && len(r[i]) > widths[i] {
				widths[i] = len(r[i])
			}
		}
	}
	pad := func(s string, w int) string { return s + strings.Repeat(" ", w-len(s)) }
	var out []string
	hc := make([]string, len(header))
	for i, h := range header {
		hc[i] = styleMDHead.Render(pad(h, widths[i]))
	}
	out = append(out, "│ "+strings.Join(hc, " │ ")+" │")
	sep := make([]string, len(header))
	for i := range header {
		sep[i] = strings.Repeat("─", widths[i])
	}
	out = append(out, styleFooter.Render("├─"+strings.Join(sep, "─┼─")+"─┤"))
	for _, r := range rows {
		cc := make([]string, len(header))
		for i := range header {
			v := ""
			if i < len(r) {
				v = r[i]
			}
			cc[i] = pad(v, widths[i])
		}
		out = append(out, "│ "+strings.Join(cc, " │ ")+" │")
	}
	return out
}
