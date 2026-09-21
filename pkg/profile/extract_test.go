package profile

import (
	"strings"
	"testing"
)

func TestExtractProfileClean(t *testing.T) {
	cv := `JANE DOE
jane@example.com | +1 555 0100 | github.com/janedoe | linkedin.com/in/janedoe

SUMMARY
Backend engineer with 8 years building APIs.

EXPERIENCE
Acme Corp
Jan 2020 – Present
Senior Engineer
Remote
• Built the billing service.
• Shipped the migration
  to Postgres.

Globex
Mar 2017 – Dec 2019
Engineer
London
• Built the reporting pipeline.

EDUCATION
MIT
2015 – 2019
BSc Computer Science
Cambridge, MA

PROJECTS
Widget | Go, Postgres, Docker
A widget service
• Handled 1M requests/day.

RELEVANT SKILLS
Languages: Go, Python, SQL
Cloud: AWS, Docker
`
	f := ExtractProfile(cv)
	if f.Name != "JANE DOE" {
		t.Fatalf("name = %q", f.Name)
	}
	if f.Links.Email != "jane@example.com" || f.Links.GitHub != "https://github.com/janedoe" || f.Links.LinkedIn != "https://linkedin.com/in/janedoe" {
		t.Fatalf("links = %+v", f.Links)
	}
	if !strings.Contains(f.Summary, "Backend engineer") {
		t.Fatalf("summary = %q", f.Summary)
	}
	if len(f.Experience) != 2 {
		t.Fatalf("expected 2 experiences, got %d: %+v", len(f.Experience), f.Experience)
	}
	if f.Experience[0].Company != "Acme Corp" || f.Experience[0].Title != "Senior Engineer" || f.Experience[0].Period != "Jan 2020 – Present" {
		t.Fatalf("experience[0] = %+v", f.Experience[0])
	}
	if !strings.Contains(f.Experience[0].Description, "billing service") {
		t.Fatalf("experience[0] description = %q", f.Experience[0].Description)
	}
	if len(f.Education) != 1 || f.Education[0].School != "MIT" || f.Education[0].Degree != "BSc Computer Science" {
		t.Fatalf("education = %+v", f.Education)
	}
	if len(f.Projects) != 1 || f.Projects[0].Name != "Widget" || len(f.Projects[0].Stack) != 3 {
		t.Fatalf("projects = %+v", f.Projects)
	}
	if f.Title == "" {
		t.Fatal("title should be derived from the latest experience")
	}
}

func TestHeaderKeySurvivesKerning(t *testing.T) {
	for in, want := range map[string]string{
		"SUMMAR Y":                "SUMMARY",
		"EDUCA TION":              "EDUCATION",
		"PR OJECTS":               "PROJECTS",
		"RELEVANT SKILLS":         "SKILLS",
		"COMMUNITY INV OL VEMENT": "COMMUNITY",
		"F reelance Engineer":     "",
		"Oct 2023 – Dec 2024":     "",
	} {
		got, ok := headerKey(in)
		if want == "" {
			if ok {
				t.Fatalf("headerKey(%q) unexpectedly = %q", in, got)
			}
			continue
		}
		if !ok || got != want {
			t.Fatalf("headerKey(%q) = (%q,%v), want %q", in, got, ok, want)
		}
	}
}

func TestExtractProfileDoesNotInvent(t *testing.T) {
	// A CV with no experience/education/projects must yield none, not guesses.
	f := ExtractProfile("BOB SMITH\nbob@example.com\n\nSUMMARY\nDesigner.")
	if len(f.Experience) != 0 || len(f.Education) != 0 || len(f.Projects) != 0 {
		t.Fatalf("extractor invented entries: %+v", f)
	}
	if f.Links.Email != "bob@example.com" {
		t.Fatalf("email = %q", f.Links.Email)
	}
}

func TestParseUpworkBudgetDoesNotApply(t *testing.T) {
	// Guard: the profile extractor must not turn generated text into facts.
	// Extraction only reads the given document.
	f := ExtractProfile("SUMMARY\nI could be a great fit.")
	if strings.Contains(f.Summary, "experience at") {
		t.Fatal("summary should be verbatim, not expanded")
	}
}
