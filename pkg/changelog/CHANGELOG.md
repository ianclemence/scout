# Changelog

Newest first. Scout shows new entries on first launch after an update;
`scout changelog` (or `scout update --notes`) reprints them.

## [0.13.11] - 2026-09-22

- Tables that cannot fit the terminal width no longer fall back to raw
  pipes: rows render as labeled field groups that stay readable at any
  width. Long words break at URL and slug boundaries without splitting
  multibyte runes.
- Inline styling split across the model's line breaks now rejoins before
  rendering, on both the live stream and the committed reply.

## [0.13.10] - 2026-09-22

- Empty sessions are no longer tracked. Launching Scout without messaging
  used to leave a contentless row in the resume list; session lists now
  show only sessions with messages, and quitting sweeps message-less rows
  (this run's if untouched, plus any legacy orphans).

## [0.13.9] - 2026-09-22

- Tables no longer chop long words mid-word. Over-wide values break at
  URL and slug boundaries (`/`, `-`) and never split a multibyte rune,
  so hyphenated terms and links stay readable and the grid stays aligned.

## [0.13.8] - 2026-09-22

Terminal parity with Ghost.

- Replies stream line-by-line into the conversation as they arrive,
  styled like committed answers — no more watching a fixed preview and
  getting the whole answer at the end. Completion prints only the
  remaining tail, never the full text again.
- Your messages show as `You ┃ text` on one row, with continuation pipes
  aligned beneath.

## [0.13.7] - 2026-09-22

Fresh start.

- **`scout reset`** wipes Scout's saved data and starts it fresh: every session
  and message, profile and CV evidence, opportunities, evaluations, proposals,
  applications, messages, learned preferences, trajectories, and history. It is
  irreversible and requires `--yes`; without it, Scout prints exactly what would
  be destroyed and deletes nothing.
- **Credentials are kept by default.** A normal reset preserves your provider
  API keys and configured connectors, so it does not force a re-login.
  `scout reset --all --yes` also clears stored keys, connector sign-in, the
  local master key, and the workspace overlay.
- The database is vacuumed after a reset, so a wiped install does not keep its
  old size.

## [0.13.6] - 2026-09-22

Evaluation substrate and an external Jev evaluation loop.

- **Full tool results are now recoverable for evaluation.** `tool_audit`
  summaries are truncated to 500 chars, which made external grounding checks
  meaningless (the same grounded answer scored unsupported=0.07 with the full
  result and 0.95 with the summary). A bounded `tool_results` table (migration
  v7; 256 KB/row cap, pruned to 500 rows) stores the full result. Runtime
  behaviour is unchanged.
- **Each agent run has a run id** (migration v8): tool results are tagged with
  the turn that produced them, so an evaluator matches evidence to a run exactly
  instead of by timestamp.
- **External evaluator:** `cmd/scout-eval` + `pkg/jeveval` + `scripts/eval-corpus.sh`
  read Scout's recorded runs, run deterministic checks and grounded Jev
  questions, store findings, and compare before/after. Scout never imports it;
  Jev stays out of Scout's runtime. See `docs/EVALUATION.md` and
  `docs/EVAL-REPORT.md`.

## [0.13.5] - 2026-09-22

Honest model list, responsive commands.

- **The model picker only offers models usable right now.** A hardcoded local
  builtin (`qwen3:0.6b`) is gone — a local model must be discovered from the
  running runtime, never assumed. Embedding-only models (e.g.
  `nomic-embed-text`) are excluded from the conversation picker, and cached
  local models the runtime no longer has are pruned, so a removed model can
  never linger.
- **`/approvals` no longer does nothing.** With nothing pending it printed
  nothing at all, because the flush command was discarded; it now shows a
  clear message (and the same fix covers `/applications`, `/sessions`,
  `/opportunities`, `/sources`).
- **`/discover` streams instead of freezing.** It ran synchronously and
  blocked the interface until the source search finished, then dumped the
  result. It now runs in the background with the dock's spinner and activity
  line, and reports a labeled summary when done.
- Empty states read as prose ("No applications yet…") rather than a bare
  notice bullet.

## [0.13.4] - 2026-09-22

Session switching that actually switches.

- **Fixed: picking a session now opens it.** Selecting a session printed a
  bare "· session <id>" notice instead of switching, because the switch
  callback was bound only after the picker had opened. Callbacks are now bound
  at startup and before any command runs, so every palette-opened picker
  (sessions, model, thinking, approvals, sources) has a live action.
- **The session selector reads like pi's.** It is titled "Resume session",
  marks the current session, and each row leads with a human title (the
  session name, or the first user message when unnamed) plus a relative age
  (now, 5m, 2h, 3d, 2w, 3mo, 1y) and the model — instead of a raw id.
- Selecting a session renders its prior conversation into the transcript
  immediately, so you see the chat you just opened.

## [0.13.3] - 2026-09-22

Visible search fields everywhere.

- **Every search input now shows where you are typing.** Model picker, list
  pickers (`/sessions`, `/opportunities`, `/applications`, `/sources`,
  `/approvals`, `/skills`, `/tools`), and the login/logout provider lists share
  one search field: a prompt glyph (`›`) and a dim placeholder
  ("type to filter…") when empty, replaced by your query with a cursor block.
  The field is never a blank line, so it is always clear it accepts input.

## [0.13.2] - 2026-09-22

Session history on open, and consistent styled command output.

- **Opening or resuming a session shows the conversation.** Prior user and
  assistant turns are rendered into the transcript (with an "Earlier in this
  session" divider) exactly as they looked when first exchanged, so switching
  sessions reveals the chat instead of a blank screen. Works on startup
  (`scout resume <id>`), the `/sessions` picker, and `/resume` — matching how
  opencode and pi restore a session.
- **One output style across every command.** A shared terminal presenter
  (`pkg/termui`) gives the scriptable CLI the same visual language as the TUI:
  dim secondary text, emphasized values, semantic glyphs (`✓`/`✗`), titled
  sections, and aligned tables with a styled header and rule. `status`,
  `providers`/`models`, `opportunities`, `sessions`, `integrations`, `skills`,
  and `tools` all use it, and styling is disabled automatically when output is
  piped or redirected so machine consumers get clean text.

## [0.13.1] - 2026-09-22

Human-readable opportunity views and a corrected Upwork submission flow.

- **Opportunity detail and analysis read as labeled prose, not field dumps.**
  `[src-upwork]`, the internal `analyzed` status, and `credits 0` are gone from
  the human view — replaced by bold labels (`Budget`, `Match`, `Assessment`),
  a humanized source (`Upwork`), and pay shown as a rate (`$15–25/hr`). The
  model's richer assessment is now its own rendered "Model notes" section
  instead of being truncated mid-word into a one-line reason, and markdown is
  styled rather than shown as literal `**`.
- **Connector auth reads plainly.** Low-level states like `token_stored` now
  surface as `connected` or `sign in needed`; the raw value stays in JSON for
  tooling. `/status` renders as a labeled block.
- **Upwork submission now matches the live MCP contract.** The proposal draft
  uses `job_reference`, requires a numeric bid, performs the mandatory
  invitation/existing-proposal pre-check (avoiding Upwork's VJ-JA-10
  rejection), and surfaces the one-time payment-protection policy
  acknowledgment instead of silently failing. Messages use the correct
  `send` / `send_to_user` params. A submission blocked by a missing rate now
  explains the exact number needed and offers to set it; a zero Connects
  balance is stated with the cost rather than attempted.
- **A manual, opt-in live Upwork probe** (`go test -tags liveupwork`) exercises
  all read-only capabilities against the real server and stops at the draft
  step, so no Connects are spent and nothing is sent.

## [0.13.0] - 2026-09-22

Terminal reading experience: tables, rich text, and a responsive dock.

- **Tables are box-drawn grids.** A markdown table now renders like the
  opencode CLI: a `┌─┬─┐` top border, a bold header row, a separator between
  every row, and a `└─┴─┘` bottom, with dim borders. Columns size to their
  natural width and shrink to fit the terminal, wrapping long cells; a table
  that cannot form a stable grid falls back to raw markdown instead of
  overflowing the screen. Widths are measured in display width, not bytes, so
  em-dashes and other wide characters stay aligned.
- **The reply is readable as it forms.** The area above the composer is now a
  stable anchor: one blank row when idle, growing up to a capped block while a
  reply streams, then collapsing back the instant the turn commits. The dock no
  longer changes height at the start or end of a turn, so the composer and
  footer stay put — and you can read the answer while it streams instead of
  watching it appear all at once.
- **Markdown syntax is concealed.** Fence markers no longer appear as literal
  ``` lines, and stacked blank lines are collapsed so code blocks read cleanly
  without double gaps. Top-level headings stay underlined so hierarchy survives.

## [0.12.0] - 2026-09-22

Readable streaming and complete answers.

- **The live reply is readable as it forms.** The dock now grows a multi-line
  markdown block instead of showing one flattened tail line, and the block is
  bounded so it never pushes the composer around. Renders are coalesced on a
  frame tick, so a fast token stream cannot flicker the frame.
- **Narration no longer masquerades as the answer.** In a multi-turn tool loop,
  each turn's prose previously concatenated with the next, so the transcript
  read as a wall of "Let me pull your CV…". The live buffer now resets each
  turn; a turn's prose segment is sealed when a tool starts, and only the turn
  that ends without a tool call is committed as the answer. Per-turn preambles
  are shown live and then discarded, never stored as findings.
- **`analyze_opportunities`: evaluate every stored opportunity in one pass.**
  A new batch tool runs the deterministic filter and heuristic match over all
  stored opportunities, returns a compact best-first ranked table, and defers
  the slow per-item model enrichment to shortlisted items — so a broad request
  cannot time out or be answered with a sample. `analyze_opportunity` (single)
  is unchanged.
- **Completeness is now a rule.** When the user asks for the full picture, the
  agent must evaluate every stored candidate, not a sample, and must not end
  its turn by asking permission to continue routine read-only work. The 4 KB
  tool-result context cap was raised to 12 KB; when it clips, it now says so
  explicitly rather than dropping the tail silently.
- **Honest budget ranking.** `apply` now requires strong skills, no risk
  signals, and a known, verified budget. With no rate floor configured, pay is
  reported as `unverified` instead of a false "acceptable", the top tier is
  capped, and the agent states that it cannot filter on pay — and offers to set
  a floor rather than inventing one.

## [0.11.1] - 2026-09-21

Learning-signal refinement.

- The preference model ignores generic role/domain words ("developer",
  "build", "data", …) that appear in nearly every posting, so a single
  feedback note cannot penalize an entire category. Learned terms are now
  distinctive skills and domains.

## [0.11.0] - 2026-09-21

Grounding, learning, and evaluation release.

- **Structured CV extraction.** Import now parses a resume deterministically
  into name, summary, skills, experience (company/title/period/bullets),
  education, projects, and contact links — all verbatim from the document. The
  agent gets citable `cv_experience` / `cv_education` / `cv_project` evidence
  instead of one raw blob, so proposals are grounded and matching is no longer
  keyword-only. Nothing is invented: uncertain text is left out.
- **Learned preferences.** Scout learns term-level preferences from the user's
  explicit feedback and notes (`scout feedback`, `/feedback`), adjusts
  deterministic evaluations (an explainable `learned_preference` dimension, and
  upgrade/downgrade of the recommendation), and injects a short, explicit
  "learned preferences" line into the model context. `scout learn` shows
  exactly what was learned. This is model-independent and needs no training.
- **Evaluation harness.** `scout eval` runs a golden decision suite (strong,
  weak, partial, missing-info, suspicious, keyword-trap, budget-floor,
  learned-preference) that adapts to the real profile, plus agent-trajectory
  checks (bounded tool use, recovery, final answer). It exits non-zero on
  failure so it can gate a release.
- **Two real matching fixes surfaced by eval:** skill terms now match on word
  boundaries (`go` no longer matches `google`), and hourly rates are compared
  against the hourly floor instead of the project-budget floor.
- **Task-scoped tool catalog.** The agent is sent only the tools relevant to
  the request (plus a core set and `list_tools`), cutting the per-turn tool
  catalog from ~4.9 KB to ~1.2 KB (~75%).
- **Trajectory logging.** Every agent turn records its request, tools, turn
  count, final answer, and error — the raw material for evaluation and future
  learning.

## [0.10.0] - 2026-09-21

Upwork adapter (live discovery).

- **Scout now searches Upwork for real.** A dedicated Upwork dialect adapter
  resolves the freelancer `org_uid` from `list_accounts`, calls `find_jobs`
  with `{action:"search", org_uid, params:{…}}`, and normalizes the results
  into Scout's opportunity model — title, description, hourly/fixed budget,
  skills, URL, category, and a stable fingerprint. `/discover` and
  `scout discover <query>` store live listings, so `/opportunities` →
  `/analyze` → `/proposal` works without manual `opportunity add`.
- **Listing detail** maps to `find_jobs {action:"get"}` (contract terms,
  category, client country).
- **Submit and message** route through Upwork's `manage_proposals` +
  `confirm_preview` and `send_message`, still behind Scout's approval gate.
- **A dialect seam, not a one-off.** `sources.NewAdapterFor` picks a dedicated
  adapter by connector; everything else uses the generic MCP adapter. LinkedIn,
  JobsDB, and other providers add an adapter + matcher without touching the
  generic path, the agent, or the tools.
- Discovery tool gains `title`, `job_type`, `rate_min`, and `rate_max`.

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

