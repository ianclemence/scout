package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/ianclemence/scout/pkg/redact"
)

// CheckApproval enforces the permission model: external and financial tools
// run only with an explicit approved action. Drafts and reads never need it.
func (c *Core) CheckApproval(tool *Tool, args map[string]any) (string, error) {
	if tool.Permission != PermExternal && tool.Permission != PermFinancial {
		return "", nil
	}
	raw, _ := args["approval_id"].(string)
	if raw == "" {
		return "", fmt.Errorf("tool %q requires approval_id: request approval first, then retry with the approved id", tool.Name)
	}
	id := c.resolveActionID(raw)
	var status, actionType, target, risk string
	err := c.DB.DB.QueryRow(`SELECT status,action_type,target,risk_level FROM pending_actions WHERE id=?`, id).
		Scan(&status, &actionType, &target, &risk)
	if err != nil {
		return "", fmt.Errorf("approval %q not found", raw)
	}
	if status != "approved" {
		return "", fmt.Errorf("approval %s is %s, not approved", id, status)
	}
	if tool.Permission == PermFinancial && risk != "high" {
		// Financial actions always ride high-risk approvals.
		return "", fmt.Errorf("financial actions require a high-risk approval")
	}
	_ = actionType
	_ = target
	return id, nil
}

// Audit records a tool invocation for traceability. Summaries pass through
// secret redaction: credentials must never reach persistent text.
func (c *Core) Audit(tool *Tool, source, oppID, approvalID string, success bool, summary string) {
	ok := 0
	if success {
		ok = 1
	}
	summary = redact.Text(summary)
	if len(summary) > 500 {
		summary = summary[:500]
	}
	_, _ = c.DB.DB.Exec(`INSERT INTO tool_audit(id,created_at,tool,source,opportunity_id,permission,approved_action_id,success,summary) VALUES(?,?,?,?,?,?,?,?,?)`,
		newID("audit"), now(), tool.Name, source, oppID, string(tool.Permission), approvalID, ok, summary)
}

// Execute runs a tool with timeout, permission enforcement, and auditing.
func (c *Core) Execute(ctx context.Context, tool *Tool, args map[string]any) (string, error) {
	approvalID, err := c.CheckApproval(tool, args)
	if err != nil {
		c.Audit(tool, strArg(args, "source"), strArg(args, "opportunity_id"), "", false, err.Error())
		return "", err
	}
	timeout := tool.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	resCh := make(chan struct {
		s string
		e error
	}, 1)
	go func() {
		s, e := tool.Handler(tctx, args)
		resCh <- struct {
			s string
			e error
		}{s, e}
	}()
	select {
	case <-tctx.Done():
		c.Audit(tool, strArg(args, "source"), strArg(args, "opportunity_id"), approvalID, false, "timeout")
		return "", fmt.Errorf("tool %q timed out", tool.Name)
	case r := <-resCh:
		c.Audit(tool, strArg(args, "source"), strArg(args, "opportunity_id"), approvalID, r.e == nil, summarize(r.s, r.e))
		return r.s, r.e
	}
}

func strArg(args map[string]any, k string) string {
	if v, ok := args[k].(string); ok {
		return v
	}
	return ""
}

func summarize(s string, err error) string {
	if err != nil {
		return err.Error()
	}
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}
