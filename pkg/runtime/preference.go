package runtime

import (
	"time"

	"github.com/ianclemence/scout/pkg/preference"
)

// PreferenceModel builds Scout's learned preference model from the user's
// explicit feedback. It returns nil before any labelled feedback exists, so it
// costs nothing until there is data. Explicit profile constraints still win;
// this only adjusts deterministic evaluations and informs the model context.
func (c *Core) PreferenceModel() *preference.Model {
	rows, err := c.DB.DB.Query(`SELECT f.signal, COALESCE(f.note,''), COALESCE(o.title,''), COALESCE(o.skills,'')
		FROM feedback f LEFT JOIN opportunities o ON o.id = f.opportunity_id`)
	if err != nil {
		return nil
	}
	type row struct{ signal, note, title, skills string }
	var rws []row
	for rows.Next() {
		var r row
		if rows.Scan(&r.signal, &r.note, &r.title, &r.skills) == nil {
			rws = append(rws, r)
		}
	}
	rows.Close()
	var samples []preference.Sample
	for _, r := range rws {
		pol := preference.SignalPolarity(r.signal)
		if pol == 0 {
			continue
		}
		terms := append([]string{}, splitCSV(r.skills)...)
		terms = append(terms, preference.NoteTerms(r.title)...)
		terms = append(terms, preference.NoteTerms(r.note)...)
		samples = append(samples, preference.Sample{Terms: terms, Positive: pol > 0})
	}
	if len(samples) == 0 {
		return nil
	}
	return preference.Train(samples)
}

// FeedbackSummary reports the raw feedback rows for transparency.
func (c *Core) FeedbackSummary(limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := c.DB.DB.Query(`SELECT f.id, f.opportunity_id, f.signal, COALESCE(f.note,''), f.created_at
		FROM feedback f ORDER BY f.created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, oid, sig, note, at string
		rows.Scan(&id, &oid, &sig, &note, &at)
		out = append(out, map[string]any{"id": id, "opportunity_id": oid, "signal": sig, "note": note, "at": at})
	}
	return out, nil
}

var _ = time.Now
