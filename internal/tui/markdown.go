package tui

import (
	"strings"
)

// RenderMarkdown is a small dependency-free renderer for assistant text:
// headings, bold, inline code, fenced code blocks, lists, quotes.
// Plain paragraphs pass through untouched.
func RenderMarkdown(s string) string {
	var b strings.Builder
	inFence := false
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimRight(line, " \t")
		if strings.HasPrefix(strings.TrimSpace(t), "```") {
			inFence = !inFence
			b.WriteString(styleDim.Render("```") + "\n")
			continue
		}
		if inFence {
			b.WriteString(styleCode.Render(t) + "\n")
			continue
		}
		ts := strings.TrimSpace(t)
		switch {
		case strings.HasPrefix(ts, "### "):
			b.WriteString(styleHeading.Render(strings.TrimPrefix(ts, "### ")) + "\n")
		case strings.HasPrefix(ts, "## "):
			b.WriteString(styleHeading.Render(strings.TrimPrefix(ts, "## ")) + "\n")
		case strings.HasPrefix(ts, "# "):
			b.WriteString(styleHeading.Render(strings.TrimPrefix(ts, "# ")) + "\n")
		case strings.HasPrefix(ts, "> "):
			b.WriteString(styleDim.Render("│ "+inline(strings.TrimPrefix(ts, "> "))) + "\n")
		case strings.HasPrefix(ts, "- ") || strings.HasPrefix(ts, "* "):
			b.WriteString(styleDim.Render("• ") + inline(ts[2:]) + "\n")
		default:
			b.WriteString(inline(t) + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// inline handles **bold** and `code` within a line.
func inline(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		switch {
		case strings.HasPrefix(s[i:], "**"):
			end := strings.Index(s[i+2:], "**")
			if end < 0 {
				b.WriteString(s[i:])
				i = len(s)
				continue
			}
			b.WriteString(styleBold.Render(s[i+2 : i+2+end]))
			i += 2 + end + 2
		case s[i] == '`':
			end := strings.IndexByte(s[i+1:], '`')
			if end < 0 {
				b.WriteString(s[i:])
				i = len(s)
				continue
			}
			b.WriteString(styleCode.Render(s[i+1 : i+1+end]))
			i += 1 + end + 1
		default:
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String()
}
