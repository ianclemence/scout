// Package eval is Scout's evaluation harness.
//
// It answers two questions that unit tests do not:
//
//  1. Does Scout make the *right decision* on representative opportunities
//     (strong, weak, partial, ambiguous, suspicious, keyword traps, budget
//     floors, learned preferences)?
//  2. Does the *agent process* behave — bounded tool use, recovery from bad
//     tool choices, a final answer — rather than only whether a single
//     function returned the expected value?
//
// Cases are hermetic and adapt to the user's real profile, so the same suite
// runs in CI and against a live install.
package eval

import (
	"context"
	"fmt"
	"strings"

	"github.com/ianclemence/scout/pkg/domain"
	"github.com/ianclemence/scout/pkg/llm"
	"github.com/ianclemence/scout/pkg/match"
	"github.com/ianclemence/scout/pkg/preference"
)

// Case is one expected decision.
type Case struct {
	Name           string
	Profile        *domain.ProfessionalProfile
	Opportunity    domain.Opportunity
	Feedback       []preference.Sample
	WantFilterPass *bool
	WantRecommend  string
	WantSkills     string
	WantRisk       []string
}

// Result is the outcome of running one case.
type Result struct {
	Name     string
	Pass     bool
	Failures []string
	Observed map[string]string
}

// Run evaluates the deterministic decision path (filter → heuristic → learned
// preference) for every case.
func Run(cases []Case) []Result {
	var out []Result
	for _, c := range cases {
		p := c.Profile
		if p == nil {
			p = &domain.ProfessionalProfile{}
		}
		o := c.Opportunity
		f := match.DeterministicFilter(p, &o)
		ev := match.HeuristicEvaluate(p, &o)
		if len(c.Feedback) > 0 {
			preference.Train(c.Feedback).Adjust(ev, &o)
		}
		r := Result{Name: c.Name, Pass: true, Observed: map[string]string{
			"filter":         fmt.Sprintf("%v", f.Pass),
			"recommendation": ev.Recommendation,
			"skills":         dimRating(ev, "skills"),
			"risks":          fmt.Sprintf("%v", ev.Risks),
		}}
		fail := func(msg string) {
			r.Pass = false
			r.Failures = append(r.Failures, msg)
		}
		if c.WantFilterPass != nil && f.Pass != *c.WantFilterPass {
			fail(fmt.Sprintf("filter pass = %v, want %v (%s)", f.Pass, *c.WantFilterPass, f.Reason))
		}
		if c.WantRecommend != "" && ev.Recommendation != c.WantRecommend {
			fail(fmt.Sprintf("recommendation = %q, want %q", ev.Recommendation, c.WantRecommend))
		}
		if c.WantSkills != "" && dimRating(ev, "skills") != c.WantSkills {
			fail(fmt.Sprintf("skills rating = %q, want %q", dimRating(ev, "skills"), c.WantSkills))
		}
		for _, want := range c.WantRisk {
			if !containsSubstring(ev.Risks, want) {
				fail(fmt.Sprintf("missing risk containing %q (have %v)", want, ev.Risks))
			}
		}
		out = append(out, r)
	}
	return out
}

func dimRating(ev *domain.MatchEvaluation, name string) string {
	for _, d := range ev.Dimensions {
		if d.Name == name {
			return d.Rating
		}
	}
	return ""
}

func containsSubstring(xs []string, sub string) bool {
	for _, x := range xs {
		if strings.Contains(x, sub) {
			return true
		}
	}
	return false
}

// ---------- trajectory evaluation ----------

// TrajectoryCase scripts the model's turns and asserts the agent loop's
// behavior — the process, not just the answer.
type TrajectoryCase struct {
	Name         string
	Turns        []string // scripted model outputs; the last repeats
	WantTools    []string // tools that must have been called
	MaxToolCalls int
	WantFinal    string // substring the final answer must contain
}

// RunTrajectory drives Core.RunAgent with a scripted provider.
func RunTrajectory(c TrajectoryRunner, cases []TrajectoryCase) []Result {
	var out []Result
	for _, tc := range cases {
		if len(tc.Turns) == 0 {
			continue
		}
		fake := &scriptedProvider{turns: tc.Turns}
		var tools []string
		final, err := c.RunScripted(context.Background(), fake, func(name string) {
			tools = append(tools, name)
		})
		r := Result{Name: tc.Name, Pass: true, Observed: map[string]string{
			"tools": fmt.Sprintf("%v", tools),
			"final": truncate(final, 60),
		}}
		fail := func(msg string) {
			r.Pass = false
			r.Failures = append(r.Failures, msg)
		}
		if err != nil {
			fail("agent error: " + err.Error())
		}
		for _, want := range tc.WantTools {
			if !contains(tools, want) {
				fail(fmt.Sprintf("tool %q not called (called %v)", want, tools))
			}
		}
		if tc.MaxToolCalls > 0 && len(tools) > tc.MaxToolCalls {
			fail(fmt.Sprintf("used %d tool calls, max %d", len(tools), tc.MaxToolCalls))
		}
		if tc.WantFinal != "" && !strings.Contains(final, tc.WantFinal) {
			fail(fmt.Sprintf("final answer %q missing %q", final, tc.WantFinal))
		}
		out = append(out, r)
	}
	return out
}

// TrajectoryRunner abstracts Core so eval can drive the agent loop without an
// import cycle (runtime implements it structurally).
type TrajectoryRunner interface {
	RunScripted(ctx context.Context, p llm.Provider, onTool func(name string)) (string, error)
}

type scriptedProvider struct {
	turns []string
	n     int
}

func (s *scriptedProvider) Name() string { return "scripted" }
func (s *scriptedProvider) Complete(llm.Request) (string, error) {
	return s.turns[len(s.turns)-1], nil
}
func (s *scriptedProvider) Stream(_ context.Context, _ llm.Request, emit func(string) error) error {
	t := s.turns[s.n]
	if s.n < len(s.turns)-1 {
		s.n++
	}
	return emit(t)
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
