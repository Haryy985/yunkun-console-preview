# Clown 2.1 Specialist Registry

This registry maps an observed signal to one primary lane. The selected lane
owns the hypothesis until it reaches a decision or records a blocking reason.
Optional expert modules may deepen the current hypothesis, but never create a
second evidence chain.

## Primary Routing

| Signal | Primary lane | First action |
| --- | --- | --- |
| New host, route, script, API, or browser signal | `snoweyes-collector` | Normalize source and timestamp |
| Technology, trust boundary, tenant, or asset relationship | `target-profiling` | Update the target profile |
| Broad authorized discovery or iterative seed work | `src-hunt-lane` | Create one bounded seed |
| REST, GraphQL, token, object, file, or transport boundary | `api-security` | Establish control and identity model |
| Payment, order, quota, coupon, approval, or workflow state | `business-logic-attack` | Write the expected invariant |
| SQL injection signal | `sqli-sql-injection` | Confirm a controlled differential |
| Reproduction, impact, severity, or report readiness | `src-report-paradigm` | Run the evidence gates |

## Optional Depth Modules

Load only the module that answers the current hypothesis:

- API identity and token behavior: `references/experts/api-auth-and-jwt-abuse/`;
- API object authorization: `references/experts/api-authorization-and-bola/`;
- API route and schema analysis: `references/experts/api-recon-and-docs/`;
- GraphQL: `references/experts/graphql-methodology/`;
- WebSocket: `references/experts/websocket-security/`;
- file processing: `references/experts/file-upload-methodology/`;
- request boundaries: `references/experts/request-smuggling/`;
- concurrency and state integrity: `references/experts/race-condition/`;
- SSRF, XSS, parser, OAuth, and agent signals: the matching directory under
  `references/experts/`.

## Handoff Record

Every handoff must preserve:

```text
seed_id:
scope_ref:
asset:
source:
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

The only valid terminal statuses are `confirmed`, `incomplete`, `negative`,
and `blocked`. A route is not evidence, and a specialist name is not a
finding.
