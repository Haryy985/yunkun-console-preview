# HTTP Proxy Format Trick (CTF)

## What

Some web servers behave differently when the request line contains a **full URL** (`GET http://host/path HTTP/1.1`) versus a **path-only** URL (`GET /path HTTP/1.1`). The full-URL format is how HTTP forward proxies send requests — the server may treat it as "coming through a proxy" and reveal hidden data (in headers or body).

## When to try

- A path returns "path not found" or "wrong way" but the endpoint clearly exists
- Challenge narrative mentions "proxy", "gateway", "中转", "破阵罗盘", "窥天之桥"
- Server responds differently to `nc`/raw requests than to `curl`

## How

```bash
# Full URL in the request line (proxy format)
printf "GET http://target:port/path HTTP/1.1\r\nHost: target:port\r\nConnection: close\r\n\r\n" | nc target port

# Equivalent with curl (curl doesn't natively do this, use nc or raw)
```

## What to look for

- **Response headers** — flag may be in a custom header like `X-Jinxi-Secret`
- **Response body** — different content than the path-only version
- **Status code changes** — 200 vs 404/403

## Why it works

Some web frameworks/applications intentionally differentiate between:
- `GET /path` → direct browser access (restricted/limited)
- `GET http://host/path` → proxy-forwarded request (trusted/internal)

This is a **access control through request format** pattern, not a real security mechanism, but it shows up in CTF challenges.
