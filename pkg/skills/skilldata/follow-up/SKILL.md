# follow-up

Relevance: follow up, follow-up, nudge, check in, bump.

Decide if a follow-up is appropriate, draft it, never auto-send.

## Inputs

- application id

## Procedure

1. prepare_follow_up (stage, elapsed time, prior contact).
2. Rules: no follow-up before ~7 days; none if already responded/rejected/closed; max two per application; each must add information, not just "bumping".
3. Present draft with the reasoning (why now). Sending needs send_message + approval.

## Tools

prepare_follow_up, search_application_history, list_messages.

## Failure modes

- Duplicate messaging risk → refuse and explain.

## Approval

Draft only. Sending requires approval.
