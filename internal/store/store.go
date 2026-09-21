// Package store provides SQLite persistence with versioned migrations.
// Uses modernc.org/sqlite (pure Go, no cgo) so the Pi needs no dev headers.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type Store struct {
	DB *sql.DB
}

func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dbPath+"?cache=shared")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;`); err != nil {
		return nil, err
	}
	s := &Store{DB: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) migrate() error {
	var v int
	if err := s.DB.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return err
	}
	for _, m := range migrations {
		if m.version > v {
			tx, err := s.DB.Begin()
			if err != nil {
				return err
			}
			if _, err := tx.Exec(m.sql); err != nil {
				tx.Rollback()
				return fmt.Errorf("migration %d: %w", m.version, err)
			}
			if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version=%d`, m.version)); err != nil {
				tx.Rollback()
				return err
			}
			if err := tx.Commit(); err != nil {
				return err
			}
			v = m.version
		}
	}
	return nil
}

type migration struct {
	version int
	sql     string
}

var migrations = []migration{
	{1, `
CREATE TABLE IF NOT EXISTS profile (id TEXT PRIMARY KEY, data TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS evidence (id TEXT PRIMARY KEY, kind TEXT, reference TEXT, content TEXT, source TEXT, created_at TEXT);
CREATE TABLE IF NOT EXISTS preferences (key TEXT PRIMARY KEY, value TEXT, reason TEXT, updated_at TEXT);
CREATE TABLE IF NOT EXISTS clients (id TEXT PRIMARY KEY, source TEXT, source_client_id TEXT, display_name TEXT, rating REAL, total_hires INTEGER, total_spend REAL, country TEXT, history_note TEXT);
CREATE TABLE IF NOT EXISTS opportunities (
  id TEXT PRIMARY KEY, source TEXT, source_opp_id TEXT, canonical_url TEXT, title TEXT, description TEXT,
  skills TEXT, category TEXT, budget_min REAL, budget_max REAL, budget_type TEXT,
  hourly_min REAL, hourly_max REAL, connects_cost INTEGER, posted_at TEXT,
  fingerprint TEXT, client_id TEXT, raw_snapshot TEXT, status TEXT, created_at TEXT, updated_at TEXT,
  UNIQUE(source, source_opp_id)
);
CREATE INDEX IF NOT EXISTS idx_opp_status ON opportunities(status);
CREATE INDEX IF NOT EXISTS idx_opp_fp ON opportunities(fingerprint);
CREATE TABLE IF NOT EXISTS evaluations (id TEXT PRIMARY KEY, opportunity_id TEXT, data TEXT NOT NULL, created_at TEXT);
CREATE TABLE IF NOT EXISTS proposals (id TEXT PRIMARY KEY, opportunity_id TEXT, application_id TEXT, cover_letter TEXT, rate REAL, rate_type TEXT, duration_est TEXT, evidence_ids TEXT, questions TEXT, status TEXT, created_at TEXT);
CREATE TABLE IF NOT EXISTS applications (id TEXT PRIMARY KEY, opportunity_id TEXT, source TEXT, stage TEXT, cost_connects INTEGER, submitted_at TEXT, updated_at TEXT);
CREATE TABLE IF NOT EXISTS messages (id TEXT PRIMARY KEY, source TEXT, thread_id TEXT, from_party TEXT, body TEXT, created_at TEXT);
CREATE TABLE IF NOT EXISTS agent_runs (id TEXT PRIMARY KEY, kind TEXT, status TEXT, summary TEXT, dry_run INTEGER, started_at TEXT, ended_at TEXT);
CREATE TABLE IF NOT EXISTS pending_actions (id TEXT PRIMARY KEY, source TEXT, action_type TEXT, target TEXT, payload TEXT, risk_level TEXT, status TEXT, created_at TEXT, decided_at TEXT);
CREATE TABLE IF NOT EXISTS events (id TEXT PRIMARY KEY, kind TEXT, detail TEXT, created_at TEXT);
CREATE TABLE IF NOT EXISTS sources (id TEXT PRIMARY KEY, name TEXT, kind TEXT, endpoint TEXT, enabled INTEGER, capabilities TEXT);
CREATE TABLE IF NOT EXISTS secrets (key TEXT PRIMARY KEY, value BLOB NOT NULL, updated_at TEXT);
CREATE TABLE IF NOT EXISTS app_settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS users (id TEXT PRIMARY KEY, password_hash TEXT NOT NULL, created_at TEXT);
CREATE TABLE IF NOT EXISTS feedback (id TEXT PRIMARY KEY, opportunity_id TEXT, signal TEXT, note TEXT, created_at TEXT);
`},
	{2, `
CREATE TABLE IF NOT EXISTS sessions (id TEXT PRIMARY KEY, name TEXT, provider TEXT, model TEXT, created_at TEXT, updated_at TEXT);
CREATE TABLE IF NOT EXISTS session_messages (session_id TEXT, idx INTEGER, role TEXT, content TEXT, PRIMARY KEY(session_id, idx));
CREATE INDEX IF NOT EXISTS idx_sess_updated ON sessions(updated_at);
`},
}
