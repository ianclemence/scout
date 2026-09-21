// Package runtime — connector surface.
//
// A "connection" is one configured work source: an MCP endpoint (remote
// Streamable HTTP) or a local stdio MCP command. This file is the single
// implementation behind three interfaces:
//
//   - the `scout integrations` CLI
//   - the /sources slash command and picker in the session
//   - the agent's list_sources / get_source_capabilities / source_health tools
//
// Keeping it here means the CLI, session, and agent can never disagree about
// what is configured or what a source can do.
package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ianclemence/scout/pkg/domain"
	"github.com/ianclemence/scout/pkg/mcpclient"
	"github.com/ianclemence/scout/pkg/sources"
)

// Connection describes a configured work source and its live state. This is
// the shape every interface renders; capability names are the domain
// vocabulary so CLI, session, and agent speak one language.
type Connection struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Kind         string   `json:"kind"` // mcp | mcp-stdio
	Endpoint     string   `json:"endpoint,omitempty"`
	Command      string   `json:"command,omitempty"`
	Enabled      bool     `json:"enabled"`
	Auth         string   `json:"auth"`                   // authenticated | token_stored | unauthenticated | unknown
	HasToken     bool     `json:"has_token"`              // a credential is stored for this source
	Capabilities []string `json:"capabilities,omitempty"` // domain.Capability values, if known
	ToolCount    int      `json:"tool_count,omitempty"`   // discovered MCP tools
	Status       string   `json:"status"`                 // connected | unauthenticated | unavailable | unknown
	Detail       string   `json:"detail,omitempty"`
}

// Connections lists configured work sources with their stored-token state.
// It never probes the network: this is the fast, always-safe listing used by
// the session startup and the CLI. Use ProbeConnection/ProbeAll to test.
func (c *Core) Connections() ([]Connection, error) {
	srcs, err := c.ListSources()
	if err != nil {
		return nil, err
	}
	tokens := c.storedMCPTokens()
	out := make([]Connection, 0, len(srcs))
	for _, s := range srcs {
		conn := Connection{
			ID:       s.ID,
			Name:     s.Name,
			Kind:     s.Kind,
			Endpoint: s.Endpoint,
			Command:  s.Command,
			Enabled:  s.Enabled,
		}
		conn.Auth = "unauthenticated"
		if tokens[s.Name] {
			conn.HasToken = true
			conn.Auth = "token_stored"
		}
		if len(s.Capabilities) > 0 {
			for _, cap := range s.Capabilities {
				conn.Capabilities = append(conn.Capabilities, string(cap))
			}
			sort.Strings(conn.Capabilities)
		}
		out = append(out, conn)
	}
	return out, nil
}

// FindConnection returns one configured source by id, name, or case-insensitive
// prefix match, so users can type `upwork` instead of `src-upwork`.
func (c *Core) FindConnection(ref string) (*Connection, error) {
	conns, err := c.Connections()
	if err != nil {
		return nil, err
	}
	norm := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
	r := norm(ref)
	for i := range conns {
		if norm(conns[i].ID) == r || norm(conns[i].Name) == r {
			return &conns[i], nil
		}
	}
	for i := range conns {
		if strings.HasPrefix(norm(conns[i].ID), r) || strings.HasPrefix(norm(conns[i].Name), r) {
			return &conns[i], nil
		}
	}
	return nil, fmt.Errorf("no work source matches %q — /sources lists configured sources", ref)
}

// storedMCPTokens returns the set of source names that have an encrypted MCP
// token. One query; callers never open a cursor while reading secrets.
func (c *Core) storedMCPTokens() map[string]bool {
	out := map[string]bool{}
	rows, err := c.DB.DB.Query(`SELECT key FROM secrets WHERE key LIKE 'mcp:%'`)
	if err != nil {
		return out
	}
	var keys []string
	for rows.Next() {
		var k string
		if rows.Scan(&k) == nil {
			keys = append(keys, k)
		}
	}
	rows.Close()
	for _, k := range keys {
		out[strings.TrimPrefix(k, "mcp:")] = true
	}
	return out
}

// ProbeConnection performs read-only capability discovery against one source
// and returns the updated connection. Bounded by timeout; a dead endpoint
// reports unavailable, never hangs.
func (c *Core) ProbeConnection(ctx context.Context, ref string, timeout time.Duration) (*Connection, error) {
	conns, err := c.Connections()
	if err != nil {
		return nil, err
	}
	var target *Connection
	norm := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
	r := norm(ref)
	for i := range conns {
		if norm(conns[i].ID) == r || norm(conns[i].Name) == r ||
			strings.HasPrefix(norm(conns[i].ID), r) || strings.HasPrefix(norm(conns[i].Name), r) {
			target = &conns[i]
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("no work source matches %q", ref)
	}
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	pctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	tok, _ := c.LoadSecret("mcp:" + target.Name)
	mc := &mcpclient.Connector{ID: target.Name, Endpoint: target.Endpoint, Token: tok}
	if target.Kind == "mcp-stdio" {
		mc.Command = strings.Fields(target.Command)
	}
	// The MCP SDK does not always observe context cancellation during the
	// dial phase (a black-holed endpoint can block a raw TCP connect). Bound
	// the probe with a hard deadline so a dead source can never hang a turn.
	type probeResult struct {
		tools []mcpclient.ToolInfo
		err   error
	}
	resCh := make(chan probeResult, 1)
	go func() {
		tools, err := mc.ListTools(pctx)
		resCh <- probeResult{tools, err}
	}()
	var tools []mcpclient.ToolInfo
	var probeErr error
	select {
	case <-pctx.Done():
		probeErr = fmt.Errorf("probe timed out after %s", timeout)
	case r := <-resCh:
		tools, probeErr = r.tools, r.err
	}
	if probeErr != nil {
		msg := probeErr.Error()
		target.Status = "unavailable"
		target.Detail = msg
		if strings.Contains(msg, "401") || strings.Contains(strings.ToLower(msg), "unauthoriz") ||
			strings.Contains(strings.ToLower(msg), "authentication") {
			target.Status = "unauthenticated"
			target.Detail = "authentication required — /sources token " + target.Name
			if target.HasToken {
				target.Auth = "token_rejected"
			}
		}
		return target, nil
	}
	target.Status = "connected"
	target.ToolCount = len(tools)
	if target.HasToken {
		target.Auth = "authenticated"
	} else {
		target.Auth = "open"
	}
	caps := discoverCapabilities(tools)
	target.Detail = fmt.Sprintf("%d tools", len(tools))
	// Replace, never merge: capabilities reflect the live tool list. A
	// capability that disappears from the source must disappear here too.
	target.Capabilities = nil
	for _, cap := range caps {
		target.Capabilities = append(target.Capabilities, string(cap))
	}
	sort.Strings(target.Capabilities)
	// Cache capabilities on the row so later listings (and the agent) are
	// accurate without re-probing.
	if b, err := jsonCapabilities(caps); err == nil {
		_, _ = c.DB.DB.Exec(`UPDATE sources SET capabilities=? WHERE id=?`, b, target.ID)
	}
	return target, nil
}

// ProbeAll probes every enabled source, isolating per-source failure.
func (c *Core) ProbeAll(ctx context.Context, timeout time.Duration) ([]Connection, error) {
	conns, err := c.Connections()
	if err != nil {
		return nil, err
	}
	for i := range conns {
		if !conns[i].Enabled {
			conns[i].Status = "disabled"
			continue
		}
		p, err := c.ProbeConnection(ctx, conns[i].ID, timeout)
		if err != nil {
			conns[i].Status = "unavailable"
			conns[i].Detail = err.Error()
			continue
		}
		conns[i] = *p
	}
	return conns, nil
}

// AddMCPConnection registers a remote MCP endpoint.
func (c *Core) AddMCPConnection(name, endpoint string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("connection name is required")
	}
	if !strings.HasPrefix(endpoint, "https://") && !strings.HasPrefix(endpoint, "http://localhost") && !strings.HasPrefix(endpoint, "http://127.0.0.1") {
		return fmt.Errorf("endpoint must be https (or localhost http)")
	}
	id := connectionID(name)
	_, err := c.DB.DB.Exec(`INSERT OR REPLACE INTO sources(id,name,kind,endpoint,command,enabled,capabilities) VALUES(?,?,?,?,?,1,'')`,
		id, name, "mcp", endpoint, "")
	return err
}

// AddStdioConnection registers a local stdio MCP command.
func (c *Core) AddStdioConnection(name, command string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("connection name is required")
	}
	if strings.TrimSpace(command) == "" {
		return fmt.Errorf("command is required")
	}
	id := connectionID(name)
	_, err := c.DB.DB.Exec(`INSERT OR REPLACE INTO sources(id,name,kind,endpoint,command,enabled,capabilities) VALUES(?,?,?,?,?,1,'')`,
		id, name, "mcp-stdio", "", command)
	return err
}

// SetConnectionEnabled enables or disables a source.
func (c *Core) SetConnectionEnabled(ref string, enabled bool) error {
	conn, err := c.FindConnection(ref)
	if err != nil {
		return err
	}
	_, err = c.DB.DB.Exec(`UPDATE sources SET enabled=? WHERE id=?`, boolToInt(enabled), conn.ID)
	return err
}

// RemoveConnection deletes a configured source and its stored token.
func (c *Core) RemoveConnection(ref string) error {
	conn, err := c.FindConnection(ref)
	if err != nil {
		return err
	}
	_, _ = c.DB.DB.Exec(`DELETE FROM secrets WHERE key=?`, "mcp:"+conn.Name)
	_, err = c.DB.DB.Exec(`DELETE FROM sources WHERE id=?`, conn.ID)
	return err
}

// StoreConnectionToken stores an encrypted MCP access token.
func (c *Core) StoreConnectionToken(ref, token string) error {
	conn, err := c.FindConnection(ref)
	if err != nil {
		return err
	}
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("empty token")
	}
	return c.SaveSecret("mcp:"+conn.Name, token)
}

func connectionID(name string) string {
	return "src-" + strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), " ", "-"))
}

// normalizeSourceID maps an opportunity's stored source value ("upwork",
// "manual") to a registry id ("src-upwork", "local").
func normalizeSourceID(source string) string {
	s := strings.TrimSpace(source)
	if s == "manual" || s == "" {
		return "local"
	}
	if strings.HasPrefix(s, "src-") {
		return s
	}
	return "src-" + s
}

// discoverCapabilities maps discovered MCP tools to the domain capability
// vocabulary. It is the single mapping used by the CLI, session, and agent.
// The keyword sets mirror sources.mapTools; the returned values are the
// domain.* constants the rest of Scout reasons about.
func discoverCapabilities(tools []mcpclient.ToolInfo) []domain.Capability {
	set := map[domain.Capability]bool{}
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
			set[domain.CapSearchOpportunities] = true
		case has("job detail", "get job", "read job", "job_detail", "get listing"):
			set[domain.CapReadOpportunity] = true
		case has("freelancer", "talent", "client", "company"):
			set[domain.CapSearchClients] = true
		// Submit/apply is checked before draft/proposal: "submit_proposal"
		// contains both words, and the more consequential intent must win.
		case has("submit", "apply", "send proposal"):
			set[domain.CapSubmitApplication] = true
		case has("proposal", "draft", "cover letter"):
			set[domain.CapDraftApplication] = true
		case has("message", "thread", "conversation"):
			set[domain.CapReadMessages] = true
		case has("contract", "offer", "earning"):
			set[domain.CapReadContract] = true
		}
	}
	var out []domain.Capability
	for c := range set {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func jsonCapabilities(caps []domain.Capability) (string, error) {
	b, err := json.Marshal(caps)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CapabilityLabels renders domain capability values as short human labels.
func CapabilityLabels(caps []string) string {
	if len(caps) == 0 {
		return "none discovered"
	}
	short := map[string]string{
		string(domain.CapSearchOpportunities): "search",
		string(domain.CapReadOpportunity):     "read",
		string(domain.CapSearchClients):       "clients",
		string(domain.CapReadMessages):        "messages",
		string(domain.CapDraftApplication):    "draft",
		string(domain.CapSubmitApplication):   "submit",
		string(domain.CapReadContract):        "contracts",
		string(domain.CapReadEarnings):        "earnings",
	}
	var out []string
	for _, c := range caps {
		if s, ok := short[c]; ok {
			out = append(out, s)
		} else {
			out = append(out, c)
		}
	}
	return strings.Join(out, ", ")
}

// MCPAdapterFor builds a live adapter for a configured source, used by tools
// that need to call the source rather than describe it.
func (c *Core) MCPAdapterFor(id string) (*sources.MCPAdapter, bool) {
	s, ok := c.SourceRegistry().Get(id)
	if !ok {
		return nil, false
	}
	a, ok := s.(*sources.MCPAdapter)
	return a, ok
}

// findSource resolves a source reference (id, name, or case-insensitive
// prefix) against a registry, so callers can say "upwork" and get
// "src-upwork". It returns false only when nothing matches.
func (c *Core) findSource(reg *sources.Registry, ref string) (sources.OpportunitySource, bool) {
	if s, ok := reg.Get(ref); ok {
		return s, true
	}
	if conn, err := c.FindConnection(ref); err == nil {
		if s, ok := reg.Get(conn.ID); ok {
			return s, true
		}
	}
	return nil, false
}
