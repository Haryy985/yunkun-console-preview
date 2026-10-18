---
name: api-security
description: >
  Unified API security lane for authorized REST, GraphQL, gateway, token,
  object authorization, file, and asynchronous API review.
---

# API Security Lane

This is the single API entry point. It owns API inventory, authentication
state, object authorization, input handling, transport behavior, and evidence
handoff. Do not fan out to several API routers for the same hypothesis.

## Intake

Capture:

- base URL, route family, method, content type, and version;
- anonymous, low-privilege, peer-role, and administrator identities;
- tenant, object, parent object, and ownership relationships;
- schema source, client source, response source, and collection timestamp;
- request budget, rate limit, and rollback plan.

Reject a candidate when the only signal is a banner, an undocumented route,
an error string, or a client-side restriction.

## Unified Decision Flow

1. **Inventory**: correlate passive search records, SnowEyes routes, scripts,
   API schemas, browser calls, and response links.
2. **Model**: write the route family, identity, object, tenant, state, and
   expected invariant before sending a mutation.
3. **Baseline**: replay one known-good request with a stable identity and
   record status, body class, headers, timing, and state effect.
4. **Authorization**: compare the same operation across identities and object
   ownership. Prefer harmless read or metadata checks.
5. **Input boundary**: change one parameter at a time. Confirm type, range,
   parser, normalization, and server-side validation.
6. **Protocol boundary**: only when supported by evidence, compare HTTP
   versions, duplicate headers, cache behavior, WebSocket upgrades, or
   asynchronous job state.
7. **Impact**: connect the response delta to a real object, identity, tenant,
   or business-state change.
8. **Evidence gate**: send the candidate to the report lane or record the
   exact missing proof and stop.

## Route Matrix

| Signal | Canonical next file |
| --- | --- |
| API discovery and route modeling | `skills/skill/knowledge-base/recon-methodology.md` |
| REST or gateway behavior | `skills/skill/knowledge-base/api-gateway-test.md` |
| GraphQL schema or resolver boundary | `skills/skill/knowledge-base/graphql-test.md` |
| Session, token, OAuth, or JWT behavior | `skills/skill/knowledge-base/oauth-jwt-test.md` |
| Object or function authorization | `skills/skill/knowledge-base/idor-test.md` |
| Business-state transition | `skills/specialized/business-logic-attack/SKILL.md` |
| File upload or path handling | `skills/skill/knowledge-base/file-upload-test.md` |
| Input parser or injection signal | `skills/skill/knowledge-base/injection-test.md` |
| HTTP/2, cache, or request boundary | `skills/skill/knowledge-base/http-smuggling-test.md` |
| WebSocket message boundary | `skills/skill/knowledge-base/websocket-test.md` |
| Evidence and submission decision | `skills/specialized/src-report-paradigm/SKILL.md` |

For deeper treatment, load one matching module from
`references/experts/`. The expert must preserve the same baseline, mutation,
impact, cleanup, and evidence fields and must return to this lane.

## Minimal Validation

Use a four-record comparison:

1. control request;
2. one-variable mutation;
3. independent identity or object comparison;
4. final-state confirmation and cleanup.

Do not enumerate unrelated IDs, brute force credentials, or send destructive
requests. Use synthetic or already-authorized objects whenever possible.

## Handoff

Return a structured result:

```text
asset:
route_family:
identity:
object_or_tenant:
hypothesis:
baseline:
mutation:
response_delta:
state_effect:
impact:
cleanup:
evidence:
status: confirmed | incomplete | negative | blocked
next_action:
```
