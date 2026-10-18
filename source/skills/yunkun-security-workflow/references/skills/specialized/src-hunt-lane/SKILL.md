---
name: src-hunt-lane
description: >
  Iterative authorized SRC discovery and validation lane. It keeps one
  bounded hypothesis active, prevents idle scanning, and returns every result
  through the common evidence contract.
---

# SRC Hunt Lane

Use this lane when the target is a confirmed authorized program or enterprise
scope and the task requires structured discovery followed by focused
validation. It is an orchestration lane, not a replacement for the API,
business-flow, profiling, collection, or evidence lanes.

## Mode Selection

| Input | Mode |
| --- | --- |
| Fixed domains, hosts, or applications | bounded scope mode |
| Authorized program with a target family but incomplete inventory | discovery mode |
| Source tree or build artifact | white-box mode |

Do not widen scope from a weak technology match. A new asset needs an
independent scope signal or explicit authorization.

## Closed Loop

1. Confirm scope, identities, request limits, exclusions, and stop conditions.
2. Create or update the target profile.
3. Seed passive collection from the approved target set.
4. Normalize and deduplicate assets, routes, scripts, and parameters.
5. Pick one seed with the strongest combination of reachability, privilege,
   state impact, and evidence quality.
6. Build one hypothesis with a control request and one expected invariant.
7. Route to exactly one specialist lane.
8. Validate the smallest reversible change.
9. Record `confirmed`, `incomplete`, `negative`, or `blocked`.
10. Return to the queue only after cleanup and evidence recording.

## Anti-Idle Gates

- A route name, banner, status code, or scanner hit is a lead only.
- Two consecutive no-change checks require a hypothesis review.
- Repeated authorization failures require a route change or a stop record.
- Rate limits pause the source and do not justify source rotation.
- Do not run a full matrix before the current seed has a decision.
- Do not enumerate unrelated objects or identities.

## Signal Routing

| Signal | Route |
| --- | --- |
| HTML, JavaScript, API routes, parameters, or rendered state | `snoweyes-collector` |
| Asset graph, technology, host, or trust boundary | `target-profiling` |
| API, token, object, schema, or transport boundary | `api-security` |
| Payment, order, quota, approval, workflow, or state integrity | `business-logic-attack` |
| SQL injection signal | `sqli-sql-injection` |
| Reproduction, severity, or submission decision | `src-report-paradigm` |

Canonical detailed checks live under
`skills/skill/knowledge-base/` and `references/playbooks/`. The expert
modules under `references/experts/` are optional depth references selected by
the primary lane.

## Seed Record

```text
seed_id:
scope:
asset:
source:
collected_at:
route_family:
identity:
object_or_tenant:
signal:
hypothesis:
control:
mutation:
stop_condition:
status:
next_action:
```

## Handoff Rules

Every specialist result must include the seed ID, the original source, the
control, the mutation, the response delta, the state effect, cleanup, and
evidence references. If the specialist cannot decide safely, return `blocked`
with the exact missing prerequisite.

This lane is complete only when the current seed is closed, handed to the
evidence lane, or explicitly stopped with a reason.
