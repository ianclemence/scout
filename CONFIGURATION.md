# CONFIGURATION

Precedence: **defaults < config file < environment variables**.
User data (profile, preferences, sources, secrets) lives in SQLite.

Config file: `~/.config/scout/config.json` (or `$SCOUT_CONFIG`):

```json
{
  "addr": "127.0.0.1:3210",
  "data_dir": "/home/pi/.local/share/scout",
  "ollama_host": "http://127.0.0.1:11434",
  "dry_run": false,
  "models": {
    "analysis": {"provider": "openai", "model": "gpt-4o-mini"},
    "proposal": {"provider": "anthropic", "model": "claude-haiku-4-5"}
  }
}
```

| Variable | Purpose |
|---|---|
| `SCOUT_CONFIG` | config file path |
| `SCOUT_DATA_DIR` | data dir (DB, key file, history) |
| `SCOUT_ADDR` | MCP HTTP listen address |
| `SCOUT_MASTER_KEY` | secret encryption key (else generated 0600 file) |
| `SCOUT_DRY_RUN` | block external writes |
| `OLLAMA_HOST` | local model endpoint |
| `OPENAI_API_KEY` / `ANTHROPIC_API_KEY` / `DEEPSEEK_API_KEY` / `MOONSHOT_API_KEY` | provider keys (credential store wins over env) |
| `OPENAI_BASE_URL` / `ANTHROPIC_BASE_URL` / `DEEPSEEK_BASE_URL` / `MOONSHOT_BASE_URL` | endpoint overrides (e.g. `MOONSHOT_BASE_URL=https://api.moonshot.cn/v1` for CN-region keys) |
| `OPENAI_COMPAT_ENDPOINT` / `OPENAI_COMPAT_KEY` | generic endpoint |
| `SCOUT_MODEL_<ROLE>` / `SCOUT_MODEL_<ROLE>_PROVIDER` | per-role override (roles: screening, analysis, proposal, conversation, deep_analysis) |

Interactive: `/model` switches the session conversation model without restart (registry-backed picker with context/reasoning metadata); `/thinking` sets the reasoning level; `/login <provider>` stores a key encrypted (masked prompt, never displayed). `scout config` shows effective config with secrets redacted. `scout models refresh [provider]` updates the cached model catalog (provider `/models` or Ollama tags; offline falls back to cache, then built-ins).

MCP sources: remote (`scout integrations add Upwork https://mcp.upwork.com/mcp`) or local stdio (`scout integrations add Name --command "prog args"`).

File permissions: data dir `0700`, DB and key file `0600`.
