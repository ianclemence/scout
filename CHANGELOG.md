# CHANGELOG

## v0.2.0 (2026-09-21)

Terminal-native redesign. The browser UI was removed; Scout is now a CLI/TUI-first agent.

- Interactive `scout` session: readline prompt, slash commands, streaming responses, tool activity lines, Ctrl-C interrupt, history file
- ReAct agent runtime over a domain tool registry, with events, bounded retry, session persistence, resume, and LLM-summarized compacting
- In-session model switching (`/model` selector), provider status, masked `/login` key storage
- Shared Scout Core (`internal/runtime`) powering CLI, session, and MCP server
- MCP server renamed to domain tools (`get_profile`, `search_opportunities`, `analyze_opportunity`, `match_opportunity`, `prepare_proposal`, `get_pipeline`, `approve_action`, …)
- Layered config (defaults < file < env), per-role models, `scout ask --json` for scripting
- SQLite schema v2 (sessions, session_messages)

## v0.1.0 (2026-09-21)

Initial release: profile/CV import, opportunity matching, proposal drafts, approval queue, Upwork MCP client, web UI (removed in v0.2.0).
