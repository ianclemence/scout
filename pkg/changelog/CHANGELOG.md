# Changelog

Newest first. Scout shows new entries on first launch after an update;
`scout changelog` (or `scout update --notes`) reprints them.

## [0.7.2] - 2026-09-21

Response rendering release.

- Welcome card no longer injects release notes; use `/changelog`.
- Model responses word-wrap to the terminal (no truncation); notices, tool, and approval rows wrap too.
- Markdown roles use Scout brand colors (accent headings/links/bullets, gold emphasis, green code, muted quotes).

## [0.7.1] - 2026-09-21

Command behavior and parity release.

- `/opportunities` opens a picker; Enter offers Analyze fit / Draft proposal / Show detail.
- `/skills` Enter loads the workflow; `/applications` Enter shows detail.
- `/changelog` added; bare `/resume` lists then prompts; `/feedback` validates signals.
- `/thinking` offers the full reference set (off/minimal/low/medium/high/xhigh/max) with per-provider mapping.
- Approval wording is honest: approving records your decision; external submission needs a connected source.

## [0.7.0] - 2026-09-21

Terminal interactivity and self-update release.

### Interactive terminal commands
Commands now open real selectors instead of printing text:
- **`/thinking`** opens a reasoning-level picker (current level marked, each level described, type-to-filter). `/thinking <level>` sets directly and validates, listing the available levels on a bad value.
- **`/sessions`** and **`/resume`** open a searchable session picker; Enter switches session in place.
- **`/approvals`** opens an approval picker: Enter approves, Ctrl+R rejects — the trust boundary without typing an id.

### Verified, user-scoped updates
`scout update` installs a verified release and never builds in place:
- targets: `--self`, `--models`, `--all`, `--check`, `--notes`, `--force`, `--dry-run`, `--channel release|dev`, `--version V`
- resolves the tag, downloads the asset, verifies sha256, smoke-tests, then atomically swaps the binary and restarts the user service
- skips when already current; `--channel dev` builds the local checkout with the version injected (no more `dev`)
- no sudo: installs to `~/.local/bin` and uses `systemctl --user`

### What's new
Embeds a changelog and shows only unseen entries on the first launch after an update; `/changelog` reprints them.

### Also
- `/login` offers account and API-key methods (matching the reference UI)
- model roles collapsed to `conversation` + `worker` (worker inherits the session model)
- model picker offers only configured providers
- idle footer justified as `/ commands … esc quit`

Full history: git log v0.6.0..v0.7.0

## [0.6.0] - 2026-09-21

Terminal interaction release.

### Staged login
`/login` is now a guided flow: pick an authentication method, choose a provider from a searchable list with live status (`✓ configured` / `• unconfigured`), then enter the key in a masked dialog. `/login <provider>` jumps straight to that provider's prompt. Keys are encrypted (AES-256-GCM) and never echoed, logged, or displayed.

### Logout
`/logout` opens a selector of stored credentials only. It removes exactly what `/login` saved; environment variables and config are untouched.

### Scoped models
`/scoped-models` enables, disables, and orders the models used when cycling with `Ctrl+P`. Toggle single models, `Ctrl+P` to toggle a provider, `Ctrl+A`/`Ctrl+X` to enable or clear (scoped to the active search), `Alt+↑/↓` to reorder, and `Ctrl+S` to persist the selection. Session-only until saved.

### Searchable model selector
`/model` opens a searchable picker with an `all`/`scoped` scope toggle (`Tab`). `Enter` switches the session model; `Ctrl+S` sets the conversation default. An exact `provider/model` argument switches immediately.

### Interactive terminal polish
Every selector supports type-to-filter, including pasted text. Footer hints change per active dialog. `/keys`, the welcome card, and command help describe the new flows. Line-mode fallbacks exist for non-TTY use.

### Cleanup
Removed all references to other coding agents from code, comments, and docs; architecture decisions are now documented tool-agnostically.



## [0.5.0] - 2026-09-21

Ghost-style project maturity release.

### User workspace
Tracked template ships with Scout; `scout init` copies it once into the data dir and never overwrites. Owner notes (SCOUT.md) travel in-context as top-priority rules. Custom skills in workspace/skills/ overlay (and can replace) built-ins.

### Project layout
Ghost-mirrored structure: pkg/ domain packages, flat cmd/scout/ topic files, docs/ topical docs, config example, Makefile with git-tag versioning (`scout version` reports tags). No changelog file — release notes live here.

### Git hygiene
.gitignore documents the tracked-vs-runtime split: code, template, docs and example config are tracked; databases, keys, user workspace copies, history, CVs, logs and binaries never are. Tree scanned clean.


## [0.4.0] - 2026-09-21

Terminal-native AI work acquisition agent.

### Terminal experience
Ghost-mirror TUI: rule-framed composer with live activity (Searching, Analyzing, Drafting), command palette, registry-backed model picker, inline approval cards with risk badges, footer with locality-colored model, full markdown including tables, width-aware wrapping, Today/Yesterday transcript dividers.

### Sessions and commands
In-place /resume and /new with history reload, auto-naming from first message, /name, /export, /copy, /keys. Every slash command executes — /doctor runs full diagnostics in-session, /profile imports CVs. /evidence renamed /cv.

### Auth
Pi-style login/logout: provider picker with live key state, masked in-TUI dialog with verification where providers allow it, stored-only removal that never touches env vars.

### Agent core
18 embedded skills with two-tier loading, 49 typed tools under permission classes with audit logging, per-tool call budgets, approval lifecycle events, secret redaction. Universal opportunity model with local, MCP, and fake source adapters.

Full changelog: CHANGELOG.md

## [0.3.0] - 2026-09-21

Terminal-native AI work acquisition agent.

### Providers
Five first-class providers with verified endpoints and reasoning mapping: OpenAI, Anthropic, Ollama, DeepSeek (api.deepseek.com, deepseek-flash), Moonshot (kimi-k3/k2.6/k2.7-code). Per-provider base-URL overrides.

### Model registry
Built-in catalog plus provider /models discovery plus live Ollama tags, cached in SQLite (scout models refresh). Offline fallback; unknown metadata stays unknown.

### Sessions
Interactive REPL with slash commands, streaming ReAct agent, resume/compact/history, in-session /model picker, per-session /thinking levels, masked /login key storage.

### MCP
Client: remote HTTP and local stdio servers, capability discovery (Upwork first). Server: 16 domain tools over stdio and Streamable HTTP for OpenCode/Codex/Claude.

### Product
Human approval gate on all consequential actions. First-time Upwork flow in README. scout update self-updates; systemd user service for boot start.

Full changelog: CHANGELOG.md

