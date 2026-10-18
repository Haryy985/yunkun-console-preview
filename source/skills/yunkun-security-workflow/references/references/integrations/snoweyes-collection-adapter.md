# SnowEyes Collection Adapter

## Role

SnowEyes is the content-aware collection adapter for Clown 2.1. It complements
FOFA, Hunter, Quake, ThreatBook, and ZoomEye by inspecting authorized page,
HTML, JavaScript, response, URL, and browser-runtime signals.

## Runtime

The local Node MCP entry is:

`mcp-servers/snoweyes-mcp/snoweyes-mcp.js`

The optional browser runtime is:

`skills/specialized/snoweyes-collector/extension/`

Configure an external browser runtime with `SNOWEYES_EXTENSION_PATH` when
needed. The packaged runtime is self-contained and does not require external
credentials.

## Routing

1. Use search MCPs to build a passive host and service candidate set.
2. Use `snoweyes_scan_content` for already captured HTML, JavaScript, or
   response content.
3. Use `snoweyes_scan` only for an authorized URL that requires local fetching.
4. Use browser collection when lazy routes or rendered state are missing.
5. Normalize all outputs with
   `skills/specialized/snoweyes-collector/references/result-schema.json`.
6. Hand only deduplicated, redacted candidates to the next specialist lane.

## Tool Policy

`snoweyes_probe` creates a bounded plan. `snoweyes_probe_exec` is disabled until
scope, method, rate, request count, impact, and rollback checks are approved.
