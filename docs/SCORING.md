# Lead Scoring

The score estimates web-service opportunity plus contactability. It is not a judgment about the business.

## Baseline

- no known website: +35
- phone: +8
- email: +8
- social: +5
- performance < 40: +15
- mobile problem: +15
- no clear CTA: +10
- no booking path in booking-relevant sector: +5
- SEO < 60: +5
- HTTPS missing: +10
- high source confidence: +4
- medium source confidence: +2

Cap at 100.

All weights and thresholds live in `lead_hunter/config.py`. A deployment can
retune them without a code change through `LEADSCOUT_SCORE_WEIGHTS` and
`LEADSCOUT_VISIBILITY_WEIGHTS` (JSON; invalid JSON falls back to defaults).

## Readiness vs measured

Two different things are reported and must not be confused:

- Readiness scores (SEO/AEO/GEO/AI Visibility/GAP) are heuristics over the
  audited page. Each carries a `score_confidence` label so a reader can tell
  how the number was produced: `lighthouse` (measured by Lighthouse), `composite`
  (blend of sub-scores), `heuristic` (markup signals only) or `verified_missing`
  (verified that no site exists).
- AI answer presence (`ai_presence`) is a measurement, not a score: when
  `LEADSCOUT_AI_PRESENCE=1`, LeadScout runs the category/city questions a person
  would ask and records whether the business is cited, with the mention rate and
  best rank. It is stored separately from the heuristic AI Visibility number.

Absolute readiness scores are not comparable across markets. `query_leads` and
the HTTP list, export and single-lead endpoints annotate each row with
`ai_visibility_score_percentile`, the share of the current sample at or below
that lead, so a report can say "top 20% here".

## Guardrails

- Missing evidence is not automatically negative.
- Booking relevance depends on category.
- Store score reasons.
- Do not use sensitive personal characteristics.
- Never present a missing measurement as a position (percentile stays null).
