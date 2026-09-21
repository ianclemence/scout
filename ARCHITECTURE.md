# ARCHITECTURE

Single Go binary. Boring, reliable, Pi-friendly.

```
cmd/scout            CLI dispatch + serve + mcp entrypoints
internal/
  domain/            marketplace-independent types (Opportunity, Proposal,
                     Application, PendingAction, Evidence, WorkSource, Capability)
  config/            one coherent config; env > db settings > defaults
  store/             SQLite (modernc.org/sqlite, pure Go) + PRAGMA user_version migrations
  secret/            AES-256-GCM master key (SCOUT_MASTER_KEY or 0600 key file)
  llm/               Provider interface; openai, anthropic, ollama, openai_compatible
  profile/           CV import → structured profile (source of truth) + evidence
  match/             deterministic filters → heuristic dimensions → dedup fingerprints
  agent/             orchestration + SystemPrompt; external content = untrusted data
  mcpclient/         Scout as MCP client (official Go SDK, Streamable HTTP + auth header)
  upwork/            adapter: capability discovery + job normalization, no hard-coded tools
  approve/           approval state machine (draft→pending→approved/rejected→executed/failed)
  mcpserver/         Scout as MCP server: domain tools, stdio + Streamable HTTP
  httpapi/           web UI (templates, no JS framework) + JSON API + /healthz /readyz
web/templates, web/static   server-rendered UI, vanilla CSS
migrations           in-code (store.go), PRAGMA user_version=1
```

Core loop: PROFILE → DISCOVERY → FILTER → ANALYZE → MATCH → EVIDENCE →
PROPOSAL → HUMAN REVIEW → APPROVAL → EXTERNAL ACTION → TRACKING → FEEDBACK.

Deterministic work never uses the LLM. The LLM only sees candidates, with
minimal evidence context, and its output is a refinement — the heuristic
evaluation is the floor. Proposals compile without any provider configured.
