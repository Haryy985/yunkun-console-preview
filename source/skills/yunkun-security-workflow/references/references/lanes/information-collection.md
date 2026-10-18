# Information Collection Lane

## Objective

Build a scoped, deduplicated, evidence-traceable asset and content graph before
active validation. The executable workflow is
`skills/specialized/snoweyes-collector/SKILL.md`.

## Collection graph

`scope -> search MCP -> SnowEyes content -> browser runtime -> normalize -> deduplicate -> prioritize -> hypothesis`

## Division of labor

- Search MCPs discover hosts, services, banners, historical assets, and
  organization relationships.
- SnowEyes parses domains, API paths, routes, scripts, frameworks,
  authentication hints, and parameter candidates from known content.
- The browser runtime supplies lazy routes, rendered state, network metadata,
  and client-side behavior that static content cannot expose.
- Grok correlates all lanes, assigns confidence, selects the next specialist,
  and enforces budgets and stop conditions.

## Required handoff

Every prioritized record must include a scope reference, input reference,
observation time, dedupe key, confidence, evidence pointer, testable
hypothesis, baseline, comparison, impact signal, and rollback rule.

## Stop conditions

Stop a collector when the target is out of scope, the request budget is
exhausted, a rate limit appears, a state-changing action is reached, sensitive
data would be unnecessarily collected, or the result cannot be attributed.
