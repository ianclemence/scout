package store

import (
	"path/filepath"
	"testing"
)

func TestOpenMigrates(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var v int
	if err := db.DB.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != 2 {
		t.Fatalf("expected user_version=2, got %d", v)
	}
	for _, tbl := range []string{"opportunities", "proposals", "pending_actions", "sources", "secrets", "users", "sessions", "session_messages"} {
		var n string
		if err := db.DB.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&n); err != nil {
			t.Fatalf("missing table %s", tbl)
		}
	}
}

func TestUniqueSourceOpp(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.DB.Exec(`INSERT INTO opportunities(id,source,source_opp_id,title,status,created_at,updated_at) VALUES('a','upwork','1','t','discovered','x','x')`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB.Exec(`INSERT INTO opportunities(id,source,source_opp_id,title,status,created_at,updated_at) VALUES('b','upwork','1','t','discovered','x','x')`); err == nil {
		t.Fatal("expected unique(source, source_opp_id) violation for dedup")
	}
}
