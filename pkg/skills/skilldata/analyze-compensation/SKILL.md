# analyze-compensation

Relevance: compensation, pay, rate, budget, salary, pricing, worth, bid.

Compensation analysis with estimates clearly marked as estimates.

## Inputs

- opportunity id, user min/target rates

## Procedure

1. Extract known compensation (get_opportunity, get_user_preferences). Unknown → unknown.
2. Handle hourly / fixed / salary / ranges / currencies as given; convert only with stated assumptions.
3. Effective-hourly math only where scope + budget both exist; label ESTIMATE.
4. Compare against min and target: below-min is a REJECT signal for the evaluate skill.
5. Output: known figures, estimate (if computable) with assumptions, fit vs floors, ambiguity notes.

## Tools

get_opportunity, get_user_preferences.

## Failure modes

- Ambiguous ("competitive", "$") → unknown, do not invent willingness to pay.

## Approval

None.
