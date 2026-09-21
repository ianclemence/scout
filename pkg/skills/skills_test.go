package skills

import (
	"strings"
	"testing"
)

func TestLoadAll(t *testing.T) {
	r, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(r.List()) < 15 {
		t.Fatalf("expected skill library, got %d", len(r.List()))
	}
	for _, s := range r.List() {
		if s.Name == "" || len(s.Triggers) == 0 || s.Body == "" {
			t.Fatalf("incomplete skill %+v", s.Name)
		}
	}
}

func TestSelectDiscovery(t *testing.T) {
	r, _ := Load()
	sel := r.Select("Find me Go backend work and tell me which fit", 3)
	names := map[string]bool{}
	for _, s := range sel {
		names[s.Name] = true
	}
	if !names["discover-opportunities"] || !names["evaluate-opportunity"] {
		t.Fatalf("bad selection: %v", names)
	}
}

func TestSelectPrepareApplication(t *testing.T) {
	r, _ := Load()
	sel := r.Select("Prepare an application for opportunity 42", 5)
	names := map[string]bool{}
	for _, s := range sel {
		names[s.Name] = true
	}
	for _, want := range []string{"prepare-application", "write-proposal", "application-qa"} {
		if !names[want] {
			t.Fatalf("missing %s in %v", want, names)
		}
	}
}

func TestSelectEmpty(t *testing.T) {
	r, _ := Load()
	if sel := r.Select("hello there", 3); len(sel) != 0 {
		t.Fatal("unrelated text should select nothing")
	}
}

func TestContextBlockBounded(t *testing.T) {
	r, _ := Load()
	block := ContextBlock(r.List())
	if !strings.Contains(block, "load_skill") {
		t.Fatal("block must point at on-demand loading")
	}
	for _, s := range r.List() {
		if !strings.Contains(block, s.Name) {
			t.Fatalf("block missing %s", s.Name)
		}
		if strings.Contains(block, "Do not silently convert inferred preferences") {
			t.Fatal("full bodies must not be injected")
		}
	}
	if ContextBlock(nil) != "" {
		t.Fatal("empty should be empty")
	}
}

func TestSkillSummaries(t *testing.T) {
	r, _ := Load()
	for _, s := range r.List() {
		if s.Summary == "" {
			t.Fatalf("skill %s missing summary", s.Name)
		}
	}
	found := false
	for _, s := range r.List() {
		if s.Name == "discover-opportunities" && len(s.Summary) < 400 {
			found = true
		}
	}
	if !found {
		t.Fatal("discover summary missing")
	}
}
