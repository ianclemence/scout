// Package mcpclient connects Scout to external MCP servers (Upwork first).
// Uses the official Go SDK (modelcontextprotocol/go-sdk). Remote servers use
// Streamable HTTP; OAuth tokens are supplied via Authorization header and are
// never logged.
package mcpclient

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ianclemence/scout/pkg/version"
)

type Connector struct {
	ID       string
	Endpoint string
	Token    string // OAuth access token, redacted everywhere
	// Command, when set, runs a local stdio MCP server instead of remote HTTP.
	Command []string
	Env     []string
}

func (c *Connector) transport() mcp.Transport {
	if len(c.Command) > 0 {
		cmd := exec.Command(c.Command[0], c.Command[1:]...)
		cmd.Env = append(os.Environ(), c.Env...)
		return &mcp.CommandTransport{Command: cmd}
	}
	t := &mcp.StreamableClientTransport{Endpoint: c.Endpoint}
	if c.Token != "" {
		t.HTTPClient = &http.Client{Transport: &authRoundTripper{token: c.Token}}
	}
	return t
}

type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (c *Connector) ListTools(ctx context.Context) ([]ToolInfo, error) {
	client := mcp.NewClient(&mcp.Implementation{Name: "scout", Version: version.Version}, nil)
	sess, err := client.Connect(ctx, c.transport(), nil)
	if err != nil {
		return nil, fmt.Errorf("mcp connect %s: %w", c.Endpoint, err)
	}
	defer sess.Close()
	res, err := sess.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp list tools: %w", err)
	}
	var out []ToolInfo
	for _, tl := range res.Tools {
		out = append(out, ToolInfo{Name: tl.Name, Description: tl.Description})
	}
	return out, nil
}

func (c *Connector) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	client := mcp.NewClient(&mcp.Implementation{Name: "scout", Version: version.Version}, nil)
	sess, err := client.Connect(ctx, c.transport(), nil)
	if err != nil {
		return "", fmt.Errorf("mcp connect: %w", err)
	}
	defer sess.Close()
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return "", fmt.Errorf("mcp call %s: %w", name, err)
	}
	out := ""
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			out += tc.Text
		}
	}
	return out, nil
}

type authRoundTripper struct {
	token string
	base  http.RoundTripper
}

func (a *authRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.Header.Set("Authorization", "Bearer "+a.token)
	base := a.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(r2)
}
