package eval

import (
	"path/filepath"
	"testing"

	"github.com/ianclemence/scout/pkg/config"
	"github.com/ianclemence/scout/pkg/runtime"
	"github.com/ianclemence/scout/pkg/store"
)

func TestDecisionCases(t *testing.T) {
	fails := 0
	for _, r := range Run(DefaultCases(nil)) {
		if !r.Pass {
			fails++
			t.Errorf("%s FAILED: %v (observed %v)", r.Name, r.Failures, r.Observed)
		}
	}
	if fails > 0 {
		t.Fatalf("%d decision case(s) failed", fails)
	}
}

func TestTrajectoryCases(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	db, err := store.Open(filepath.Join(t.TempDir(), "eval.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	c, err := runtime.New(cfg, db)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range RunTrajectory(c, TrajectoryCases()) {
		if !r.Pass {
			t.Errorf("%s FAILED: %v (observed %v)", r.Name, r.Failures, r.Observed)
		}
	}
}
