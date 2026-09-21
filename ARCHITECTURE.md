# ARCHITECTURE

Terminal-native, single Go binary. No browser app, no microservices.

```text
cmd/scout            entrypoints: interactive REPL + one-shot commands + mcp
internal/
  domain/            marketplace-independent types (Opportunity, Proposal,
                     Application, PendingAction, Evidence, WorkSource, Capability)
  config/            defaults <- config file <- env. One coherent model.
  store/             SQLite (pure Go) + PRAGMA user_version migrations (v2)
  csession/          session CRUD, message history, compact support
  secret/            AES-256-GCM master key (SCOUT_MASTER_KEY or 0600 key file)
  llm/               Provider interface: Complete + Stream (SSE). Providers:
                     openai, anthropic, deepseek, moonshot, ollama,
                     openai_compatible. Normalized reasoning levels mapped
                     per provider (reasoning_effort, thinking blocks, think flag)
  registry/          model catalog: builtins + /models discovery + Ollama tags
                     + SQLite cache + offline fallback
  profile/           CV import → structured profile (source of truth) + evidence
  match/             deterministic filters, fingerprints, heuristic dimensions, risks
  agent/             system prompt + proposal/match LLM enrichment helpers
  runtime/           SCOUT CORE: services, tool registry, ReAct agent loop w/ events
  isession/          REPL (readline), slash registry, streaming render,
                     model selector, login/logout, compact, interrupt handling
  mcpclient/         Scout as MCP client (official Go SDK, Streamable HTTP)
  upwork/            adapter: capability discovery + normalization, no scraping
  approve/           approval state machine
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

ReAct over the tool registry: system prompt + catalog; the model emits one
```tool {"name","arguments"} block per turn; results return as untrusted data;
loop ends with a plain-text answer or MaxTurns. Events (agent_start,
turn_start, token, tool_start/end, error, agent_end) drive the transcript
renderer. Context cancel = Ctrl-C interrupt. One bounded retry on provider
failure.

ReAct (not native function-calling) is deliberate: it works uniformly across
OpenAI, Anthropic, DeepSeek, Ollama, and small local models without
per-provider tool-binding code.

## Context layers

System instructions > profile/preferences > relevant evidence > session
history > tool results > external marketplace content (untrusted data, never
instructions). The loop re-labels tool results as data on every turn.

## Comparative review: Pi, OpenCode vs Scout (summary)

OpenCode (v2.0.11, inspected on this Pi) contributed: named MCP servers with
local-command vs remote-URL kinds, global/project config layering, separate
OAuth auth flow (`mcp auth`/`logout`), `auth login/logout/switch` credential
management, `models` listing, SQLite-backed persistence, background service
vs `--standalone`, and non-interactive `run`. Scout adopts: remote + stdio
MCP kinds, layered config (file/env), masked key storage with store-over-env
precedence, registry-backed `models`, SQLite everything. Scout rejects:
background-service architecture (single process on a Pi), plugin system,
project-scoped configs (single-user tool).

| Area | Pi | Scout v0.2 | Adopt / Reject |
|---|---|---|---|
| Terminal UX | custom alt/main-screen TUI framework | scrolling transcript + readline prompt | Adopt transcript + status info; reject alt-screen framework (SSH/scripting cost) |
| Sessions | manager: resume/fork/tree/compact/share | SQLite sessions: create/resume/compact, history file | Adopt resume + compact; reject fork/tree/share (no coding-session need) |
| Providers | registry + generated model catalog + OAuth | interface + 5 providers + env/file/stored keys | Adopt registry + switching; reject generated catalog + OAuth (vendor weight) |
| Agent loop | native tool declarations, streaming events | ReAct JSON blocks, streaming events | Adopt event sourcing + retry + interrupt; adapt tool binding for small models |
| Tools | bash/fs/editing with approval prompts | domain tools; consequential = PendingAction | Adopt compact tool lines; approvals are first-class records, not prompts |
| MCP | extensions via MCP | client (Upwork) + server (Scout) via official SDK | Adopt capability discovery; reject extension/plugin system |
| Config | file + interactive menus + reload | file + env + /model + scout config | Adopt layered model; reject live-reload complexity |
| Secrets | OS keychain/auth storage | encrypted SQLite + env | Adapt: keychain unavailable headless; 0600 + AES-GCM documented |
| Persistence | sqlite session backend + JSONL export | SQLite everything (v2 migrations) | Adopt SQLite; reject JSONL export (add only on demand) |
| Testing | vitest + harness + faux provider | go test + fakeProvider in runtime | Adopt faux-provider loop tests; reject e2e-with-keys (never hit real writes) |
