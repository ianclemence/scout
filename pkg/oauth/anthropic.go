// Package oauth implements account (subscription) sign-in for Scout using the
// same PKCE authorization-code flow the Pi coding agent uses. Anthropic
// (Claude Pro/Max) is the first account provider; the flow is generic and can
// back more providers later.
//
// The credential is stored encrypted by the caller. Access tokens are used as
// Bearer credentials with the Claude Code identity headers, matching how the
// provider is authenticated by a subscription.
package oauth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Anthropic OAuth constants. These mirror the values the Pi coding agent uses
// so account sign-in behaves identically.
const (
	AnthropicClientID     = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	AnthropicAuthorize    = "https://claude.ai/oauth/authorize"
	AnthropicTokenURL     = "https://platform.claude.com/v1/oauth/token"
	anthropicCallbackHost = "127.0.0.1"
	anthropicCallbackPort = 53692
	anthropicCallbackPath = "/callback"
	// AnthropicRedirectURI is the loopback redirect the authorization server
	// returns to. It must match the value registered with the client id.
	AnthropicRedirectURI = "http://localhost:53692/callback"
	anthropicScopes      = "org:create_api_key user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"
)

// Credential is a stored OAuth credential. It is JSON-encoded as a single
// secret value; non-JSON secrets are treated as plain API keys.
type Credential struct {
	Access    string    `json:"access"`
	Refresh   string    `json:"refresh,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

// IsOAuth reports whether a credential string is a stored OAuth credential.
func IsOAuth(s string) bool {
	_, ok := Decode(s)
	return ok
}

// Encode serializes the credential for storage.
func (c *Credential) Encode() string {
	b, _ := json.Marshal(c)
	return string(b)
}

// Decode parses a stored credential, reporting ok=false for a plain API key.
func Decode(s string) (*Credential, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") {
		return nil, false
	}
	var c Credential
	if json.Unmarshal([]byte(s), &c) != nil || c.Access == "" {
		return nil, false
	}
	return &c, true
}

// Expired reports whether the access token is expired or within the refresh
// window.
func (c *Credential) Expired() bool {
	if c.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(c.ExpiresAt)
}

// Flow drives one OAuth authorization-code exchange. Create it with NewFlow,
// surface AuthorizeURL to the user, Start the callback listener, then Wait for
// the code (or accept a manual paste via Submit) and Exchange it.
type Flow struct {
	verifier  string
	challenge string

	srv    *http.Server
	codeCh chan callbackResult
	once   sync.Once
}

type callbackResult struct {
	code  string
	state string
	err   error
}

// NewFlow generates a fresh PKCE challenge.
func NewFlow() (*Flow, error) {
	verifier, challenge, err := generatePKCE()
	if err != nil {
		return nil, err
	}
	return &Flow{verifier: verifier, challenge: challenge, codeCh: make(chan callbackResult, 1)}, nil
}

// AuthorizeURL is the URL the user opens in a browser.
func (f *Flow) AuthorizeURL() string {
	params := url.Values{
		"code":                  {"true"},
		"client_id":             {AnthropicClientID},
		"response_type":         {"code"},
		"redirect_uri":          {AnthropicRedirectURI},
		"scope":                 {anthropicScopes},
		"code_challenge":        {f.challenge},
		"code_challenge_method": {"S256"},
		"state":                 {f.verifier},
	}
	return AnthropicAuthorize + "?" + params.Encode()
}

// Start begins listening on the loopback callback address. A failure here is
// non-fatal: the user can still paste the redirect URL.
func (f *Flow) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc(anthropicCallbackPath, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if e := q.Get("error"); e != "" {
			http.Error(w, "Authentication failed: "+e, http.StatusBadRequest)
			f.deliver(callbackResult{err: fmt.Errorf("authorization failed: %s", e)})
			return
		}
		code, state := q.Get("code"), q.Get("state")
		if code == "" || state == "" {
			http.Error(w, "Missing code or state", http.StatusBadRequest)
			return
		}
		if state != f.verifier {
			http.Error(w, "State mismatch", http.StatusBadRequest)
			f.deliver(callbackResult{err: errors.New("oauth state mismatch")})
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, oauthSuccessHTML)
		f.deliver(callbackResult{code: code, state: state})
	})
	addr := fmt.Sprintf("%s:%d", anthropicCallbackHost, anthropicCallbackPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	f.srv = &http.Server{Handler: mux}
	go func() { _ = f.srv.Serve(ln) }()
	return nil
}

// Wait blocks until the callback arrives, the user submits a manual code, or
// ctx is cancelled. It returns the authorization code.
func (f *Flow) Wait(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case r := <-f.codeCh:
		if r.err != nil {
			return "", r.err
		}
		return r.code, nil
	}
}

// Submit accepts a pasted authorization code or full redirect URL. It is the
// path for a browser running on a different machine than Scout. It reports
// false when the input has no usable code.
func (f *Flow) Submit(input string) bool {
	code, state := parseAuthorizationInput(input)
	if code == "" {
		return false
	}
	if state != "" && state != f.verifier {
		f.deliver(callbackResult{err: errors.New("oauth state mismatch")})
		return true
	}
	f.deliver(callbackResult{code: code, state: f.verifier})
	return true
}

// Close stops the callback listener.
func (f *Flow) Close() {
	if f.srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = f.srv.Shutdown(ctx)
	}
}

func (f *Flow) deliver(r callbackResult) {
	f.once.Do(func() { f.codeCh <- r })
}

// Exchange trades an authorization code for tokens.
func (f *Flow) Exchange(ctx context.Context, code string) (*Credential, error) {
	body := map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     AnthropicClientID,
		"code":          code,
		"state":         f.verifier,
		"redirect_uri":  AnthropicRedirectURI,
		"code_verifier": f.verifier,
	}
	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := postJSON(ctx, AnthropicTokenURL, body, &out); err != nil {
		return nil, err
	}
	if out.AccessToken == "" {
		return nil, errors.New("token exchange returned no access token")
	}
	return credentialFrom(out.AccessToken, out.RefreshToken, out.ExpiresIn), nil
}

// Refresh exchanges a refresh token for a fresh access token.
func Refresh(ctx context.Context, refreshToken string) (*Credential, error) {
	body := map[string]string{
		"grant_type":    "refresh_token",
		"client_id":     AnthropicClientID,
		"refresh_token": refreshToken,
	}
	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := postJSON(ctx, AnthropicTokenURL, body, &out); err != nil {
		return nil, err
	}
	if out.AccessToken == "" {
		return nil, errors.New("token refresh returned no access token")
	}
	refresh := out.RefreshToken
	if refresh == "" {
		refresh = refreshToken
	}
	return credentialFrom(out.AccessToken, refresh, out.ExpiresIn), nil
}

func credentialFrom(access, refresh string, expiresIn int) *Credential {
	exp := time.Time{}
	if expiresIn > 0 {
		exp = time.Now().Add(time.Duration(expiresIn)*time.Second - 5*time.Minute)
	}
	return &Credential{Access: access, Refresh: refresh, ExpiresAt: exp}
}

// IsOAuthToken reports whether an access token is an Anthropic OAuth token,
// which requires Bearer auth, the Claude Code beta headers, and the Claude
// Code identity system block.
func IsOAuthToken(token string) bool {
	return strings.Contains(token, "sk-ant-oat")
}

func generatePKCE() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

// parseAuthorizationInput extracts code and state from a pasted redirect URL,
// a "code#state" string, or a bare code.
func parseAuthorizationInput(input string) (code, state string) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", ""
	}
	if u, err := url.Parse(value); err == nil && (u.Scheme != "" || u.Host != "") {
		return u.Query().Get("code"), u.Query().Get("state")
	}
	if i := strings.Index(value, "#"); i >= 0 {
		return value[:i], value[i+1:]
	}
	if strings.Contains(value, "code=") {
		vals, _ := url.ParseQuery(value)
		return vals.Get("code"), vals.Get("state")
	}
	return value, ""
}

func postJSON(ctx context.Context, endpoint string, body map[string]string, into any) error {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("token request failed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if err := json.Unmarshal(data, into); err != nil {
		return fmt.Errorf("token response was not valid JSON: %w", err)
	}
	return nil
}

const oauthSuccessHTML = `<!doctype html><html><head><meta charset="utf-8"><title>Scout authorized</title>
<style>body{font-family:system-ui,sans-serif;background:#17130f;color:#efe9dc;display:flex;align-items:center;justify-content:center;height:100vh;margin:0}
main{text-align:center}h1{font-size:1.3rem}code{color:#c9a86a}</style></head>
<body><main><h1>Scout is authorized</h1><p>You can close this window and return to the terminal.</p></main></body></html>`
