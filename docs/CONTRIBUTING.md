# CONTRIBUTING

1. `go test ./...`, `go vet ./...`, `gofmt -l .` — clean before review.
2. CLI, session, and MCP server share `pkg/runtime`. No duplicated business rules.
3. Domain stays marketplace-independent; platform logic goes in adapters.
4. Stdlib first; new deps need justification (modest hardware).
5. Never commit secrets, DBs, CVs, history files, or logs. Stage explicit paths; check `git status`.
6. Tests for matching/approval/persistence/loop behavior; `fakeProvider` for agent tests — never hit real marketplace writes.
7. Docs describe the shipped terminal product only.
