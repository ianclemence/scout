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

	"github.com/ianclemence/scout/pkg/domain"
	"github.com/ianclemence/scout/pkg/store"
)

func newID() string {
	return fmt.Sprintf("%d-%x", time.Now().UnixNano(), sha256.Sum256([]byte(fmt.Sprint(time.Now().UnixNano()))))[:24]
}

// ImportDocument reads a CV/resume file (txt, md, pdf-as-text fallback) and
// extracts a starter structured profile. PDF parsing is intentionally
// minimal: plain-text extraction only, no heavy deps.
func ImportDocument(db *store.Store, filename string, raw []byte) (*domain.ProfessionalProfile, []domain.Evidence, error) {
	text := string(raw)
	if strings.HasSuffix(strings.ToLower(filename), ".pdf") {
		text = extractPDFText(raw)
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

var knownSkills = []string{"go", "python", "php", "laravel", "react", "react native", "node.js", "node", "typescript", "javascript", "aws", "docker", "kubernetes", "postgres", "mysql", "sqlite", "llm", "openai", "anthropic", "mcp", "api", "saas", "flutter", "django", "rails", "java", "rust", "graphql", "redis", "terraform", "ai", "ml"}

func guessSkills(text string) []string {
	low := strings.ToLower(text)
	var out []string
	seen := map[string]bool{}
	for _, s := range knownSkills {
		if strings.Contains(low, s) && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func guessName(text string) string {
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
	return ""
}

func guessExperience(text string) []domain.Experience {
	return nil // user edits in UI; imports stay as evidence
}

func extractPDFText(raw []byte) string {
	// Minimal: pull parenthesized literal strings from PDF content streams.
	// Good enough for text-based CVs without a PDF dep on the Pi.
	var sb strings.Builder
	inStr, esc := false, false
	var cur strings.Builder
	for _, c := range raw {
		if inStr {
			if esc {
				cur.WriteByte(c)
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == ')' {
				inStr = false
				s := cur.String()
				if len(s) > 2 {
					sb.WriteString(s)
					sb.WriteByte(' ')
				}
				cur.Reset()
			} else {
				cur.WriteByte(c)
			}
		} else if c == '(' {
			inStr = true
		}
		if sb.Len() > 60000 {
			break
		}
	}
	if sb.Len() < 100 {
		return fmt.Sprintf("[binary PDF, %d bytes — paste text manually]", len(raw))
	}
	return sb.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

var _ = os.Getenv
