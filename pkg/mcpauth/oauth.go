// Package mcpauth implements OAuth 2.1 for remote MCP servers, matching the
// flow Upwork (and other hosts) require: protected-resource and authorization-
// server discovery (RFC 9728 / RFC 8414), dynamic client registration
// (RFC 7591), PKCE authorization code (RFC 7636), a loopback callback, a
// manual paste path for remote browsers, and refresh-token renewal.
package mcpauth

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

// ServerMetadata is the subset of OAuth discovery Scout needs.
type ServerMetadata struct {
	Resource              string
	AuthorizationEndpoint string
	TokenEndpoint         string
	RegistrationEndpoint  string
	ScopesSupported       []string
}

// Client is a dynamically registered OAuth client.
type Client struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
}

// Credential is a stored OAuth credential (JSON-encoded as one secret value).
type Credential struct {
	AccessToken   string    `json:"access_token"`
	RefreshToken  string    `json:"refresh_token,omitempty"`
	TokenType     string    `json:"token_type,omitempty"`
	ExpiresAt     time.Time `json:"expires_at,omitempty"`
	Scope         string    `json:"scope,omitempty"`
	ClientID      string    `json:"client_id,omitempty"`
	ClientSecret  string    `json:"client_secret,omitempty"`
	TokenEndpoint string    `json:"token_endpoint,omitempty"`
	Resource      string    `json:"resource,omitempty"`
}

// Encode serializes the credential for encrypted storage.
func (c *Credential) Encode() string {
	b, _ := json.Marshal(c)
	return string(b)
}

// Decode parses a stored credential, reporting ok=false for a plain token.
func Decode(s string) (*Credential, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") {
		return nil, false
	}
	var c Credential
	if json.Unmarshal([]byte(s), &c) != nil || c.AccessToken == "" {
		return nil, false
	}
	return &c, true
}

// Expired reports whether the access token is expired or within the refresh
// window. A zero expiry (unknown) is never treated as expired.
func (c *Credential) Expired() bool {
	if c.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(c.ExpiresAt)
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

// Discover resolves the OAuth metadata for a remote MCP server URL.
func Discover(ctx context.Context, serverURL string) (*ServerMetadata, error) {
	u, err := url.Parse(strings.TrimSpace(serverURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid MCP server URL %q", serverURL)
	}
	prURL := fmt.Sprintf("%s://%s/.well-known/oauth-protected-resource%s", u.Scheme, u.Host, u.Path)
	var pr struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
	}
	if err := getJSON(ctx, prURL, &pr); err != nil {
		return nil, fmt.Errorf("protected-resource discovery failed: %w", err)
	}
	if len(pr.AuthorizationServers) == 0 {
		return nil, fmt.Errorf("no authorization server advertised by %s", serverURL)
	}
	issuer := strings.TrimSuffix(pr.AuthorizationServers[0], "/")
	var as struct {
		AuthorizationEndpoint string   `json:"authorization_endpoint"`
		TokenEndpoint         string   `json:"token_endpoint"`
		RegistrationEndpoint  string   `json:"registration_endpoint"`
		ScopesSupported       []string `json:"scopes_supported"`
	}
	if err := getJSON(ctx, issuer+"/.well-known/oauth-authorization-server", &as); err != nil {
		return nil, fmt.Errorf("authorization-server discovery failed: %w", err)
	}
	if as.AuthorizationEndpoint == "" || as.TokenEndpoint == "" {
		return nil, fmt.Errorf("authorization server %s is missing endpoints", issuer)
	}
	resource := pr.Resource
	if resource == "" {
		resource = serverURL
	}
	return &ServerMetadata{
		Resource:              resource,
		AuthorizationEndpoint: as.AuthorizationEndpoint,
		TokenEndpoint:         as.TokenEndpoint,
		RegistrationEndpoint:  as.RegistrationEndpoint,
		ScopesSupported:       as.ScopesSupported,
	}, nil
}

// Register performs dynamic client registration for a loopback redirect.
func Register(ctx context.Context, meta *ServerMetadata, redirectURI string) (*Client, error) {
	if meta.RegistrationEndpoint == "" {
		return nil, errors.New("the server does not support dynamic client registration")
	}
	body := map[string]any{
		"client_name":                "Scout",
		"redirect_uris":              []string{redirectURI},
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	}
	if len(meta.ScopesSupported) > 0 {
		body["scope"] = strings.Join(meta.ScopesSupported, " ")
	}
	var out struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := postJSON(ctx, meta.RegistrationEndpoint, body, &out); err != nil {
		return nil, fmt.Errorf("dynamic client registration failed: %w", err)
	}
	if out.ClientID == "" {
		return nil, errors.New("client registration returned no client_id")
	}
	return &Client{ClientID: out.ClientID, ClientSecret: out.ClientSecret, RedirectURI: redirectURI}, nil
}

// Flow drives one authorization-code + PKCE exchange. Begin starts the loopback
// listener and registers the client; surface AuthorizeURL to the user, then
// Wait (or Submit a pasted code) to obtain the credential.
type Flow struct {
	meta        *ServerMetadata
	client      *Client
	verifier    string
	challenge   string
	state       string
	redirectURI string

	srv    *http.Server
	codeCh chan codeResult
	once   sync.Once
}

type codeResult struct {
	code string
	err  error
}

// Begin discovers the server, opens a loopback listener, registers a client,
// and generates the PKCE challenge.
func Begin(ctx context.Context, serverURL string) (*Flow, error) {
	meta, err := Discover(ctx, serverURL)
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("could not open loopback callback: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	client, err := Register(ctx, meta, redirectURI)
	if err != nil {
		_ = ln.Close()
		return nil, err
	}
	verifier, challenge, err := pkce()
	if err != nil {
		_ = ln.Close()
		return nil, err
	}
	state, err := randomState()
	if err != nil {
		_ = ln.Close()
		return nil, err
	}
	f := &Flow{
		meta: meta, client: client,
		verifier: verifier, challenge: challenge, state: state,
		redirectURI: redirectURI,
		codeCh:      make(chan codeResult, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", f.handleCallback)
	f.srv = &http.Server{Handler: mux}
	go func() { _ = f.srv.Serve(ln) }()
	return f, nil
}

func (f *Flow) handleCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		http.Error(w, "Authentication failed: "+e, http.StatusBadRequest)
		f.deliver(codeResult{err: fmt.Errorf("authorization failed: %s", e)})
		return
	}
	code, state := q.Get("code"), q.Get("state")
	if code == "" || state == "" {
		http.Error(w, "Missing code or state", http.StatusBadRequest)
		return
	}
	if state != f.state {
		http.Error(w, "State mismatch", http.StatusBadRequest)
		f.deliver(codeResult{err: errors.New("oauth state mismatch")})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, successHTML)
	f.deliver(codeResult{code: code})
}

// AuthorizeURL is the URL the user opens in a browser.
func (f *Flow) AuthorizeURL() string {
	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {f.client.ClientID},
		"redirect_uri":          {f.redirectURI},
		"code_challenge":        {f.challenge},
		"code_challenge_method": {"S256"},
		"state":                 {f.state},
	}
	if len(f.meta.ScopesSupported) > 0 {
		params.Set("scope", strings.Join(f.meta.ScopesSupported, " "))
	}
	if f.meta.Resource != "" {
		params.Set("resource", f.meta.Resource)
	}
	return f.meta.AuthorizationEndpoint + "?" + params.Encode()
}

// Submit accepts a pasted authorization code or full redirect URL. It reports
// false when the input contains no usable code.
func (f *Flow) Submit(input string) bool {
	code, state := parseAuthorizationInput(input)
	if code == "" {
		return false
	}
	if state != "" && state != f.state {
		f.deliver(codeResult{err: errors.New("oauth state mismatch")})
		return true
	}
	f.deliver(codeResult{code: code})
	return true
}

// Wait blocks until the callback arrives, a code is submitted, or ctx ends,
// then exchanges the code for a credential.
func (f *Flow) Wait(ctx context.Context) (*Credential, error) {
	var code string
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-f.codeCh:
		if r.err != nil {
			return nil, r.err
		}
		code = r.code
	}
	return f.exchange(ctx, code)
}

// Close stops the loopback listener.
func (f *Flow) Close() {
	if f.srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = f.srv.Shutdown(ctx)
	}
}

func (f *Flow) exchange(ctx context.Context, code string) (*Credential, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {f.redirectURI},
		"client_id":     {f.client.ClientID},
		"code_verifier": {f.verifier},
	}
	if f.client.ClientSecret != "" {
		form.Set("client_secret", f.client.ClientSecret)
	}
	if f.meta.Resource != "" {
		form.Set("resource", f.meta.Resource)
	}
	var tok tokenResponse
	if err := postForm(ctx, f.meta.TokenEndpoint, form, &tok); err != nil {
		return nil, err
	}
	return tokenToCredential(tok, f.client, f.meta), nil
}

// Refresh exchanges a stored refresh token for a fresh access token.
func Refresh(ctx context.Context, cred *Credential) (*Credential, error) {
	if cred.RefreshToken == "" {
		return nil, errors.New("no refresh token")
	}
	if cred.TokenEndpoint == "" {
		return nil, errors.New("credential has no token endpoint")
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {cred.RefreshToken},
		"client_id":     {cred.ClientID},
	}
	if cred.ClientSecret != "" {
		form.Set("client_secret", cred.ClientSecret)
	}
	if cred.Resource != "" {
		form.Set("resource", cred.Resource)
	}
	var tok tokenResponse
	if err := postForm(ctx, cred.TokenEndpoint, form, &tok); err != nil {
		return nil, err
	}
	client := &Client{ClientID: cred.ClientID, ClientSecret: cred.ClientSecret, RedirectURI: ""}
	out := tokenToCredential(tok, client, &ServerMetadata{TokenEndpoint: cred.TokenEndpoint, Resource: cred.Resource})
	if out.RefreshToken == "" {
		out.RefreshToken = cred.RefreshToken
	}
	return out, nil
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

func tokenToCredential(tok tokenResponse, client *Client, meta *ServerMetadata) *Credential {
	if tok.TokenType == "" {
		tok.TokenType = "Bearer"
	}
	exp := time.Time{}
	if tok.ExpiresIn > 0 {
		exp = time.Now().Add(time.Duration(tok.ExpiresIn)*time.Second - 5*time.Minute)
	}
	return &Credential{
		AccessToken:   tok.AccessToken,
		RefreshToken:  tok.RefreshToken,
		TokenType:     tok.TokenType,
		ExpiresAt:     exp,
		Scope:         tok.Scope,
		ClientID:      client.ClientID,
		ClientSecret:  client.ClientSecret,
		TokenEndpoint: meta.TokenEndpoint,
		Resource:      meta.Resource,
	}
}

func (f *Flow) deliver(r codeResult) {
	f.once.Do(func() { f.codeCh <- r })
}

func pkce() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// parseAuthorizationInput extracts code and state from a pasted redirect URL, a
// "code#state" string, or a bare code.
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

func getJSON(ctx context.Context, endpoint string, into any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return json.Unmarshal(data, into)
}

func postJSON(ctx context.Context, endpoint string, body any, into any) error {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return json.Unmarshal(data, into)
}

func postForm(ctx context.Context, endpoint string, form url.Values, into any) error {
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
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

const successHTML = `<!doctype html><html><head><meta charset="utf-8"><title>Scout authorized</title>
<style>body{font-family:system-ui,sans-serif;background:#17130f;color:#efe9dc;display:flex;align-items:center;justify-content:center;height:100vh;margin:0}main{text-align:center}</style></head>
<body><main><h1>Scout is authorized</h1><p>You can close this window and return to the terminal.</p></main></body></html>`
