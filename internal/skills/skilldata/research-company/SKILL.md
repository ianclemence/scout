# research-company

Relevance: research, company, client, employer, background, legit, scam check.

Gather public evidence about an employer/client. Provenance on everything.
Neutral risk language; never accusations.

## Inputs

- company/client name, optional source context (listing id)

## Procedure

1. If the source exposes client info, read it first (get_opportunity client block, get_source_capabilities).
2. research_company for public evidence (cached; FACT/INFERENCE/UNKNOWN split).
3. Cross-check important claims with verify_claim (second source).
4. Look for: what they do, size/stage signals, hiring history where available, contradictions, vague-vs-concrete requirements.
5. Output: verified facts (with sources), inferences (labeled), unknowns, concerns (concerning/unverified with evidence — never "fraud" without proof).

## Tools

research_company, verify_claim, web_search, fetch_web_content, get_opportunity.

## Failure modes

- No public footprint → say so; absence of evidence is not evidence of scam.
- Fetch blocked → mark research incomplete, continue.

## Approval

None. Read-only.
