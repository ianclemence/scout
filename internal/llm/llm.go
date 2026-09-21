// Package llm defines a clean provider abstraction.
// Supports OpenAI, Anthropic, DeepSeek, Ollama (native + OpenAI-compatible),
// and generic OpenAI-compatible endpoints. One shared HTTP core.
package llm

import (
	"bytes"
	"context"
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
	// Stream emits response tokens as they arrive; returns on completion or error.
	// Implementations must honor ctx cancellation (interrupt).
	Stream(ctx context.Context, req Request, emit func(token string) error) error
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

// ---- streaming ----

func chatMsgs(req Request) []map[string]string {
	msgs := []map[string]string{}
	if req.System != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": req.System})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
	}
	return msgs
}

// sseDeltas calls emit for each SSE "data:" payload until [DONE] or EOF.
func sseDeltas(ctx context.Context, body io.Reader, emit func(token string) error, delta func(raw json.RawMessage) string) error {
	var line []byte
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	flush := func() error {
		s := string(line)
		line = line[:0]
		if !strings.HasPrefix(s, "data:") {
			return nil
		}
		payload := strings.TrimSpace(strings.TrimPrefix(s, "data:"))
		if payload == "" || payload == "[DONE]" {
			return nil
		}
		if tok := delta(json.RawMessage(payload)); tok != "" {
			return emit(tok)
		}
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, err := body.(io.Reader).Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			for {
				i := -1
				for j, c := range buf {
					if c == '\n' {
						i = j
						break
					}
				}
				if i < 0 {
					break
				}
				line = append(line[:0], buf[:i]...)
				if len(line) > 0 && line[len(line)-1] == '\r' {
					line = line[:len(line)-1]
				}
				buf = buf[i+1:]
				if err := flush(); err != nil {
					return err
				}
			}
		}
		if err != nil {
			return nil // EOF or closed: normal stream end
		}
	}
}

func (p *openaiCompat) Stream(ctx context.Context, req Request, emit func(string) error) error {
	model := req.Model
	if model == "" {
		model = p.model
	}
	body, _ := json.Marshal(map[string]any{
		"model": model, "messages": chatMsgs(req), "temperature": req.Temperature, "stream": true,
	})
	hreq, err := http.NewRequestWithContext(ctx, "POST", strings.TrimSuffix(p.endpoint, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Accept", "text/event-stream")
	if p.apiKey != "" {
		hreq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := p.c.Do(hreq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return fmt.Errorf("%s: HTTP %d: %s", p.name, resp.StatusCode, truncate(string(b), 500))
	}
	return sseDeltas(ctx, resp.Body, emit, func(raw json.RawMessage) string {
		var ev struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal(raw, &ev) != nil || len(ev.Choices) == 0 {
			return ""
		}
		return ev.Choices[0].Delta.Content
	})
}

func (p *ollama) Stream(ctx context.Context, req Request, emit func(string) error) error {
	oc := &openaiCompat{name: "ollama", endpoint: p.endpoint + "/v1", model: p.model, c: p.c}
	if err := oc.Stream(ctx, req, emit); err == nil {
		return nil
	}
	// Fallback: non-streaming native call, single emit.
	out, err := p.Complete(req)
	if err != nil {
		return err
	}
	return emit(out)
}

func (p *anthropic) Stream(ctx context.Context, req Request, emit func(string) error) error {
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
		maxTok = 2048
	}
	body, _ := json.Marshal(map[string]any{
		"model": model, "max_tokens": maxTok, "system": req.System, "messages": msgs, "stream": true,
	})
	hreq, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return err
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Accept", "text/event-stream")
	hreq.Header.Set("x-api-key", p.apiKey)
	hreq.Header.Set("anthropic-version", "2023-06-01")
	resp, err := p.c.Do(hreq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return fmt.Errorf("anthropic: HTTP %d: %s", resp.StatusCode, truncate(string(b), 500))
	}
	return sseDeltas(ctx, resp.Body, emit, func(raw json.RawMessage) string {
		var ev struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
		}
		if json.Unmarshal(raw, &ev) != nil {
			return ""
		}
		if ev.Type == "content_block_delta" {
			return ev.Delta.Text
		}
		return ""
	})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
