---
name: target-profiling
description: >
  Target inventory, technology graph, trust-boundary, and attack-surface
  prioritization lane for authorized reviews.
---

# Target Profiling Lane

This lane converts collection records into a bounded target profile. It does
not replace collection and it does not claim a vulnerability from exposure
alone.

## Profile Schema

```text
target_id:
scope:
domains:
hosts:
applications:
route_families:
identities:
tenants:
technologies:
data_classes:
trust_boundaries:
exclusions:
sources:
last_seen:
confidence:
open_questions:
```

## Build Process

1. Merge records by canonical host, route, application, and object key.
2. Preserve every source, timestamp, query, and confidence value.
3. Separate direct evidence from inference.
4. Map public entry points to authentication, API, storage, job, and
   administrative boundaries.
5. Rank only in-scope assets by exposure, privilege, data sensitivity, and
   business value.
6. Produce two or three concrete hypotheses with a stated stop condition.

## Priority Signals

Prioritize an asset when several independent signals agree:

- a route is live and tied to a known application;
- an identity or tenant boundary is observable;
- a sensitive operation has a stable baseline;
- a high-value object or state transition is reachable;
- a technology fingerprint is confirmed by response or source evidence.

Treat shared hosting, banners, open ports, and stale DNS as context only until
the application relationship is confirmed.

## Output

Return a profile, a ranked shortlist, missing data, and the next bounded
collection or validation action. Link each candidate to the evidence record
that created it.
