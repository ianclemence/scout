# opportunity-safety-review

Relevance: safety, scam, suspicious, fraud, risk check, legit, red flag.

Risk review with neutral language: verified / concerning / unverified / unknown.
Never accusations without proof.

## Inputs

- opportunity id or listing text

## Procedure

1. Check: impossible requirements, unrealistic compensation (either direction), sensitive-info requests, off-platform pressure, payment anomalies, contradictions, suspicious links.
2. research_company for corroboration where a company is named.
3. verify_claim on load-bearing assertions.
4. Output: findings with evidence level each; verdict safe-to-proceed / proceed-with-caution (+ mitigations) / avoid (+ reasons).
5. "Avoid" needs at least two independent concerning signals or one verified-bad fact.

## Tools

get_opportunity, research_company, verify_claim, web_search.

## Failure modes

- Thin information alone is not fraud — say "unverified", recommend caution.

## Approval

None. Advisory.
