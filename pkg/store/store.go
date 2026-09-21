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

// GetSetting reads a value from app_settings. ok is false when the key is absent.
func (s *Store) GetSetting(key string) (value string, ok bool, err error) {
	row := s.DB.QueryRow(`SELECT value FROM app_settings WHERE key=?`, key)
	switch err = row.Scan(&value); err {
	case nil:
		return value, true, nil
	case sql.ErrNoRows:
		return "", false, nil
	default:
		return "", false, err
	}
}

// SetSetting upserts a value into app_settings.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.DB.Exec(`INSERT INTO app_settings(key, value) VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// DeleteSetting removes a key from app_settings (missing keys are a no-op).
func (s *Store) DeleteSetting(key string) error {
	_, err := s.DB.Exec(`DELETE FROM app_settings WHERE key=?`, key)
	return err
}

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
	{3, `
CREATE TABLE IF NOT EXISTS models_cache (provider TEXT, id TEXT, display_name TEXT, context_window INTEGER, reasoning TEXT, tools INTEGER, vision INTEGER, source TEXT, updated_at TEXT, PRIMARY KEY(provider, id));
`},
	{4, `
ALTER TABLE sessions ADD COLUMN thinking TEXT DEFAULT '';
ALTER TABLE sources ADD COLUMN command TEXT DEFAULT '';
ALTER TABLE sources ADD COLUMN env TEXT DEFAULT '';
`},
	{5, `
ALTER TABLE opportunities ADD COLUMN company TEXT DEFAULT '';
ALTER TABLE opportunities ADD COLUMN employment_type TEXT DEFAULT '';
ALTER TABLE opportunities ADD COLUMN engagement_type TEXT DEFAULT '';
ALTER TABLE opportunities ADD COLUMN location TEXT DEFAULT '';
ALTER TABLE opportunities ADD COLUMN remote_status TEXT DEFAULT '';
ALTER TABLE opportunities ADD COLUMN technologies TEXT DEFAULT '';
ALTER TABLE opportunities ADD COLUMN requirements TEXT DEFAULT '';
ALTER TABLE opportunities ADD COLUMN currency TEXT DEFAULT '';
ALTER TABLE opportunities ADD COLUMN deadline TEXT DEFAULT '';
ALTER TABLE opportunities ADD COLUMN provenance TEXT DEFAULT '';
ALTER TABLE opportunities ADD COLUMN live_status TEXT DEFAULT '';
ALTER TABLE opportunities ADD COLUMN source_url TEXT DEFAULT '';
CREATE TABLE IF NOT EXISTS research_cache (key TEXT PRIMARY KEY, kind TEXT, data TEXT, sources TEXT, created_at TEXT);
CREATE TABLE IF NOT EXISTS tool_audit (id TEXT PRIMARY KEY, created_at TEXT, tool TEXT, source TEXT, opportunity_id TEXT, permission TEXT, approved_action_id TEXT, success INTEGER, summary TEXT);
CREATE TABLE IF NOT EXISTS learned_observations (id TEXT PRIMARY KEY, pattern TEXT, signal TEXT, created_at TEXT);
`},
	{6, `
CREATE TABLE IF NOT EXISTS trajectories (id TEXT PRIMARY KEY, created_at TEXT, request TEXT, tools TEXT, turns INTEGER, final TEXT, error TEXT);
`},
	{7, `
-- tool_results keeps the FULL result of a tool call, separately from the
-- truncated tool_audit.summary. Evaluation (grounding checks) needs the real
-- evidence, not a 500-char preview; keeping it here bounds growth (retention is
-- applied on write) without changing runtime behaviour.
CREATE TABLE IF NOT EXISTS tool_results (id TEXT PRIMARY KEY, created_at TEXT, tool TEXT, result TEXT);
CREATE INDEX IF NOT EXISTS idx_tool_results_created ON tool_results(created_at);
`},
	{8, `
-- run_id ties a tool result to the agent turn that produced it, so an external
-- evaluator matches evidence to the turn exactly instead of by timestamp.
ALTER TABLE tool_results ADD COLUMN run_id TEXT DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_tool_results_run ON tool_results(run_id);
`},
}
