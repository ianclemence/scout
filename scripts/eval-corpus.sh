#!/bin/sh
# scout-eval-corpus — run a fixed corpus of real Scout turns, then evaluate the
# recorded trajectories with Jev. This is the repeatable improvement loop:
#
#   corpus -> real Scout runs -> trajectories/tool_audit -> deterministic + Jev
#   -> findings.json -> (change Scout) -> same corpus -> compare
#
# Scout is never modified by this script, and Jev is never called from Scout.
# It uses the already-configured provider; no external writes are performed.
#
# Usage:
#   TYPESAFE_API_KEY=... scripts/eval-corpus.sh <label>
#
# Writes build/eval/<label>.json (findings) and prints averages.
set -eu
cd "$(dirname "$0")/.."

LABEL="${1:-run}"
SCOUT="${SCOUT_BIN:-./build/scout}"
EVAL="${EVAL_BIN:-./build/scout-eval}"
OUT_DIR="build/eval"
mkdir -p "$OUT_DIR"

if [ -z "${TYPESAFE_API_KEY:-}" ]; then
  echo "TYPESAFE_API_KEY is required (external evaluator; never used by Scout)" >&2
  exit 2
fi

# Marker so this corpus's runs can be isolated from other history in the DB.
MARK="corpus-$LABEL-$(date +%s)"

# Each prompt exercises a different part of Scout: profile grounding, tool use,
# coverage, and honest behaviour under missing constraints. Keep them real and
# answerable with the local data.
run_case() {
  echo "--- $1"
  # $SCOUT ask writes the request into trajectories; we prefix the marker so
  # the evaluator can select only this corpus.
  "$SCOUT" ask "[$MARK] $2" >/dev/null 2>&1 || echo "  (turn returned non-zero)"
}

run_case profile  "What skills are in my profile? Answer only from my profile; if a skill is not there, say so."
run_case count    "How many opportunities are stored? Then give the top 2 by fit, with one line each."
run_case coverage "Which opportunity best matches my profile, and what is the single biggest risk with it?"
run_case honesty  "Give me a ranked shortlist of my top 3 opportunities against my profile and rate floor."

echo
echo "Evaluating corpus '$LABEL' ($MARK)…"
"$EVAL" run --limit 60 --request "$MARK" --version "$LABEL" --out "$OUT_DIR/$LABEL.json"
echo
echo "Findings: $OUT_DIR/$LABEL.json"
echo "Compare a later run with: $EVAL compare $OUT_DIR/<before>.json $OUT_DIR/<after>.json"
