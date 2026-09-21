// Package jeveval is Scout's EXTERNAL evaluation layer. It is deliberately not
// part of Scout's runtime: Scout never imports jeveval, and Scout keeps working
// with jeveval absent. The evaluator reads what Scout already records (the
// trajectories and tool_audit tables, evaluations, proposals) and asks TypeSafe
// Jev structured questions about it.
//
// Design rule learned experimentally (see docs/EVALUATION.md): Jev is reliable
// at comparing an answer against supplied evidence, and unreliable when asked
// an abstract judgement with no reference to compare against. Every grounding
// question therefore puts the actual tool result in the state and asks Jev to
// compare, not to opine.
package jeveval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// Client talks to the TypeSafe System One API. It is small on purpose: three
// question types, one endpoint.
type Client struct {
	APIKey  string
	BaseURL string
	HTTP    *http.Client
}

// NewClient reads TYPESAFE_API_KEY from the environment. The key is never
// logged or persisted by this package.
func NewClient() (*Client, error) {
	key := os.Getenv("TYPESAFE_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("TYPESAFE_API_KEY is not set")
	}
	return &Client{
		APIKey:  key,
		BaseURL: "https://api.typesafe.ai/v1/systemone",
		HTTP:    &http.Client{Timeout: 120 * time.Second},
	}, nil
}

// Noul is a yes/no question answered with a probability.
type Noul struct {
	Type         string `json:"type"`
	Instructions any    `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

// Choice is a single-option question answered with a distribution.
type Choice struct {
	Type         string `json:"type"`
	Instructions any    `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

// Score rates content against ordered levels.
type Score struct {
	Type         string `json:"type"`
	Instructions any    `json:"instructions"`
	Criteria     []any  `json:"criteria"`
}

// Answer is the typed response for one question. Only the fields relevant to
// the question type are populated.
type Answer struct {
	Type          string             `json:"type"`
	Noul          float64            `json:"noul,omitempty"`
	Choice        string             `json:"choice,omitempty"`
	Score         float64            `json:"score,omitempty"`
	Confidence    float64            `json:"confidence,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Legend        map[string]string  `json:"legend,omitempty"`
}

// Response is one System One evaluation.
type Response struct {
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// Ask evaluates state against questions. State may be any JSON-marshalable
// value; questions is the typed map.
func (c *Client) Ask(ctx context.Context, state any, questions map[string]any) (*Response, error) {
	body := map[string]any{"model": "jev-latest", "state": state, "questions": questions}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	// Bounded backoff on 429/529 per the API docs.
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(1<<attempt) * time.Second):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(buf))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.HTTP.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		var out Response
		dec := json.NewDecoder(resp.Body)
		derr := dec.Decode(&out)
		resp.Body.Close()
		switch {
		case resp.StatusCode == 200 && derr == nil:
			return &out, nil
		case resp.StatusCode == 401:
			return nil, fmt.Errorf("jeveval: authentication failed (check TYPESAFE_API_KEY)")
		case resp.StatusCode == 422:
			return nil, fmt.Errorf("jeveval: request rejected (422): %v", derr)
		case resp.StatusCode == 429 || resp.StatusCode == 529:
			lastErr = fmt.Errorf("jeveval: status %d (retrying)", resp.StatusCode)
			continue
		default:
			return nil, fmt.Errorf("jeveval: status %d: %v", resp.StatusCode, derr)
		}
	}
	return nil, lastErr
}
