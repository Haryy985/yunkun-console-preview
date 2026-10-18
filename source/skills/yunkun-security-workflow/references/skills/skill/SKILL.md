---
name: skill
description: >
  Deep routing entry for the Clown 2.1 authorized security workflow. It
  coordinates collection, target profiling, specialist validation, evidence
  quality, and replay without opening unrelated branches.
---

# Clown 2.1 Deep Entry

Read this file after the package root entry when the task needs detailed
routing. The canonical workflow is:

```text
scope -> profile -> collect -> normalize -> prioritize
      -> hypothesis -> validate -> evidence -> report -> replay
```

## Route Table

| Need | Primary lane |
| --- | --- |
| Search, HTML, JavaScript, browser, or response extraction | `skills/specialized/snoweyes-collector/SKILL.md` |
| Asset graph, technology, role, tenant, and trust boundary | `skills/specialized/target-profiling/SKILL.md` |
| Iterative authorized discovery and seed management | `skills/specialized/src-hunt-lane/SKILL.md` |
| API, token, object, schema, transport, or file boundary | `skills/specialized/api-security/SKILL.md` |
| Payment, order, quota, workflow, approval, or state integrity | `skills/specialized/business-logic-attack/SKILL.md` |
| Reproduction, impact, severity, and submission readiness | `skills/specialized/src-report-paradigm/SKILL.md` |

## Canonical Knowledge Base

The detailed checks live in `knowledge-base/`. Start with
`knowledge-base/README.md`, then open the smallest file matching the current
signal:

- `recon-methodology.md` for asset and route modeling;
- `idor-test.md` for object and function authorization;
- `authbypass-test.md` and `oauth-jwt-test.md` for identity and token
  boundaries;
- `api-gateway-test.md`, `graphql-test.md`, and `websocket-test.md` for API
  transport and schema boundaries;
- `logic-test.md` and `race-condition-test.md` for business-state integrity;
- `injection-test.md`, `ssrf-test.md`, `xss-test.md`, and
  `file-upload-test.md` for input and content boundaries;
- `info-leak-test.md`, `path-traversal-lfi-test.md`, and
  `http-smuggling-test.md` for exposure and request-boundary signals.

## Evidence Contract

Every lane must preserve:

```text
scope_ref:
run_id:
asset:
source:
observed_at:
hypothesis:
baseline:
mutation:
response_delta:
state_effect:
impact:
cleanup:
evidence:
status:
next_action:
```

The status is one of `confirmed`, `incomplete`, `negative`, or `blocked`.
Missing proof is recorded as a gap; it is never filled by inference.

## Operating Rules

- Use passive collection first.
- Keep one active hypothesis per branch.
- Change one variable per validation.
- Preserve provenance and redact secrets.
- Stop on scope ambiguity, rate limiting, unexpected state change, or
  unbounded impact.
- Load one expert module only when a primary lane identifies a concrete need.
- Return all expert results to the same evidence contract.

The architecture contract is in `references/architecture/route-contract.md`.
The machine-readable registry is in
`references/architecture/skill-registry.json`.
