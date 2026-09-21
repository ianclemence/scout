# evaluate-opportunity

Relevance: evaluate, assess, analyze, fit, match, qualify, review, worth it.

Qualify one opportunity against the profile with evidence-backed reasoning.
Output explains WHY, never a bare number.

## Inputs

- opportunity id (stored) or full listing data

## Procedure

1. Load opportunity (get_opportunity) and profile/preferences/evidence.
2. Run analyze_opportunity for the structured baseline (filter + dimensions).
3. For each important requirement, determine: supported / partially_supported / unsupported / unknown, citing evidence ids (get_portfolio_evidence, search_user_evidence).
4. Compensation: compare known amounts to min/target; unknown stays unknown — never invent.
5. Constraints: location, timezone, contract, availability, language, schedule.
6. Risks: vague scope, unrealistic requirements, suspicious signals, ambiguity. Neutral language: verified / concerning / unverified / unknown.
7. Classify: MATCH (substantial alignment + reasons), MAYBE (potential + exact uncertainties), REJECT (clear mismatch + concise reason).
8. Recommend: ignore / review / prepare application / request approval.

## Tools

get_opportunity, analyze_opportunity, get_portfolio_evidence, search_user_evidence, get_user_preferences, check_opportunity_status.

## Failure modes

- Missing fields → mark unknown, continue; do not fabricate.
- Closed/expired listing → report status, stop.

## Approval

None. Evaluation never acts externally.
