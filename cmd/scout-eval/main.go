// Command scout-eval is Scout's EXTERNAL evaluation harness. It is a separate
// binary, deliberately not part of `scout`: Scout never depends on it, and it
// never runs Jev inside Scout's runtime.
//
// It reads what Scout already records (trajectories + tool_audit + evaluations)
// from the real database, runs deterministic checks and grounded Jev questions,
// prints findings, and stores them so a later run can be compared.
//
// Usage:
//
//	scout-eval run [--limit N] [--version V]   evaluate recent real trajectories
//	scout-eval compare <before.json> <after.json>
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ianclemence/scout/pkg/config"
	"github.com/ianclemence/scout/pkg/jeveval"
	"github.com/ianclemence/scout/pkg/store"
	"github.com/ianclemence/scout/pkg/version"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "run":
		must(run(os.Args[2:]))
	case "compare":
		if len(os.Args) < 4 {
			usage()
		}
		must(compare(os.Args[2], os.Args[3]))
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `scout-eval — external Jev evaluation of Scout

  scout-eval run [--limit N] [--version V] [--out FILE]
  scout-eval compare <before.json> <after.json>

Requires TYPESAFE_API_KEY. Reads Scout's real database; never writes to it.`)
	os.Exit(2)
}

func openDB() (*store.Store, error) {
	cfg := config.Load()
	return store.Open(cfg.DBPath)
}

func run(args []string) error {
	limit, out, ver := 10, "", version.Version
	var requestLike string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--limit":
			i++
			if i < len(args) {
				limit, _ = strconv.Atoi(args[i])
			}
		case "--version":
			i++
			if i < len(args) {
				ver = args[i]
			}
		case "--out":
			i++
			if i < len(args) {
				out = args[i]
			}
		case "--request":
			i++
			if i < len(args) {
				requestLike = args[i]
			}
		}
	}
	client, err := jeveval.NewClient()
	if err != nil {
		return err
	}
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	trajs, err := jeveval.LoadTrajectories(db, limit)
	if err != nil {
		return err
	}
	// Skip the garbage rows the test suite writes ('t'/'f').
	var real []jeveval.Trajectory
	for _, t := range trajs {
		if len(t.Request) <= 3 || len(t.Final) <= 20 {
			continue
		}
		if requestLike != "" && !strings.Contains(t.Request, requestLike) {
			continue
		}
		real = append(real, t)
	}
	if len(real) == 0 {
		return fmt.Errorf("no real trajectories found — run some Scout turns first")
	}

	fmt.Printf("Evaluating %d real trajectory(ies) with Jev (Scout %s)…\n\n", len(real), ver)
	findings := make([]jeveval.Finding, 0, len(real))
	var sumQuality, sumUnsup, sumAnswered float64
	var n int
	for _, t := range real {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		f := jeveval.Evaluate(ctx, client, t)
		cancel()
		f.Version = ver
		findings = append(findings, f)
		fmt.Print(f.Summary())
		fmt.Println()
		if f.JevError == "" {
			sumQuality += f.Jev["decision_quality"].Score
			sumUnsup += f.Jev["unsupported_claims"].Noul
			sumAnswered += f.Jev["answered_request"].Noul
			n++
		}
	}
	if n > 0 {
		fmt.Printf("Averages over %d evaluated: quality=%.2f unsupported=%.2f answered=%.2f\n",
			n, sumQuality/float64(n), sumUnsup/float64(n), sumAnswered/float64(n))
	}
	if out != "" {
		blob, _ := json.MarshalIndent(map[string]any{
			"version": ver, "evaluated_at": time.Now().UTC(), "findings": findings,
		}, "", "  ")
		if err := os.WriteFile(out, blob, 0o644); err != nil {
			return err
		}
		fmt.Printf("\nWrote findings to %s\n", out)
	}
	return nil
}

func compare(beforePath, afterPath string) error {
	load := func(p string) (map[string]any, []jeveval.Finding, error) {
		var raw struct {
			Version  string            `json:"version"`
			Findings []jeveval.Finding `json:"findings"`
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, nil, err
		}
		if err := json.Unmarshal(b, &raw); err != nil {
			return nil, nil, err
		}
		return map[string]any{"version": raw.Version}, raw.Findings, nil
	}
	bm, bf, err := load(beforePath)
	if err != nil {
		return err
	}
	_, af, err := load(afterPath)
	if err != nil {
		return err
	}
	avg := func(fs []jeveval.Finding) (q, u, a float64, n int) {
		for _, f := range fs {
			if f.JevError != "" {
				continue
			}
			q += f.Jev["decision_quality"].Score
			u += f.Jev["unsupported_claims"].Noul
			a += f.Jev["answered_request"].Noul
			n++
		}
		if n > 0 {
			q, u, a = q/float64(n), u/float64(n), a/float64(n)
		}
		return
	}
	bq, bu, ba, bn := avg(bf)
	aq, au, aa, an := avg(af)
	fmt.Printf("Before (%v, n=%d): quality=%.2f unsupported=%.2f answered=%.2f\n", bm["version"], bn, bq, bu, ba)
	fmt.Printf("After  (%v, n=%d): quality=%.2f unsupported=%.2f answered=%.2f\n", "current", an, aq, au, aa)
	fmt.Println()
	fmt.Printf("Δquality=%+.2f  Δunsupported=%+.2f (lower better)  Δanswered=%+.2f\n", aq-bq, au-bu, aa-ba)
	return nil
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
