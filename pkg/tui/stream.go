package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Progressive reply streaming (Ghost parity): the answer grows line-by-line
// in the scrollback as tokens arrive instead of appearing whole at the end.
// Completion prints only the unprinted tail, never the full text again.

// nextStreamBlock decides what the progressive printer may emit for the
// current streaming buffer: whether the header is due, and which completed
// raw lines have not been printed yet. The trailing partial line is never
// emitted here; finishTurn prints it. Pure so turns can be asserted without
// a running program.
func nextStreamBlock(streaming string, headerShown bool, flushedLines int) (wantHeader bool, lines []string, flushed int) {
	parts := strings.Split(streaming, "\n")
	complete := parts[:len(parts)-1]
	if !headerShown && (len(complete) > flushedLines || strings.TrimSpace(streaming) != "") {
		wantHeader = true
	}
	for i, ln := range complete {
		if i < flushedLines {
			continue
		}
		lines = append(lines, ln)
	}
	return wantHeader, lines, len(complete)
}

// streamStyler renders progressive reply lines with the same markdown as
// committed replies. Code fences toggle a running state (concealed, with a
// single separating blank like the full renderer); table rows buffer until
// a non-row line or the turn end decides them. Single lines go through
// renderMarkdownLine, so live text matches committed text.
type streamStyler struct {
	width        int
	inCode       bool
	table        []string
	lastWasBlank bool
}

func (s *streamStyler) line(raw string) []string {
	if s.width < 20 {
		s.width = 20 // same floor as committed rendering; never wrap per-rune
	}
	trim := strings.TrimSpace(raw)
	if strings.HasPrefix(trim, "```") {
		out := s.flushTable()
		s.inCode = !s.inCode
		if !s.lastWasBlank {
			out = append(out, "")
			s.lastWasBlank = true
		}
		return out
	}
	if s.inCode {
		var out []string
		for _, wl := range wrap(raw, s.width) {
			out = append(out, " "+styleMDCodeBlock.Render(wl))
		}
		s.lastWasBlank = false
		return out
	}
	if isTableRow(trim) {
		s.table = append(s.table, raw)
		return nil
	}
	out := s.flushTable()
	rendered := renderMarkdownLine(trim, raw, s.width)
	for _, ln := range strings.Split(rendered, "\n") {
		if ln == "" {
			out = append(out, "")
			s.lastWasBlank = true
		} else {
			out = append(out, " "+ln)
			s.lastWasBlank = false
		}
	}
	return out
}

// flushTable renders buffered table rows as a grid when they form one,
// otherwise as plain lines. Always empties the buffer.
func (s *streamStyler) flushTable() []string {
	if len(s.table) == 0 {
		return nil
	}
	buf := s.table
	s.table = nil
	if len(buf) >= 2 && isTableDelimiter(strings.TrimSpace(buf[1])) {
		var out []string
		for _, rl := range renderTable(buf, s.width) {
			out = append(out, " "+rl)
		}
		s.lastWasBlank = false
		return out
	}
	var out []string
	for _, raw := range buf {
		trim := strings.TrimSpace(raw)
		for _, ln := range strings.Split(renderMarkdownLine(trim, raw, s.width), "\n") {
			if ln == "" {
				out = append(out, "")
				s.lastWasBlank = true
			} else {
				out = append(out, " "+ln)
				s.lastWasBlank = false
			}
		}
	}
	return out
}

// flush ends the turn: any pending table is decided.
func (s *streamStyler) flush() []string {
	defer func() { s.inCode = false }()
	return s.flushTable()
}

// scoutHead is the reply header line (no duration; finishTurn appends it).
// Nil-safe: headless/test models without a session still render.
func (m *model) scoutHead() string {
	prov, mod := "", ""
	if m.st != nil && m.st.Sess != nil {
		prov, mod = m.st.Sess.Provider, m.st.Sess.Model
	}
	if prov == "" && mod == "" {
		return " " + styleAssistantName.Render("👷 Scout")
	}
	return " " + styleAssistantName.Render("👷 Scout · "+prov+"/"+mod)
}

// flushStreamLines prints the reply's newly completed lines into the
// scrollback as they arrive, styled exactly like committed replies. A blank
// line separates the reply from the user's message above. Returns nil when
// nothing new is printable yet.
func (m *model) flushStreamLines() tea.Cmd {
	wantHeader, raws, flushed := nextStreamBlock(m.stream.String(), m.streamHeaderShown, m.streamFlushedLines)
	var styled []string
	for _, raw := range raws {
		styled = append(styled, m.streamSty.line(raw)...)
	}
	if !wantHeader && len(styled) == 0 {
		return nil
	}
	m.streamHeaderShown = true
	m.streamFlushedLines = flushed
	var b strings.Builder
	if wantHeader {
		b.WriteString("\n")
		b.WriteString(m.scoutHead())
	}
	for _, ln := range styled {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(ln)
	}
	text := b.String()
	m.lastFlush = text
	return tea.Println(text)
}

// streamTail returns the not-yet-printed remainder of the streaming buffer,
// styled, with a trailing blank line separating the reply from what follows.
// Buffered table rows render before the trailing partial line: the buffer
// only ever holds a trailing run of table rows, so this preserves order.
func (m *model) streamTail() string {
	parts := strings.Split(m.stream.String(), "\n")
	var tail []string
	for i, raw := range parts {
		if i < m.streamFlushedLines {
			continue
		}
		tail = append(tail, raw)
	}
	var out []string
	out = append(out, m.streamSty.flush()...)
	for _, raw := range tail {
		out = append(out, m.streamSty.line(raw)...)
	}
	out = append(out, m.streamSty.flush()...)
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n") + "\n"
}

func streamWidth(m *model) int {
	w := m.width
	if w < 20 {
		w = 80
	}
	return w - 1
}
