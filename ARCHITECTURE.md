# ARCHITECTURE

Terminal-native, single Go binary. No browser app, no microservices.

```text
cmd/scout            entrypoints: interactive REPL + one-shot commands + mcp
internal/
  domain/            marketplace-independent types (Opportunity, Proposal,
                     Application, PendingAction, Evidence, WorkSource, Capability)
  config/            defaults <- config file <- env. One coherent model.
  store/             SQLite (pure Go) + versioned migrations (v5)
  csession/          session CRUD, message history, compact support
  secret/            AES-256-GCM master key (SCOUT_MASTER_KEY or 0600 key file)
  redact/            secret masking for logs, errors, and audit rows
  llm/               Provider interface: Complete + Stream (SSE). Providers:
                     openai, anthropic, deepseek, moonshot, ollama,
                     openai_compatible. Normalized reasoning levels mapped
                     per provider (reasoning_effort, thinking blocks, think flag)
  registry/          model catalog: builtins + /models discovery + Ollama tags
                     + SQLite cache + offline fallback
  profile/           CV import → structured profile (source of truth) + evidence
  match/             deterministic filters, fingerprints, heuristic dimensions, risks
  agent/             core agent instructions + proposal/match LLM helpers
  runtime/           SCOUT CORE: services, typed tool registry, permissions,
                     audit log, ReAct agent loop w/ events, skill selection
  skills/            18 embedded SKILL.md workflows + registry/selection
  sources/           OpportunitySource interface: local, MCP, fake adapters
  isession/          slash registry + line-mode loop (non-TTY fallback)
  tui/               Bubble Tea session: scrollback transcript, composer,
                     palette, pickers, approval card, footer, markdown
  mcpclient/         Scout as MCP client (official Go SDK; HTTP + stdio)
  upwork/            first MCP adapter helpers (endpoint, discovery mapping)
  approve/           approval state machine + lifecycle events
  mcpserver/         Scout as MCP server on top of runtime tools
  version/
scripts/             systemd unit (MCP server) + install script
```

## Request flow

```text
scout (REPL) ──┐
scout ask ─────┼──▶ runtime.Core ──▶ store (SQLite)
one-shot cmds ─┘         ├── tools (search/analyze/propose/approve/…)
                         ├── agent loop (ReAct, streams via llm.Provider)
MCP server ──────────────┘         ├── providers (cloud or Ollama)
                                   └── integrations (Upwork MCP …)
```

CLI, session, and MCP server call the same Core. No duplicated business rules.

## Agent loop

ReAct over the tool registry: system prompt + selected skill summaries +
compact tool catalog; the model emits one ```tool {"name","arguments"} block
per turn (full skill bodies load via load_skill); results return as untrusted
data; loop ends with a plain-text answer or MaxTurns (8 turns, max 3 calls per
tool per turn). Events (agent_start, turn_start, token, tool_start/end,
error, agent_end) drive the transcript renderer. Context cancel = Ctrl-C
interrupt. One bounded retry on provider failure.

ReAct (not native function-calling) is deliberate: it works uniformly across
OpenAI, Anthropic, DeepSeek, Ollama, and small local models without
per-provider tool-binding code.

## Context layers

System instructions > profile/preferences > relevant evidence > session
history > tool results > external marketplace content (untrusted data, never
instructions). The loop re-labels tool results as data on every turn.

## Comparative review: Pi, OpenCode, Ghost vs Scout (summary)

OpenCode (v2.0.11, inspected on this Pi) contributed: named MCP servers with
local-command vs remote-URL kinds, global/project config layering, separate
OAuth auth flow (`mcp auth`/`logout`), `auth login/logout/switch` credential
management, `models` listing, SQLite-backed persistence, background service
vs `--standalone`, and non-interactive `run`. Scout adopts: remote + stdio
MCP kinds, layered config (file/env), masked key storage with store-over-env
precedence, registry-backed `models`, SQLite everything. Scout rejects:
background-service architecture (single process on a Pi), plugin system,
project-scoped configs (single-user tool).

| Area | Pi | Ghost | Scout v0.3 | Adopt / Reject |
|---|---|---|---|---|
| Terminal UX | custom alt/main-screen TUI framework | Bubble Tea scrollback transcript + dock + footer | same model, Scout theme | Adopt Ghost's layout; Scout colors/commands |
| Sessions | manager: resume/fork/tree/compact/share | threads, contexts, history | SQLite sessions: create/resume/compact, history file | Adopt resume + compact; reject fork/tree/contexts |
| Providers | registry + generated model catalog + OAuth | provider/modelreg + routing | interface + 5 providers + registry + env/file/stored keys | Adopt registry + switching; reject generated catalogs |
| Agent loop | native tool declarations, streaming events | gated capabilities + budgets | ReAct JSON blocks, events, per-tool call budget | Adopt event sourcing + budgets; adapt tool binding |
| Tools | bash/fs/editing with approval prompts | tool registry + permission broker (allow/ask/deny) | domain tools + permission classes + approvals | Adopt broker thinking, always-ask for external; reject auto modes |
| Approvals | prompts | durable permission requests + standing grants | PendingAction records + lifecycle events | Adopt lifecycle events; reject standing auto-grants |
| Secrets | OS keychain/auth storage | vault + redaction | encrypted SQLite + env + redact helper | Adopt redaction; keychain unavailable headless |
| Persistence | sqlite session backend + JSONL export | SQLite + canonical events | SQLite v5 (events + tool_audit + cache tables) | Adopt canonical-event thinking; reject export formats |
| Testing | vitest + harness + faux provider | golden suites + arch tests | go test + fake source + fake provider | Adopt fakes; reject heavy golden harness (for now) |
