package termui

import (
	"strings"
	"testing"
)

func TestTableAlignsAndStyles(t *testing.T) {
	out := Table([]string{"Name", "State"}, [][]string{
		{"Upwork", Good("connected")},
		{"Local", "off"},
	})
	if !strings.Contains(out, "Name") || !strings.Contains(out, "State") {
		t.Fatalf("header missing:\n%s", out)
	}
	if !strings.Contains(out, "Upwork") || !strings.Contains(out, "connected") {
		t.Fatalf("rows missing:\n%s", out)
	}
	if !strings.Contains(out, "─") {
		t.Fatalf("rule missing:\n%s", out)
	}
}

func TestStyleDisabledWhenPiped(t *testing.T) {
	orig := StyleEnabled
	StyleEnabled = false
	defer func() { StyleEnabled = orig }()

	out := Bold("value") + Dim("meta")
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("no ANSI codes should be emitted when styling is off: %q", out)
	}
	if out != "valuemeta" {
		t.Fatalf("plain text expected, got %q", out)
	}
}

func TestKV(t *testing.T) {
	out := KV("Scout", [][2]string{{"Version", "v1"}, {"Profile", "Ian"}})
	if !strings.Contains(out, "Scout") || !strings.Contains(out, "Version") {
		t.Fatalf("KV output missing content:\n%s", out)
	}
}
