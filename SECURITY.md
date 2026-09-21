# SECURITY

Scout handles CVs, messages, API keys, and OAuth tokens. Threat model and controls:

## Secrets

- Master key: `SCOUT_MASTER_KEY` env, else auto-generated 0600 file `$DATA_DIR/.masterkey`.
- Credentials encrypted with AES-256-GCM in SQLite `secrets` table.
- Never in source control (`.gitignore` covers `.env`, `*.db`, `.masterkey`).
- Never in logs, errors, or API responses. `scout config` redacts.
- Tradeoff (explicit): anyone with master key + DB can decrypt. Backups contain the secrets table — keep them private.

## Web

- First-run admin password (bcrypt) + random session tokens (sha256-hashed at rest, HttpOnly, SameSite=Lax).
- `AuthDisabled` exists only for localhost dev; never enable on a network interface.
- Recommended: bind `127.0.0.1`, access via Tailscale. No public exposure.

## Input handling

- Uploads: 5 MB limit, filename reduced to `filepath.Base`, never used as a path; PDF parsed as text only.
- Endpoint allowlist for connectors: `https://` or localhost-http only (SSRF guard).
- HTML templates use `html/template` (auto-escaping). No raw markdown rendering of external content.

## Prompt injection

- System prompt separates roles: profile evidence vs. external opportunity/message content.
- External content is labeled UNTRUSTED DATA in every prompt; instructions inside it are never followed.
- Tool results treated per trust level; consequential actions always human-approved.

## Marketplace safety

- Only official MCP/API integrations. No scraping, no credential stuffing, no rate-limit evasion.
- Dry-run mode (`SCOUT_DRY_RUN=1`) blocks external writes.
- Connects/credits treated as a guarded resource with per-app and per-day caps.
