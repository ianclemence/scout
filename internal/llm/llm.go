// Package llm defines a clean provider abstraction.
// Supports OpenAI, Anthropic, DeepSeek, Ollama (native + OpenAI-compatible),
// and generic OpenAI-compatible endpoints. One shared HTTP core.
package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	Model       string    `json:"model"`
	System      string    `json:"system,omitempty"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature"`
}

type Provider interface {
	Name() string
	Complete(req Request) (string, error)
}

// Config describes one provider instance. APIKey comes from env/secrets,
// never hard-coded.
type Config struct {
	Provider string // openai, anthropic, ollama, openai_compatible
	Model    string
	Endpoint string
	APIKey   string
}

func New(cfg Config) (Provider, error) {
	httpClient := &http.Client{Timeout: 120 * time.Second}
	switch cfg.Provider {
	case "openai":
		return &openaiCompat{name: "openai", endpoint: epOr(cfg.Endpoint, "https://api.openai.com/v1"), apiKey: cfg.APIKey, model: cfg.Model, c: httpClient}, nil
	case "deepseek":
		return &openaiCompat{name: "deepseek", endpoint: epOr(cfg.Endpoint, "https://api.deepseek.io"), apiKey: cfg.APIKey, model: cfg.Model, c: httpClient}, nil
	case "openai_compatible":
		if cfg.Endpoint == "" {
			return nil, fmt.Errorf("openai_compatible requires endpoint")
		}
		return &openaiCompat{name: "openai_compatible", endpoint: cfg.Endpoint, apiKey: cfg.APIKey, model: cfg.Model, c: httpClient}, nil
	case "ollama":
		host := cfg.Endpoint
		if host == "" {
			host = "http://127.0.0.1:11434"
		}
		return &ollama{endpoint: strings.TrimSuffix(host, "/"), model: cfg.Model, c: httpClient}, nil
	case "anthropic":
		return &anthropic{apiKey: cfg.APIKey, model: cfg.Model, c: httpClient}, nil
	default:
		return nil, fmt.Errorf("unknown provider %q", cfg.Provider)
	}
}

// ---- OpenAI-compatible (OpenAI, OpenRouter, Ollama /v1, custom) ----

type openaiCompat struct {
	name     string
	endpoint string
	apiKey   string
	model    string
	c        *http.Client
}

func (p *openaiCompat) Name() string { return p.name }

func (p *openaiCompat) Complete(req Request) (string, error) {
	model := req.Model
	if model == "" {
		model = p.model
	}
	msgs := []map[string]string{}
	if req.System != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": req.System})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
	}
	body, _ := json.Marshal(map[string]any{
		"model": model, "messages": msgs, "temperature": req.Temperature,
	})
	hreq, _ := http.NewRequest("POST", strings.TrimSuffix(p.endpoint, "/")+"/chat/completions", bytes.NewReader(body))
	hreq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		hreq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := p.c.Do(hreq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s: HTTP %d: %s", p.name, resp.StatusCode, truncate(string(b), 500))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("%s: no choices", p.name)
	}
	return out.Choices[0].Message.Content, nil
}

// ---- Ollama native ----

type ollama struct {
	endpoint string
	model    string
	c        *http.Client
}

func (p *ollama) Name() string { return "ollama" }

func (p *ollama) Complete(req Request) (string, error) {
	// Prefer OpenAI-compatible /v1/chat/completions (modern Ollama).
	oc := &openaiCompat{name: "ollama", endpoint: p.endpoint + "/v1", model: p.model, c: p.c}
	if s, err := oc.Complete(req); err == nil && s != "" {
		return s, nil
	}
	// Fallback to native /api/chat.
	model := req.Model
	if model == "" {
		model = p.model
	}
	msgs := []map[string]string{}
	if req.System != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": req.System})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
	}
	body, _ := json.Marshal(map[string]any{"model": model, "messages": msgs, "stream": false})
	hreq, _ := http.NewRequest("POST", p.endpoint+"/api/chat", bytes.NewReader(body))
	hreq.Header.Set("Content-Type", "application/json")
	resp, err := p.c.Do(hreq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("ollama: HTTP %d: %s", resp.StatusCode, truncate(string(b), 500))
	}
	var out struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", err
	}
	return out.Message.Content, nil
}

// ---- Anthropic ----

type anthropic struct {
	apiKey string
	model  string
	c      *http.Client
}

func (p *anthropic) Name() string { return "anthropic" }

func (p *anthropic) Complete(req Request) (string, error) {
	model := req.Model
	if model == "" {
		model = p.model
	}
	if model == "" {
		model = "claude-haiku-4-5"
	}
	msgs := []map[string]string{}
	for _, m := range req.Messages {
		role := m.Role
		if role == "system" {
			continue
		}
		if role != "assistant" {
			role = "user"
		}
		msgs = append(msgs, map[string]string{"role": role, "content": m.Content})
	}
	maxTok := req.MaxTokens
	if maxTok == 0 {
		maxTok = 1024
	}
	body, _ := json.Marshal(map[string]any{
		"model": model, "max_tokens": maxTok, "system": req.System, "messages": msgs,
	})
	hreq, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("x-api-key", p.apiKey)
	hreq.Header.Set("anthropic-version", "2023-06-01")
	resp, err := p.c.Do(hreq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("anthropic: HTTP %d: %s", resp.StatusCode, truncate(string(b), 500))
	}
	var out struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, c := range out.Content {
		sb.WriteString(c.Text)
	}
	return sb.String(), nil
}

func epOr(v, d string) string {
	if v != "" {
		return v
	}
	return d
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
