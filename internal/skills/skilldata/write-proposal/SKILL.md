# write-proposal

Relevance: proposal, application, bid, pitch, apply for, application draft.

Tailored proposals from strategy + verified evidence. No filler, no fabrication.

## Inputs

- opportunity id + application strategy (prepare-application-strategy first)

## Procedure

1. Use prepare_proposal tool (grounds draft in profile evidence).
2. Rewrite against the strategy: client's actual problem first, specific approach, evidence woven in, 2 sharp questions.
3. Constraints: concise (120–200 words default), user's proposal_style, no generic openers, no superlatives without proof, no AI mention unless relevant.
4. Run validate_application on the result; fix blocking issues.

## Tools

prepare_proposal, validate_application, list_proposals, get_user_preferences.

## Failure modes

- Missing strategy → build it first, don't freestyle.
- Validation blocks → revise, never submit around it.

## Approval

Draft only. Submission requires request_approval + human approval.
