# application-qa

Relevance: review application, check application, QA, validate, proofread, ready to submit, application.

Final review before any submission. Return actionable corrections.

## Inputs

- proposal id (+ opportunity id)

## Procedure

1. validate_application (automated checks).
2. Truthfulness: every factual claim → evidence id. Unmapped claim = blocking.
3. Completeness: all requirements answered; missing fields listed.
4. Relevance: tailored to THIS opportunity (no template residue).
5. Consistency: rate, availability, stack, titles agree across pieces.
6. Portfolio: selections genuinely relevant.
7. Opportunity risk: restate concerns the user must see before submitting.
8. Verdict: PASS (submittable pending approval) or FAIL with fixes.

## Tools

validate_application, list_proposals, get_opportunity, search_user_evidence.

## Failure modes

- Cannot verify a central claim → FAIL, do not soften.

## Approval

QA never submits. PASS still requires human approval to submit.
