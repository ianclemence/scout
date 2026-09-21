# SECURITY

Scout handles CVs, messages, API keys, and OAuth tokens on a self-hosted Pi.

## Secrets

- Master key: `SCOUT_MASTER_KEY` env, else auto-generated 0600 file `$DATA_DIR/.masterkey`.
- Credentials encrypted with AES-256-GCM in the SQLite `secrets` table.
- Never in git (`.gitignore`: `.env*`, `*.db*`, `.masterkey`, history), logs, errors, API responses, or example files. `scout config` redacts.
- Tradeoff (explicit): holder of master key + DB can decrypt. Backups include the encrypted secrets table — documented in `scout backup` output.

## Terminal

- `scout login` / `/login` read keys with echo disabled; full values are never printed after entry.
- History file stores commands typed, so prefer `/login` (masked) over pasting keys into the prompt. Rotate a key if it ever lands in history.
- Ctrl-C cancels in-flight model/tool work via context; no partial external writes (writes are approval-gated anyway).

## Approvals and autonomy

- Read/analyze/draft are free. Submit, spend credits, send, accept, and fund are `pending_approval` records with risk levels — never executed silently.
- `approve_action` (MCP) and `/approvals approve` record intent; external execution runs only through official integration paths.
- `SCOUT_DRY_RUN=1` blocks external writes.

## Input handling

- Uploads: 5 MB limit, filename reduced to base name, never used as a path; PDF parsed as text only.
- Connector endpoints: `https://` or localhost-http only (SSRF guard).
- Terminal output is plain text; external content is never interpreted as control sequences beyond newlines.

## Prompt injection

- Context layers separate system instructions, profile, evidence, history, tool results, and external marketplace content.
- Job postings, client messages, and tool results are labeled UNTRUSTED DATA every turn; instructions inside them are never followed.
- Tool names validate against the registry; unknown tools return errors the loop recovers from (tested).

## Marketplace safety

- Official MCP/API only. No scraping, no private endpoints, no rate-limit evasion.
- Credits treated as a guarded resource (per-app and per-day caps in profile).
