# Scout

Self-hosted AI work acquisition agent.

Scout lives in your terminal. It keeps your professional profile, discovers freelance opportunities that genuinely match your skills, explains why they match, prepares evidence-based proposals, and tracks your pipeline — while every consequential action stays under your explicit control.

```
Profile
   ↓
Discover
   ↓
Analyze
   ↓
Match
   ↓
Prepare
   ↓
Approve
   ↓
Apply
   ↓
Track
```

## A session

```text
$ scout
Scout v0.2.0 · ollama/qwen3:0.6b · session sess-1789962
Type /help for commands, or just ask. Ctrl-C interrupts · Ctrl-D exits.

scout› /status
scout v0.2.0 · session sess-1789962 (interactive)
provider ollama · model qwen3:0.6b
profile Ian Clemence · 6 skills
opportunities 1 · pending approvals 0 · applications 0

scout› What opportunities do I have and which match best?
◐ search_opportunities {"query":"go api"}
✓ [{"id":"opp-...","title":"Go SaaS API backend","status":"review"}]
The Go SaaS API backend is your strongest match: 10 profile terms hit,
budget acceptable, one scope question open. Recommendation: review.

scout› /proposal 1
Drafting proposal for Go SaaS API backend…
[proposal draft with evidence refs and client questions]

scout› /approvals
ACTION REQUIRES APPROVAL
  submit_proposal → opp-17899620  [risk high]
  /approvals approve opp-17899620 · /approvals reject opp-17899620
```

Scout drafts and recommends. You decide. Scout never submits, spends credits, sends messages, or accepts offers on its own.

## Install (Raspberry Pi 5, ARM64)

```sh
git clone https://github.com/ianclemence/scout.git && cd scout
go build -o scout ./cmd/scout
./scout init
./scout               # interactive session
```

Single Go binary, SQLite, no services to operate. Optional systemd unit for the MCP server in `scripts/`. Access over SSH or Tailscale; never expose the MCP port publicly.

## Profile and evidence

```sh
scout profile import cv.txt     # txt, md, or text PDF
scout profile show
```

The CV becomes evidence. The structured profile is the source of truth and is editable. Every proposal cites the evidence it uses; unsupported claims never appear as facts.

## Models and providers

Scout is not locked to any vendor. Per-role models (screening, analysis, proposal, conversation, deep_analysis), switchable without restart:

```text
scout› /model
  1  ollama/qwen3:0.6b      role:conversation  ← current
  2  openai/gpt-4o-mini     role:analysis
  ...
```

Supported: OpenAI, Anthropic, DeepSeek, Moonshot, Ollama, any OpenAI-compatible endpoint. Bring your own keys:

```sh
export OPENAI_API_KEY=…        # or ANTHROPIC_API_KEY, DEEPSEEK_API_KEY, MOONSHOT_API_KEY
scout login moonshot           # stored encrypted instead
```

Provider details (verified against current docs): DeepSeek (`https://api.deepseek.com`, `deepseek-flash`/`deepseek-v4-pro`); Moonshot (`https://api.moonshot.ai/v1`, `MOONSHOT_API_KEY`, `kimi-k3`/`kimi-k2.6`/`kimi-k2.7-code`). `scout models refresh` discovers provider models via `/models` (or Ollama tags) into a local cache; offline it falls back to cache, then the built-in catalog. Unknown metadata is shown as unknown, never invented.

Reasoning is a per-session level (`/thinking off|low|medium|high|max`), mapped to each provider's real controls (e.g. `reasoning_effort`, thinking blocks, Ollama `think`). Credential precedence: credential store, then environment.

Local-first option: run everything on Ollama (`qwen3:0.6b` works on the Pi) and nothing leaves the device except marketplace calls you approve.

## Matching

Deterministic filters first (budget minimums, credit caps, excluded work, dedup), then structured evaluation (skills, budget, scope, client signals, risks) with optional LLM enrichment. Output explains *why* — never a single magic score.

## Approvals

Read, search, analyze, and draft freely. Anything consequential — submit, spend credits, send, accept, fund — becomes a `pending_approval` item with risk level. Approve or reject in the session, via `scout approvals`, or through the MCP server. `SCOUT_DRY_RUN=1` disables external writes entirely.

## MCP

**Scout as client** (consumes work platforms, Upwork first):

```sh
scout integrations add Upwork https://mcp.upwork.com/mcp
scout integrations test Upwork   # capability discovery, read-only
```

Only official MCP/API interfaces. No scraping, no private endpoints, no automation that bypasses platform confirmation.

**Scout as server** (drives OpenCode, Codex, Claude, ChatGPT, Cursor):

```sh
scout mcp              # stdio
scout mcp serve        # Streamable HTTP on 127.0.0.1:3210
```

Domain tools: `get_profile`, `search_opportunities`, `discover_opportunities`, `analyze_opportunity`, `match_opportunity`, `prepare_proposal`, `list_applications`, `list_messages`, `get_pipeline`, `list_pending_approvals`, `approve_action`, `reject_action`, `get_status`, plus evidence/sources introspection.

## Scriptable CLI

Every workflow works non-interactively, with `--json` where it matters:

```sh
scout discover
scout opportunity show <id> | scout analyze <id> | scout proposal <id>
scout approvals list && scout approvals approve <id>
scout ask --json "summarize today's pipeline"
scout doctor
scout backup scout.db.bak && scout restore scout.db.bak
```

## Architecture

```text
              HUMAN
                │
           Scout CLI/TUI ── interactive session + one-shot commands
                │
          Scout Core (internal/runtime)
           ├── Profile/Memory ── SQLite
           ├── Opportunity engine ── filter → analyze → match → evidence
           ├── Agent runtime ── ReAct loop, tools, streaming, interrupt
           │       ├── LLMs (openai/anthropic/deepseek/ollama/compatible)
           │       └── Integrations (Upwork MCP, generic MCP)
           └── Scout MCP server ── stdio + Streamable HTTP
                       ├── OpenCode   ├── Codex   ├── Claude
```

Design notes (studied the Pi coding agent; adapted, not copied):

- **Event-sourced agent loop** (`agent_start → turn → token/tool events → agent_end`) driving a scrolling transcript with compact tool lines (`◐`/`✓`/`✗`).
- **Declarative slash-command registry** shared by the session; one-shot CLI calls the same Core.
- **Session persistence** (SQLite) with resume, history file, and LLM-summarized compacting.
- **Provider/model registry with in-session switching** (`/model` selector, `/login` for keys).
- Deliberate divergences from Pi: no alt-screen TUI framework (plain transcript fits SSH + scripting); ReAct tool blocks instead of native function-calling (portable across tiny local models); no extension/plugin system (single binary on a Pi); deterministic matching core instead of model-only judgment.

See [ARCHITECTURE.md](ARCHITECTURE.md), [SECURITY.md](SECURITY.md), [CONFIGURATION.md](CONFIGURATION.md).

## Privacy

Profile, opportunities, and credentials stay in SQLite on your device. Only the evidence needed for a task is sent to your chosen LLM. Nothing reaches a marketplace except through an approved action.

## Development

```sh
go test ./... && go vet ./... && gofmt -l .
```

## Roadmap

- Upwork OAuth browser flow inside the terminal session
- Offer/contract/message lifecycle sync where the MCP exposes it
- Explicit feedback → preference rules (visible, never hidden)
- More legitimate marketplace adapters via MCP

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT — see [LICENSE](LICENSE).
