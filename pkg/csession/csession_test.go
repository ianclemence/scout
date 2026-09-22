package csession

import (
	"path/filepath"
	"testing"

	"github.com/ianclemence/scout/pkg/store"
)

func testDB(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSessionRoundtrip(t *testing.T) {
	db := testDB(t)
	s, err := Create(db, "work", "ollama", "qwen3:0.6b")
	if err != nil {
		t.Fatal(err)
	}
	if err := AppendMessages(db, s.ID, []Message{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "hello"}}); err != nil {
		t.Fatal(err)
	}
	msgs, err := LoadMessages(db, s.ID, 50)
	if err != nil || len(msgs) != 2 || msgs[0].Content != "hi" {
		t.Fatalf("bad messages: %v %v", msgs, err)
	}
	got, err := Resolve(db, s.ID[:8])
	if err != nil || got.ID != s.ID {
		t.Fatal("prefix resolve failed")
	}
	list, _ := List(db)
	if len(list) != 1 {
		t.Fatal("expected 1 session")
	}
	if err := ReplaceTail(db, s.ID, 1, "summary"); err != nil {
		t.Fatal(err)
	}
	msgs, _ = LoadMessages(db, s.ID, 50)
	if len(msgs) != 2 {
		t.Fatalf("expected compacted pair, got %d", len(msgs))
	}
}

func TestRename(t *testing.T) {
	db := testDB(t)
	s, _ := Create(db, "old", "ollama", "m")
	if err := Rename(db, s.ID, "new-name"); err != nil {
		t.Fatal(err)
	}
	got, _ := Get(db, s.ID)
	if got.Name != "new-name" {
		t.Fatal("rename failed")
	}
	if err := Rename(db, s.ID, "  "); err == nil {
		t.Fatal("blank rename should fail")
	}
}

// Empty sessions are never tracked: List excludes rows with no messages,
// and DeleteEmpty removes only those rows.
func TestListHidesEmptySessions(t *testing.T) {
	db := testDB(t)
	full, err := Create(db, "work", "ollama", "qwen3:0.6b")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Create(db, "interactive", "ollama", "qwen3:0.6b"); err != nil {
		t.Fatal(err)
	}
	if err := AppendMessages(db, full.ID, []Message{{Role: "user", Content: "hi"}}); err != nil {
		t.Fatal(err)
	}
	list, err := List(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != full.ID {
		t.Fatalf("List must show only the messaged session, got %v", list)
	}
	// Direct resolve by id still reaches the empty row (hiding is not loss).
	if _, err := Resolve(db, full.ID); err != nil {
		t.Fatalf("resolve must keep working: %v", err)
	}
}

func TestDeleteEmptyRemovesOnlyEmpties(t *testing.T) {
	db := testDB(t)
	full, _ := Create(db, "work", "ollama", "qwen3:0.6b")
	empty, _ := Create(db, "interactive", "ollama", "qwen3:0.6b")
	_ = AppendMessages(db, full.ID, []Message{{Role: "user", Content: "hi"}})
	n, err := DeleteEmpty(db)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("DeleteEmpty must remove exactly the empty row, removed %d", n)
	}
	if _, err := Get(db, full.ID); err != nil {
		t.Fatalf("content session must survive: %v", err)
	}
	if _, err := Get(db, empty.ID); err == nil {
		t.Fatal("empty session must be gone")
	}
}
