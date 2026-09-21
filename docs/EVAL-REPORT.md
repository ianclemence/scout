# Scout — End-to-End Jev Evaluation Report

Empirical record of the evaluation → finding → improvement → re-evaluation loop
run against Scout. Everything here is measured, not asserted. See
`docs/EVALUATION.md` for how to run the loop.

## 1. Scout as discovered

Single Go binary, 23.6k LOC. Real execution path (verified in code, not taken
from docs):

```
scout / scout ask / MCP server
  → cmd/scout/main.go → runtime.Core
      → store (SQLite, migrations)
      → llm.Provider (openai/anthropic/deepseek/moonshot/ollama/openai_compatible)
      → tools (typed registry + permission classes)   pkg/runtime/tools.go
      → ReAct agent loop                               pkg/runtime/loop.go
          system prompt (pkg/agent/prompt.go) + skills (pkg/skills) +
          compact tool catalog (pkg/runtime/catalog.go)
          one ```tool {"name","arguments"} block per turn; MaxTurns=8
          tool results returned to the model as untrusted data
      → match/preference (deterministic filter + heuristic + learned prefs)
      → approvals (pkg/approve) — external/financial tools need an approved id
      → sources (local/MCP/Upwork dialect)             pkg/sources
```

Recording that already existed (the evaluation substrate):

- `trajectories` — request, tools, turns, final, error (`RecordTrajectory`).
- `tool_audit` — tool, permission, success, **500-char** summary (`Audit`).
- `evaluations`, `proposals`, `applications`, `pending_actions`, `feedback`.
- `pkg/eval` — a hermetic deterministic suite (`scout eval`, 11 cases).

## 2. How Jev was used

Externally only. `pkg/jeveval` (client + evaluation schema) and `cmd/scout-eval`
(separate binary) read Scout's recorded runs and ask Jev structured questions.
Scout does not import `jeveval`; Jev is never in Scout's runtime.

## 3. Methodology

- **Cases:** a fixed corpus of four real prompts (`scripts/eval-corpus.sh`):
  profile grounding, count/top-2, coverage/risk, ranked shortlist vs rate floor.
  Run through the real `scout ask` against the real profile and Upwork data.
- **Deterministic checks (ground truth, code-decided):** did tools run, empty
  answer, error, turn budget, claim/audit conflict.
- **Jev questions (judgment, evidence supplied in state):** `unsupported_claims`
  (Noul), `answered_request` (Noul), `invented_user_facts` (Noul),
  `decision_quality` (Score).
- **Ground-truth control:** identical answer scored against full vs truncated
  tool results to validate Jev itself before trusting it.
- **No external writes.** The corpus only reads; approval boundaries untouched.

## 4. Findings

**F1 — Tool results were truncated, making grounding evaluation invalid.**
Controlled experiment: the *same grounded answer* scored `unsupported=0.07`
with the full tool result and `unsupported=0.95` with the 500-char summary Jev
was actually given. The high unsupported rate in the first corpus run was an
artifact of missing evidence, not a Scout defect. (Jev was right; the pipeline
was wrong.)

**F2 — Tool results were not linked to the run that produced them.**
Matching evidence to a trajectory by timestamp pulled a neighbouring turn's
result, which produced an unstable outlier (quality 1.41 vs ~2.7 for the same
scenario). Fixed by tagging results with a run id.

**F3 — A question's wording caused false positives.**
`invented_user_facts` scored 0.83 on an answer that only restated the user's
stored profile and labeled its own claims FACT/INFERENCE. Tightening the
question to "asserts a credential/employer/degree/skill not in the evidence"
dropped it to 0.47 with no change to Scout. This is evaluator calibration, not a
Scout fault.

**F4 — No real agent-quality defects were found in the corpus.**
After F1–F3 were fixed, all four real answers scored quality 2.6–2.85,
unsupported 0.23–0.34, and were correctly grounded and candid about the missing
rate floor. The deterministic suite (11/11) also passed. Scout's answers in
these cases were good.

**F5 — A real, minor gap: no downstream-outcome signal.**
Scout records applications and their stage, but not whether an application led
to a reply/interview/hire. This bounds what the evaluator can ever measure: it
can judge answer quality, not job-search effectiveness.

## 5. Root causes

| Finding | Cause | Category |
|---|---|---|
| F1 | `tool_audit.summary` truncated to 500 chars | Scout persistence gap (for evaluation) |
| F2 | results not linked to a run | Scout persistence gap |
| F3 | ambiguous Jev question | evaluation limitation |
| F4 | — | none (good behaviour) |
| F5 | no outcome capture | Scout data limitation |

None of the failures were model limitations. Two were persistence gaps in Scout;
one was evaluator wording.

## 6. Improvements (implemented)

1. **`tool_results` table + retention** (migration v7): stores the full tool
   result (best-effort, capped at 256 KB/row, bounded to 500 rows pruned on
   write). Runtime behaviour is unchanged; only the evidence is now recoverable.
2. **Run id** (migration v8): `RunAgent` mints a run id, `Execute` tags each
   tool result with it, `RecordTrajectory` records it, and the evaluator matches
   by it. Removes the timestamp heuristic.
3. **Evaluator** (`pkg/jeveval`, `cmd/scout-eval`, `scripts/eval-corpus.sh`):
   the repeatable loop, with a calibrated `invented_user_facts` question.
4. Tests: `tool_results` recorded, and retention stays bounded.

## 7. Before vs after (same four cases)

| Metric | Before | After | Δ |
|---|---|---|---|
| decision quality (0–3) | 2.29 | 2.75 | **+0.45** |
| unsupported-claim prob (lower better) | 0.77 | 0.32 | **−0.44** |
| answered-request prob | 0.88 | 0.92 | +0.03 |
| Jev confidence on quality | 0.01–0.50 | 0.61–0.85 | more decisive |

Raw: `build/eval/before.json`, `build/eval/after2.json`,
`build/eval/after2_refined.json`. Deterministic suite 11/11 throughout.

## 8. Remaining weaknesses

- **Corpus size.** Four cases is a signal, not statistical proof; treat the
  deltas as directional. The loop is built to grow the corpus cheaply.
- **No outcome ground truth** (F5). Quality is judged, not validated against
  real results.
- **Pre-run-id rows.** Evidence matching falls back to a time window for old
  rows; only new runs get exact matching.
- **Synthetic rows.** The deterministic suite writes fake trajectories; the
  evaluator skips them by length heuristic rather than a flag.

## 9. Jev assessment

**Useful:** for comparing an answer against supplied evidence, Jev separated
grounded from fabricated answers sharply (unsupported 0.32 vs 0.99 with evidence
present), and its confidence tracked evidence quality (0.01–0.50 with poor
evidence → 0.61–0.85 with full evidence). Decomposed, grounded questions gave
diagnostic value a single "rate this" prompt would not.

**Not useful as-is:** abstract grounding questions with no reference produced
near-random separation (grounded 0.52 vs fabricated 0.72). Jev cannot supply the
evidence — the caller must. Its verdicts also needed calibration (`invented_user_facts`).

**Verdict:** worth retaining, *provided* every grounding question carries the
real evidence in the state. Without that, it is noise.

## 10. Next research direction (evidence-based)

The highest-value next step is **F5: outcome capture**. Scout already records
applications and stages; adding a `record_opportunity_outcome` signal (reply /
interview / hire / no-response) would give the evaluator real ground truth and
let Scout test whether its ranking actually predicts success — the one thing the
current harness cannot measure. Ranking changes should not be made until that
signal exists; today they can only be scored on plausibility, not on outcomes.
