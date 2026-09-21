package approve

import (
	"path/filepath"
	"testing"

	"github.com/ianclemence/scout/pkg/store"
)

func testStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestApprovalLifecycle(t *testing.T) {
	db := testStore(t)
	a, err := Create(db, "manual", "submit_proposal", "opp-1", "payload", "high")
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != "pending_approval" {
		t.Fatal("expected pending_approval")
	}
	pending, _ := List(db, true)
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(pending))
	}
	if err := SetStatus(db, a.ID, "approved"); err != nil {
		t.Fatal(err)
	}
	pending, _ = List(db, true)
	if len(pending) != 0 {
		t.Fatal("expected 0 pending after approval")
	}
	if err := SetStatus(db, "nope", "approved"); err == nil {
		t.Fatal("expected not-found error")
	}
}
