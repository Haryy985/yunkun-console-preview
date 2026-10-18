# Agent Skills Go Integration

## Runtime Changes

- `analyze_web_surface` is a Go-native, read-only analyzer for HTML and JavaScript text already supplied to the tool. It extracts forms, script references, links, route paths and API-like paths. It does not fetch URLs, submit forms or expand task scope.
- Active crawling is not exposed as an Agent tool. The current crawler implementation uses its own HTTP transport and cannot guarantee the server-owned task proxy route and traffic capture are used. For target access, use the existing managed `WebFetch` path, then pass its returned HTML/JavaScript to `analyze_web_surface`.
- `asset_intel_search` calls FOFA, Quake, Hunter or ZoomEye from the Agent runtime. The query must include a domain or IP already present in the current task scope. Results are filtered against that scope again and returned as candidates only; they are not written to assets or task scope.
- `threatbook_lookup` provides scoped IP reputation or domain DNS-threat lookups. It does not change task scope or findings.
- The two external-query tools are seeded into the tool catalog but default to unbound. An administrator must bind them to the intended Agent before use; provider calls can consume third-party quota.

## Credentials

Credentials are read from the service process environment. They are never included in tool descriptions or returned results.

| Provider | Environment variables |
| --- | --- |
| FOFA | `FOFA_EMAIL`, `FOFA_KEY` (or `FOFA_API_KEY`); optional backup pairs `FOFA_EMAIL_BACKUP`, `FOFA_KEY_BACKUP` (or `FOFA_API_KEY_BACKUP`) and `FOFA_EMAIL_BACKUP2`, `FOFA_KEY_BACKUP2` (or `FOFA_API_KEY_BACKUP2`) |
| Quake | `QUAKE_API_KEY`; optional `QUAKE_API_KEY_BACKUP` |
| Hunter | `HUNTER_API_KEY`; optional `HUNTER_API_KEY_BACKUP` |
| ZoomEye | `ZOOMEYE_API_KEY`; optional `ZOOMEYE_API_KEY_BACKUP` |
| ThreatBook | `THREATBOOK_API_KEY`; optional `THREATBOOK_API_KEY_BACKUP` |

Set credentials in the operating-system service environment before starting the Go service. Do not put real keys in source control, task prompts, reports or screenshots.

## Boundaries

- External search results are not authorization. Only task scopes already stored by the application authorize returned candidates.
- Provider queries require an exact domain or IP anchor. Broad title/body/port-only searches are not exposed to the Agent tool.
- Search results are not automatically persisted, probed or used to create task scopes.
- Provider requests use fixed HTTPS endpoints, bounded response bodies and a 30-second client timeout. Provider calls go from the application service to the named intelligence vendor; they are not target probes and are not recorded as target traffic through the task proxy gateway. Provider errors are summarized without returning credentials or raw response bodies.
- The Skills tools do not replace the Agent planner, model selection, scheduling or execution loop. Separately, the 1.0.3 compatibility fixes remove the added unknown-model 64K request rejection while retaining configurable upstream compaction; see the release notes.

## Bundled Workflow

- `yunkun-security-workflow/SKILL.md` is the on-demand entry point for scope, validation, evidence, HTTP packet reporting and reviewed knowledge proposals.
- New installations seed visibility for the existing auto, planner, pentest, worker and reporter roles. Existing user visibility preferences are preserved.
- Selected Markdown rules, references, specialized workflow documents and templates are bundled beneath this workflow's `references` directory. Load only relevant sections; do not concatenate the whole collection into model requests.
- Original provider accounts, MCP configuration, hooks and supervisor/coordinator settings are excluded. Optional public api-recon, playwright-cli and scopesentry resources remain separate; their presence is not a claim that every dependency or Python script has been converted into Go.

## Source Disposition

- Static surface extraction from `api-recon` is represented by a Go tool. Active crawling is not runtime-exposed because it cannot currently guarantee use of managed task egress and traffic capture. `build_perm_tree.py`'s target-specific permission-tree extraction remains unported.
- FOFA, Quake, Hunter and ZoomEye MCP clients are represented by one provider-selecting Go tool; ThreatBook is a separate scoped lookup tool.
- Finding, evidence, report, scope and coverage guardrails are implemented in application code and are not model-callable bypass switches.
- WSL setup, package validation, local link auditing and the Skills supervisor/hooks remain developer-side material; they are not copied into the Agent process.
- Other scripts that mirror existing task workflow or require specialized target-specific authorization are not automatically exposed as tools.

## Verification Limits

- Unit tests cover provider query syntax, task-scope matching, bounded static extraction, crawler parser/boundary helpers and mocked FOFA parsing. The crawler is not runtime-exposed until it can use the managed task egress path without weakening scope enforcement.
- Live provider checks require deployment-owned API credentials and may consume quota. They are not part of the automated test suite.
