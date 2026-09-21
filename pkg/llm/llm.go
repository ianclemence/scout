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
	// Thinking normalizes reasoning effort: off, low, medium, high, max.
	// Each provider maps these to its own controls; unsupported levels
	// degrade to the closest capability (never an invalid value).
	Thinking string `json:"thinking,omitempty"`
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
		return &openaiCompat{name: "openai", endpoint: epOr(cfg.Endpoint, "https://api.openai.com/v1"), apiKey: cfg.APIKey, model: cfg.Model, c: httpClient, think: thinkOpenAI}, nil
	case "deepseek":
		return &openaiCompat{name: "deepseek", endpoint: epOr(cfg.Endpoint, "https://api.deepseek.com"), apiKey: cfg.APIKey, model: cfg.Model, c: httpClient, think: thinkDeepSeek}, nil
	case "moonshot":
		return &openaiCompat{name: "moonshot", endpoint: epOr(cfg.Endpoint, "https://api.moonshot.ai/v1"), apiKey: cfg.APIKey, model: cfg.Model, c: httpClient, think: thinkMoonshot}, nil
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
		return &anthropic{apiKey: cfg.APIKey, model: cfg.Model, endpoint: cfg.Endpoint, c: httpClient}, nil
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
	// think maps the normalized Thinking level onto the request body.
	think func(model, level string, body map[string]any)
}

// Normalized thinking levels, ordered from least to most reasoning. One
// vocabulary for every provider so the same choices behave the same way.
const (
	ThinkOff     = "off"
	ThinkMinimal = "minimal"
	ThinkLow     = "low"
	ThinkMedium  = "medium"
	ThinkHigh    = "high"
	ThinkXHigh   = "xhigh"
	ThinkMax     = "max"
)

// ThinkLevels is the ordered level set the UI offers.
var ThinkLevels = []string{ThinkOff, ThinkMinimal, ThinkLow, ThinkMedium, ThinkHigh, ThinkXHigh, ThinkMax}

func normThink(s string) string {
	switch s {
	case ThinkOff, ThinkMinimal, ThinkLow, ThinkMedium, ThinkHigh, ThinkXHigh, ThinkMax:
		return s
	default:
		return ""
	}
}

// ThinkMapping returns the concrete request control a level maps to for a
// provider, so the UI can show exactly what a choice does.
func ThinkMapping(provider, level string) string {
	lvl := normThink(level)
	if lvl == "" {
		return ""
	}
	switch provider {
	case "openai", "openai_compatible":
		if lvl == ThinkOff {
			return "no reasoning_effort"
		}
		effort := map[string]string{ThinkMinimal: "minimal", ThinkLow: "low", ThinkMedium: "medium", ThinkHigh: "high", ThinkXHigh: "high", ThinkMax: "high"}[lvl]
		return "reasoning_effort=" + effort
	case "deepseek":
		if lvl == ThinkOff {
			return "thinking disabled"
		}
		if lvl == ThinkHigh || lvl == ThinkXHigh || lvl == ThinkMax {
			return "thinking.enabled + reasoning_effort=high"
		}
		return "thinking.enabled"
	case "moonshot":
		return "thinking/ reasoning_effort (Kimi family)"
	case "ollama":
		if lvl == ThinkOff {
			return "think=false"
		}
		return "think=true"
	}
	return ""
}

// ThinkDescription returns a human description of a level, including what it
// maps to for the given provider so the choice is never mysterious.
func ThinkDescription(provider, level string) string {
	base := map[string]string{
		ThinkOff:     "No reasoning — fastest, direct answers",
		ThinkMinimal: "Very brief reasoning",
		ThinkLow:     "Light reasoning",
		ThinkMedium:  "Moderate reasoning",
		ThinkHigh:    "Deep reasoning",
		ThinkXHigh:   "Extra-high reasoning",
		ThinkMax:     "Maximum reasoning",
	}[level]
	if m := ThinkMapping(provider, level); m != "" {
		if base == "" {
			return m
		}
		return base + " · " + m
	}
	return base
}

// thinkOpenAI maps to reasoning_effort (gpt-5 family: minimal/low/medium/high).
// Off (and the default) omits the field: non-reasoning models reject it, and
// reasoning models never put chain-of-thought in content, so the parser drops
// it regardless.
func thinkOpenAI(model, level string, body map[string]any) {
	switch normThink(level) {
	case ThinkMinimal:
		body["reasoning_effort"] = "minimal"
	case ThinkLow:
		body["reasoning_effort"] = "low"
	case ThinkMedium:
		body["reasoning_effort"] = "medium"
	case ThinkHigh, ThinkXHigh, ThinkMax:
		body["reasoning_effort"] = "high"
	}
}

// thinkDeepSeek maps to thinking.type + reasoning_effort (per current API docs).
// Off is sent explicitly as disabled so a hybrid reasoning model does not
// spend the turn thinking by default.
func thinkDeepSeek(model, level string, body map[string]any) {
	switch normThink(level) {
	case ThinkOff, "":
		body["thinking"] = map[string]string{"type": "disabled"}
	case ThinkMinimal, ThinkLow, ThinkMedium:
		body["thinking"] = map[string]string{"type": "enabled"}
	case ThinkHigh, ThinkXHigh, ThinkMax:
		body["thinking"] = map[string]string{"type": "enabled"}
		body["reasoning_effort"] = "high"
	}
}

// thinkMoonshot maps per Kimi model family: kimi-k3 uses reasoning_effort
// (low/high/max; it always reasons), kimi-k2.x uses thinking.type.
func thinkMoonshot(model, level string, body map[string]any) {
	m := strings.ToLower(model)
	if strings.HasPrefix(m, "kimi-k3") {
		switch normThink(level) {
		case ThinkOff, "", ThinkMinimal, ThinkLow:
			body["reasoning_effort"] = "low"
		case ThinkMedium, ThinkHigh:
			body["reasoning_effort"] = "high"
		default:
			body["reasoning_effort"] = "max"
		}
		return
	}
	if lvl := normThink(level); lvl == ThinkOff || lvl == "" {
		body["thinking"] = map[string]string{"type": "disabled"}
	} else {
		body["thinking"] = map[string]string{"type": "enabled"}
	}
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
	bodyMap := map[string]any{
		"model": model, "messages": msgs, "temperature": req.Temperature,
	}
	if p.think != nil {
		p.think(model, req.Thinking, bodyMap)
	}
	body, _ := json.Marshal(bodyMap)
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
			Message map[string]any `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("%s: no choices", p.name)
	}
	// Only "content" is the answer. Reasoning fields on the message are
	// parsed and discarded (see reasoningFields). Inline thinking tags some
	// models embed in content are stripped so they never surface either.
	content, _ := out.Choices[0].Message["content"].(string)
	_ = reasoningFromMap(out.Choices[0].Message)
	return stripReasoning(content), nil
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
	bodyMap := map[string]any{"model": model, "messages": msgs, "stream": false}
	// Ollama native thinking switch (qwen3/gpt-oss style `think` flag).
	// Empty defaults to off so a hybrid model never reasons by default.
	switch normThink(req.Thinking) {
	case ThinkOff, "":
		bodyMap["think"] = false
	case ThinkLow, ThinkMedium, ThinkHigh, ThinkMax:
		bodyMap["think"] = true
	}
	body, _ := json.Marshal(bodyMap)
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
			Content  string `json:"content"`
			Thinking string `json:"thinking"`
		} `json:"message"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", err
	}
	// Ollama reports reasoning on message.thinking; it is never answer text.
	_ = out.Message.Thinking
	return stripReasoning(out.Message.Content), nil
}

// ---- Anthropic ----

// isAnthropicOAuth reports whether a credential is a subscription (OAuth)
// token. Such tokens use Bearer auth, the Claude Code beta headers, and the
// Claude Code identity system block instead of x-api-key.
func isAnthropicOAuth(apiKey string) bool {
	return strings.Contains(apiKey, "sk-ant-oat")
}

// setAnthropicAuth applies the correct authentication headers for a plain API
// key or a subscription (OAuth) token.
func setAnthropicAuth(hreq *http.Request, apiKey string) {
	hreq.Header.Set("anthropic-version", "2023-06-01")
	if isAnthropicOAuth(apiKey) {
		hreq.Header.Set("Authorization", "Bearer "+apiKey)
		hreq.Header.Set("anthropic-beta", "claude-code-20250219,oauth-2025-04-20")
		return
	}
	hreq.Header.Set("x-api-key", apiKey)
}

// anthropicSystem builds the system field. Subscription (OAuth) requests must
// begin with the Claude Code identity block, followed by the caller's system
// instructions when present.
func anthropicSystem(system string, oauth bool) any {
	if !oauth {
		return system
	}
	blocks := []map[string]string{{"type": "text", "text": "You are Claude Code, Anthropic's official CLI for Claude."}}
	if strings.TrimSpace(system) != "" {
		blocks = append(blocks, map[string]string{"type": "text", "text": system})
	}
	return blocks
}

type anthropic struct {
	apiKey   string
	model    string
	endpoint string
	c        *http.Client
}

func (p *anthropic) base() string {
	if p.endpoint != "" {
		return strings.TrimSuffix(p.endpoint, "/")
	}
	return "https://api.anthropic.com"
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
	bodyMap := map[string]any{
		"model": model, "max_tokens": maxTok, "system": anthropicSystem(req.System, isAnthropicOAuth(p.apiKey)), "messages": msgs,
	}
	// Anthropic extended thinking: budget must be < max_tokens.
	if budget := anthropicBudget(req.Thinking, maxTok); budget > 0 {
		bodyMap["thinking"] = map[string]any{"type": "enabled", "budget_tokens": budget}
	}
	body, _ := json.Marshal(bodyMap)
	hreq, _ := http.NewRequest("POST", p.base()+"/v1/messages", bytes.NewReader(body))
	hreq.Header.Set("Content-Type", "application/json")
	setAnthropicAuth(hreq, p.apiKey)
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
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", err
	}
	// Anthropic returns separate blocks for thinking and text. Only "text"
	// blocks are the answer; "thinking" (and redacted thinking) blocks are
	// chain-of-thought and are discarded here.
	var sb strings.Builder
	for _, c := range out.Content {
		if c.Type == "text" {
			sb.WriteString(c.Text)
		}
	}
	return sb.String(), nil
}

// anthropicBudget maps levels to thinking budgets (clamped below maxTok).
func anthropicBudget(level string, maxTok int) int {
	var want int
	switch normThink(level) {
	case ThinkLow:
		want = 1024
	case ThinkMedium:
		want = 4000
	case ThinkHigh:
		want = 10000
	case ThinkMax:
		want = 20000
	default:
		return 0
	}
	if want >= maxTok {
		want = maxTok - 1
	}
	if want < 1024 {
		want = 1024
	}
	return want
}

func epOr(v, d string) string {
	if v != "" {
		return v
	}
	return d
}

// ---- reasoning isolation ----
//
// Scout never surfaces a model's chain-of-thought. Providers expose reasoning
// on dedicated fields and/or inline tags; both are parsed here and discarded,
// so no caller (agent loop, TUI, CLI) can accidentally render it.

// reasoningFields is the allowlist of message/delta fields that carry model
// reasoning across OpenAI-compatible servers. These are never treated as
// answer text. The names match the de-facto standard used by llama.cpp,
// DeepSeek, Moonshot, and other OpenAI-compatible endpoints.
var reasoningFields = []string{"reasoning_content", "reasoning", "reasoning_text"}

// inlineThinkTags are the tag pairs some models emit directly inside the text
// stream. Content between them is reasoning, not answer text.
var inlineThinkTags = [][2]string{
	{"<thinking>", "</thinking>"},
	{"<think>", "</think>"},
}

// reasoningSanitizer removes inline reasoning tags from a token stream while
// preserving ordinary text, even when a tag is split across tokens. It is the
// single place inline chain-of-thought is removed.
type reasoningSanitizer struct {
	inThink bool
	hold    string
}

// write consumes a token and returns the visible text safe to emit.
func (s *reasoningSanitizer) write(tok string) string {
	s.hold += tok
	var out strings.Builder
	for len(s.hold) > 0 {
		if s.inThink {
			idx, close := earliestTag(s.hold, false)
			if idx < 0 {
				// Retain only a possible partial closing tag; drop the rest.
				s.hold = partialTagSuffix(s.hold, false)
				return out.String()
			}
			s.hold = s.hold[idx+len(close):]
			s.inThink = false
			continue
		}
		idx, open := earliestTag(s.hold, true)
		if idx < 0 {
			keep := partialTagSuffix(s.hold, true)
			out.WriteString(s.hold[:len(s.hold)-len(keep)])
			s.hold = keep
			return out.String()
		}
		out.WriteString(s.hold[:idx])
		s.hold = s.hold[idx+len(open):]
		s.inThink = true
	}
	return out.String()
}

// flush releases any buffered non-reasoning text at end of stream. A dangling
// unclosed think tag is dropped (it was reasoning).
func (s *reasoningSanitizer) flush() string {
	out := ""
	if !s.inThink && len(s.hold) > 0 {
		// If the hold is a prefix of a thinking tag, treat it as reasoning.
		if !isPartialTag(s.hold, true) {
			out = s.hold
		}
	}
	s.hold = ""
	return out
}

// stripReasoning removes inline reasoning tags from a complete string.
func stripReasoning(s string) string {
	san := &reasoningSanitizer{}
	return san.write(s) + san.flush()
}

// earliestTag returns the index and tag of the first opening (open=true) or
// closing (open=false) reasoning tag in s, or -1 if none.
func earliestTag(s string, open bool) (int, string) {
	best := -1
	bestTag := ""
	for _, pair := range inlineThinkTags {
		tag := pair[1]
		if open {
			tag = pair[0]
		}
		if i := strings.Index(s, tag); i >= 0 && (best < 0 || i < best) {
			best, bestTag = i, tag
		}
	}
	return best, bestTag
}

// partialTagSuffix returns the longest suffix of s that is a proper prefix of
// any opening (open=true) or closing tag, so it can be held for the next token.
func partialTagSuffix(s string, open bool) string {
	best := ""
	for _, pair := range inlineThinkTags {
		tag := pair[1]
		if open {
			tag = pair[0]
		}
		max := len(tag) - 1
		if len(s) < max {
			max = len(s)
		}
		for n := max; n > 0; n-- {
			if strings.HasSuffix(s, tag[:n]) && n > len(best) {
				best = tag[:n]
			}
		}
	}
	return best
}

// isPartialTag reports whether s is a proper prefix of an opening tag.
func isPartialTag(s string, open bool) bool {
	for _, pair := range inlineThinkTags {
		tag := pair[1]
		if open {
			tag = pair[0]
		}
		if len(s) < len(tag) && strings.HasPrefix(tag, s) {
			return true
		}
	}
	return false
}

// reasoningFromMap returns the first non-empty reasoning field in a decoded
// provider message or delta, or "" when there is none. Its value is discarded
// by callers; it exists so reasoning is identified and never mistaken for text.
func reasoningFromMap(m map[string]any) string {
	for _, f := range reasoningFields {
		if v, ok := m[f].(string); ok && v != "" {
			return v
		}
	}
	return ""
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
	bodyMap := map[string]any{
		"model": model, "messages": chatMsgs(req), "temperature": req.Temperature, "stream": true,
	}
	if p.think != nil {
		p.think(model, req.Thinking, bodyMap)
	}
	body, _ := json.Marshal(bodyMap)
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
	san := &reasoningSanitizer{}
	return sseDeltas(ctx, resp.Body, func(tok string) error {
		if out := san.write(tok); out != "" {
			return emit(out)
		}
		return nil
	}, func(raw json.RawMessage) string {
		var ev struct {
			Choices []struct {
				Delta map[string]any `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal(raw, &ev) != nil || len(ev.Choices) == 0 {
			return ""
		}
		// Reasoning fields are deliberately ignored: only "content" is the
		// answer. The reasoningFields allowlist documents the contract and
		// guards against ever treating a reasoning field as text.
		if c, ok := ev.Choices[0].Delta["content"].(string); ok {
			return c
		}
		return ""
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
	bodyMapA := map[string]any{
		"model": model, "max_tokens": maxTok, "system": anthropicSystem(req.System, isAnthropicOAuth(p.apiKey)), "messages": msgs, "stream": true,
	}
	if budget := anthropicBudget(req.Thinking, maxTok); budget > 0 {
		bodyMapA["thinking"] = map[string]any{"type": "enabled", "budget_tokens": budget}
	}
	body, _ := json.Marshal(bodyMapA)
	hreq, err := http.NewRequestWithContext(ctx, "POST", p.base()+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return err
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Accept", "text/event-stream")
	setAnthropicAuth(hreq, p.apiKey)
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
		// Only text deltas are answer text. Anthropic emits
		// "thinking_delta" (extended thinking) and "signature_delta" on the
		// same event shape; both are chain-of-thought and must be dropped.
		if ev.Type == "content_block_delta" && ev.Delta.Type == "text_delta" {
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
