package sources

import (
	"context"
	"fmt"
	"strings"

	"github.com/ianclemence/scout/pkg/domain"
	"github.com/ianclemence/scout/pkg/mcpclient"
)

// mcpCaller is the subset of the MCP client the adapters use. Using an
// interface keeps the adapters testable with a fake and lets any MCP transport
// plug in.
type mcpCaller interface {
	ListTools(ctx context.Context) ([]mcpclient.ToolInfo, error)
	CallTool(ctx context.Context, name string, args map[string]any) (string, error)
}

// MCPAdapter exposes a generic MCP server as an OpportunitySource.
// Capabilities come from live tool discovery; unknown tool shapes are
// reported as unsupported rather than guessed into fake success.
type MCPAdapter struct {
	IDValue   string
	NameValue string
	Conn      mcpCaller
	caps      []Capability
	tools     []mcpclient.ToolInfo
}

func NewMCPAdapter(id, name string, conn mcpCaller) *MCPAdapter {
	return &MCPAdapter{IDValue: id, NameValue: name, Conn: conn}
}

func (m *MCPAdapter) ID() string   { return m.IDValue }
func (m *MCPAdapter) Name() string { return m.NameValue }

func (m *MCPAdapter) Capabilities() []Capability { return m.caps }

func (m *MCPAdapter) Has(c Capability) bool { return Has(m.caps, c) }

// Discover refreshes capabilities from the server's tool list.
func (m *MCPAdapter) Discover(ctx context.Context) error {
	tools, err := m.Conn.ListTools(ctx)
	if err != nil {
		return err
	}
	m.tools = tools
	m.caps = mapTools(tools)
	return nil
}

func mapTools(tools []mcpclient.ToolInfo) []Capability {
	set := map[Capability]bool{}
	for _, t := range tools {
		n := strings.ToLower(t.Name + " " + t.Description)
		has := func(ss ...string) bool {
			for _, s := range ss {
				if strings.Contains(n, s) {
					return true
				}
			}
			return false
		}
		switch {
		case has("search job", "find job", "job search", "search_job", "search gig", "search work"):
			set[CapSearch] = true
		case has("job detail", "get job", "read job", "job_detail", "get listing"):
			set[CapReadListing] = true
		case has("freelancer", "talent", "client", "company"):
			set[CapResearch] = true
		// Submit is matched before draft: "submit_proposal" contains
		// "proposal" but is a submit action, and the stronger intent wins.
		case has("submit", "apply", "send proposal"):
			set[CapSubmit] = true
		case has("proposal", "draft", "cover letter"):
			set[CapDraft] = true
		case has("message", "thread", "conversation"):
			set[CapMessage] = true
		case has("contract", "offer", "earning"):
			set[CapContracts] = true
		}
	}
	var out []Capability
	for c := range set {
		out = append(out, c)
	}
	return out
}

func (m *MCPAdapter) findTool(want Capability) (string, bool) {
	// Reuse the same keyword order as mapTools by scanning tools.
	cands := map[Capability][]string{
		CapSearch:      {"search job", "find job", "job search", "search_job", "search"},
		CapReadListing: {"job detail", "get job", "read job", "job_detail"},
		CapResearch:    {"client", "company", "freelancer", "talent"},
		CapDraft:       {"draft", "cover letter", "proposal"},
		CapSubmit:      {"submit", "apply", "send proposal"},
		CapMessage:     {"message", "thread"},
		CapContracts:   {"contract", "offer"},
	}
	for _, t := range m.tools {
		n := strings.ToLower(t.Name + " " + t.Description)
		for _, kw := range cands[want] {
			if strings.Contains(n, kw) {
				// A tool matching a stronger capability belongs to that one;
				// never hand a submit tool to the draft path.
				if want != CapSubmit && strings.Contains(n, "submit") && hasCapabilityKeyword(n, cands[CapSubmit]) {
					continue
				}
				return t.Name, true
			}
		}
	}
	return "", false
}

func hasCapabilityKeyword(hay string, keywords []string) bool {
	for _, k := range keywords {
		if strings.Contains(hay, k) {
			return true
		}
	}
	return false
}

func (m *MCPAdapter) Search(ctx context.Context, f SearchFilter) ([]domain.Opportunity, error) {
	if len(m.tools) == 0 {
		if err := m.Discover(ctx); err != nil {
			return nil, err
		}
	}
	name, ok := m.findTool(CapSearch)
	if !ok {
		return nil, fmt.Errorf("source %s does not expose search", m.NameValue)
	}
	raw, err := m.Conn.CallTool(ctx, name, map[string]any{
		"query": f.Query, "skills": f.Skills, "limit": f.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("source %s search failed: %w", m.NameValue, err)
	}
	// Best-effort normalization is the adapter's job; unparseable payloads
	// are errors, never silent empty success.
	opps, err := NormalizeSearchPayload(m.IDValue, []byte(raw))
	if err != nil {
		return nil, fmt.Errorf("source %s returned unparseable search payload: %w", m.NameValue, err)
	}
	return opps, nil
}

func (m *MCPAdapter) Get(ctx context.Context, sourceID string) (*domain.Opportunity, error) {
	if len(m.tools) == 0 {
		if err := m.Discover(ctx); err != nil {
			return nil, err
		}
	}
	name, ok := m.findTool(CapReadListing)
	if !ok {
		return nil, fmt.Errorf("source %s does not expose listing retrieval", m.NameValue)
	}
	raw, err := m.Conn.CallTool(ctx, name, map[string]any{"id": sourceID, "job_id": sourceID})
	if err != nil {
		return nil, err
	}
	return NormalizeListingPayload(m.IDValue, sourceID, []byte(raw))
}

func (m *MCPAdapter) Status(ctx context.Context, sourceID string) (string, error) {
	o, err := m.Get(ctx, sourceID)
	if err != nil {
		return "unknown", err
	}
	if o.LiveStatus != "" {
		return o.LiveStatus, nil
	}
	return "unknown", nil
}

func (m *MCPAdapter) Health(ctx context.Context) Health {
	if err := m.Discover(ctx); err != nil {
		msg := err.Error()
		if strings.Contains(msg, "401") || strings.Contains(msg, "Unauthorized") {
			return Health{State: "unauthenticated", Detail: "authentication required"}
		}
		return Health{State: "unavailable", Detail: msg}
	}
	return Health{State: "connected", Detail: fmt.Sprintf("%d tools", len(m.tools))}
}

// Tools returns the discovered MCP tools (discovering once if needed).
func (m *MCPAdapter) Tools(ctx context.Context) ([]mcpclient.ToolInfo, error) {
	if len(m.tools) == 0 {
		if err := m.Discover(ctx); err != nil {
			return nil, err
		}
	}
	return m.tools, nil
}

// SubmitApplication calls the source's discovered submit tool with the given
// arguments. It runs only after Core has verified an approved action; the
// adapter is the last mile, not a policy surface. A source without a submit
// tool reports that truthfully instead of pretending success.
func (m *MCPAdapter) SubmitApplication(ctx context.Context, args map[string]any) (string, error) {
	if len(m.tools) == 0 {
		if err := m.Discover(ctx); err != nil {
			return "", err
		}
	}
	name, ok := m.findTool(CapSubmit)
	if !ok {
		return "", fmt.Errorf("source %s does not expose application submission", m.NameValue)
	}
	return m.Conn.CallTool(ctx, name, args)
}

// SendMessage calls the source's discovered messaging tool.
func (m *MCPAdapter) SendMessage(ctx context.Context, args map[string]any) (string, error) {
	if len(m.tools) == 0 {
		if err := m.Discover(ctx); err != nil {
			return "", err
		}
	}
	name, ok := m.findTool(CapMessage)
	if !ok {
		return "", fmt.Errorf("source %s does not expose messaging", m.NameValue)
	}
	return m.Conn.CallTool(ctx, name, args)
}

func (m *MCPAdapter) Close() error { return nil }
