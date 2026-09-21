// Package termui renders command output for the terminal with a consistent,
// readable style: labeled sections, dim secondary text, emphasized values, and
// status glyphs. It is used by the scriptable CLI so its output matches the
// interactive TUI's visual language (the TUI renders markdown through pkg/tui,
// this renders plain ANSI for one-shot commands).
package termui

import (
	"fmt"
	"strings"
)

// ANSI styles. Kept minimal: dim meta, bold values, a few semantic colors that
// match the TUI palette so CLI and TUI read the same.
const (
	reset = "\x1b[0m"
	dim   = "\x1b[2m"
	bold  = "\x1b[1m"
	green = "\x1b[32m"
	red   = "\x1b[31m"
	cyan  = "\x1b[36m"
)

// StyleEnabled is false when output is not a terminal (piped/redirected), so
// machine consumers get clean text without escape codes.
var StyleEnabled = true

func wrap(code, s string) string {
	if !StyleEnabled || s == "" {
		return s
	}
	return code + s + reset
}

// Dim renders secondary/meta text.
func Dim(s string) string { return wrap(dim, s) }

// Bold renders an emphasized value.
func Bold(s string) string { return wrap(bold, s) }

// Good renders a success value or glyph.
func Good(s string) string { return wrap(green, s) }

// Bad renders a failure value or glyph.
func Bad(s string) string { return wrap(red, s) }

// Accent renders an identity/highlight value.
func Accent(s string) string { return wrap(cyan, s) }

// Bool renders a yes/no fact with a semantic glyph.
func Bool(v bool) string {
	if v {
		return Good("✓ yes")
	}
	return Dim("✗ no")
}

// Section prints a titled block header, e.g. "Status" under a rule.
func Section(title string) string {
	return "\n" + Accent(title) + "\n"
}

// Field renders a labeled fact: a dim label, an emphasized value.
func Field(label, value string) string {
	return "  " + Dim(label+":") + " " + value
}

// Table renders aligned columns with a styled header, a dim rule, and
// per-row cells. Cols are the header names; rows must match their count. The
// last column is not padded.
func Table(cols []string, rows [][]string) string {
	if len(cols) == 0 {
		return ""
	}
	n := len(cols)
	w := make([]int, n)
	for i, c := range cols {
		w[i] = len(c)
	}
	for _, r := range rows {
		for i := 0; i < n && i < len(r); i++ {
			if l := len(r[i]); l > w[i] {
				w[i] = l
			}
		}
	}
	var b strings.Builder
	for i, c := range cols {
		if i == n-1 {
			b.WriteString(Bold(c))
		} else {
			b.WriteString(Bold(pad(c, w[i])))
			b.WriteString("  ")
		}
	}
	b.WriteString("\n")
	total := 0
	for i, x := range w {
		total += x
		if i < n-1 {
			total += 2
		}
	}
	b.WriteString(Dim(strings.Repeat("─", total)))
	b.WriteString("\n")
	for _, r := range rows {
		for i := 0; i < n; i++ {
			cell := ""
			if i < len(r) {
				cell = r[i]
			}
			if i == n-1 {
				b.WriteString(cell)
			} else {
				b.WriteString(pad(cell, w[i]))
				b.WriteString("  ")
			}
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// KV renders a heading followed by aligned key/value rows, for compact
// summaries (status, doctor, account).
func KV(title string, pairs [][2]string) string {
	var b strings.Builder
	if title != "" {
		b.WriteString(Accent(title))
		b.WriteString("\n")
	}
	max := 0
	for _, p := range pairs {
		if l := len(p[0]); l > max {
			max = l
		}
	}
	for _, p := range pairs {
		b.WriteString("  ")
		b.WriteString(Dim(pad(p[0], max)))
		b.WriteString("  ")
		b.WriteString(p[1])
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func pad(s string, w int) string {
	if len(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-len(s))
}

// Print writes a block followed by a newline.
func Print(s string) { fmt.Println(s) }
