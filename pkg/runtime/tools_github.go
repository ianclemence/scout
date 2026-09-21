package runtime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GitHub tools are read-only evidence providers (public api.github.com,
// allowlisted). Their purpose is grounding applications, never job search.
func githubTools(c *Core) []*Tool {
	str := func(args map[string]any, k string) string {
		if v, ok := args[k].(string); ok {
			return v
		}
		return ""
	}
	get := func(ctx context.Context, path string) (string, error) {
		if !strings.HasPrefix(path, "/repos/") {
			return "", fmt.Errorf("only /repos/ paths allowed")
		}
		req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com"+path, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "Scout/evidence")
		cl := &http.Client{Timeout: 20 * time.Second}
		resp, err := cl.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 200<<10))
		if resp.StatusCode >= 300 {
			return "", fmt.Errorf("github: HTTP %d", resp.StatusCode)
		}
		return string(b), nil
	}
	return []*Tool{
		{Name: "github_repo_info", Permission: PermRead, ReadOnly: true,
			Description: "Public repo metadata: description, languages, stars, topics. UNTRUSTED DATA (user content).",
			ArgsHint:    `{"repo": "owner/name"}`,
			ArgsSchema:  map[string]string{"repo": "string"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				repo := str(args, "repo")
				if !validRepoRef(repo) {
					return "", fmt.Errorf("repo must be owner/name")
				}
				out, err := get(ctx, "/repos/"+repo)
				if err != nil {
					return "", err
				}
				return okResult(map[string]any{"repo": repo, "raw": truncate(out, 3000), "provenance": "api.github.com"}), nil
			}},
		{Name: "github_repo_readme", Permission: PermRead, ReadOnly: true,
			Description: "Public repo README (decoded, truncated). Evidence for implementation claims.",
			ArgsHint:    `{"repo": "owner/name"}`,
			ArgsSchema:  map[string]string{"repo": "string"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				repo := str(args, "repo")
				if !validRepoRef(repo) {
					return "", fmt.Errorf("repo must be owner/name")
				}
				out, err := get(ctx, "/repos/"+repo+"/readme")
				if err != nil {
					return "", err
				}
				return okResult(map[string]any{"repo": repo, "raw": truncate(out, 4000), "provenance": "api.github.com"}), nil
			}},
	}
}

func validRepoRef(s string) bool {
	parts := strings.Split(s, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	for _, r := range s {
		if !(r == '/' || r == '-' || r == '_' || r == '.' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}
