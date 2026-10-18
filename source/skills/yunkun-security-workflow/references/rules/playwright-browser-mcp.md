# Playwright Browser MCP — Always Prefer for Browser Work

This rule applies to **every session**. The official Claude Code browser-control MCP is **Playwright MCP** (`@playwright/mcp`), configured globally as the Grok MCP server named `playwright`.

## Mandatory tool routing

Whenever the user asks to do any of the following, **do not** use shell/`npx playwright` scripts, raw Chrome CDP hacks, or built-in `web_fetch`/`open_page` as a substitute for interactive control:

- open / control a real browser
- click, type, scroll, fill forms, submit
- take page snapshots / screenshots of a live UI
- verify UI after code changes
- navigate multi-step web flows
- extract content that needs JS rendering or authenticated pages
- generate PDF from a page
- run browser-based QA / smoke checks

**Always:**

1. Call `search_tool` with query like `playwright browser navigate snapshot click` (or the needed action) first.
2. Call the discovered tools via `use_tool` with fully-qualified names such as `playwright__browser_navigate`, `playwright__browser_snapshot`, `playwright__browser_click`, etc.
3. Prefer **accessibility snapshots** over screenshots when understanding page structure; use vision/screenshot tools when visual verification is required.
4. Keep the browser session open across multi-step tasks; do not relaunch unnecessarily.

## When NOT to use Playwright MCP

- Pure static HTTP fetch of a public URL with no interaction → built-in `web_fetch` / `web_search` is fine.
- Local file edits, git, terminal, code analysis → built-in tools only.
- The `playwright` MCP server is disabled or `search_tool` returns no playwright tools → report that clearly and fall back.

## Auto-invocation checklist

Before answering browser-related requests with only text or shell:

- [ ] Did I `search_tool` for playwright?
- [ ] Did I use `playwright__*` tools for actual control?
- [ ] Did I avoid inventing bash one-liners to drive the browser?

If any checkbox fails and the task needs a real browser, **stop and call the MCP**.

## Session expectation

Playwright MCP is **always enabled** in `~/.grok/config.toml` as `[mcp_servers.playwright]`. Treat it as a first-class tool path, not an optional plugin the user must re-enable each time.
