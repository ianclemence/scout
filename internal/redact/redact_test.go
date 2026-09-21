package redact

import (
	"strings"
	"testing"
)

func TestMasksBearerAndKeys(t *testing.T) {
	if s := Text("Authorization: Bearer abc123XYZ-_"); !strings.Contains(s, "<redacted>") || strings.Contains(s, "abc123") {
		t.Fatalf("bearer not masked: %q", s)
	}
	if s := Text("key sk-d6c4ca872a62xxx"); strings.Contains(s, "sk-d6c4") {
		t.Fatalf("api key not masked: %q", s)
	}
	if Text("nothing secret here") != "nothing secret here" {
		t.Fatal("clean text altered")
	}
}

func TestArgsMasking(t *testing.T) {
	m := Args(map[string]any{"token": "abc", "query": "go work", "api_key": "sk-x"})
	if m["token"] != "<redacted>" || m["api_key"] != "<redacted>" || m["query"] != "go work" {
		t.Fatalf("bad masking: %v", m)
	}
}
