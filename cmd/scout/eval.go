package main

import (
	"fmt"

	"github.com/ianclemence/scout/pkg/eval"
	"github.com/ianclemence/scout/pkg/runtime"
)

// evalCmd runs Scout's evaluation suite against the real profile and exits
// non-zero on failure, so it can gate a release.
func evalCmd(c *runtime.Core, args []string) error {
	p, _ := c.Profile()
	all := eval.Run(eval.DefaultCases(p))
	all = append(all, eval.RunTrajectory(c, eval.TrajectoryCases())...)

	passed := 0
	for _, r := range all {
		status := "PASS"
		if !r.Pass {
			status = "FAIL"
		} else {
			passed++
		}
		fmt.Printf("[%s] %s\n", status, r.Name)
		for _, f := range r.Failures {
			fmt.Printf("       %s\n", f)
		}
	}
	fmt.Printf("\n%d/%d eval cases passed\n", passed, len(all))
	if passed < len(all) {
		return fmt.Errorf("%d eval case(s) failed", len(all)-passed)
	}
	return nil
}
