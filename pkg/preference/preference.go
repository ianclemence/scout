// Package preference turns Scout's explicit feedback into a small, explainable
// model of what the user reacts well and poorly to.
//
// This is Scout's first real learning layer. It does not train the LLM: it
// learns term-level preferences from the user's own signals (good_match,
// bad_match, too_low_budget, …) and the reasons they wrote, then uses them to
// adjust deterministic evaluations and to inform the model's context. It is
// deliberately simple, inspectable, and correctable: explicit preferences
// always override it, and the user can see exactly what was learned.
package preference

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/ianclemence/scout/pkg/domain"
)

// Sample is one labelled example: the terms present in an opportunity (and the
// user's note) and whether the user reacted positively.
type Sample struct {
	Terms    []string
	Positive bool
}

// Model is a learned term-weight model.
type Model struct {
	// Weights maps a term to its polarity in [-1, 1] (positive = favored).
	Weights map[string]float64
	// Positive/Negative are the number of positive/negative samples seen.
	Positive int
	Negative int
	// Seen is the number of distinct terms observed.
	Seen int
}

// SignalPolarity maps a feedback signal to +1 (favored), -1 (disfavored), or 0
// (neutral / unknown signal, ignored).
func SignalPolarity(signal string) int {
	switch strings.ToLower(strings.TrimSpace(signal)) {
	case "good_match":
		return 1
	case "bad_match", "too_low_budget", "unclear_scope", "bad_client", "already_applied":
		return -1
	default:
		return 0
	}
}

// Train builds a model from labelled samples. The weight is a Laplace-smoothed
// polarity, so a term seen once does not dominate and a term seen many times
// consistently moves strongly.
func Train(samples []Sample) *Model {
	pos := map[string]int{}
	neg := map[string]int{}
	m := &Model{Weights: map[string]float64{}}
	for _, s := range samples {
		if s.Positive {
			m.Positive++
		} else {
			m.Negative++
		}
		seen := map[string]bool{}
		for _, t := range s.Terms {
			t = normalizeTerm(t)
			if t == "" || seen[t] {
				continue
			}
			seen[t] = true
			if s.Positive {
				pos[t]++
			} else {
				neg[t]++
			}
		}
	}
	for t := range unionKeys(pos, neg) {
		w := float64(pos[t]-neg[t]) / float64(pos[t]+neg[t]+2)
		if math.Abs(w) < 0.001 {
			continue
		}
		m.Weights[t] = w
	}
	m.Seen = len(m.Weights)
	return m
}

// Score returns the evidence-weighted preference score for an opportunity:
// the sum of learned term weights present in it. Positive means "like work
// you've favored", negative means "like work you've rejected". Accumulating
// (rather than averaging) lets stronger evidence accumulate as more feedback
// arrives; a single sample moves the needle only a little.
func (m *Model) Score(o *domain.Opportunity) float64 {
	if m == nil || len(m.Weights) == 0 {
		return 0
	}
	var score float64
	for _, t := range Terms(o) {
		if w, ok := m.Weights[normalizeTerm(t)]; ok {
			score += w
		}
	}
	return score
}

// Matched returns the learned terms present in an opportunity, with their
// weights, sorted by absolute weight (strongest first).
type MatchedTerm struct {
	Term   string
	Weight float64
}

func (m *Model) Matched(o *domain.Opportunity) []MatchedTerm {
	if m == nil || len(m.Weights) == 0 {
		return nil
	}
	var out []MatchedTerm
	seen := map[string]bool{}
	for _, t := range Terms(o) {
		t = normalizeTerm(t)
		if seen[t] {
			continue
		}
		seen[t] = true
		if w, ok := m.Weights[t]; ok {
			out = append(out, MatchedTerm{t, w})
		}
	}
	sort.Slice(out, func(i, j int) bool { return math.Abs(out[i].Weight) > math.Abs(out[j].Weight) })
	return out
}

// Top returns the n strongest favored and disfavored terms.
func (m *Model) Top(n int) (favored, disfavored []string) {
	if m == nil {
		return nil, nil
	}
	type tw struct {
		t string
		w float64
	}
	var all []tw
	for t, w := range m.Weights {
		all = append(all, tw{t, w})
	}
	sort.Slice(all, func(i, j int) bool { return math.Abs(all[i].w) > math.Abs(all[j].w) })
	for _, x := range all {
		if x.w > 0 && len(favored) < n {
			favored = append(favored, x.t)
		} else if x.w < 0 && len(disfavored) < n {
			disfavored = append(disfavored, x.t)
		}
	}
	return favored, disfavored
}

// ContextLine renders a short, explicit block for the model context. It is
// empty when nothing has been learned, so it costs nothing before any feedback.
func (m *Model) ContextLine() string {
	if m == nil || len(m.Weights) == 0 {
		return ""
	}
	fav, dis := m.Top(6)
	var b strings.Builder
	b.WriteString("Learned preferences (from the user's explicit feedback; advisory — explicit profile constraints always win):")
	if len(fav) > 0 {
		b.WriteString(" favored terms: " + strings.Join(fav, ", ") + ".")
	}
	if len(dis) > 0 {
		b.WriteString(" disfavored terms: " + strings.Join(dis, ", ") + ".")
	}
	return b.String()
}

// Adjust folds the model into a deterministic evaluation. It adds an
// explainable dimension and shifts the recommendation when the evidence is
// strong enough. It never overrides explicit profile constraints (those are
// enforced by the deterministic filter before this runs).
func (m *Model) Adjust(ev *domain.MatchEvaluation, o *domain.Opportunity) {
	if m == nil || ev == nil || len(m.Weights) == 0 {
		return
	}
	matched := m.Matched(o)
	if len(matched) == 0 {
		return
	}
	score := m.Score(o)
	var parts []string
	for i, x := range matched {
		if i >= 4 {
			break
		}
		parts = append(parts, fmt.Sprintf("%s %+.2f", x.Term, x.Weight))
	}
	rating := "neutral"
	switch {
	case score <= -0.5:
		rating = "weak"
	case score >= 0.5:
		rating = "good"
	}
	ev.Dimensions = append(ev.Dimensions, domain.MatchDimension{
		Name: "learned_preference", Rating: rating,
		Detail: fmt.Sprintf("score %+.2f from %s", score, strings.Join(parts, ", ")),
	})
	switch {
	case score <= -0.5:
		ev.Recommendation = downgrade(ev.Recommendation)
		ev.Risks = append(ev.Risks, "matches work you've rejected before")
	case score >= 0.5:
		ev.Recommendation = upgrade(ev.Recommendation)
	}
}

func upgrade(rec string) string {
	if rec == "ignore" {
		return "review"
	}
	return "apply"
}

func downgrade(rec string) string {
	switch rec {
	case "apply":
		return "review"
	case "review":
		return "ignore"
	}
	return rec
}

// ---------- term extraction ----------

// stopwords are common words that carry no preference signal.
var stopwords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "you": true, "your": true,
	"our": true, "are": true, "this": true, "that": true, "from": true, "will": true,
	"have": true, "has": true, "not": true, "but": true, "all": true, "any": true,
	"can": true, "who": true, "what": true, "new": true, "work": true, "role": true,
	"job": true, "need": true, "looking": true, "must": true, "able": true, "using": true,
	"help": true, "into": true, "over": true, "they": true, "them": true, "their": true,
	"about": true, "would": true, "should": true, "could": true, "when": true, "where": true,
	"which": true, "than": true, "then": true, "also": true, "more": true, "most": true,
	"other": true, "some": true, "such": true, "only": true, "just": true, "very": true,
	"each": true, "does": true, "how": true, "why": true, "well": true, "make": true,
}

// Terms returns the candidate preference terms for an opportunity: its declared
// skills plus significant title words.
func Terms(o *domain.Opportunity) []string {
	if o == nil {
		return nil
	}
	var out []string
	out = append(out, o.Skills...)
	for _, w := range tokenize(o.Title) {
		out = append(out, w)
	}
	return out
}

// NoteTerms extracts candidate terms from a free-text feedback note.
func NoteTerms(note string) []string {
	return tokenize(note)
}

func tokenize(s string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '.' && r != '#' && r != '+' && r != '-'
	}) {
		w = strings.Trim(w, ".-+")
		if len(w) < 3 || stopwords[w] {
			continue
		}
		out = append(out, w)
	}
	return out
}

func normalizeTerm(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	t = strings.Trim(t, ".,;:()[]{}|/\\!?\"'")
	return t
}

func unionKeys(a, b map[string]int) map[string]struct{} {
	out := map[string]struct{}{}
	for k := range a {
		out[k] = struct{}{}
	}
	for k := range b {
		out[k] = struct{}{}
	}
	return out
}
