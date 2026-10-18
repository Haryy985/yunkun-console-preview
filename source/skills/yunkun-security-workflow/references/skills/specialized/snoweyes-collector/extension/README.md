# SnowEyes Browser Collector

This optional browser runtime feeds the Clown 2.1 collection lane with:

- rendered-page and lazy-route observations;
- client-side API, route, script, and framework signals;
- runtime navigation and request metadata;
- local fingerprint and parameter hints.

The runtime performs local analysis only. Keep it limited to the active
authorized scope and send normalized records to the SnowEyes MCP contract.
