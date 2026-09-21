// Package approve implements the human approval state machine.
package approve

import (
	"fmt"
	"time"

	"github.com/ianclemence/scout/internal/domain"
	"github.com/ianclemence/scout/internal/store"
)

func newID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

// Create inserts a pending action in pending_approval status and records
// the canonical lifecycle event (model output is never the record).
func Create(db *store.Store, source, actionType, target, payload, risk string) (*domain.PendingAction, error) {
	a := &domain.PendingAction{
		ID: newID("act"), Source: source, ActionType: actionType, Target: target,
		Payload: payload, RiskLevel: risk, Status: "pending_approval", CreatedAt: time.Now().UTC(),
	}
	_, err := db.DB.Exec(`INSERT INTO pending_actions(id,source,action_type,target,payload,risk_level,status,created_at) VALUES(?,?,?,?,?,?,?,?)`,
		a.ID, a.Source, a.ActionType, a.Target, a.Payload, a.RiskLevel, a.Status, a.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	event(db, "approval_requested", a.ID+" "+a.ActionType+" "+a.Target)
	return a, nil
}

func SetStatus(db *store.Store, id, status string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	r, err := db.DB.Exec(`UPDATE pending_actions SET status=?, decided_at=? WHERE id=?`, status, now, id)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return fmt.Errorf("action %s not found", id)
	}
	switch status {
	case "approved", "rejected", "executed", "failed", "cancelled":
		event(db, "approval_"+status, id)
	}
	return nil
}

func event(db *store.Store, kind, detail string) {
	_, _ = db.DB.Exec(`INSERT INTO events(id,kind,detail,created_at) VALUES(?,?,?,?)`,
		newID("evt"), kind, detail, time.Now().UTC().Format(time.RFC3339))
}

func List(db *store.Store, onlyPending bool) ([]domain.PendingAction, error) {
	q := `SELECT id,source,action_type,target,payload,risk_level,status,created_at FROM pending_actions ORDER BY created_at DESC LIMIT 200`
	if onlyPending {
		q = `SELECT id,source,action_type,target,payload,risk_level,status,created_at FROM pending_actions WHERE status IN ('draft','pending_approval') ORDER BY created_at DESC LIMIT 200`
	}
	rows, err := db.DB.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.PendingAction
	for rows.Next() {
		var a domain.PendingAction
		var ts string
		rows.Scan(&a.ID, &a.Source, &a.ActionType, &a.Target, &a.Payload, &a.RiskLevel, &a.Status, &ts)
		a.CreatedAt, _ = time.Parse(time.RFC3339, ts)
		out = append(out, a)
	}
	return out, nil
}
