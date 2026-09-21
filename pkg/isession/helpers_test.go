package isession

import (
	"path/filepath"
	"testing"

	"github.com/ianclemence/scout/pkg/config"
	"github.com/ianclemence/scout/pkg/runtime"
	"github.com/ianclemence/scout/pkg/store"
)

func testCore(t *testing.T) *runtime.Core {
	t.Helper()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	db, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	core, err := runtime.New(cfg, db)
	if err != nil {
		t.Fatal(err)
	}
	return core
}

// configureProvider stores a credential so the provider counts as configured.
func configureProvider(t *testing.T, core *runtime.Core, provider string) {
	t.Helper()
	if err := core.SaveSecret("llm:"+provider, "test-key"); err != nil {
		t.Fatal(err)
	}
}
