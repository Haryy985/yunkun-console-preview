# Clown 2.1 Route Contract

## State Machine

```text
scope
  -> profile
  -> collect
  -> normalize
  -> prioritize
  -> hypothesis
  -> validate
  -> evidence
  -> report
  -> replay
```

Each transition must write a state record. A transition may stop with
`blocked` when authorization, safety, rate limits, or missing prerequisites
prevent a valid decision.

## Single-Hypothesis Rule

One active branch contains one asset, one identity context, one object or
state boundary, one expected invariant, and one stop condition. A new
specialist can be loaded only when it answers the current hypothesis.

## Evidence Record

```text
record_id:
source:
collected_at:
asset:
route:
identity:
object:
baseline:
mutation:
response_delta:
state_effect:
impact:
cleanup:
confidence:
status:
```

## Quality Rules

- Preserve provenance through every transformation.
- Redact secrets before persistence or handoff.
- Prefer independent sources for discovery, not duplicated noise.
- Prefer read-only or reversible validation.
- Mark inference separately from direct observation.
- Never assign a vulnerability label before impact is demonstrated.
