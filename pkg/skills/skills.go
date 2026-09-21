package skills

import (
	"embed"
	"sort"
	"strings"
)

//go:embed skilldata/*/SKILL.md
var skillFiles embed.FS

// Skill is a reusable agent workflow: behavior, not just a function.
type Skill struct {
	Name     string
	Triggers []string
	Summary  string
	Body     string
}

// Registry discovers embedded skills and selects relevant ones per request.
// Only selected skill bodies enter model context — never the whole library.
type Registry struct {
	skills []Skill
}

func Load() (*Registry, error) {
	entries, err := skillFiles.ReadDir("skilldata")
	if err != nil {
		return nil, err
	}
	r := &Registry{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		raw, err := skillFiles.ReadFile("skilldata/" + e.Name() + "/SKILL.md")
		if err != nil {
			continue
		}
		s := parseSkill(e.Name(), string(raw))
		if s.Name != "" {
			r.skills = append(r.skills, s)
		}
	}
	sort.Slice(r.skills, func(i, j int) bool { return r.skills[i].Name < r.skills[j].Name })
	return r, nil
}

func parseSkill(dir, raw string) Skill {
	name := dir
	body := strings.TrimSpace(raw)
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "# ") {
			name = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			break
		}
	}
	// Summary: first prose paragraph after the headers.
	summary := ""
	paras := strings.SplitN(body, "\n\n", 4)
	if len(paras) >= 3 {
		summary = strings.TrimSpace(paras[2])
		if i := strings.IndexByte(summary, '\n'); i >= 0 {
			summary = summary[:i]
		}
	}
	var triggers []string
	if i := strings.Index(raw, "Relevance:"); i >= 0 {
		rest := raw[i+len("Relevance:"):]
		if j := strings.IndexByte(rest, '\n'); j >= 0 {
			rest = rest[:j]
		}
		for _, t := range strings.Split(rest, ",") {
			t = strings.ToLower(strings.TrimSpace(t))
			t = strings.Trim(t, ".")
			if t != "" {
				triggers = append(triggers, t)
			}
		}
	}
	return Skill{Name: name, Triggers: triggers, Summary: summary, Body: strings.TrimSpace(raw)}
}

// List returns metadata for all skills (for /skills, docs).
func (r *Registry) List() []Skill { return r.skills }

// Select returns up to max skills whose triggers overlap the request text,
// scored by distinct trigger hits. Deterministic and testable.
func (r *Registry) Select(request string, max int) []Skill {
	low := strings.ToLower(request)
	type scored struct {
		s Skill
		n int
	}
	var out []scored
	for _, s := range r.skills {
		hits := map[string]bool{}
		for _, t := range s.Triggers {
			if strings.Contains(low, t) {
				hits[t] = true
			}
		}
		if len(hits) > 0 {
			out = append(out, scored{s, len(hits)})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].n != out[j].n {
			return out[i].n > out[j].n
		}
		return out[i].s.Name < out[j].s.Name
	})
	if max <= 0 || max > 3 {
		max = 3
	}
	var res []Skill
	for i, s := range out {
		if i >= max {
			break
		}
		res = append(res, s.s)
	}
	return res
}

// ContextBlock renders selected skills for system-context injection.
// Summaries only — full procedures load on demand via the load_skill tool,
// keeping every turn cheap on small local models.
func ContextBlock(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Relevant skills (call load_skill {\"name\": ...} for the full procedure before acting):\n")
	for _, s := range skills {
		sum := s.Summary
		if len(sum) > 300 {
			sum = sum[:300]
		}
		b.WriteString("- " + s.Name + ": " + sum + "\n")
	}
	return b.String()
}

// Find returns a skill by exact name.
func (r *Registry) Find(name string) (Skill, bool) {
	for _, s := range r.skills {
		if s.Name == name {
			return s, true
		}
	}
	return Skill{}, false
}
