# CHANGELOG

## v0.4.0 (2026-09-21)

- Pi-style login/logout: provider picker with live key state, masked in-TUI dialog, verification where providers allow it, stored-only removal
- Commands execute: /doctor runs full diagnostics in-session, /profile imports CVs, plus /name, /export, /copy, /keys; /evidence renamed /cv
- Pi-style sessions: in-place /resume and /new, auto-naming from first message
- Ghost-hardened core: secret redaction, per-tool call budgets, approval lifecycle events, richer doctor (disk, service, skills/tools counts)
- Ghost-mirror TUI: rule-framed composer with live activity, palette, model picker, approval cards, footer, markdown with tables, width-aware wrapping
- Skills library (18) with two-tier loading, 49 typed tools with permission classes and audit log, source adapters (local, MCP, fake)
- Universal opportunity model, research and GitHub evidence tools, application validation

- Five first-class providers: OpenAI, Anthropic, Ollama, DeepSeek, Moonshot — each with verified endpoints, model defaults, and provider-specific reasoning mapping
- Model registry: built-in catalog + provider `/models` discovery + live Ollama tags + SQLite cache + `scout models refresh`, offline fallback, unknown metadata stays unknown
- Normalized reasoning levels (`off/low/medium/high/max`) per session (`/thinking`), mapped per provider (reasoning_effort, thinking blocks, Ollama think flag)
- Credential precedence: credential store > environment; masked `/login`; `scout login`
- MCP client supports local stdio/command servers in addition to remote HTTP; `integrations add --command`
- Registry-backed `/model` picker with context/reasoning metadata

## v0.2.0 (2026-09-21)

Terminal-native agent: interactive session with slash commands, streaming ReAct runtime over a domain tool registry, session persistence/resume/compact, in-session model switching, shared Core for CLI/session/MCP server, domain-named MCP tools, layered configuration.

## v0.1.0 (2026-09-21)

Initial release: profile/CV import, opportunity matching, proposal drafts, approval queue, Upwork MCP client.
