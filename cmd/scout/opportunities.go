// Command scout — terminal-native AI work acquisition agent.
// Bare `scout` enters the interactive session; subcommands are scriptable.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ianclemence/scout/pkg/runtime"
	"github.com/ianclemence/scout/pkg/version"
)

func statusCmd(c *runtime.Core) error {
	var o, p, a int
	_ = c.DB.DB.QueryRow(`SELECT COUNT(*) FROM opportunities`).Scan(&o)
	_ = c.DB.DB.QueryRow(`SELECT COUNT(*) FROM pending_actions WHERE status IN ('draft','pending_approval')`).Scan(&p)
	_ = c.DB.DB.QueryRow(`SELECT COUNT(*) FROM applications`).Scan(&a)
	pr, _ := c.Profile()
	fmt.Printf("scout %s · profile %s (%d skills) · opportunities=%d pending=%d applications=%d\n",
		version.Version, pr.DisplayName, len(pr.Skills), o, p, a)
	return nil
}

func discoverCmd(c *runtime.Core, args []string) error {
	dry := true
	for _, a := range args {
		if a == "--live" {
			dry = false
		}
	}
	s, err := c.RunDiscovery(dry)
	if err != nil {
		return err
	}
	fmt.Printf("discovered=%d candidates=%d dry_run=%v (no external writes)\n", s.Total, s.Candidates, s.DryRun)
	return nil
}

func oppsCmd(c *runtime.Core, args []string) error {
	opps, err := c.ListOpportunities(runtime.OpportunityFilter{Query: strings.Join(args, " "), Limit: 50})
	if err != nil {
		return err
	}
	for _, o := range opps {
		fmt.Printf("%s\t[%s] %s (%s)\n", o.ID, o.Source, o.Title, o.Status)
	}
	return nil
}

func resolveID(c *runtime.Core, ref string) (string, error) {
	var id string
	err := c.DB.DB.QueryRow(`SELECT id FROM opportunities WHERE id=? OR id LIKE ? ORDER BY updated_at DESC LIMIT 1`, ref, ref+"%").Scan(&id)
	if err != nil {
		return "", fmt.Errorf("opportunity %q not found", ref)
	}
	return id, nil
}

func oppCmd(c *runtime.Core, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: scout opportunity <show|add> ...")
	}
	switch args[0] {
	case "show":
		if len(args) < 2 {
			return fmt.Errorf("usage: scout opportunity show <id>")
		}
		id, err := resolveID(c, args[1])
		if err != nil {
			return err
		}
		o, err := c.GetOpportunity(id)
		if err != nil {
			return err
		}
		fmt.Printf("[%s] %s (%s)\nbudget %s %.0f–%.0f · credits %d\n\n%s\n", o.Source, o.Title, o.Status,
			o.BudgetType, o.BudgetMin, o.BudgetMax, o.ConnectsCost, o.Description)
		if ev, err := c.LatestEvaluation(id); err == nil {
			b, _ := json.MarshalIndent(ev, "", "  ")
			fmt.Printf("\n--- evaluation ---\n%s\n", b)
		}
		if pr, err := c.LatestProposal(id); err == nil {
			fmt.Printf("\n--- proposal (%s) ---\n%s\n", pr.Status, pr.CoverLetter)
		}
		return nil
	case "add":
		var title, descFile, skills string
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--title":
				i++
				if i < len(args) {
					title = args[i]
				}
			case "--description-file":
				i++
				if i < len(args) {
					descFile = args[i]
				}
			case "--skills":
				i++
				if i < len(args) {
					skills = args[i]
				}
			}
		}
		if descFile == "" {
			return fmt.Errorf("usage: scout opportunity add --title T --description-file F [--skills s]")
		}
		raw, err := os.ReadFile(descFile)
		if err != nil {
			return err
		}
		o, err := c.AddOpportunity(title, string(raw), skills)
		if err != nil {
			return err
		}
		fmt.Printf("added %s\n", o.ID)
		return nil
	default:
		return fmt.Errorf("usage: scout opportunity <show|add> ...")
	}
}

func analyzeCmd(c *runtime.Core, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: scout analyze <opp-id>")
	}
	id, err := resolveID(c, args[0])
	if err != nil {
		return err
	}
	ev, f, err := c.Analyze(context.Background(), id, c.EngineForRole("analysis"))
	if err != nil {
		return err
	}
	fmt.Printf("filter: pass=%v reason=%s\n", f.Pass, f.Reason)
	b, _ := json.MarshalIndent(ev, "", "  ")
	fmt.Println(string(b))
	return nil
}

func proposalCmd(c *runtime.Core, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: scout proposal <opp-id>")
	}
	id, err := resolveID(c, args[0])
	if err != nil {
		return err
	}
	pr, err := c.DraftProposal(context.Background(), id, c.EngineForRole("proposal"))
	if err != nil {
		return err
	}
	fmt.Println(pr.CoverLetter)
	fmt.Printf("\n[questions: %s]\n(draft %s saved, no external writes)\n", strings.Join(pr.Questions, " | "), pr.ID)
	return nil
}

func approvalsCmd(c *runtime.Core, args []string) error {
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "list", "pending":
		acts, err := c.PendingApprovals()
		if err != nil {
			return err
		}
		for _, a := range acts {
			fmt.Printf("%s\t%s\t%s\t%s\trisk=%s\n", a.ID, a.Status, a.ActionType, a.Target, a.RiskLevel)
		}
	case "approve", "reject":
		if len(args) < 2 {
			return fmt.Errorf("usage: scout approvals %s <id>", sub)
		}
		status := "approved"
		if sub == "reject" {
			status = "rejected"
		}
		return c.SetApprovalStatus(args[1], status)
	default:
		return fmt.Errorf("usage: scout approvals [list|approve|reject]")
	}
	return nil
}

func appsCmd(c *runtime.Core) error {
	apps, err := c.ListApplications(100)
	if err != nil {
		return err
	}
	for _, a := range apps {
		fmt.Printf("%s\t%s\t%s\t%s\tconnects=%d\n", a.ID, a.OpportunityID, a.Source, a.Stage, a.CostConnects)
	}
	pipe, _ := c.Pipeline()
	if len(pipe) > 0 {
		fmt.Printf("pipeline: %v\n", pipe)
	}
	return nil
}

func inboxCmd(c *runtime.Core) error {
	tool := c.FindTool("list_messages")
	out, err := tool.Handler(context.Background(), map[string]any{})
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}

func runCmd(c *runtime.Core, args []string) error {
	dry := true
	for _, a := range args[1:] {
		if a == "--live" {
			dry = false
		}
	}
	_ = args
	s, err := c.RunDiscovery(dry)
	if err != nil {
		return err
	}
	fmt.Printf("discovered=%d candidates=%d dry_run=%v (no external writes)\n", s.Total, s.Candidates, s.DryRun)
	return nil
}
