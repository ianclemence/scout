# CONFIGURATION

Precedence: **environment variables > database/app settings > defaults**.

| Variable | Default | Purpose |
|---|---|---|
| `SCOUT_DATA_DIR` | `~/.local/share/scout` | data dir (DB, key file) |
| `SCOUT_ADDR` | `127.0.0.1:3210` | web listen address |
| `SCOUT_MASTER_KEY` | — (generated file) | secret encryption key |
| `SCOUT_DRY_RUN` | `false` | block external writes |
| `OLLAMA_HOST` | `http://127.0.0.1:11434` | local model endpoint |
| `OPENAI_API_KEY` | — | OpenAI provider |
| `ANTHROPIC_API_KEY` | — | Anthropic provider |
| `OPENAI_COMPAT_ENDPOINT` / `OPENAI_COMPAT_KEY` | — | generic endpoint (e.g. OpenRouter) |
| `SCOUT_MODEL_{SCREENING,ANALYSIS,PROPOSAL,CONVERSATION,DEEP}[_PROVIDER]` | ollama / `qwen3:0.6b` | per-role routing |

User configuration (profile, preferences, sources, secrets) lives in SQLite, editable via web UI and CLI. File permissions: data dir `0700`, DB/key `0600`.
