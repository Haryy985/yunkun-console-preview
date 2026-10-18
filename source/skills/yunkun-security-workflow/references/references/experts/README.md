# Expert Module Registry

The expert modules are optional depth references for the six primary lanes.
They are part of the Clown 2.1 system and are loaded only after a concrete
signal has been recorded.

## Loading Contract

Every expert module must:

1. accept a target, scope, identity, and hypothesis from a primary lane;
2. preserve the baseline, mutation, impact, cleanup, and evidence fields;
3. use bounded, reversible actions;
4. return one of `confirmed`, `incomplete`, `negative`, or `blocked`;
5. hand control back to the primary lane.

An expert is never a substitute for scope confirmation or the evidence gate.

## Module Map

| Signal family | Expert module |
| --- | --- |
| API authentication and tokens | `api-auth-and-jwt-abuse` |
| API object authorization | `api-authorization-and-bola` |
| API route and schema discovery | `api-recon-and-docs` |
| Deserialization and parser boundaries | `deserialization-insecure` |
| File handling | `file-upload-methodology` |
| GraphQL | `graphql-methodology` |
| Object authorization | `idor-broken-object-authorization` |
| MCP and tool security | `mcp-security` |
| OAuth and SSO | `oauth-sso-attack` |
| Prompt and agent security | `prompt-injection`, `agent-security`, `ai-identity-security` |
| Race and state integrity | `race-condition` |
| Request boundary behavior | `request-smuggling` |
| Recon analysis | `recon-and-methodology` |
| General review model | `security-testing-methodology` |
| SSRF | `ssrf-methodology` |
| WebSocket | `websocket-security` |
| XSS | `xss-methodology` |

Use package-root paths when linking from an expert:
`skills/...`, `references/...`, or `references/experts/...`.
