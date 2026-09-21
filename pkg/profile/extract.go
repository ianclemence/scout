// Deterministic CV/resume extraction.
//
// Scout's profile is the user's source of truth. Before this, an import set
// only name + skills, leaving experience, education, projects, and links empty
// so matching degenerated to keyword hits. This extractor recovers those from
// the document itself by parsing its section structure.
//
// It is deliberately conservative: every value is taken verbatim from the
// document. It never infers a company, title, date, or achievement that the
// text does not contain. PDF kerning artifacts ("Soft w are") are preserved
// rather than guessed at.
package profile

import (
	"regexp"
	"sort"
	"strings"

	"github.com/ianclemence/scout/pkg/domain"
)

// ProjectFact is one project entry from a resume.
type ProjectFact struct {
	Name        string
	Stack       []string
	Description string
}

// Links are contact URLs recovered from the document.
type Links struct {
	Email    string
	Phone    string
	GitHub   string
	LinkedIn string
	Website  string
}

// Facts is the structured result of parsing a CV/resume.
type Facts struct {
	Name       string
	Title      string
	Summary    string
	Skills     []string
	Experience []domain.Experience
	Education  []domain.Education
	Projects   []ProjectFact
	Links      Links
}

var (
	fieldSep  = regexp.MustCompile(`(?:\r?\n| {2,}|\t)+`)
	yearRe    = regexp.MustCompile(`(?:19|20)\d{2}`)
	emailRe   = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	phoneRe   = regexp.MustCompile(`\+[0-9][0-9 ()\-]{6,18}[0-9]`)
	githubRe  = regexp.MustCompile(`(?i)github\.com/[A-Za-z0-9_.\-]+`)
	linkedRe  = regexp.MustCompile(`(?i)linkedin\.com/[A-Za-z0-9_./\-]+`)
	websiteRe = regexp.MustCompile(`(?i)https?://[A-Za-z0-9_./\-]+`)
)

// splitFields breaks a document into fields on blank lines, newlines, or runs
// of two-or-more spaces (the pattern PDF text extraction produces).
func splitFields(s string) []string {
	parts := fieldSep.Split(strings.TrimSpace(s), -1)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// sectionAliases maps a normalized heading to a canonical section key.
var sectionAliases = map[string]string{
	"SUMMARY": "SUMMARY", "PROFILE": "SUMMARY", "OBJECTIVE": "SUMMARY",
	"ABOUT": "SUMMARY", "ABOUTME": "SUMMARY", "PROFESSIONALSUMMARY": "SUMMARY",
	"EDUCATION": "EDUCATION", "EDUCATIONANDTRAINING": "EDUCATION",
	"EXPERIENCE": "EXPERIENCE", "WORKEXPERIENCE": "EXPERIENCE",
	"EMPLOYMENT": "EXPERIENCE", "WORKHISTORY": "EXPERIENCE",
	"PROFESSIONALEXPERIENCE": "EXPERIENCE",
	"PROJECTS":               "PROJECTS", "SELECTEDPROJECTS": "PROJECTS", "PORTFOLIO": "PROJECTS",
	"SKILLS": "SKILLS", "RELEVANTSKILLS": "SKILLS", "TECHNICALSKILLS": "SKILLS",
	"CORESKILLS": "SKILLS", "SKILLSANDTOOLS": "SKILLS",
	"COMMUNITYINVOLVEMENT": "COMMUNITY", "VOLUNTEERING": "COMMUNITY",
	"CERTIFICATIONS": "CERTIFICATIONS", "CERTIFICATES": "CERTIFICATIONS",
	"LANGUAGES": "LANGUAGES",
}

// headerKey returns the canonical section key for a heading line. A heading is
// an all-uppercase, short line (letters only after removing spaces) whose
// normalized form is a known section name. This survives PDF kerning such as
// "EDUCA TION" and "PR OJECTS".
func headerKey(line string) (string, bool) {
	letters := make([]rune, 0, len(line))
	hasLower := false
	for _, r := range line {
		switch {
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= 'A' && r <= 'Z':
			letters = append(letters, r)
		case r == ' ' || r == '\t':
		default:
			return "", false
		}
	}
	key := string(letters)
	if hasLower || len(key) == 0 || len(key) > 24 {
		return "", false
	}
	if canon, ok := sectionAliases[key]; ok {
		return canon, true
	}
	return "", false
}

// isDateLine reports whether a field is a date range ("Aug 2022 – Present",
// "Oct 2023 – Dec 2024"), tolerating kerning.
func isDateLine(s string) bool {
	d := strings.ToLower(strings.Join(strings.Fields(s), ""))
	if !strings.ContainsAny(d, "–—-") {
		return false
	}
	years := yearRe.FindAllString(d, -1)
	if len(years) >= 2 {
		return true
	}
	return len(years) >= 1 && strings.Contains(d, "present")
}

// ExtractProfile deterministically parses a CV/resume into structured facts.
func ExtractProfile(text string) Facts {
	fields := splitFields(text)
	f := Facts{
		Name:  guessName(text),
		Links: extractLinks(fields),
	}
	sections := map[string][]string{}
	cur := ""
	for _, ln := range fields {
		if key, ok := headerKey(ln); ok {
			cur = key
			continue
		}
		if cur != "" {
			sections[cur] = append(sections[cur], ln)
		}
	}
	f.Summary = strings.Join(sections["SUMMARY"], " ")
	f.Experience = parseExperience(sections["EXPERIENCE"])
	f.Education = parseEducation(sections["EDUCATION"])
	f.Projects = parseProjects(sections["PROJECTS"])
	f.Skills = mergeSkills(guessSkills(text), parseSkillLines(sections["SKILLS"]))
	if len(f.Experience) > 0 {
		f.Title = cleanField(f.Experience[0].Title)
	}
	return f
}

// collectBullets gathers bullet item text in lines[start:end], joining wrapped
// continuation lines (which start lowercase). It accepts a bullet marker on
// its own field (PDF extraction) or at the start of the field (plain text).
func collectBullets(lines []string, start, end int) string {
	if end > len(lines) {
		end = len(lines)
	}
	var out []string
	for i := start; i < end; i++ {
		ln := strings.TrimSpace(lines[i])
		text := ""
		switch {
		case ln == "•":
			if i+1 < end {
				text = strings.TrimSpace(lines[i+1])
				i++
			}
		case strings.HasPrefix(ln, "•"):
			text = strings.TrimSpace(strings.TrimPrefix(ln, "•"))
		case strings.HasPrefix(ln, "- "), strings.HasPrefix(ln, "* "):
			text = strings.TrimSpace(ln[2:])
		default:
			continue
		}
		for i+1 < end {
			next := strings.TrimSpace(lines[i+1])
			if next == "" || strings.HasPrefix(next, "•") || strings.HasPrefix(next, "- ") || strings.HasPrefix(next, "* ") || isDateLine(next) || !startsWithLower(next) {
				break
			}
			text += " " + next
			i++
		}
		if text != "" {
			out = append(out, text)
		}
	}
	return strings.Join(out, "; ")
}

func startsWithLower(s string) bool {
	for _, r := range s {
		return r >= 'a' && r <= 'z'
	}
	return false
}

func parseExperience(lines []string) []domain.Experience {
	var out []domain.Experience
	for i, ln := range lines {
		if !isDateLine(ln) {
			continue
		}
		// The entry's bullets run until the next date line.
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			if isDateLine(lines[j]) {
				end = j - 1
				break
			}
		}
		e := domain.Experience{Period: cleanField(ln)}
		if i-1 >= 0 {
			e.Company = cleanField(lines[i-1])
		}
		// The title usually follows the date; skip a location-only line.
		for j := i + 1; j < len(lines) && j <= i+2; j++ {
			cand := strings.TrimSpace(lines[j])
			if cand == "•" || isDateLine(cand) {
				break
			}
			if isLocationLine(cand) {
				continue
			}
			e.Title = cleanField(cand)
			break
		}
		e.Description = collectBullets(lines, i+1, end)
		if e.Title == "" && e.Company == "" {
			continue
		}
		out = append(out, e)
	}
	return out
}

func parseEducation(lines []string) []domain.Education {
	var out []domain.Education
	for i, ln := range lines {
		if !isDateLine(ln) {
			continue
		}
		ed := domain.Education{Year: cleanField(ln)}
		if i-1 >= 0 {
			ed.School = cleanField(lines[i-1])
		}
		if i+1 < len(lines) && !isDateLine(lines[i+1]) && strings.TrimSpace(lines[i+1]) != "•" {
			ed.Degree = cleanField(lines[i+1])
		}
		if ed.School == "" || ed.School == ed.Degree {
			continue
		}
		out = append(out, ed)
	}
	return out
}

func parseProjects(lines []string) []ProjectFact {
	var out []ProjectFact
	for i := 0; i < len(lines); i++ {
		ln := strings.TrimSpace(lines[i])
		var name, stack string
		switch {
		case ln == "|":
			if i-1 >= 0 {
				name = strings.TrimSpace(lines[i-1])
			}
			if i+1 < len(lines) {
				stack = strings.TrimSpace(lines[i+1])
			}
		case strings.Contains(ln, "|"):
			parts := strings.SplitN(ln, "|", 2)
			name, stack = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		default:
			continue
		}
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			if strings.Contains(lines[j], "|") {
				end = j
				break
			}
		}
		p := ProjectFact{Name: cleanField(name), Stack: splitCSVish(stack), Description: collectBullets(lines, i+1, end)}
		if p.Name != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseSkillLines extracts "Category: a, b, c" skill lines.
func parseSkillLines(lines []string) []string {
	var out []string
	for _, ln := range lines {
		_, rest, ok := strings.Cut(ln, ":")
		if !ok {
			continue
		}
		out = append(out, splitCSVish(rest)...)
	}
	return out
}

func splitCSVish(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' }) {
		p = cleanField(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// cleanField trims whitespace and dangling separators from a verbatim field.
func cleanField(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "•|,;:-–—")
	return strings.TrimSpace(s)
}

func isLocationLine(s string) bool {
	low := strings.ToLower(s)
	return strings.Contains(low, "(remote)") || strings.Contains(low, "remote)") ||
		strings.Contains(low, "thailand") || strings.Contains(low, "tanzania") ||
		strings.Contains(low, "kenya") || strings.Contains(low, "online)")
}

func mergeSkills(lists ...[]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, l := range lists {
		for _, s := range l {
			s = strings.ToLower(strings.TrimSpace(s))
			if s == "" || seen[s] {
				continue
			}
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// extractLinks recovers contact URLs from individual fields, so a link cannot
// bleed into the next field when spaces are stripped for kerning.
func extractLinks(fields []string) Links {
	var l Links
	for _, f := range fields {
		compact := strings.Join(strings.Fields(f), "")
		if l.Email == "" {
			l.Email = emailRe.FindString(compact)
		}
		if l.Phone == "" {
			l.Phone = phoneRe.FindString(compact)
		}
		if l.GitHub == "" {
			if m := githubRe.FindString(compact); m != "" {
				l.GitHub = "https://" + m
			}
		}
		if l.LinkedIn == "" {
			if m := linkedRe.FindString(compact); m != "" {
				l.LinkedIn = "https://" + m
			}
		}
		if l.Website == "" {
			if m := websiteRe.FindString(compact); m != "" {
				low := strings.ToLower(m)
				if !strings.Contains(low, "github.com") && !strings.Contains(low, "linkedin.com") {
					l.Website = m
				}
			}
		}
	}
	return l
}
