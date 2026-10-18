# SnowEyes Collection Workflow

## Run State

Use the following state machine:

`created -> scoped -> discovered -> content-collected -> normalized -> deduplicated -> prioritized -> handed-off`

Optional branches:

- `scoped -> blocked` when authorization or limits are missing;
- `content-collected -> needs-browser` when runtime-only content is required;
- `prioritized -> probe-pending` when a hypothesis needs an active request;
- any state -> `stopped` when a stop condition is reached.

## Collection Lanes

### Search lane

Search MCPs produce passive candidates. Query by the narrowest known scope,
preserve the query and time, and group results by host, service, certificate,
technology, and organization relationship.

### Content lane

SnowEyes parses known HTML, JavaScript, response bodies, and URLs. Prefer
content parsing over a new fetch. A content record should identify the input
kind, parser mode, extracted categories, and confidence.

### Browser lane

Use the extension only when rendered state, lazy routes, client-side requests,
or user-flow context cannot be recovered from saved content. Capture the page
URL, navigation action, request metadata, and session owner. Do not store raw
session values.

### Validation handoff

The collection lane ends with a bounded hypothesis, not a vulnerability
claim. The next lane must receive a baseline, comparison, impact signal, and
rollback rule.

## Priority Score

Use a 0-5 score for each dimension:

`priority = 3*scope + 3*business + 2*identity + 2*reachability + novelty + evidence`

Subtract:

- 5 for uncertain scope;
- 3 for an unverified fingerprint;
- 3 for duplicate-only observations;
- 2 for excessive request cost.

Do not use the score to override a scope or safety block.

## Stop Conditions

Stop the current branch when:

- the asset is outside the authorized scope;
- a request or time budget is exhausted;
- a rate limit, WAF response, or operational warning appears;
- a state-changing endpoint is reached without an approved plan;
- secret-like or unnecessary personal data is exposed;
- the result cannot be attributed to the target;
- the candidate has no testable differential.
