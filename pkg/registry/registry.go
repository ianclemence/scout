// Package registry is Scout's model catalog: built-in known models +
// provider-discovered models + locally discovered Ollama models + user
// custom models. Metadata is cached in SQLite; refresh hits provider list
// APIs where they exist; offline falls back to cache, then built-ins.
// Unknown fields stay "unknown" — never invented.
package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ianclemence/scout/pkg/store"
)

// ModelInfo describes one model. Zero values mean unknown.
type ModelInfo struct {
	Provider    string `json:"provider"`
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Context     int    `json:"context_window"` // 0 = unknown
	Reasoning   string `json:"reasoning"`      // none, optional, always, unknown
	Tools       bool   `json:"tools"`
	Vision      bool   `json:"vision"`
	Source      string `json:"source"` // builtin, discovered, custom, ollama
}

// Builtins are verified against current provider docs; curated, not exhaustive.
func Builtins() []ModelInfo {
	return []ModelInfo{
		{Provider: "deepseek", ID: "deepseek-flash", DisplayName: "DeepSeek Flash", Reasoning: "optional", Tools: true, Source: "builtin"},
		{Provider: "deepseek", ID: "deepseek-v4-pro", DisplayName: "DeepSeek V4 Pro", Reasoning: "optional", Tools: true, Source: "builtin"},
		{Provider: "moonshot", ID: "kimi-k3", DisplayName: "Kimi K3", Context: 262144, Reasoning: "always", Tools: true, Vision: true, Source: "builtin"},
		{Provider: "moonshot", ID: "kimi-k2.6", DisplayName: "Kimi K2.6", Context: 262144, Reasoning: "optional", Tools: true, Vision: true, Source: "builtin"},
		{Provider: "moonshot", ID: "kimi-k2.7-code", DisplayName: "Kimi K2.7 Code", Context: 262144, Reasoning: "always", Tools: true, Source: "builtin"},
		{Provider: "anthropic", ID: "claude-haiku-4-5", DisplayName: "Claude Haiku 4.5", Reasoning: "optional", Tools: true, Vision: true, Source: "builtin"},
	}
}

// IsEmbeddingModel reports whether a model id names an embedding model, which
// cannot serve a chat turn and must never appear in the conversation picker.
// The check is substring-based on the model id (e.g. nomic-embed-text,
// text-embedding-3-small, bge-m3).
func IsEmbeddingModel(id string) bool {
	low := strings.ToLower(id)
	for _, marker := range []string{"embed", "bge-", "gte-", "e5-", "rerank"} {
		if strings.Contains(low, marker) {
			return true
		}
	}
	return false
}

type Registry struct {
	DB         *store.Store
	HTTP       *http.Client
	OllamaHost string
	// Key resolves Authorization headers per provider (env/store), redacted.
	Key func(provider string) string
	// Endpoint resolves chat-compatible base URLs per provider.
	Endpoint func(provider string) string
}

func (r *Registry) http() *http.Client {
	if r.HTTP != nil {
		return r.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second}
}

// List merges cache + builtins (+ live Ollama tags when reachable). For a
// local provider the live tags are the source of truth: a cached Ollama model
// that is no longer installed must not be offered, so cached Ollama rows are
// dropped and replaced by what the runtime actually has.
func (r *Registry) List(ctx context.Context, provider string) []ModelInfo {
	byID := map[string]ModelInfo{}
	for _, b := range Builtins() {
		if provider == "" || b.Provider == provider {
			byID[b.Provider+"/"+b.ID] = b
		}
	}
	rows, err := r.DB.DB.Query(`SELECT provider,id,display_name,context_window,reasoning,tools,vision,source FROM models_cache WHERE provider LIKE ? ORDER BY updated_at DESC`, provider+"%")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var m ModelInfo
			var tools, vision int
			rows.Scan(&m.Provider, &m.ID, &m.DisplayName, &m.Context, &m.Reasoning, &tools, &vision, &m.Source)
			m.Tools = tools == 1
			m.Vision = vision == 1
			// A local model is whatever the runtime currently has, discovered
			// live below. A cached local row proves nothing was uninstalled or not.
			if m.Provider == "ollama" {
				continue
			}
			if IsEmbeddingModel(m.ID) {
				continue
			}
			byID[m.Provider+"/"+m.ID] = m
		}
	}
	if provider == "" || provider == "ollama" {
		for _, m := range r.ollamaTags(ctx) {
			byID["ollama/"+m.ID] = m
		}
	}
	out := make([]ModelInfo, 0, len(byID))
	for _, m := range byID {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Refresh discovers models from provider list APIs and caches them.
// OpenAI-compatible family (openai, deepseek, moonshot, custom): GET /models.
// Ollama: GET /api/tags. Anthropic: no list API — builtins only.
func (r *Registry) Refresh(ctx context.Context, provider string) (int, error) {
	total := 0
	targets := []string{provider}
	if provider == "" {
		targets = []string{"openai", "deepseek", "moonshot", "openai_compatible", "ollama"}
	}
	var firstErr error
	for _, p := range targets {
		n, err := r.refreshOne(ctx, p)
		total += n
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return total, firstErr
}

func (r *Registry) refreshOne(ctx context.Context, provider string) (int, error) {
	if provider == "anthropic" {
		return 0, fmt.Errorf("anthropic exposes no model-list API; using built-in catalog")
	}
	if provider == "ollama" {
		ms := r.ollamaTags(ctx)
		for _, m := range ms {
			r.upsert(m)
		}
		// Prune cached local models the runtime no longer has, so a removed
		// model cannot linger in the catalog.
		if len(ms) > 0 {
			_, _ = r.DB.DB.Exec(`DELETE FROM models_cache WHERE provider='ollama' AND updated_at < ?`, time.Now().UTC().Add(-time.Minute).Format(time.RFC3339))
		}
		return len(ms), nil
	}
	ep := ""
	if r.Endpoint != nil {
		ep = r.Endpoint(provider)
	}
	if ep == "" {
		return 0, fmt.Errorf("no endpoint for %s", provider)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimSuffix(ep, "/")+"/models", nil)
	if err != nil {
		return 0, err
	}
	if r.Key != nil {
		if k := r.Key(provider); k != "" {
			req.Header.Set("Authorization", "Bearer "+k)
		}
	}
	resp, err := r.http().Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return 0, fmt.Errorf("%s /models: HTTP %d", provider, resp.StatusCode)
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return 0, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	n := 0
	for _, d := range out.Data {
		if d.ID == "" {
			continue
		}
		display := d.ID
		for _, bi := range Builtins() {
			if bi.Provider == provider && bi.ID == d.ID {
				display = bi.DisplayName
			}
		}
		_, _ = r.DB.DB.Exec(`INSERT OR REPLACE INTO models_cache(provider,id,display_name,context_window,reasoning,tools,vision,source,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`,
			provider, d.ID, display, 0, "unknown", 0, 0, "discovered", now)
		n++
	}
	return n, nil
}

func (r *Registry) ollamaTags(ctx context.Context) []ModelInfo {
	host := r.OllamaHost
	if host == "" {
		host = "http://127.0.0.1:11434"
	}
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimSuffix(host, "/")+"/api/tags", nil)
	if err != nil {
		return nil
	}
	resp, err := r.http().Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if json.Unmarshal(b, &out) != nil {
		return nil
	}
	var ms []ModelInfo
	for _, m := range out.Models {
		if IsEmbeddingModel(m.Name) {
			continue // embeddings cannot chat; never offer them
		}
		ms = append(ms, ModelInfo{Provider: "ollama", ID: m.Name, DisplayName: m.Name + " (local)", Source: "ollama"})
	}
	return ms
}

func (r *Registry) upsert(m ModelInfo) {
	now := time.Now().UTC().Format(time.RFC3339)
	tools, vision := 0, 0
	if m.Tools {
		tools = 1
	}
	if m.Vision {
		vision = 1
	}
	_, _ = r.DB.DB.Exec(`INSERT OR REPLACE INTO models_cache(provider,id,display_name,context_window,reasoning,tools,vision,source,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		m.Provider, m.ID, m.DisplayName, m.Context, m.Reasoning, tools, vision, m.Source, now)
}
