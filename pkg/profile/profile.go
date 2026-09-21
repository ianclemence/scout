// Package profile handles CV import and structured profile management.
// The structured profile is the source of truth; uploads become evidence.
package profile

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ianclemence/scout/pkg/docparse"
	"github.com/ianclemence/scout/pkg/domain"
	"github.com/ianclemence/scout/pkg/store"
)

func newID() string {
	return fmt.Sprintf("%d-%x", time.Now().UnixNano(), sha256.Sum256([]byte(fmt.Sprint(time.Now().UnixNano()))))[:24]
}

// ImportDocument reads CV/resume content and extracts a starter profile.
// ImportPath is preferred (format detected from the real path).
func ImportDocument(db *store.Store, filename string, raw []byte) (*domain.ProfessionalProfile, []domain.Evidence, error) {
	text, _, err := docparse.ParseBytes(raw, filename)
	if err != nil {
		return nil, nil, err
	}
	p := &domain.ProfessionalProfile{
		ID:            "default",
		DisplayName:   guessName(text),
		Skills:        guessSkills(text),
		Technologies:  guessSkills(text),
		ProposalStyle: "concise",
		UpdatedAt:     time.Now().UTC(),
	}
	p.Experience = guessExperience(text)
	ev := []domain.Evidence{{
		ID: newID(), Kind: "cv_section", Reference: filename,
		Content: truncate(text, 8000), Source: "upload", CreatedAt: time.Now().UTC(),
	}}
	if err := Save(db, p); err != nil {
		return nil, nil, err
	}
	for _, e := range ev {
		b, _ := json.Marshal(e)
		_ = b
		_, _ = db.DB.Exec(`INSERT OR REPLACE INTO evidence(id,kind,reference,content,source,created_at) VALUES(?,?,?,?,?,?)`,
			e.ID, e.Kind, e.Reference, e.Content, e.Source, e.CreatedAt.Format(time.RFC3339))
	}
	_, _ = db.DB.Exec(`INSERT INTO events(id,kind,detail,created_at) VALUES(?,?,?,?)`,
		newID(), "profile_import", filename, time.Now().UTC().Format(time.RFC3339))
	return p, ev, nil
}

func Save(db *store.Store, p *domain.ProfessionalProfile) error {
	p.UpdatedAt = time.Now().UTC()
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = db.DB.Exec(`INSERT OR REPLACE INTO profile(id,data,updated_at) VALUES('default',?,?)`, string(b), p.UpdatedAt.Format(time.RFC3339))
	return err
}

func Load(db *store.Store) (*domain.ProfessionalProfile, error) {
	var data string
	err := db.DB.QueryRow(`SELECT data FROM profile WHERE id='default'`).Scan(&data)
	if err != nil {
		return &domain.ProfessionalProfile{ID: "default", ProposalStyle: "concise", MaxAppsPerDay: 10, MaxConnectsPerApp: 20}, nil
	}
	var p domain.ProfessionalProfile
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func ListEvidence(db *store.Store) ([]domain.Evidence, error) {
	rows, err := db.DB.Query(`SELECT id,kind,reference,content,source,created_at FROM evidence ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Evidence
	for rows.Next() {
		var e domain.Evidence
		var ts string
		rows.Scan(&e.ID, &e.Kind, &e.Reference, &e.Content, &e.Source, &ts)
		e.CreatedAt, _ = time.Parse(time.RFC3339, ts)
		out = append(out, e)
	}
	return out, nil
}

// ---- lightweight heuristics (deterministic, honest: keyword extraction only) ----

var knownSkills = []string{
	"go", "python", "php", "laravel", "react", "react native", "node.js", "node",
	"typescript", "javascript", "aws", "docker", "kubernetes", "postgres", "postgresql",
	"mysql", "sqlite", "llm", "openai", "anthropic", "mcp", "api", "saas", "flutter",
	"django", "rails", "java", "rust", "graphql", "redis", "terraform", "ai", "ml",
	"tailwind", "mongodb", "next.js", "expo", "firebase", "express", "ollama", "hnsw",
}

// guessSkills extracts known skills by keyword. It matches against both the
// raw text and a space-collapsed copy: PDF extraction often breaks words at
// kerning gaps ("T yp eScript"), so collapsing single spaces between letters
// recovers the real token without guessing.
func guessSkills(text string) []string {
	low := strings.ToLower(text)
	collapsed := strings.ToLower(collapseLetterSpaces(text))
	var out []string
	seen := map[string]bool{}
	for _, s := range knownSkills {
		matched := false
		if len([]rune(s)) <= 4 {
			// Short tokens (go, ai, ml, api, expo) match only as whole words,
			// so collapsing never turns "MongoDB" into a spurious "go".
			matched = containsWord(low, s) || containsWord(collapsed, s)
		} else {
			// Multi-word skills may have had their space consumed by the
			// PDF collapse ("ReactNative"), so also test the joined form.
			joined := strings.ReplaceAll(s, " ", "")
			matched = strings.Contains(low, s) || strings.Contains(collapsed, s) || strings.Contains(collapsed, joined)
		}
		if matched && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// containsWord reports whether token appears in s delimited by non-letters.
func containsWord(s, token string) bool {
	from := 0
	for {
		i := strings.Index(s[from:], token)
		if i < 0 {
			return false
		}
		i += from
		leftOK := i == 0 || !isLetter(rune(s[i-1]))
		end := i + len(token)
		rightOK := end >= len(s) || !isLetter(rune(s[end]))
		if leftOK && rightOK {
			return true
		}
		from = i + 1
	}
}

// collapseLetterSpaces removes a single space that sits between two letters.
// This repairs PDF kerning artifacts ("soft w are" → "software") while leaving
// punctuation-adjacent spacing intact.
func collapseLetterSpaces(s string) string {
	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(s))
	for i, r := range runes {
		if r == ' ' && i > 0 && i+1 < len(runes) && isLetter(runes[i-1]) && isLetter(runes[i+1]) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func guessName(text string) string {
	// Line-based: a short early line with 2–4 words and no contact marker.
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || len(line) > 60 {
			continue
		}
		words := strings.Fields(line)
		if len(words) >= 2 && len(words) <= 4 && !strings.Contains(line, "@") {
			return line
		}
	}
	// Blob fallback: PDFs often extract as one long line. The name is the
	// leading run of capitalized words before the first contact marker.
	head := text
	if len(head) > 400 {
		head = head[:400]
	}
	if i := firstContactIndex(head); i > 0 {
		head = head[:i]
	}
	var name []string
	for _, w := range strings.Fields(head) {
		w = strings.Trim(w, ".,;:()[]{}|/\\!?\"'“”")
		if !isASCIIWord(w) {
			break
		}
		name = append(name, w)
		if len(name) == 4 {
			break
		}
	}
	if len(name) >= 2 {
		return strings.Join(name, " ")
	}
	return ""
}

// firstContactIndex returns the index of the first email, phone, or URL
// marker in s, or -1 when there is none.
func firstContactIndex(s string) int {
	best := -1
	mark := func(i int) {
		if i >= 0 && (best < 0 || i < best) {
			best = i
		}
	}
	mark(strings.IndexByte(s, '@'))
	for _, u := range []string{"http://", "https://", "www."} {
		mark(strings.Index(s, u))
	}
	// A plus followed by a digit marks a phone number.
	for i := 0; i+1 < len(s); i++ {
		if s[i] == '+' && s[i+1] >= '0' && s[i+1] <= '9' {
			mark(i)
			break
		}
	}
	return best
}

// isASCIIWord reports whether w is a plain ASCII word starting uppercase,
// which is what a name token looks like.
func isASCIIWord(w string) bool {
	if w == "" || w[0] < 'A' || w[0] > 'Z' {
		return false
	}
	for _, r := range w {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '-' || r == '\'' {
			continue
		}
		return false
	}
	return true
}

func guessExperience(text string) []domain.Experience {
	return nil // user edits in UI; imports stay as evidence
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

var _ = os.Getenv
