package runtime

import (
	"context"

	"github.com/ianclemence/scout/pkg/agent"
	"github.com/ianclemence/scout/pkg/llm"
)

// RunScripted drives one agent turn with a caller-supplied provider and reports
// each tool the agent chose. It exists so the evaluation harness can assert
// agent-process behavior (bounded tool use, recovery, a final answer) without
// a live model.
func (c *Core) RunScripted(ctx context.Context, p llm.Provider, onTool func(name string)) (string, error) {
	return c.RunAgent(ctx, &agent.Engine{LLM: p}, []llm.Message{{Role: "user", Content: "eval turn"}}, "", func(ev Event) {
		if ev.Type == "tool_start" {
			onTool(ev.Name)
		}
	})
}
