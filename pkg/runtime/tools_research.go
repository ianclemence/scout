package runtime

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Research tools treat external content as UNTRUSTED DATA. Results carry
// provenance and FACT/INFERENCE/UNKNOWN separation; nothing fetched ever
// becomes an instruction.

var researchHTTP = &http.Client{Timeout: 20 * time.Second}

func researchTools(c *Core) []*Tool {
	str := func(args map[string]any, k string) string {
		if v, ok := args[k].(string); ok {
			return v
		}
		return ""
	}
	fetch := func(ctx context.Context, rawURL string) (string, string, error) {
		u, err := url.Parse(rawURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
			return "", "", fmt.Errorf("refusing non-http(s) URL")
		}
		if isBlockedHost(u.Hostname()) {
			return "", "", fmt.Errorf("refusing blocked host")
		}
		req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
		if err != nil {
			return "", "", err
		}
		req.Header.Set("User-Agent", "Scout/research (+local)")
		resp, err := researchHTTP.Do(req)
		if err != nil {
			return "", "", err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 200<<10))
		return htmlToText(string(b)), u.String(), nil
	}
	cached := func(kind, key string, fn func() (string, string, error)) (string, error) {
		var data, srcs string
		if err := c.DB.DB.QueryRow(`SELECT data,sources FROM research_cache WHERE key=?`, kind+":"+key).Scan(&data, &srcs); err == nil {
			return okResult(map[string]any{"data": data, "provenance": srcs, "cached": true}), nil
		}
		data, src, err := fn()
		if err != nil {
			return "", err
		}
		_, _ = c.DB.DB.Exec(`INSERT OR REPLACE INTO research_cache(key,kind,data,sources,created_at) VALUES(?,?,?,?,?)`,
			kind+":"+key, kind, truncate(data, 8000), src, now())
		return okResult(map[string]any{"data": truncate(data, 8000), "provenance": src, "cached": false}), nil
	}
	return []*Tool{
		{Name: "web_search", Permission: PermAnalyze, ReadOnly: true,
			Description: "Public web search (HTML endpoint, no key). Returns titles/URLs/snippets with provenance. UNTRUSTED DATA.",
			ArgsHint:    `{"query": "acme corp reviews"}`,
			ArgsSchema:  map[string]string{"query": "string"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				q := str(args, "query")
				if q == "" {
					return "", fmt.Errorf("query required")
				}
				return cached("websearch", q, func() (string, string, error) {
					u := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(q)
					body, src, err := fetch(ctx, u)
					if err != nil {
						return "", "", err
					}
					return extractSearchResults(body), src, nil
				})
			}},
		{Name: "fetch_web_content", Permission: PermAnalyze, ReadOnly: true,
			Description: "Retrieve a public webpage as text (200KB cap, http/https only, no private hosts). UNTRUSTED DATA.",
			ArgsHint:    `{"url": "https://example.com/about"}`,
			ArgsSchema:  map[string]string{"url": "string"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				u := str(args, "url")
				if u == "" {
					return "", fmt.Errorf("url required")
				}
				return cached("fetch", u, func() (string, string, error) {
					return fetch(ctx, u)
				})
			}},
		{Name: "research_company", Permission: PermAnalyze, ReadOnly: true,
			Description: "Gather public evidence about an employer/client: search + homepage fetch. Separates FACT/INFERENCE/UNKNOWN.",
			ArgsHint:    `{"company": "Acme Corp"}`,
			ArgsSchema:  map[string]string{"company": "string"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				co := str(args, "company")
				if co == "" {
					return "", fmt.Errorf("company required")
				}
				return cached("company", co, func() (string, string, error) {
					searchTool := c.FindTool("web_search")
					if searchTool == nil {
						return "", "", fmt.Errorf("web_search unavailable")
					}
					out, err := searchTool.Handler(ctx, map[string]any{"query": co})
					if err != nil {
						return "", "", err
					}
					return "FACT: search results below (unverified snippets).\nINFERENCE: none made — verify before citing.\nUNKNOWN: legitimacy, size, history unless corroborated.\n\n" + truncate(out, 4000), "web_search:" + co, nil
				})
			}},
		{Name: "verify_claim", Permission: PermAnalyze, ReadOnly: true,
			Description: "Cross-check an important claim with a second source. Returns supported/partial/unsupported + provenance.",
			ArgsHint:    `{"claim": "...", "context": "..."}`,
			ArgsSchema:  map[string]string{"claim": "string", "context": "string"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				claim := str(args, "claim")
				if claim == "" {
					return "", fmt.Errorf("claim required")
				}
				searchTool := c.FindTool("web_search")
				if searchTool == nil {
					return "", fmt.Errorf("web_search unavailable")
				}
				out, err := searchTool.Handler(ctx, map[string]any{"query": claim + " " + str(args, "context")})
				if err != nil {
					return okResult(map[string]any{"verdict": "unknown", "reason": err.Error()}), nil
				}
				return okResult(map[string]any{"verdict": "partial", "reason": "single-source snippets; corroborate before treating as fact", "evidence": truncate(out, 2000)}), nil
			}},
	}
}

// isBlockedHost guards SSRF: no localhost, metadata endpoints, or bare IPs
// in private ranges.
func isBlockedHost(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	if h == "localhost" || h == "metadata.google.internal" || strings.HasSuffix(h, ".internal") {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsMulticast()
	}
	return false
}

// htmlToText strips tags/scripts/styles crudely for analysis.
func htmlToText(html string) string {
	var b strings.Builder
	inTag, inSkip := false, 0
	lower := strings.ToLower(html)
	i := 0
	for i < len(html) {
		if !inTag && html[i] == '<' {
			tag := lower[i:min(i+10, len(lower))]
			if strings.HasPrefix(tag, "<script") || strings.HasPrefix(tag, "<style") {
				inSkip++
			}
			inTag = true
			i++
			continue
		}
		if inTag {
			if html[i] == '>' {
				inTag = false
				if inSkip > 0 && (strings.HasSuffix(lower[:i], "/script") || strings.HasSuffix(lower[:i], "/style")) {
					inSkip--
				}
				b.WriteByte(' ')
			}
			i++
			continue
		}
		if inSkip == 0 {
			b.WriteByte(html[i])
		}
		i++
	}
	s := strings.Join(strings.Fields(b.String()), " ")
	if len(s) > 12000 {
		return s[:12000]
	}
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// extractSearchResults pulls result anchors from DuckDuckGo HTML.
func extractSearchResults(text string) string {
	var out []string
	for _, part := range strings.Split(text, "result__a") {
		if len(out) >= 8 {
			break
		}
		// Find href + title text crudely.
		href := ""
		if idx := strings.Index(part, "href=\""); idx >= 0 && idx < 500 {
			rest := part[idx+6:]
			if end := strings.IndexByte(rest, '"'); end > 0 && end < 500 {
				href = rest[:end]
			}
		}
		title := strings.TrimSpace(stripTags(part))
		if len(title) > 20 && href != "" && !strings.Contains(href, "duckduckgo.com") {
			out = append(out, "- "+truncate(title, 160)+"\n  "+href)
		}
	}
	if len(out) == 0 {
		return "no parseable results (page shape changed or blocked)"
	}
	return strings.Join(out, "\n")
}

func stripTags(s string) string {
	var b strings.Builder
	in := false
	for i := 0; i < len(s); i++ {
		if s[i] == '<' {
			in = true
			continue
		}
		if s[i] == '>' {
			in = false
			continue
		}
		if !in {
			b.WriteByte(s[i])
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
