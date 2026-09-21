package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPKCEChallengeMatchesVerifier(t *testing.T) {
	verifier, challenge, err := generatePKCE()
	if err != nil {
		t.Fatal(err)
	}
	if verifier == "" || challenge == "" {
		t.Fatal("empty PKCE values")
	}
	sum := sha256.Sum256([]byte(verifier))
	if want := base64.RawURLEncoding.EncodeToString(sum[:]); want != challenge {
		t.Fatalf("challenge = %q, want %q", challenge, want)
	}
	if _, err := base64.RawURLEncoding.DecodeString(verifier); err != nil {
		t.Fatalf("verifier is not base64url: %v", err)
	}
}

func TestCredentialEncodeDecode(t *testing.T) {
	cred := &Credential{Access: "sk-ant-oat-abc", Refresh: "r", ExpiresAt: time.Now().Add(time.Hour)}
	decoded, ok := Decode(cred.Encode())
	if !ok || decoded.Access != cred.Access || decoded.Refresh != cred.Refresh {
		t.Fatalf("roundtrip failed: %+v ok=%v", decoded, ok)
	}
	if _, ok := Decode("sk-ant-api03-plainkey"); ok {
		t.Fatal("a plain API key must not decode as OAuth")
	}
	if IsOAuth("sk-ant-api03-plainkey") {
		t.Fatal("plain key reported as OAuth")
	}
}

func TestCredentialExpiry(t *testing.T) {
	expired := &Credential{Access: "a", ExpiresAt: time.Now().Add(-time.Minute)}
	if !expired.Expired() {
		t.Fatal("past expiry must be expired")
	}
	fresh := &Credential{Access: "a", ExpiresAt: time.Now().Add(time.Hour)}
	if fresh.Expired() {
		t.Fatal("future expiry must not be expired")
	}
	if (&Credential{Access: "a"}).Expired() {
		t.Fatal("zero expiry (unknown) must not report expired")
	}
}

func TestParseAuthorizationInput(t *testing.T) {
	verifier := "state123"
	cases := []struct{ in, code, state string }{
		{AnthropicRedirectURI + "?code=abc&state=" + verifier, "abc", verifier},
		{"abc#" + verifier, "abc", verifier},
		{"code=abc&state=" + verifier, "abc", verifier},
		{"abc", "abc", ""},
	}
	for _, c := range cases {
		code, state := parseAuthorizationInput(c.in)
		if code != c.code || state != c.state {
			t.Fatalf("parse(%q) = (%q,%q), want (%q,%q)", c.in, code, state, c.code, c.state)
		}
	}
}

func TestFlowManualSubmit(t *testing.T) {
	f, err := NewFlow()
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if !f.Submit("abc#" + f.verifier) {
		t.Fatal("Submit should accept a code#state input")
	}
	code, err := f.Wait(context.Background())
	if err != nil || code != "abc" {
		t.Fatalf("Wait = (%q,%v)", code, err)
	}
}

func TestFlowCallbackServer(t *testing.T) {
	f, err := NewFlow()
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := f.Start(); err != nil {
		t.Skipf("loopback listener unavailable: %v", err)
	}
	url := AnthropicRedirectURI + "?code=xyz&state=" + f.verifier
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("callback request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("callback status %d", resp.StatusCode)
	}
	code, err := f.Wait(context.Background())
	if err != nil || code != "xyz" {
		t.Fatalf("Wait = (%q,%v)", code, err)
	}
}

func TestAuthorizeURLShape(t *testing.T) {
	f, _ := NewFlow()
	u := f.AuthorizeURL()
	for _, want := range []string{
		AnthropicAuthorize + "?",
		"client_id=" + AnthropicClientID,
		"code_challenge_method=S256",
		"state=" + f.verifier,
		"redirect_uri=",
	} {
		if !strings.Contains(u, want) {
			t.Fatalf("authorize URL missing %q:\n%s", want, u)
		}
	}
}

func TestIsOAuthToken(t *testing.T) {
	if !IsOAuthToken("sk-ant-oat01-abc") {
		t.Fatal("sk-ant-oat token should be OAuth")
	}
	if IsOAuthToken("sk-ant-api03-abc") {
		t.Fatal("api key must not be OAuth")
	}
}
