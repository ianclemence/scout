package tui

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/ianclemence/scout/pkg/runtime"
)

// TestDumpStatesForJevReview renders real Scout transcript states for the
// external JEV review. Runs only when SCOUT_TUI_DUMP is set to a file path.
func TestDumpStatesForJevReview(t *testing.T) {
	path := os.Getenv("SCOUT_TUI_DUMP")
	if path == "" {
		t.Skip("set SCOUT_TUI_DUMP to dump review states")
	}
	type state struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Render string `json:"render"`
	}
	var out []state
	emit := func(id, title, render string) {
		out = append(out, state{ID: id, Title: title, Render: render})
	}

	m := testModel()
	emit("bubble-single", "single-line user message", m.renderEntry(entry{kind: eUser, text: "hello"}))
	emit("bubble-multi", "multi-paragraph user message",
		m.renderEntry(entry{kind: eUser, text: "first **not bold**\n\nsecond"}))

	m2 := testModel()
	m2.working = true
	for _, tok := range []string{"## Head\n", "Body line\n", "Tail."} {
		m2.handleEvent(runtime.Event{Type: "token", Text: tok})
	}
	emit("dock-mid-stream", "dock preview while reply streams", m2.dockPreview())
	emit("commit-full", "committed markdown reply",
		m2.renderEntry(entry{kind: eScout, text: "## Head\nBody line\nTail."}))

	m25 := testModel()
	m25.width, m25.height, m25.ready = 80, 24, true
	m25.working = true
	m25.streamSty = streamStyler{width: streamWidth(m25)}
	var prog strings.Builder
	pfeed := func(text string) {
		m25.handleEvent(runtime.Event{Type: "token", Text: text})
		if m25.lastFlush != "" {
			prog.WriteString(m25.lastFlush + "\n")
			m25.lastFlush = ""
		}
	}
	pfeed("## Head\n")
	pfeed("Body line\nPartial")
	emit("scrollback-mid-stream", "reply growing line by line in the scrollback", prog.String())

	m3 := testModel()
	m3.ta.SetValue(strings.Repeat("word ", 40))
	m3.layoutComposer()
	emit("composer-long", "long input in composer box",
		"height="+strconv.Itoa(m3.ta.Height())+"\n"+m3.promptBox())

	raw, _ := json.MarshalIndent(out, "", " ")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}
