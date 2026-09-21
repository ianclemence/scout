# Scout

Self-hosted AI work acquisition agent for discovering, evaluating, and applying to freelance opportunities with human-controlled actions.

Scout helps you find legitimate work that matches your skills, understand each opportunity, prepare a strong tailored proposal, and manage the pipeline — while keeping every consequential action (submitting, spending credits, messaging, accepting offers) under your explicit control.

Scout is **not an Upwork bot**. It is a general-purpose, marketplace-independent work agent. Upwork (via its official MCP server) is the first integration; the domain model, capability system, and connector registry are designed for other work platforms.

## Why Scout

- Job feeds reward speed; good freelancers need *fit*.
- Proposals fail when generic; Scout grounds every proposal in your evidence.
- Autonomous bidders burn Connects and risk bans; Scout is approval-gated by design.
- Your CV and keys should stay on your hardware: Scout is local-first, SQLite-backed, runs on a Raspberry Pi 5.

## What Scout does (v0.1.0)

- Professional profile: CV import (txt/md/pdf-text), structured editing, evidence store
- Opportunity intake: manual add, local search, deterministic dedup (source + ID + fingerprint)
- Matching: deterministic gates (budget, Connects, excluded work) → heuristic dimensions → optional LLM enrichment. Structured output, never a single magic score
- Proposals: tailored drafts with evidence refs + client questions, style configurable
- Approvals: draft → pending_approval → approved/rejected → executed/failed state machine; consequential actions never run silently
- Pipeline: discovered → analyzed → review → proposal → submitted → viewed/replied/interview/offer/contract/won (+ rejected/dismissed/expired/lost/withdrawn)
- Integrations: MCP connector registry with capability discovery (Upwork pre-registered)
- Interfaces: responsive web UI, full CLI (SSH-friendly), Scout MCP server (stdio + Streamable HTTP) for Claude/Codex/OpenCode/ChatGPT/Cursor
- LLM: OpenAI, Anthropic, Ollama, OpenAI-compatible endpoints; per-role models (screening/analysis/proposal/conversation/deep_analysis)
- Safety: secrets encrypted (AES-256-GCM), auth-gated UI, prompt-injection defenses (external content = untrusted data), dry-run mode

## Architecture

```
                    ┌───────────────────┐
                    │      SCOUT        │
                    │    Go backend     │
                    └─────────┬─────────┘
                              │
         ┌────────────────────┼────────────────────┐
         ▼                    ▼                    ▼
      Web UI                CLI              Scout MCP
      (templates)      (cmd/scout)        (stdio + HTTP)
                                                  ├── Claude / Codex
                                                  ├── OpenCode / Cursor
                                                  └── ChatGPT / others
                              │
                       Agent Engine
      ┌────────────────────────┼────────────────────────┐
      ▼                        ▼                        ▼
 LLM Providers          Work Integrations          Local Data
 OpenAI · Anthropic      Upwork MCP (official)      SQLite
 Ollama · OpenAI-comp.   Generic MCP connectors
```

See [ARCHITECTURE.md](ARCHITECTURE.md), [SECURITY.md](SECURITY.md), [CONFIGURATION.md](CONFIGURATION.md).

## Quick start

```sh
# build (ARM64 native on the Pi)
cd scout && go build -o scout ./cmd/scout

./scout init
./scout serve            # http://127.0.0.1:3210
```

1. Open the web UI → set admin password (first run).
2. Profile → upload your CV → review extracted skills → set rates/minimums.
3. Add LLM: `OPENAI_API_KEY=…` / `ANTHROPIC_API_KEY=…`, or use local Ollama (default `qwen3:0.6b`).
4. Integrations → verify Upwork (`scout integrations test Upwork`).
5. Add an opportunity → Analyze → Generate proposal → Request approval → Approve.

CLI equivalents: `scout profile import cv.txt`, `scout analyze <id>`, `scout proposal <id>`, `scout approvals list`.

## Raspberry Pi deployment

```sh
sudo cp scout /usr/local/bin/scout
sudo cp scripts/scout.service /etc/systemd/system/scout.service
sudo systemctl enable --now scout
```

Recommended: bind `127.0.0.1` and expose over **Tailscale** only. Never put the unauthenticated UI on the public internet. See `scripts/install.sh`.

## MCP

**Scout as client:** `scout integrations add Upwork https://mcp.upwork.com/mcp`, then `scout integrations test Upwork` for capability discovery. OAuth tokens are stored encrypted, never logged.

**Scout as server:**
- stdio: `scout mcp` (for Claude Code, Codex, OpenCode)
- HTTP: `scout mcp serve` → `http://127.0.0.1:3210/mcp`

Tools: `scout_status`, `scout_search`, `scout_review_opportunity`, `scout_match_opportunity`, `scout_list_applications`, `scout_review_pending_actions`, `scout_approve_action`, `scout_reject_action`, `scout_pipeline`, `scout_profile`.

## Upwork policy note

Scout uses only the official Upwork MCP (`https://mcp.upwork.com/mcp`). No scraping, no private endpoints, no rate-limit evasion. Proposal submission spends Connects and creates real obligations: it is always approval-gated. Check Upwork's current API & MCP Terms before enabling any scheduled/chained workflows.

## Privacy

Local-first: profile, opportunities, and tokens stay in SQLite on your device. Only the evidence needed for a task is sent to your chosen LLM; nothing is sent to a marketplace except through explicit approved actions. `SCOUT_DRY_RUN=1` disables external writes.

## Development / Testing

```sh
go test ./... && go vet ./... && gofmt -l .
```

## Roadmap (realistic)

- Upwork OAuth browser flow inside Scout (today: token paste + discovery)
- Offer/contract/message lifecycle sync where the MCP exposes it
- Feedback-driven preference learning (explicit rules, not hidden behavior)
- Additional legitimate marketplace adapters via MCP

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT — see [LICENSE](LICENSE).
