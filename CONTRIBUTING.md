# CONTRIBUTING

1. `go build ./...`, `go test ./...`, `go vet ./...`, `gofmt -l .` — all clean before PR.
2. Domain stays marketplace-independent; platform logic goes in adapters (`internal/upwork`, future `internal/<platform>`).
3. No new runtime deps without justification; prefer stdlib.
4. Never commit secrets, DBs, CVs, or logs. Check `git status` before committing.
5. Tests for new matching/approval/persistence behavior; mocks for external services — never hit real Upwork writes in tests.
6. Docs updated if behavior changes (README describes implemented features only).
