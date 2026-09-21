# CHANGELOG

## v0.3.0 (2026-09-21)

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
