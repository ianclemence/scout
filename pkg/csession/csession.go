// Package csession persists interactive agent sessions and their messages.
package csession

import (
	"fmt"
	"strings"
	"time"

	"github.com/ianclemence/scout/pkg/store"
)

type Session struct {
	ID        string
	Name      string
	Provider  string
	Model     string
	Thinking  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Message struct {
	Role    string // user, assistant, tool
	Content string
}

func newID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func Create(db *store.Store, name, provider, model string) (*Session, error) {
	s := &Session{ID: newID("sess"), Name: name, Provider: provider, Model: model,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	_, err := db.DB.Exec(`INSERT INTO sessions(id,name,provider,model,thinking,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`,
		s.ID, s.Name, s.Provider, s.Model, "", ts(s.CreatedAt), ts(s.UpdatedAt))
	return s, err
}

func Get(db *store.Store, id string) (*Session, error) {
	var s Session
	var c, u string
	err := db.DB.QueryRow(`SELECT id,name,provider,model,COALESCE(thinking,''),created_at,updated_at FROM sessions WHERE id=?`, id).
		Scan(&s.ID, &s.Name, &s.Provider, &s.Model, &s.Thinking, &c, &u)
	if err != nil {
		return nil, err
	}
	s.CreatedAt, _ = time.Parse(time.RFC3339, c)
	s.UpdatedAt, _ = time.Parse(time.RFC3339, u)
	return &s, nil
}

// Resolve accepts an id prefix or name.
func Resolve(db *store.Store, ref string) (*Session, error) {
	if s, err := Get(db, ref); err == nil {
		return s, nil
	}
	var id string
	err := db.DB.QueryRow(`SELECT id FROM sessions WHERE id LIKE ? OR name=? ORDER BY updated_at DESC LIMIT 1`, ref+"%", ref).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("session %q not found", ref)
	}
	return Get(db, id)
}

func List(db *store.Store) ([]Session, error) {
	rows, err := db.DB.Query(`SELECT id,name,provider,model,COALESCE(thinking,''),created_at,updated_at FROM sessions ORDER BY updated_at DESC LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var s Session
		var c, u string
		rows.Scan(&s.ID, &s.Name, &s.Provider, &s.Model, &s.Thinking, &c, &u)
		s.CreatedAt, _ = time.Parse(time.RFC3339, c)
		s.UpdatedAt, _ = time.Parse(time.RFC3339, u)
		out = append(out, s)
	}
	return out, nil
}

func Rename(db *store.Store, id, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("name required")
	}
	_, err := db.DB.Exec(`UPDATE sessions SET name=?, updated_at=? WHERE id=?`, name, ts(time.Now().UTC()), id)
	return err
}

func Touch(db *store.Store, id, provider, model string) {
	_, _ = db.DB.Exec(`UPDATE sessions SET updated_at=?, provider=?, model=? WHERE id=?`,
		ts(time.Now().UTC()), provider, model, id)
}

// SetThinking persists the session reasoning level.
func SetThinking(db *store.Store, id, level string) {
	_, _ = db.DB.Exec(`UPDATE sessions SET thinking=?, updated_at=? WHERE id=?`, level, ts(time.Now().UTC()), id)
}

func AppendMessages(db *store.Store, sessionID string, msgs []Message) error {
	var max int
	_ = db.DB.QueryRow(`SELECT COALESCE(MAX(idx),-1) FROM session_messages WHERE session_id=?`, sessionID).Scan(&max)
	for i, m := range msgs {
		if _, err := db.DB.Exec(`INSERT INTO session_messages(session_id,idx,role,content) VALUES(?,?,?,?)`,
			sessionID, max+1+i, m.Role, m.Content); err != nil {
			return err
		}
	}
	_, _ = db.DB.Exec(`UPDATE sessions SET updated_at=? WHERE id=?`, ts(time.Now().UTC()), sessionID)
	return nil
}

func LoadMessages(db *store.Store, sessionID string, limit int) ([]Message, error) {
	rows, err := db.DB.Query(`SELECT role,content FROM session_messages WHERE session_id=? ORDER BY idx DESC LIMIT ?`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rev []Message
	for rows.Next() {
		var m Message
		rows.Scan(&m.Role, &m.Content)
		rev = append(rev, m)
	}
	out := make([]Message, len(rev))
	for i, m := range rev {
		out[len(rev)-1-i] = m
	}
	return out, nil
}

// ReplaceTail drops the last n messages (used by compact).
func ReplaceTail(db *store.Store, sessionID string, keep int, summary string) error {
	var max int
	_ = db.DB.QueryRow(`SELECT COALESCE(MAX(idx),-1) FROM session_messages WHERE session_id=?`, sessionID).Scan(&max)
	if _, err := db.DB.Exec(`DELETE FROM session_messages WHERE session_id=? AND idx>=?`, sessionID, keep); err != nil {
		return err
	}
	_, err := db.DB.Exec(`INSERT INTO session_messages(session_id,idx,role,content) VALUES(?,?,?,?)`,
		sessionID, keep, "user", "[context compacted — summary]\n"+summary)
	return err
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }
