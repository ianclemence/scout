# CHANGELOG

## v0.1.0 (2026-09-21)

Initial release. Local-first work acquisition agent (Go, SQLite, single binary, ARM64):

- Structured professional profile with CV import (txt/md/pdf-text) and evidence store
- Opportunity model with dedup (source + ID + fingerprint), local search
- Matching engine: deterministic gates + structured heuristic dimensions + optional LLM enrichment
- Proposal drafting grounded in evidence, configurable style, client questions
- Human approval state machine; dry-run mode; Connects guardrails
- MCP client (official Go SDK, Streamable HTTP) with Upwork capability discovery + normalization
- Scout MCP server (stdio + Streamable HTTP): status, search, review, match, applications, approvals, pipeline, profile
- Web UI + JSON API + health/readiness; first-run auth; CLI covering full loop
- LLM providers: OpenAI, Anthropic, Ollama, OpenAI-compatible; per-role routing
- Systemd deployment, backup/restore, docs (README/ARCHITECTURE/SECURITY/CONFIGURATION)
