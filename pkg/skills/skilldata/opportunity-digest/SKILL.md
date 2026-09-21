# opportunity-digest

Relevance: digest, summary, brief, overview, what's new, roundup.

Concise actionable digest. Prioritize; never dump hundreds of listings.

## Inputs

- optional: since, sources, focus area

## Procedure

1. search_opportunity_history for recent finds; get_pipeline for in-flight; list_pending_approvals for decisions due.
2. Buckets: new / worth reviewing / strong matches / questionable / rejected / previously seen / deadlines / follow-ups due.
3. Each item: one line (title, source, budget, verdict + reason).
4. Cap at ~15 items; link to full review (/opportunity, analyze).

## Tools

search_opportunity_history, get_pipeline, list_pending_approvals, list_applications.

## Failure modes

- Empty buckets omitted, not padded.

## Approval

None. Read-only summary.
