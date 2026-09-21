# match-portfolio-evidence

Relevance: portfolio, evidence, projects, github, prove, experience for.

Map opportunity requirements to real user evidence. Only stored data counts.

## Inputs

- opportunity id

## Procedure

1. Load requirements/skills from get_opportunity.
2. get_portfolio_evidence for requirement-by-requirement mapping.
3. search_user_evidence per key requirement for CV/portfolio hits.
4. Optionally inspect GitHub repos (github_repo_info/readme) named in the profile — read-only, to confirm technologies claimed.
5. Exclude weak/unrelated projects explicitly ("Not used: …" with reason).
6. Output: per-requirement status (supported/partial/unsupported) + evidence refs + gaps (missing evidence the user could add).

## Tools

get_portfolio_evidence, search_user_evidence, get_user_portfolio, get_user_experience, github_repo_info, github_repo_readme.

## Failure modes

- No evidence for a key requirement → say unsupported; never upgrade it.
- GitHub unreachable → use stored evidence only.

## Approval

None. Read-only.
