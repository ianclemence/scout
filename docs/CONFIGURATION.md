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
    "conversation": {"provider": "deepseek", "model": "deepseek-flash"},
    "worker": {"provider": "deepseek", "model": "deepseek-flash"}
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
| `SCOUT_MODEL_CONVERSATION` / `SCOUT_MODEL_CONVERSATION_PROVIDER` | conversation (session) model override |
| `SCOUT_MODEL_WORKER` / `SCOUT_MODEL_WORKER_PROVIDER` | worker (background tasks) model override; inherits the conversation model when unset |
| `SCOUT_MODEL` / `SCOUT_MODEL_PROVIDER` | default model/provider for both roles |

The worker covers fit analysis and proposal drafting, plus future tool work. One model is used everywhere unless you set a worker override. Legacy `SCOUT_MODEL_ANALYSIS` / `_PROPOSAL` / `_SCREENING` / `_DEEP` variables still map to the worker role.

Interactive: `/model` opens a searchable picker of models from **configured providers only** (Tab toggles all/scoped scope, Ctrl+S sets the conversation default); an exact `provider/model` argument switches immediately even for a provider you have not logged into yet. Built-ins remain known offline but are not offered for selection until their provider is configured (or, on a fresh install with nothing configured, the picker shows the full catalog with a warning); `/scoped-models` enables/disables and orders the models offered when cycling with Ctrl+P (Ctrl+S persists); `/thinking` sets the reasoning level; `/login` opens a staged flow offering both authentication methods — "Sign in with an account" (OAuth/subscription; no providers yet) and "Sign in with an API key" (provider → masked key prompt) — or `/login <provider>` jumps straight to that provider's prompt; `/logout` lists stored credentials and removes the one you pick. Keys are encrypted and never displayed. `scout config` shows effective config with secrets redacted. `scout models refresh [provider]` updates the cached model catalog (provider `/models` or Ollama tags; offline falls back to cache, then built-ins).

MCP sources: remote (`scout integrations add Upwork https://mcp.upwork.com/mcp`) or local stdio (`scout integrations add Name --command "prog args"`).

File permissions: data dir `0700`, DB and key file `0600`.
