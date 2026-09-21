# deduplicate-opportunities

Relevance: duplicate, dedupe, same job, repost, already seen.

Canonicalize listings across sources. Multiple signals, never title-only.

## Inputs

- candidate listings (title, company, description, compensation, URL)

## Procedure

1. find_duplicate_opportunity per candidate (fingerprint first).
2. Cross-source signals: identical company + overlapping description, matching compensation + requirements, same canonical URL.
3. Reposts: same client/project, changed title/description → link, keep newest.
4. Merge: canonical record keeps all source references (provenance).
5. Output: new vs known, canonical ids, merged references.

## Tools

find_duplicate_opportunity, save_opportunity, search_opportunity_history.

## Failure modes

- Uncertain match → keep separate, note possible relation. Never merge aggressively.

## Approval

Local writes only (mutate_local, audited).
