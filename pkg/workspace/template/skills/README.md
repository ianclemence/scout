# Custom skills

Each subfolder with a `SKILL.md` becomes a Scout skill, loaded on top of
the built-ins. Same structure:

```md
# my-skill

Relevance: keyword, trigger, phrases that select it.

<procedure, tools, rules…>
```

Rules:

- `Relevance:` line drives selection — write triggers the way you'd phrase requests.
- Same `Name` as a built-in replaces it for your install only.
- Keep procedures honest: cite tools that exist (`scout tools`), never invent.
- Secrets don't belong here. Ever.
