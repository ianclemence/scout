package tui

import (
	"regexp"
	"sort"
	"strings"
)

// This file ports the fuzzy matcher used by the Pi coding agent's selectors:
// every query rune must appear in order in the text, and matches are scored so
// the best results sort first (consecutive and word-boundary matches win).

var (
	fuzzyAN = regexp.MustCompile(`^([a-z]+)([0-9]+)$`)
	fuzzyNA = regexp.MustCompile(`^([0-9]+)([a-z]+)$`)
	fuzzyWS = regexp.MustCompile(`[\s/]+`)
)

// fuzzyMatch reports whether query matches text in order, with a score
// (lower is better).
func fuzzyMatch(query, text string) (bool, float64) {
	q := strings.ToLower(query)
	t := strings.ToLower(text)
	score, ok := fuzzyScore(q, t)
	if ok {
		return true, score
	}
	// Swap an alphanumeric tail/head (e.g. "haiku45" vs "45haiku").
	if m := fuzzyAN.FindStringSubmatch(q); m != nil {
		if s, ok := fuzzyScore(m[2]+m[1], t); ok {
			return true, s + 5
		}
	} else if m := fuzzyNA.FindStringSubmatch(q); m != nil {
		if s, ok := fuzzyScore(m[2]+m[1], t); ok {
			return true, s + 5
		}
	}
	return false, 0
}

func fuzzyScore(q, t string) (float64, bool) {
	if q == "" {
		return 0, true
	}
	if len(q) > len(t) {
		return 0, false
	}
	qi, last, consecutive := 0, -1, 0
	var score float64
	for qi < len(q) {
		i := strings.IndexByte(t[last+1:], q[qi])
		if i < 0 {
			break
		}
		i += last + 1
		boundary := i == 0 || strings.IndexByte(" \t-_./:", t[i-1]) >= 0
		if last == i-1 {
			consecutive++
			score -= float64(consecutive) * 5
		} else {
			consecutive = 0
			if last >= 0 {
				score += float64(i-last-1) * 2
			}
		}
		if boundary {
			score -= 10
		}
		score += float64(i) * 0.1
		last = i
		qi++
	}
	if qi < len(q) {
		return 0, false
	}
	if q == t {
		score -= 100
	}
	return score, true
}

// fuzzyFilter returns items whose search text matches every whitespace- or
// slash-separated query token, best matches first.
func fuzzyFilter[T any](items []T, query string, text func(T) string) []T {
	if strings.TrimSpace(query) == "" {
		return items
	}
	tokens := fuzzyWS.Split(strings.TrimSpace(query), -1)
	type scored struct {
		item  T
		score float64
	}
	var results []scored
	for _, it := range items {
		hay := text(it)
		var total float64
		all := true
		for _, tok := range tokens {
			if tok == "" {
				continue
			}
			ok, s := fuzzyMatch(tok, hay)
			if !ok {
				all = false
				break
			}
			total += s
		}
		if all {
			results = append(results, scored{it, total})
		}
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].score < results[j].score })
	out := make([]T, len(results))
	for i, r := range results {
		out[i] = r.item
	}
	return out
}
