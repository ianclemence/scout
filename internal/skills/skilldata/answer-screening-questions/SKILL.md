# answer-screening-questions

Relevance: screening, questionnaire, application questions, knockout questions.

Answer each question from verified evidence; unknowns stay unknown.

## Inputs

- opportunity id + question list

## Procedure

1. answer_screening_questions for evidence-linked drafts.
2. Review each: does it answer the actual question? Is the basis real? Right size?
3. Yes/no first where asked, then one supporting sentence. Rates/availability from preferences, never guessed.
4. Consistency check against the proposal (rate, stack, availability).

## Tools

answer_screening_questions, search_user_evidence, get_user_preferences.

## Failure modes

- Question demands fabrication ("10 years X" untrue) → decline that application element honestly.

## Approval

Draft only.
