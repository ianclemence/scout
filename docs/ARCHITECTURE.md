# ARCHITECTURE

Terminal-native, single Go binary. No browser app, no microservices.

```text
cmd/scout            entrypoints: interactive REPL + one-shot commands + mcp
pkg/
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
  isession/          slash registry + line-mode loop (non-TTY fallback);
                     scoped-model state (filter/cycle/persist)
  tui/               Bubble Tea session: scrollback transcript, composer,
                     palette, approval card, footer, markdown;
                     scoped-models + searchable model selectors;
                     staged login/logout flows (method → provider → key)
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

## Design decisions

Scout is a single-user, single-process tool that must run comfortably on
modest hardware. These choices follow from that:

| Area | Decision | Why |
|---|---|---|
| Terminal UX | Scrollback transcript + dock composer + status footer | Keeps full history while the live area stays small and stable |
| Sessions | SQLite sessions: create / resume / compact | Durable without a daemon; compact keeps context bounded |
| Providers | Interface + 5 providers + registry + env/file/stored keys | One shape for cloud and local models; keys never in config files |
| Model picking | `AllModels` (full registry, resolution) vs `AvailableModels` (configured only, selection) | Selectors offer only usable models; builtins stay available offline and for exact references |
| Agent loop | ReAct tool blocks + streamed events + per-tool call budget | Provider-agnostic, works on small local models, no per-provider tool binding |
| Tools | Typed registry + permission classes + approvals | Every capability is auditable and gated |
| Approvals | PendingAction records + lifecycle events | Consequential actions are records, not side effects |
| Secrets | AES-256-GCM in SQLite + env + redaction helper | No OS keychain is available on a headless device |
| Persistence | SQLite (events + tool_audit + cache tables) | One file, pure Go driver, no external services |
| Testing | `go test` + fake sources + fake providers | Fast, hermetic, no network in unit tests |
| MCP | Client (remote + stdio) and server (stdio + HTTP) | Official integrations only; other agents can call the same Core |
