# Scout — External Evaluation with Jev

This document records the external evaluation loop around Scout. It is a
research/regression tool, not part of Scout's runtime:

- **Scout never imports the evaluator.** `scout` works with the evaluator absent.
- **The evaluator never runs inside Scout.** Jev is called only by the separate
  `scout-eval` binary.
- The evaluator reads what Scout already records; it does not change Scout's
  behaviour.

## What a Scout run contains (discovered from the code)

Scout already persists enough to evaluate a run. Relevant tables
(`pkg/store/store.go`):

| Table | Written by | Contains |
|---|---|---|
| `trajectories` | `Core.RecordTrajectory` (`pkg/runtime/catalog.go`) | request, tools used, turns, final answer, error |
| `tool_audit` | `Core.Audit` (`pkg/runtime/audit.go`) | tool, permission, success, **truncated** summary |
| `tool_results` | `Core.recordToolResult` (`pkg/runtime/audit.go`) | **full** tool result (bounded, migration v7) |
| `evaluations` | `Core.Analyze` | stored match evaluations |
| `proposals`, `applications`, `pending_actions`, `feedback` | various | downstream state |

The agent loop emits events (`agent_start`, `turn_start`, `token`,
`tool_start`/`tool_end`, `agent_end`, `error`) via `Core.RunAgent`
(`pkg/runtime/loop.go`), but only the summarized trajectory and tool audit are
persisted.

## Why `tool_results` exists (the key experiment)

Grounding questions ("is the final answer supported by evidence?") are only
meaningful if the evaluator is given the evidence. `tool_audit.summary` is
truncated to 500 characters, so it is not enough.

Controlled experiment (identical grounded answer, only the supplied evidence
differs):

| Evidence supplied | Jev `unsupported_claims` |
|---|---|
| full tool result | 0.07 |
| truncated tool result (500 chars) | 0.95 |

The evaluator was correct; the evidence pipeline was broken. Migration v7 adds
`tool_results`, which stores the full result with a retention cap
(`toolResultRetention`, pruned on write), keeping the short summary for listing.
This is the only change made to Scout for evaluation, and it does not alter
runtime behaviour.

## How the evaluator works

`cmd/scout-eval` + `pkg/jeveval`.

1. `jeveval.LoadTrajectories` reads recent real turns from `trajectories`, and
   attaches the full tool results from `tool_results` (falling back to
   `tool_audit` for pre-migration rows), **bounded by a time window** so a turn
   never inherits another turn's evidence.
2. Deterministic checks compute exact facts in code (tool use, empty answer,
   error, turn budget, obvious claim/audit conflicts). Jev is never asked to
   re-decide these.
3. Jev is asked a small set of grounded questions with the tool results in the
   state:
   - `unsupported_claims` (Noul) — does the answer state a specific fact not in
     the tool results?
   - `answered_request` (Noul) — does it address the request?
   - `invented_user_facts` (Noul) — does it claim a user fact the state does not
     support?
   - `decision_quality` (Score) — how well the request was handled.
4. Findings are printed and written to JSON, tagged with a version label, so a
   later run can be compared.

### Methodological rule

Jev is reliable at **comparing an answer against supplied evidence**, and
unreliable when asked an abstract judgement with nothing to compare against
(measured: it barely separated grounded from fabricated answers when no tool
result was provided). Every grounding question therefore puts the evidence in
the state and asks Jev to compare.

## Running it

```sh
# One-time: the evaluator reads this from the environment only.
export TYPESAFE_API_KEY=...

# Run a fixed corpus of real Scout turns, then evaluate them.
scripts/eval-corpus.sh before

# ... change Scout ...

scripts/eval-corpus.sh after
./build/scout-eval compare build/eval/before.json build/eval/after.json
```

`scout-eval` also runs standalone:

```sh
scout-eval run --limit 20 --version v0.14.0 --out build/eval/run.json
scout-eval compare build/eval/before.json build/eval/after.json
```

No external writes are performed: the corpus only calls `scout ask`, which is
read-only unless the agent requests an approval.

## Limitations

- **Synthetic rows.** The deterministic test suite writes trajectories with a
  fake provider (`request="r"`, etc.). The evaluator skips rows shorter than a
  real request/answer.
- **Tool-result linkage.** `tool_audit`/`tool_results` are not linked to a
  trajectory id, so results are matched by tool name and a time window. A future
  change could add a run id to both to remove the heuristic.
- **Ground truth is partial.** Deterministic checks cover process facts; Jev
  covers grounding and quality. Neither covers real-world outcomes (whether an
  application led to an interview), which Scout does not yet record.
- **Jev is an experimental component.** Its verdicts are compared against
  deterministic checks and human inspection; disagreements are treated as data,
  not as failures of Scout.
