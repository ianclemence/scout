# Scout

> **Find work worth doing.**
> *A self-hosted AI work acquisition agent that lives in your terminal — it finds legitimate work, explains why it fits you, drafts the proposal, and never acts without your approval.*

---

## What Scout does for you

Scout runs the job hunt so you don't have to live in the feed:

- **It watches the market for you.** "Find Go + AI backend work on Upwork under my rates." Scout searches, filters out the noise, and keeps a shortlist.
- **It tells you why something fits.** Not a score — skills matched, budget fit, scope clarity, client signals, risks, and the exact profile evidence behind each claim.
- **It drafts the proposal.** Grounded in your real projects, tailored to the posting, with questions for the client. Never generic, never invented experience.
- **It asks before anything consequential.** Submitting, spending Connects, messaging a client, accepting an offer — every one waits for your explicit approval, with the full context to decide.
- **It tracks the pipeline.** Discovered → analyzed → proposed → submitted → replied → interview → offer. You always know what needs attention.

You describe what you want in plain language, in your terminal. Scout does the research. You make the decisions.

---

## Why human control is the point

Most "auto-apply bots" treat your accounts like a slot machine: spray applications, burn credits, risk your reputation and your standing. Scout is built on the opposite rule: **the model drafts, the human decides.**

- **Consequential actions are records, not side effects.** Submit, spend, send, accept, fund — each becomes a pending approval with its risk level. Unknown risk fails closed.
- **Every claim cites evidence.** A proposal sentence exists because a CV section, project, or portfolio item backs it. Unsupported claims never ship as facts.
- **Your data stays on your machine.** Profile, opportunities, keys, and history live in SQLite on your hardware. Cloud models are optional intelligence; local Ollama keeps working offline.
- **Official integrations only.** Scout talks to work sources through their official MCPs/APIs — no scraping, no private endpoints, no platform-rule evasion.

You get leverage on the boring parts. You keep the authority on everything that matters.

---

## How it works

You state a goal. Scout checks your profile, finds fitting work, drafts the application — and files anything consequential as an approval for you to decide.

```
You → Scout → fitting work → draft → YOUR APPROVAL → done → tracked
```

One shared core drives the interactive session, one-shot commands, and the MCP server. Policy lives in the system prompt, workflows in 18 skills, capabilities in 49 permission-gated tools, job sources behind one adapter interface.

---

## Requirements

### Hardware

Reference target: Raspberry Pi 5 (8 GB), 32 GB SD, ARM64. Any Linux ARM64/x86-64 machine works.

### Software

- Go (to build; 1.24+)
- Git + GitHub CLI (for install/update)
- Ollama (optional, recommended for local models): `curl -fsSL https://ollama.com/install.sh | sh`

---

## Quick Start

This is the whole journey on a fresh machine. Each step builds on the last. The worked example uses an Upwork posting — but every step below works the same for any job listing you paste in, and Scout's work-source model is built for more sources than one.

### 1. Install

```bash
git clone https://github.com/ianclemence/scout.git
cd scout
make install
```

`~/.local/bin` is on `PATH` on a standard Pi. From here on, `scout` works anywhere.

### 2. Initialize

```bash
scout init
```

Creates `~/.local/share/scout` (database, key file, history) with locked-down permissions. Then verify:

```bash
scout doctor
```

You want `[OK]` on data-dir and sqlite, and `[OK] ollama` if Ollama is running. Cloud provider keys can come later — Scout is fully usable on the local model first.

### 3. Run as a service (recommended)

```bash
cp scripts/scout.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now scout
```

This keeps the Scout MCP server listening on `127.0.0.1:3210` across reboots (with user lingering on). The interactive session doesn't need it, but other agents — and your future self over SSH — will use it. Check with `systemctl --user status scout`.

### 4. Start your first session

```bash
scout
```

A full-screen chat session opens. Type `/status` — it shows your provider, model, profile state, and counts. On a fresh install the profile is empty; that's step 5.

Type `/` for commands, `Ctrl+L` to switch models, `Esc` to stop a running turn.

### 5. Teach Scout who you are

In another terminal (one-shot commands work while a session is open):

```bash
scout profile import ~/my-cv.pdf   # pdf, docx, md, txt, html, csv
scout profile show
```

Your data lives outside the repo in `~/.local/share/scout`: the database, your workspace (`workspace/` — owner notes, custom skills, imports), history, and key file. `scout init` creates the workspace from a tracked template once and never overwrites it. None of it is committed to git — see `.gitignore`.

Back in the session:

```text
scout› /profile
Profile: Ian Clemence — Senior Go Developer
Skills: go, postgres, docker, react, …
Evidence items: 1 (latest: cv_section:my-cv.txt)
```

Your CV becomes **evidence**. The structured profile is the source of truth Scout reasons from. Set your floor so Scout can filter for you — minimum budget, excluded work, max Connects per application (edit via `scout profile`, or the config file).

### 6. Bring a job posting

Copy a real posting into a file — the example below uses Upwork, but any job description works (full text matters — Scout reads the whole thing, not just keywords):

```bash
scout opportunity add --title "Go SaaS API backend" \
  --description-file job.txt --skills "go, postgres, docker"
```

```text
scout› /opportunities
OPPORTUNITIES (1)
 1  Go SaaS API backend
    opp-… · discovered
```

### 7. Analyze the fit

```text
scout› /analyze 1
Filter: pass=true (passed deterministic gates)
Recommendation: review — skills=strong risks=1
  skills       strong       10 profile terms matched: go, docker, postgres, …
  budget       acceptable   fixed 0–0 vs min 0
  scope        weak         heuristic: description length
  risks        1 signal     very short description — scope unclear
```

Or conversationally: *"Why does this one fit me, and what's the weakest part of the match?"* Plain text goes to the agent loop (with tools); `/commands` run locally without spending a model turn.

### 8. Draft the proposal

```text
scout› /proposal 1
PROPOSAL DRAFT
[tailored draft citing your Go/Postgres/Docker evidence,
 ending with sharp client questions]
Evidence: <ids>   Rate 0 hourly
```

Inspect what's citable: `/cv` shows your resume content and the items a proposal may cite. If a claim isn't supported, it doesn't go in.

### 9. Approve the submission

Requesting submission files an approval instead of acting:

```text
scout› /approvals
ACTION REQUIRES APPROVAL
  submit_proposal → opp-…  [risk high]
  [proposal text, bid, Connects cost]
  /approvals approve opp-… · /approvals reject opp-…
```

This is the trust boundary. Nothing reaches any work source until you say so.

### 10. Connect a work source (Upwork example)

```bash
scout integrations add Upwork https://mcp.upwork.com/mcp
scout integrations test Upwork
```

The test performs read-only capability discovery. Full OAuth sign-in completes in the browser at Upwork's authorization page; the token is stored encrypted in Scout, never logged. Until authenticated, discovery reports what the integration *can* do (search, proposals, messaging, contracts) and Scout maps those into its normalized work-source model.

Once connected, approved submissions execute through the official integration — with the same draft/confirm semantics the platform itself enforces — and the application lands in your pipeline (`/applications`, `/pipeline`). Other MCP-enabled job sources connect the same way (`scout integrations add Name <url-or-command>`); Upwork is simply the first one.

### 11. Keep it fresh

```bash
scout update              # pull + rebuild + reinstall + restart service
scout backup ~/scout-backup.db   # profile, pipeline, and history (encrypted secrets included — keep private)
```

---

## Updating

```bash
cd ~/scout            # your checkout (or set SCOUT_REPO)
scout update          # fetch, refuse dirty trees, skip if current, rebuild, restart
```

`scout update --dry-run` previews. `scout update --force` redeploys regardless. Production runs releases, not working trees.

---

## Commands

### One-shot CLI

| Command | Description |
|---------|-------------|
| `scout` | Interactive session (resume: `scout resume <id>`) |
| `scout ask [--json] "…"` | One agent turn, scriptable |
| `scout status` | Profile, counts, state |
| `scout discover` | Discovery summary (no external writes) |
| `scout opportunities [query]` | List stored opportunities |
| `scout opportunity show <id>` | Detail + evaluation + proposal |
| `scout opportunity add --title T --description-file F` | Add a posting |
| `scout analyze <id>` | Filter + structured fit evaluation |
| `scout proposal <id>` | Draft proposal (no external writes) |
| `scout approvals [list\|approve <id>\|reject <id>]` | Human control queue |
| `scout applications` / `scout inbox` | Pipeline and messages |
| `scout profile show\|import <file>` | Profile and CV evidence |
| `scout providers` / `scout models [refresh [provider]]` | Providers and model catalog |
| `scout login <provider>` | Store API key (masked prompt) |
| `scout integrations [list\|add\|test]` | Work sources and MCP connectors |
| `scout sessions [list]` | Persistent sessions |
| `scout skills [query]` / `scout tools` | Skill workflows and typed tool registry |
| `scout doctor` | Diagnostics (DB, providers, Ollama, disk, version, service) |
| `scout backup <file>` / `scout restore <file>` | Data backup and restore |
| `scout update [--dry-run] [--force]` | Self-update |
| `scout mcp [stdio\|serve]` | MCP server for other agents |
| `scout version` | Version |

### Session slash commands

| Command | Description |
|---------|-------------|
| `/help` | All commands |
| `/status` | Provider, model, thinking, profile, counts |
| `/profile`, `/cv` | Who Scout thinks you are; resume content and citable items |
| `/profile import <path>` | Import a CV without leaving the session |
| `/opportunities [query]`, `/opportunity <id\|#>`, `/discover` | Pipeline intake |
| `/analyze <id>`, `/proposal <id>` | Fit reasoning and drafting |
| `/approvals [approve\|reject <id>]` | Decide consequential actions |
| `/applications`, `/pipeline`, `/inbox` | Track outcomes |
| `/feedback <id> <signal> [note]` | Explicit preference data (visible, never hidden) |
| `/model [provider/model]` | Switch conversation model in-session (numbered picker) |
| `/thinking <off\|low\|medium\|high\|max>` | Reasoning level, mapped to provider controls |
| `/models`, `/providers`, `/sources` | Catalog, credentials, integrations |
| `/skills`, `/tools` | Skill workflows, tool registry with permission classes |
| `/login <provider>`, `/logout <provider>` | Key management |
| `/session`, `/sessions`, `/new`, `/resume`, `/compact`, `/clear` | Session lifecycle (resume/new switch in place) |
| `/name <name>`, `/export <path>`, `/copy`, `/keys` | Rename, export transcript, copy answer, shortcuts |
| `/doctor`, `/quit` | Full diagnostics in-session, exit |

Anything without a slash is a request to the agent. Piped (non-TTY) input falls back to the classic line loop automatically.

---

## Configuration

Precedence: **defaults < config file < environment**. User data lives in SQLite.

Config file `~/.config/scout/config.json` (or `$SCOUT_CONFIG`) sets addresses, data dir, Ollama host, dry-run, and per-role models (`screening`, `analysis`, `proposal`, `conversation`, `deep_analysis`). See [CONFIGURATION.md](docs/CONFIGURATION.md).

### Providers and models

Five first-class providers, each verified against current docs and Pi/Ghost's own registries:

| Provider | Base URL | Key | Notes |
|----------|----------|-----|-------|
| OpenAI | `api.openai.com/v1` | `OPENAI_API_KEY` | `reasoning_effort` mapping; refresh via `/models` |
| Anthropic | `api.anthropic.com` | `ANTHROPIC_API_KEY` | Thinking budgets; no list API (built-ins) |
| DeepSeek | `api.deepseek.com` | `DEEPSEEK_API_KEY` | `deepseek-flash`, `deepseek-v4-pro` |
| Moonshot | `api.moonshot.ai/v1` | `MOONSHOT_API_KEY` | `kimi-k3`, `kimi-k2.6`, `kimi-k2.7-code`; CN keys use `MOONSHOT_BASE_URL=https://api.moonshot.cn/v1` |
| Ollama | `127.0.0.1:11434` | none | Auto-discovered local models; `think` flag |

`OPENAI_BASE_URL` / `ANTHROPIC_BASE_URL` / `DEEPSEEK_BASE_URL` / `MOONSHOT_BASE_URL` override endpoints. Credential precedence: **credential store, then environment** — `scout login` / `/login` stores keys encrypted (AES-256-GCM); full values are never displayed, logged, or committed.

`scout models refresh [provider]` updates the cached catalog from provider `/models` endpoints and Ollama tags; offline it falls back to cache, then built-ins. Unknown metadata renders as unknown — never invented.

---

## Running as a service

The systemd user unit runs the Scout MCP server (`scripts/scout.service`):

```bash
systemctl --user status scout     # active, 127.0.0.1:3210
journalctl --user -u scout -f     # logs
systemctl --user restart scout
```

Enabled with user lingering, so it starts at device boot without login. Never expose the port publicly — reach it over SSH or Tailscale.

---

## MCP

**Scout as client** — remote (`https://…`) or local stdio (`--command "prog args"`) MCP servers, capability discovery, OAuth tokens stored encrypted. Upwork is the first work source; LinkedIn-style listings and other job platforms fit the same normalized model as they gain usable official interfaces. The domain never assumes one platform\u2019s concepts.

**Scout as server** — `scout mcp` (stdio) or `scout mcp serve` (Streamable HTTP) exposes domain tools (`get_profile`, `search_opportunities`, `analyze_opportunity`, `prepare_proposal`, `get_pipeline`, `approve_action`, …) to OpenCode, Codex, Claude, and other MCP clients. Same Core, same rules — including approvals.

---

## Security and privacy

Everything sensitive stays on your hardware: profile, opportunities, messages, keys, OAuth tokens. Only the evidence needed for a task reaches your chosen LLM; marketplaces receive nothing except approved actions. `SCOUT_DRY_RUN=1` disables external writes. Details: [SECURITY.md](docs/SECURITY.md).

---

## Development

```sh
make test && make vet && make fmt
```

Single Go binary, SQLite, stdlib-first dependencies. One-shot commands, session, and MCP server share `pkg/runtime` — no duplicated business rules. See [ARCHITECTURE.md](docs/ARCHITECTURE.md), [CONTRIBUTING.md](docs/CONTRIBUTING.md).

---

## License

MIT — see [LICENSE](LICENSE).
