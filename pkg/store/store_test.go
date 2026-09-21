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
	if v != 6 {
		t.Fatalf("expected user_version=6, got %d", v)
	}
	for _, tbl := range []string{"opportunities", "proposals", "pending_actions", "sources", "secrets", "users", "sessions", "session_messages", "models_cache"} {
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

func TestSettingsRoundTrip(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, ok, err := db.GetSetting("missing"); err != nil || ok {
		t.Fatalf("missing key: ok=%v err=%v", ok, err)
	}
	if err := db.SetSetting("scoped_models", `["a/b"]`); err != nil {
		t.Fatal(err)
	}
	v, ok, err := db.GetSetting("scoped_models")
	if err != nil || !ok || v != `["a/b"]` {
		t.Fatalf("get after set: %q ok=%v err=%v", v, ok, err)
	}
	// Upsert overwrites.
	if err := db.SetSetting("scoped_models", `["c/d"]`); err != nil {
		t.Fatal(err)
	}
	if v, _, _ := db.GetSetting("scoped_models"); v != `["c/d"]` {
		t.Fatalf("upsert failed: %q", v)
	}
	if err := db.DeleteSetting("scoped_models"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := db.GetSetting("scoped_models"); ok {
		t.Fatal("delete failed")
	}
}
