# track-applications

Relevance: track, pipeline, status, application history, where things stand.

Lifecycle management with timestamps and provenance. Manual updates where
sources don't expose status.

## Inputs

- optional stage filter, opportunity/application id

## Procedure

1. get_pipeline for counts; list_applications / search_application_history for rows.
2. check_opportunity_status where the source supports it; otherwise mark unknown.
3. Stage transitions via record_application_status (prepared → approved → submitted → viewed → responded → interview → offer/won or rejected/withdrawn/closed).
4. New submissions via record_application after real submission (never invent).
5. Output: per-application stage + age + next action.

## Tools

get_pipeline, list_applications, search_application_history, record_application, record_application_status, check_opportunity_status.

## Failure modes

- Source silent → unknown status, manual update path. Never assume viewed/rejected.

## Approval

Recording is local. Status changes from real events only.
