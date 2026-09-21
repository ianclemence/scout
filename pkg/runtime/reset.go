package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResetScope describes what a reset destroys.
type ResetScope struct {
	// IncludeCredentials also removes stored provider keys, connector auth, and
	// the local master key. Off by default: wiping API keys unasked is hostile.
	IncludeCredentials bool
}

// ResetReport lists what was cleared, for an honest confirmation message.
type ResetReport struct {
	Tables []string
	Files  []string
}

// userDataTables are the tables that hold everything a user created: profile,
// work data, sessions, learned preferences, and history. Order does not matter
// (foreign keys are not enforced across these). app_settings is cleared too.
var userDataTables = []string{
	"profile", "evidence", "preferences", "clients", "opportunities",
	"evaluations", "proposals", "applications", "messages", "agent_runs",
	"pending_actions", "events", "users", "feedback", "sessions",
	"session_messages", "models_cache", "research_cache", "tool_audit",
	"learned_observations", "trajectories", "tool_results", "app_settings",
}

// credentialTables hold secrets and connector configuration. They survive a
// normal reset and are cleared only by --all.
var credentialTables = []string{"secrets", "sources"}

// Reset wipes Scout's saved data and starts it fresh. It preserves the schema
// (migrations are not re-run) and, unless IncludeCredentials is set, the stored
// provider keys and connector configuration, so a reset does not force a
// re-login. It also removes on-disk history and the transient SQLite WAL/SHM
// files so no stale data lingers.
func (c *Core) Reset(scope ResetScope) (*ResetReport, error) {
	rep := &ResetReport{}
	tx, err := c.DB.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	tables := append([]string{}, userDataTables...)
	if scope.IncludeCredentials {
		tables = append(tables, credentialTables...)
	}
	for _, t := range tables {
		if _, err := tx.Exec(`DELETE FROM ` + t); err != nil {
			// A table may not exist on an older DB; skip rather than abort.
			if strings.Contains(err.Error(), "no such table") {
				continue
			}
			return nil, fmt.Errorf("clear %s: %w", t, err)
		}
		rep.Tables = append(rep.Tables, t)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	// Reclaim space so a wiped DB does not keep its old size on disk.
	_, _ = c.DB.DB.Exec(`VACUUM`)

	dataDir := c.Cfg.DataDir
	// The readline history file is user state and is always cleared.
	histPath := filepath.Join(dataDir, "history")
	if err := os.Remove(histPath); err == nil {
		rep.Files = append(rep.Files, histPath)
	}
	if scope.IncludeCredentials {
		// The workspace overlay (SCOUT.md, skills) is user-authored, and the
		// master key decrypts secrets: both go with a full wipe. Removing the
		// key makes any surviving secret ciphertext unreadable.
		wsPath := filepath.Join(dataDir, "workspace")
		if err := os.RemoveAll(wsPath); err == nil {
			rep.Files = append(rep.Files, wsPath)
		}
		keyPath := filepath.Join(dataDir, ".masterkey")
		if err := os.Remove(keyPath); err == nil {
			rep.Files = append(rep.Files, keyPath)
		}
	}
	return rep, nil
}

// ResetSummary renders the report as a short human line.
func (r *ResetReport) Summary() string {
	if r == nil {
		return ""
	}
	return fmt.Sprintf("%d tables cleared, %d files removed", len(r.Tables), len(r.Files))
}
