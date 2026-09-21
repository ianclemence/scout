package mcpauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newOAuthServer stands up a minimal OAuth host: protected-resource metadata,
// authorization-server metadata, dynamic client registration, and a token
// endpoint that supports authorization_code and refresh_token.
func newOAuthServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/.well-known/oauth-protected-resource/mcp", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"resource":              base + "/mcp",
			"authorization_servers": []string{base},
		})
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"authorization_endpoint": base + "/authorize",
			"token_endpoint":         base + "/token",
			"registration_endpoint":  base + "/register",
			"scopes_supported":       []string{"read", "write"},
		})
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		json.NewEncoder(w).Encode(map[string]any{"client_id": "client-123"})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			if r.Form.Get("code") != "code-ok" || r.Form.Get("code_verifier") == "" {
				http.Error(w, "bad code", 400)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"access_token": "access-1", "refresh_token": "refresh-1",
				"token_type": "Bearer", "expires_in": 3600,
			})
		case "refresh_token":
			if r.Form.Get("refresh_token") != "refresh-1" {
				http.Error(w, "bad refresh", 400)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"access_token": "access-2", "refresh_token": "refresh-2", "expires_in": 3600,
			})
		default:
			http.Error(w, "unsupported", 400)
		}
	})
	srv := httptest.NewServer(mux)
	base = srv.URL
	t.Cleanup(srv.Close)
	return srv
}

func TestDiscoverAndRegister(t *testing.T) {
	srv := newOAuthServer(t)
	meta, err := Discover(context.Background(), srv.URL+"/mcp")
	if err != nil {
		t.Fatal(err)
	}
	if meta.AuthorizationEndpoint != srv.URL+"/authorize" || meta.TokenEndpoint != srv.URL+"/token" {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
	if meta.Resource != srv.URL+"/mcp" {
		t.Fatalf("resource = %q", meta.Resource)
	}
	client, err := Register(context.Background(), meta, "http://127.0.0.1:1/callback")
	if err != nil {
		t.Fatal(err)
	}
	if client.ClientID != "client-123" {
		t.Fatalf("client_id = %q", client.ClientID)
	}
}

func TestFlowAuthorizeAndExchange(t *testing.T) {
	srv := newOAuthServer(t)
	f, err := Begin(context.Background(), srv.URL+"/mcp")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	authURL := f.AuthorizeURL()
	for _, want := range []string{srv.URL + "/authorize", "client_id=client-123", "code_challenge_method=S256", "state=" + f.state, "resource="} {
		if !strings.Contains(authURL, want) {
			t.Fatalf("authorize URL missing %q:\n%s", want, authURL)
		}
	}
	// Manual paste path (no browser): code#state.
	if !f.Submit("code-ok#" + f.state) {
		t.Fatal("Submit should accept a code#state input")
	}
	cred, err := f.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cred.AccessToken != "access-1" || cred.RefreshToken != "refresh-1" {
		t.Fatalf("unexpected credential: %+v", cred)
	}
	if cred.ClientID != "client-123" || cred.TokenEndpoint != srv.URL+"/token" {
		t.Fatalf("credential missing client/endpoint: %+v", cred)
	}
	if cred.Expired() {
		t.Fatal("fresh credential should not be expired")
	}
}

func TestRefresh(t *testing.T) {
	srv := newOAuthServer(t)
	cred := &Credential{
		AccessToken: "access-1", RefreshToken: "refresh-1",
		ClientID: "client-123", TokenEndpoint: srv.URL + "/token",
		ExpiresAt: time.Now().Add(-time.Minute),
	}
	fresh, err := Refresh(context.Background(), cred)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.AccessToken != "access-2" || fresh.RefreshToken != "refresh-2" {
		t.Fatalf("unexpected refreshed credential: %+v", fresh)
	}
	if fresh.TokenEndpoint != srv.URL+"/token" || fresh.ClientID != "client-123" {
		t.Fatalf("refresh lost client metadata: %+v", fresh)
	}
}

func TestCredentialEncodeDecode(t *testing.T) {
	cred := &Credential{AccessToken: "a", RefreshToken: "r", ExpiresAt: time.Now().Add(time.Hour)}
	got, ok := Decode(cred.Encode())
	if !ok || got.AccessToken != "a" || got.RefreshToken != "r" {
		t.Fatalf("roundtrip failed: %+v ok=%v", got, ok)
	}
	if _, ok := Decode("plain-token"); ok {
		t.Fatal("a plain token must not decode as OAuth")
	}
}

func TestParseAuthorizationInput(t *testing.T) {
	cases := []struct{ in, code, state string }{
		{"http://127.0.0.1:1/callback?code=abc&state=s", "abc", "s"},
		{"abc#s", "abc", "s"},
		{"code=abc&state=s", "abc", "s"},
		{"abc", "abc", ""},
	}
	for _, c := range cases {
		code, state := parseAuthorizationInput(c.in)
		if code != c.code || state != c.state {
			t.Fatalf("parse(%q) = (%q,%q), want (%q,%q)", c.in, code, state, c.code, c.state)
		}
	}
}
