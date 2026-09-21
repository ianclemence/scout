# discover-opportunities

Relevance: find, search, discover, look for, opportunities, jobs, work, gigs, hunt.

Broad discovery across connected sources, qualified aggressively afterward.
Optimize for worthwhile opportunities, not listing counts.

## Inputs

- User profile and preferences (via get_user_profile, get_user_preferences)
- Optional: query, skills, sources, location, remote, min budget

## Procedure

1. Read profile + preferences to derive search criteria (skills, technologies, floors, exclusions).
2. List sources (list_sources) and check health (source_health). Skip unhealthy ones; note them.
3. Search relevant sources in parallel via discover_opportunities (one call can cover all; per-source calls only when filters differ).
4. Deduplicate: find_duplicate_opportunity on each candidate; keep canonical record.
5. Drop obvious rejects with deterministic filters (budget floor, excluded work, credit caps).
6. Persist new finds with save_opportunity (provenance preserved).
7. Return at most ~12 candidates: title, source, budget, one-line why.

## Tools

discover_opportunities, list_sources, source_health, find_duplicate_opportunity, save_opportunity, get_user_profile, get_user_preferences.

## Failure modes

- Source down → warn and continue with others; never fail the run.
- Zero results → broaden query once (drop one filter), then report honestly.
- Unparseable payload → record warning, continue.

## Approval

None. Discovery is read-only. Submission always goes through approvals.
