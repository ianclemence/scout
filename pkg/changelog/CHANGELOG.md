# Changelog

Newest first. Scout shows new entries on first launch after an update;
`scout changelog` (or `scout update --notes`) reprints them.

## [0.9.0] - 2026-09-21

MCP account sign-in.

- **Connect Upwork (and any remote MCP host) with OAuth 2.1.**
  `scout integrations login Upwork` / `/sources login Upwork` performs the
  full flow: protected-resource + authorization-server discovery, dynamic
  client registration, PKCE authorization code against a `127.0.0.1` loopback
  callback, and token exchange. The authorization URL opens in your browser;
  a paste field handles a browser on another machine. Tokens are stored
  encrypted and refreshed automatically.
- Token resolution now transparently refreshes a connector's OAuth credential
  when it is near expiry, so probes and discovery keep working.
- The `/sources` picker gains a **Sign in** action for remote connectors;
  `scout integrations login <name>` is the CLI equivalent (paste supported).
- New `pkg/mcpauth` package: discovery (RFC 9728/8414), dynamic client
  registration (RFC 7591), PKCE (RFC 7636), loopback callback, manual paste,
  and refresh.

## [0.8.0] - 2026-09-21

Model selector redesign, fuzzy search everywhere, and sign-in simplification.

- **The `/model` selector now follows the Pi coding agent.** A bordered panel
  with a live fuzzy search (provider-first ranking), a list sorted current →
  default → provider, a `→` cursor, `✓` current marker, `[provider]` badge and
  `· default` marker, a scroll indicator, and a `Model Name:` line. Opening it
  refreshes model catalogs in the background (15s bound) and reports the
  result. `/model <ref>` resolves exact references the Pi way: canonical
  `provider/id`, split forms, or a unique bare id.
- **Fuzzy search everywhere.** The model selector's fuzzy matcher is now used
  by every searchable surface: the `/` command palette, the list pickers
  (sessions, approvals, sources, opportunities, applications, thinking), and
  the login provider selector — fuzzy on the label, with a substring fallback
  over secondary text.
- **Streaming preview without a marker.** The live response preview no longer
  appends a `▍` cursor; only the assistant text is shown, and the idle dock no
  longer reserves an empty preview line.
- **Consistent spacing.** The pickers and login panel use one blank line
  between sections (title, search, list, footer), matching the model selector.
- **Account sign-in removed.** `/login` goes straight to the provider selector
  and a masked API-key prompt; "Sign in with an account" (the Anthropic OAuth
  flow) is gone, along with `pkg/oauth`. The Anthropic provider still honors a
  subscription token supplied as a key.

## [0.7.9] - 2026-09-21

Command-surface trim.

The interactive session had grown to 35 commands, many of which described
Scout's internals or duplicated others. The session now keeps the core loop
and the CLI keeps the rest.

- **Removed:** `/providers`, `/scoped-models`, `/skills`, `/tools`, `/copy`,
  `/keys`, `/session`, `/pipeline`, `/cv`, `/clear`, `/changelog` (still
  `scout changelog`), `/inbox`.
- **Merged:** `/cv` → `/profile evidence`; `/pipeline` → `/applications`
  (counts printed above the list).
- **`/discover` is now real:** it searches every connected source, stores new
  opportunities (deduplicated by source identity), and reports per-source
  failures instead of only counting local rows.
- **Scoped models and Ctrl+P cycling are gone.** `/model` is the single model
  surface (searchable, Ctrl+S sets the default); the scope state and selector
  were removed.
- The remaining 23 commands are grouped Work / Decide / You / Connect /
  Session in `/help` and the palette.

## [0.7.8] - 2026-09-21

Account sign-in and terminal-polish release.

- **Account sign-in (Anthropic Claude Pro/Max).** `/login` → "Sign in with an
  account" runs the same PKCE authorization-code flow the Pi agent uses: a
  loopback callback on `127.0.0.1:53692`, the authorization URL opened in your
  browser, and a paste field for a remote browser. Tokens are stored encrypted
  and refreshed transparently; requests use Bearer auth with the Claude Code
  beta headers and identity block. API-key sign-in is unchanged.
- **Pi-style command palette.** Rows show the command name and a one-line
  description; the argument hint folds in as `<hint> — description`. The group
  tags (`[Connect]`, `[Session]`) are gone, and the primary column sizes to the
  widest visible command.
- **Command results are Scout's answer.** Local command output is rendered in
  the same wrapped-prose style as a model reply rather than as an aligned data
  table. `/tools` now lists tools by permission class as readable bullets.
- **Consistent argument hints.** Optional `[level]`/`[provider]`-style brackets
  are replaced with concrete `<placeholder>` hints, and descriptions no longer
  repeat the command's own invocation.
- **Cleaner welcome card.** The get-started hints are aligned as one centered
  block with accent-coloured commands, and configured work sources sit on their
  own labelled line.

## [0.7.7] - 2026-09-21

Self-update fix.

- A downloaded release asset is now made executable (chmod 0755) before the
  checksum check and smoke test. Release assets are stored without the
  executable bit, so `scout update` previously failed with
  `staged binary failed its smoke test: permission denied`.

## [0.7.6] - 2026-09-21

Connector, reasoning-hygiene, and terminal release.

- **Reasoning never surfaces.** A model's chain-of-thought is parsed and
  discarded at the provider boundary: OpenAI-compatible `reasoning_content` /
  `reasoning` / `reasoning_text` fields are ignored, Anthropic `thinking`
  blocks and `thinking_delta` events are dropped, Ollama `message.thinking`
  is dropped, and inline `<think>`/`<thinking>` tags are stripped even across
  token boundaries. The stream carries only answers.
- **Thinking defaults to off.** New sessions start at `off`; DeepSeek and
  Moonshot are told `thinking: {type: "disabled"}` so a hybrid reasoning model
  does not spend a turn thinking by default. `/thinking` changes it per session.
- **Neutral status label.** The activity line says "Working" rather than
  "Thinking" when no tool is running, since reasoning is off.
- **Fixed a session hang.** Asking about MCP or any source tool could deadlock:
  the source registry read credentials while a database cursor was still open
  on a single-connection SQLite handle. Building the registry no longer queries
  inside an open row cursor, and a regression test locks the invariant.
- **A real connector surface.** `/sources` (alias `/integrations`, CLI
  `scout integrations`) lists every configured work source with its kind,
  endpoint, enabled state, auth state, and discovered capabilities — and lets
  you test, add, enable/disable, or remove one from a picker. The agent's
  `list_sources` / `get_source_capabilities` / `source_health` tools return the
  same facts, so "what MCP is configured?" is answered honestly.
- **Source names resolve.** `discover_opportunities` accepts a source id,
  name, or prefix ("upwork" → "src-upwork"), and discovery over all sources
  no longer silently skips un-probed MCP sources.
- **Bounded probes.** Source discovery has a hard deadline even when the MCP
  SDK ignores context cancellation mid-dial; a dead endpoint reports
  `unavailable` instead of hanging the turn.
- **Correct capability mapping.** `submit_proposal`-style tools are classified
  as submit, not draft, so approved actions route to the right tool.
- **Executable last mile.** An approved submission or message dispatches to the
  source's own discovered tool; a source without the capability says so.
- **Better CV import.** PDF names are extracted from the leading words before
  the first contact marker, and skills are matched across PDF kerning artifacts
  ("T yp eScript" → TypeScript) without guessing.
- **Grouped commands.** `/help` and the palette organize commands by job
  (Work, Decide, You, Connect, Session).
- **Startup names your sources**, and the footer flags a source that needs auth.

## [0.7.5] - 2026-09-21

Stream-hygiene release.

- The ReAct tool block is never streamed: the live preview shows only the visible reply, not the tool JSON.
- `source_health` (and source probes) are bounded to 8s so a dead or slow MCP endpoint cannot stall a turn.

## [0.7.4] - 2026-09-21

Activity and log-hygiene release.

- The composer's live status names the activity in product language: "Working", "Searching work…", "Analyzing fit…", "Drafting a proposal…" — no tool names, no tool count.
- Raw tool names/arguments and tool output no longer appear in the chat (TUI, line mode, or `scout ask`).
- The TUI silences stderr and the std logger so dependency logs can never paint over the composer.

## [0.7.3] - 2026-09-21

Transcript styling release.

- Removed the Today/Yesterday/date dividers from the Scout transcript.
- Entries flush as one block separated by a blank line.
- Command output stays a quiet notice entry, distinct from model replies.

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

Project maturity release.

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
Client: remote HTTP and local stdio servers, capability discovery (Upwork first). Server: 16 domain tools over stdio and Streamable HTTP for any MCP client.

### Product
Human approval gate on all consequential actions. First-time Upwork flow in README. scout update self-updates; systemd user service for boot start.

Full changelog: CHANGELOG.md

