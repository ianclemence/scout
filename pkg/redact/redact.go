// Package redact masks secrets in logs, audit rows, and error output.
// Anything that looks like a credential never reaches persistent text.
package redact

import (
	"regexp"
	"strings"
)

var secretKeys = []string{
	"api_key", "apikey", "token", "secret", "password", "passwd",
	"authorization", "bearer", "private_key", "client_secret",
}

var bearerRe = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9\-._~+/=]+`)
var keyRe = regexp.MustCompile(`(?i)\b(sk-[A-Za-z0-9\-_]{8,}|sk-ant-[A-Za-z0-9\-_]{8,}|xox[bpas]-[A-Za-z0-9\-_]{8,}|gh[pousr]_[A-Za-z0-9_]{8,})\b`)

func isSecretKey(name string) bool {
	n := strings.ToLower(name)
	for _, k := range secretKeys {
		if strings.Contains(n, k) {
			return true
		}
	}
	return false
}

// Text masks credential-looking values inside free text.
func Text(s string) string {
	s = bearerRe.ReplaceAllString(s, "Bearer <redacted>")
	s = keyRe.ReplaceAllString(s, "<redacted>")
	return s
}

// Pair masks a key=value pair when the key looks secret.
func Pair(key, value string) string {
	if isSecretKey(key) {
		return "<redacted>"
	}
	return Text(value)
}

// Args masks secret-looking tool arguments for audit rows.
func Args(args map[string]any) map[string]any {
	out := make(map[string]any, len(args))
	for k, v := range args {
		if isSecretKey(k) {
			out[k] = "<redacted>"
			continue
		}
		if s, ok := v.(string); ok {
			out[k] = Text(s)
		} else {
			out[k] = v
		}
	}
	return out
}
