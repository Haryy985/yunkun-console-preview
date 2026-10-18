# Authorized Use and Safety Boundary

Use the package only against assets and environments with explicit
authorization. Keep the approved scope, test identities, time window, rate
limits, and prohibited actions attached to the run.

## Required Controls

- prefer passive collection and read-only checks;
- use synthetic or disposable objects for stateful validation;
- change one variable at a time;
- stop on unexpected state changes, sensitive data exposure, or rate limits;
- redact credentials, tokens, personal data, and private network details;
- restore test state and record cleanup before handoff.

## Restricted Actions

Do not perform persistence, evasion, uncontrolled lateral movement,
destructive actions, bulk enumeration, credential spraying, or data
collection beyond the approved objective. A finding is complete only when
the impact is reproducible and the final state is confirmed.
