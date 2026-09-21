package main

import (
	"fmt"
	"strings"

	"github.com/ianclemence/scout/pkg/runtime"
)

// feedbackSignals is the closed set of preference signals Scout records.
var feedbackSignals = []string{"good_match", "bad_match", "too_low_budget", "unclear_scope", "bad_client", "already_applied"}

// feedbackCmd records an explicit preference signal from the CLI.
func feedbackCmd(c *runtime.Core, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: scout feedback <opportunity-id> <%s> [note]", strings.Join(feedbackSignals, "|"))
	}
	ref := args[0]
	signal := strings.ToLower(args[1])
	valid := false
	for _, s := range feedbackSignals {
		if s == signal {
			valid = true
		}
	}
	if !valid {
		return fmt.Errorf("unknown signal %q — use one of: %s", signal, strings.Join(feedbackSignals, ", "))
	}
	o, err := c.GetOpportunity(ref)
	if err != nil {
		// Accept an id prefix like the session does.
		var id string
		if qerr := c.DB.DB.QueryRow(`SELECT id FROM opportunities WHERE id=? OR id LIKE ? ORDER BY updated_at DESC LIMIT 1`, ref, ref+"%").Scan(&id); qerr == nil {
			o, err = c.GetOpportunity(id)
		}
		if err != nil {
			return err
		}
	}
	note := ""
	if len(args) > 2 {
		note = strings.Join(args[2:], " ")
	}
	if err := c.AddFeedback(o.ID, signal, note); err != nil {
		return err
	}
	fmt.Printf("Recorded %s on %s. Scout learns from this.\n", signal, o.Title)
	return nil
}

// learnCmd shows what Scout has learned from explicit feedback.
func learnCmd(c *runtime.Core, args []string) error {
	pm := c.PreferenceModel()
	if pm == nil {
		fmt.Println("Nothing learned yet — no labelled feedback.")
		fmt.Println("Record some: scout feedback <opportunity-id> good_match|bad_match|too_low_budget|... [note]")
		return nil
	}
	fmt.Printf("Learned preference model — %d positive, %d negative samples, %d terms.\n",
		pm.Positive, pm.Negative, pm.Seen)
	fav, dis := pm.Top(12)
	if len(fav) > 0 {
		fmt.Printf("  favored:    %s\n", strings.Join(fav, ", "))
	}
	if len(dis) > 0 {
		fmt.Printf("  disfavored: %s\n", strings.Join(dis, ", "))
	}
	fmt.Println("\nThis is advisory. Explicit profile constraints (rates, exclusions) always win.")
	fmt.Println("It adjusts deterministic evaluations and informs the agent's context.")
	return nil
}
