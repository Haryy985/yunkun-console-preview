# Lane: Shadow Architecture

## Purpose

Use a current model as an architecture-hypothesis engine after there is enough observed evidence. The output is a compact, falsifiable model of actors, objects, services, state transitions, and trust boundaries. It is not recovered source code and it does not create white-box evidence.

## Entry Criteria

Load this lane only when both are true:

1. At least one runtime source exists: captured request/response, HAR, runtime snapshot, WMPF inventory, browser XHR/fetch, WebSocket trace, or server log.
2. At least one independent structural or flow source exists: JS bundle, route table, OpenAPI/schema, source artifact, object lifecycle, auth flow, callback, queue, or state transition.

Do not load it from a page title, a framework fingerprint, a single static route list, or model intuition.

## Model Contract

The blueprint contains only these evidence-backed items:

- **Actor**: current identity, role, service account, callback sender, worker.
- **Entity**: business object, file, token, export job, tenant, or record.
- **Operation**: observed read/write/export/upload/login/callback action.
- **State**: observed or explicitly documented lifecycle state.
- **Boundary**: authorization, ownership, tenant, transport, queue, or external dependency boundary.
- **Relation**: a claimed interaction between two items, always marked `hypothesis`, `confirmed`, `discarded`, or `blocked`.

Every item must cite raw artifacts in the current run's `evidence/` directory. Keep the model small: start with no more than 8 nodes and 6 relations. Add detail only when a validation result requires it.

## Workflow

1. Normalize real observations into a redacted draft JSON file. Preserve original raw traffic separately.
2. Ask the current model to infer competing architecture hypotheses, not implementation code. It must name its uncertainty and propose a one-variable probe for each relation.
3. Run `scripts/shadow_architecture.py validate` to reject missing, external, or invented evidence references.
4. Run `plan`; test only the highest-value relation using baseline, variant, and negative control.
5. Run `record` with raw verification evidence. Confirmed relations may guide subsequent coverage; discarded relations must not reappear as facts.
6. Freeze the lane after two relations fail to produce new evidence. Return to the primary surface lane.

## Probe Discipline

- A relation is not confirmed because the model produced plausible code or a familiar framework pattern.
- A relation is not a vulnerability. Promote findings only through the normal evidence and impact rules.
- Prefer identity -> object -> state -> operation and producer -> queue -> consumer chains over generic endpoint speculation.
- For an unknown route, test the smallest legitimate action first; do not infer all hidden endpoints from an imagined controller.
- Do not send raw credentials, session values, PII, or file contents to an external model. Use redacted field names, lengths, types, routes, and response classes.

## Draft Shape

`shadow_architecture.py` validates a JSON object with `evidence`, `nodes`, and `relations`. Each evidence entry contains an ID, type, and a path under `<run>/evidence/`. Each relation includes a single-variable probe with baseline, variant, negative control, and decisive signal. See `scripts/shadow_architecture.py --help` for the exact contract.
