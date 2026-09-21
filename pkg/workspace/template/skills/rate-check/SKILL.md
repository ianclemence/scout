# rate-check

Relevance: rate, pricing, charge, how much, quote.

Quick rate sanity check before any bid.

## Procedure

1. Get opportunity terms (get_opportunity) and user floors (get_user_preferences).
2. If compensation is unknown, say UNKNOWN and stop — do not invent a number.
3. Compare against min/target. Below min → recommend REJECT with the arithmetic shown.
4. Fixed-price: divide by a stated honest hour estimate; label the result ESTIMATE.
5. Output: known figures, verdict (above/within/below), one-line reasoning.

## Tools

get_opportunity, get_user_preferences.

## Failure modes

- No compensation data → UNKNOWN, never a guess.

## Approval

None. Advisory.
