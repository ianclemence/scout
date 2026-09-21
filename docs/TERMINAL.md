# Scout Terminal Design

*The terminal is Scout's only interface. Everything a work marketplace's web
UI does — browse work, read a job, check fit, draft, submit, message, track the
contract — happens here, in the session, under human control.*

This document is the design contract for the interactive terminal.

---

## 1. Design principles

1. **The session is the product.** `scout` opens one full-screen session that
   can answer any question and perform any capability. One-shot subcommands
   exist for scripting; the session must never feel like a lesser surface.
2. **Every answer is reachable two ways.** By typing (`/command`, natural
   language) and by picking. A picker that mirrors a command's argument is not
   a convenience — it is the primary path on a phone-sized terminal.
3. **Plain language goes to the agent; slashes run locally.** A slash command
   is deterministic, instant, and costs no model turn. Anything else is a
   request to the agent loop with tools.
4. **The transcript is scrollback; the dock is live.** Committed messages are
   printed once into terminal scrollback (native scroll, copy, search). Only
   the live region — streaming tail, composer, selector, footer — is redrawn
   each frame.
5. **Consequential actions are cards, never side effects.** Submit, spend,
   send, accept, fund each become a pending approval. The card is inline,
   keyboard-decidable, and states what approving actually does.
6. **No hidden machinery.** Tool JSON, reasoning traces, and internal names
   never reach the transcript. The user sees their words, Scout's prose, and
   named activity ("Searching work…", "Drafting a proposal…").
7. **Fail closed and fail fast.** Unknown risk is high risk. A source that
   cannot act says so instead of pretending. No operation can hang the session.
8. **Reasoning is never shown.** A model may think internally, but Scout
   surfaces only answers. Reasoning fields and inline thinking tags are parsed
   and discarded at the provider boundary, and thinking defaults to off.

---

## 2. Layout

```text
┌───────────────────────────────────────────────────────────┐
│ terminal scrollback (committed transcript)                 │
│   You                                                      │
│   ┃ find Go + AI work under my rate                        │
│   👷 Scout · deepseek/deepseek-flash · 4s                  │
│   [markdown answer, wrapped, never truncated]              │
│                                                            │
├───────────────────────────────────────────────────────────┤
│ dock (redrawn every frame, bottom-anchored)               │
│   ▍ streaming tail (one line while a turn runs)            │
│   ── ▘ Working · 2s ──────────────────────────────────    │
│   > composer                                                │
│   ───────────────────────────────────────────────────────  │
│   [selector / approval card, when open]                    │
│   3 turns · 1 approval(s)          (cloud) deepseek/flash  │
│   / commands                                    esc quit   │
└───────────────────────────────────────────────────────────┘
```

- **Committed** = user turns, Scout answers, notices, errors, approval
  decisions. Printed with `tea.Println`, so the terminal owns them.
- **Live dock** = streaming preview, composer/approval card, one selector,
  stats line, key hints.
- Exactly one modal (login flow, approval card, scoped-models, model picker,
  list picker, command palette) is active at a time.

---

## 3. Commands

Commands are grouped by job. `/help` prints the same grouping. The registry
(`pkg/isession/commands.go`) is the single source of truth; the TUI palette,
the line-mode fallback, and `scout help` all read it.

### Work
| Command | Purpose |
|---|---|
| `/discover [query]` | Run discovery across connected sources (read-only) |
| `/opportunities [query]` | Browse stored opportunities (picker when bare) |
| `/opportunity <id>` | Full posting + evaluation + proposal |
| `/analyze <id>` | Deterministic filter + structured fit |
| `/proposal <id>` | Draft a grounded proposal (never sends) |
| `/applications` | Pipeline by stage |
| `/pipeline` | Counts by stage |
| `/inbox` | Messages needing attention |
| `/feedback <id> <signal> [note]` | Record explicit preference |

### Decide
| Command | Purpose |
|---|---|
| `/approvals` | Review/decide pending actions (picker when bare) |

### You
| Command | Purpose |
|---|---|
| `/profile` | Structured profile summary, `import <path>` to update |
| `/cv` | Resume content and citable items |

### Connect
| Command | Purpose |
|---|---|
| `/sources`, `/integrations` | Work sources & MCP connectors: status, capabilities, add/test |
| `/providers` | Provider availability |
| `/login [provider]`, `/logout` | Staged sign-in / remove stored key |
| `/model`, `/scoped-models`, `/thinking` | Reasoning configuration |

### Session
| Command | Purpose |
|---|---|
| `/status`, `/session`, `/sessions`, `/new`, `/resume`, `/name` | Lifecycle |
| `/export`, `/copy`, `/keys`, `/clear`, `/compact` | Transcript |
| `/skills`, `/tools`, `/doctor`, `/changelog`, `/help`, `/quit` | Reference |

---

## 4. The MCP / connector surface

"what mcp is configured" must be answerable both locally and by the agent.

- `/sources` (alias `/integrations`) lists every configured connector with:
  name, kind (mcp / mcp-stdio), endpoint, enabled, auth state, discovered
  capabilities, and last-probe result.
- `/sources add <name> <url | --command "…">` adds a connector.
- `/sources test <name>` performs read-only capability discovery.
- `/sources token <name>` stores an OAuth token (masked, encrypted).
- The agent's `list_sources` / `get_source_capabilities` / `source_health`
  tools expose the same facts, so the conversational path is honest.

**Critical invariant:** building the source registry must never run a nested
query on a single-connection SQLite handle. Secrets are loaded from a pre-read
map, not inside an open row cursor.

---

## 5. Reasoning isolation

Scout's answer stream never contains chain-of-thought, by construction:

- **OpenAI-compatible** (OpenAI, DeepSeek, Moonshot, Ollama `/v1`, custom):
  only `delta.content` / `message.content` is answer text. The reasoning
  allowlist (`reasoning_content`, `reasoning`, `reasoning_text`) is parsed and
  discarded. Thinking is off by default, sent explicitly as disabled where the
  provider supports it.
- **Anthropic**: only `type == "text"` content blocks and `text_delta` events
  are emitted; `thinking` blocks and `thinking_delta` events are dropped.
- **Ollama native**: `message.thinking` is dropped; only `message.content` is
  returned.
- **Inline tags**: `<think>…</think>` and `<thinking>…</thinking>` are stripped
  from the text stream, including across token boundaries.

---

## 6. Failure behavior

- Every tool has a bounded timeout; source probes default to 8s and isolate
  per-source failure (a dead source never fails the turn).
- No DB path may acquire a second connection while iterating rows.
- `SCOUT_DRY_RUN=1` disables external writes before any adapter is reached.
- Unknown provider/source/tool names produce a stated error, never invention.
