---
name: src-report-paradigm
description: >
  Evidence, severity, reproduction, and submission-quality lane for
  authorized security findings.
---

# Report and Evidence Lane

This lane decides whether a candidate is ready to report. It does not turn
weak signals into findings. Use it after a specialist has produced a concrete
baseline and impact record.

## Required Structure

1. **Summary**: one sentence naming the affected function and security effect.
2. **Scope**: exact host, route family, identity, and authorization context.
3. **Prerequisites**: only the minimum setup needed to reproduce.
4. **Reproduction**: complete requests and responses with secrets redacted.
5. **Control**: the normal request or expected behavior.
6. **Impact**: the changed object, identity, tenant, or business state.
7. **Cleanup**: restored state, deleted test data, and final-state check.
8. **Fix direction**: server-side control that enforces the invariant.

## Three Gates

### Gate A: Scope

The asset and action are authorized, in scope, and within the request budget.
The identity and test data are identified.

### Gate B: Proof

The report contains a control, one bounded mutation, a reproducible response
delta, and an observable security impact. A route name, version, scanner
finding, or copied token is not proof by itself.

### Gate C: Reproducibility

Another reviewer can reproduce the result from the written sequence without
guessing omitted headers, cookies, bodies, object IDs, or state transitions.

## Decision

Use one of these states:

- `confirmed`: all gates pass and impact is demonstrated;
- `incomplete`: one proof item is missing;
- `negative`: the control and mutation show no security impact;
- `blocked`: authorization, state, rate limit, or safety prevents a valid
  decision.

Do not assign severity before Gate B. Keep the rating proportional to the
demonstrated impact, affected identities, data sensitivity, and required
conditions.

## Output Record

```text
title:
asset:
affected_function:
identity:
precondition:
control_request:
mutated_request:
observed_delta:
security_impact:
cleanup:
evidence_refs:
status:
severity:
fix_direction:
```

See `references/accepted-report-templates.md` for the stable report shape.
