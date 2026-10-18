---
name: snoweyes-collector
description: >
  Clown 2.1 information collection and evidence-normalization workflow for
  authorized targets. Use Grok orchestration, FOFA-style search MCPs, the
  SnowEyes MCP, and optional browser extension signals to build a scoped asset,
  route, API, technology, identity, and evidence graph before validation.
---

# SnowEyes Collector

SnowEyes Collector is the content-aware collection lane in Clown 2.1. It turns
authorized page, HTML, JavaScript, response, browser, and search-MCP signals
into a deduplicated collection graph that downstream validation skills can use.
It is a workflow skill with a local MCP implementation, not a passive file
bundle.

## Operating Contract

Before collecting, require:

- `scope_ref`: stable task identifier;
- `authorized_assets`: domains, hosts, applications, accounts, or test ranges;
- `input_kind`: `url`, `html`, `javascript`, `response`, `browser`, or `search_mcp`;
- `collection_mode`: `passive`, `content`, `browser`, or `active_probe`;
- `budget`: request, host, time, and concurrency limits.

Never infer authorization from a hostname, a search result, a framework name,
or a browser page alone. Keep the scope reference on every record.

## Full Workflow

### 1. Scope and intake

Grok creates a run profile and records the allowed assets, exclusions, time
window, rate limit, session owner, and permitted action classes. Reject
out-of-scope hosts and third-party redirects before they enter the queue.

### 2. External surface collection

Use FOFA, Hunter, Quake, ThreatBook, and ZoomEye MCPs for passive discovery:

- hostnames, ports, banners, certificates, historical records;
- organization relationships and likely application groupings;
- service technology hints and candidate entry points.

Search MCP output is a candidate layer. Normalize it, attach the query and
timestamp, then compare it with live or content-derived signals.

### 3. Content collection

Prefer `snoweyes_scan_content` when HTML, JavaScript, or a response is already
available. Use `snoweyes_scan` only when an authorized URL must be fetched by
the local MCP. Extract:

- absolute and relative URLs, domains, ports, API paths, routes, and methods;
- script, module, source-map, documentation, upload, callback, and auth hints;
- framework, library, build, feature, and deployment fingerprints;
- parameter names, object identifiers, state tokens, and error markers;
- sensitive-value categories without retaining secret values.

The optional browser extension is useful for runtime-only content such as
lazy-loaded routes, navigation state, network metadata, and rendered
application behavior. Send its output through the same normalization contract.

### 4. Normalize and protect

Convert every observation to the schema in `references/result-schema.json`.
Redact cookies, JWTs, API keys, passwords, private data, internal paths, and
session values. Keep only a type, location, masked preview, hash, or length
when a secret-like value is detected.

Attach:

- `scope_ref`, `run_id`, `observed_at`, and `input_ref`;
- `collector` and `collector_version`;
- `confidence` and `status`;
- a short evidence excerpt or hash;
- a stable `dedupe_key`.

### 5. Deduplicate and correlate

Deduplicate in this order:

1. normalized host and port;
2. normalized route and HTTP method;
3. parameter set and technology fingerprint;
4. observation time and collector overlap.

Preserve independent observations as links on one record. Do not merge records
when scope, authentication state, method, or response behavior differs.

### 6. Prioritize

Grok ranks records using scope confidence, business relevance, identity
boundary, data sensitivity, reachability, novelty, and evidence quality.
High-value candidates should have a concrete next action and a stop condition.
Unverified fingerprints remain leads and do not become findings.

### 7. Hypothesis handoff

For each prioritized candidate, emit:

- `hypothesis`: the smallest testable claim;
- `required_context`: login role, tenant, object, or browser state;
- `baseline_request`: read-only request or captured observation;
- `comparison`: expected control and differential signal;
- `impact_signal`: what would justify escalation;
- `rollback`: how to restore any state;
- `owner`: the next Clown 2.1 specialist lane.

### 8. Active probe gate

Active probing is disabled by default. Enable it only after explicit scope,
method, rate, request-count, impact, and rollback checks pass. Use
`snoweyes_probe` for a planned request and reserve `snoweyes_probe_exec` for a
separately approved execution. Stop on rate limiting, state change, sensitive
data exposure, unexpected redirect, or inability to attribute the response.

## MCP Tool Routing

| Need | Tool | Default |
|---|---|---|
| Discover hosts and services | FOFA/Hunter/Quake/ThreatBook/ZoomEye MCP | passive |
| Parse known HTML, JS, or response | `snoweyes_scan_content` | preferred |
| Fetch and inspect an authorized URL | `snoweyes_scan` | bounded |
| Inspect local dictionaries | `snoweyes_dict` | read-only |
| Find likely parameter names | `snoweyes_params` | read-only |
| Fingerprint a known response | `snoweyes_fingerprint` | read-only |
| Build a probe plan | `snoweyes_probe` | approval required |
| Execute a planned probe | `snoweyes_probe_exec` | explicit approval |
| Review prior runs | `snoweyes_history` | read-only |

## Grok Output Contract

Return a compact run summary with:

```json
{
  "scope_ref": "scope-id",
  "run_id": "run-id",
  "status": "complete|partial|blocked",
  "coverage": {
    "hosts": 0,
    "routes": 0,
    "apis": 0,
    "technologies": 0,
    "hypotheses": 0
  },
  "top_candidates": [],
  "blocked_reasons": [],
  "next_lane": "clown-specialist-name"
}
```

Keep the full records in the run artifact directory and pass only the
deduplicated summary into the next lane. Load
`references/collection-workflow.md` for decision details and
`references/result-schema.json` for the record contract.

## Safety Boundary

This skill is for authorized security research, asset inventory, defensive
review, and controlled validation. It must not bypass access controls, collect
unnecessary personal data, retain secrets, perform uncontrolled crawling, or
execute state-changing requests without the required approval.
