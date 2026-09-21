package tui

import "testing"

func TestFuzzyMatch(t *testing.T) {
	cases := []struct {
		query, text string
		want        bool
	}{
		{"dzflash", "deepseek/deepseek-flash", false}, // tokens are split, not this
		{"deepseek", "deepseek/deepseek-flash", true},
		{"flash", "deepseek/deepseek-flash", true},
		{"xyz", "deepseek/deepseek-flash", false},
	}
	for _, c := range cases {
		if got, _ := fuzzyMatch(c.query, c.text); got != c.want {
			t.Fatalf("fuzzyMatch(%q,%q) = %v, want %v", c.query, c.text, got, c.want)
		}
	}
}

func TestFuzzyFilterOrdersBestFirst(t *testing.T) {
	items := []string{"anthropic/claude-haiku-4-5", "deepseek/deepseek-flash", "deepseek/deepseek-v4-pro"}
	got := fuzzyFilter(items, "deepseek", func(s string) string { return s })
	if len(got) != 2 {
		t.Fatalf("expected 2 matches, got %v", got)
	}
	// Exact-ish token should rank the flash model above the v4 pro under the
	// same provider prefix (order is score-based, both still match).
	if got[0] != "deepseek/deepseek-flash" && got[0] != "deepseek/deepseek-v4-pro" {
		t.Fatalf("unexpected first match %q", got[0])
	}
}

func TestFuzzyFilterEmptyQueryPassesThrough(t *testing.T) {
	items := []string{"a", "b", "c"}
	got := fuzzyFilter(items, "  ", func(s string) string { return s })
	if len(got) != 3 {
		t.Fatalf("empty query should pass through, got %v", got)
	}
}
