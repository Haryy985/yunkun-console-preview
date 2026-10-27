import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || "playwright");
const base = "http://127.0.0.1:3019";
const staticRoot = path.resolve(process.env.PROXY_PREVIEW_STATIC);
const pools = [1, 2].map((id) => ({
  id,
  credentials_available: id === 2,
  has_credentials: false,
  quota: {},
  config: {
    name: id === 1 ? "Old pool" : "123sec",
    kind: "fixed",
    enabled: true,
    protocol: "http",
    addresses: [],
    ttl_seconds: 120,
    fetch_count: 1,
    safety_seconds: 15,
    quota_supported: false,
    low_quota: 100,
    expiry_warning_seconds: 3600,
  },
}));
const result = {
  id: 41,
  source_id: 10,
  address: "http://192.0.2.10:8080",
  protocol: "http",
  host: "192.0.2.10",
  port: 8080,
  metadata: {},
  state: "verified",
  authorized: true,
};
const requests = [];
const errors = [];
const browser = await chromium.launch({ headless: true, executablePath: process.env.PROXY_PREVIEW_BROWSER });
try {
  const context = await browser.newContext({ viewport: { width: 1440, height: 960 }, locale: "zh-CN" });
  await context.addCookies([{ name: "yunkun_token", value: "fixture-only", url: base }]);
  await context.addInitScript(() => localStorage.setItem("yunkun_token", "fixture-only"));
  await context.route("**/*", async (route) => {
    const url = new URL(route.request().url());
    if (url.origin !== base) return route.abort();
    const target = path.resolve(staticRoot, `.${decodeURIComponent(url.pathname)}`);
    if (target !== staticRoot && !target.startsWith(staticRoot + path.sep)) return route.abort();
    const file = [target, `${target}.html`, path.join(target, "index.html")]
      .find((name) => fs.existsSync(name) && fs.statSync(name).isFile());
    if (!file) return route.fulfill({ status: 404, body: "Missing fixture asset" });
    const contentType = {
      ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".json": "application/json",
      ".svg": "image/svg+xml", ".png": "image/png", ".woff": "font/woff", ".woff2": "font/woff2",
    }[path.extname(file)] || "application/octet-stream";
    return route.fulfill({ status: 200, contentType, body: fs.readFileSync(file) });
  });
  await context.route("**/api/**", async (route) => {
    const request = route.request();
    const endpoint = new URL(request.url()).pathname.slice(4);
    const json = (body) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body) });
    if (request.method() !== "GET") {
      requests.push({ endpoint, body: request.postDataJSON() });
      if (endpoint === "/proxy-discovery/results/41/approve") return json({ node_id: 99 });
      throw new Error(`Unexpected write: ${endpoint}`);
    }
    if (endpoint === "/auth/me") return json({ id: 1, username: "fixture", role: "admin" });
    if (endpoint === "/proxy-settings") return json({ enabled: true, default_pool_id: 1 });
    if (endpoint === "/proxy-pools") return json({ pools });
    if (endpoint.endsWith("/nodes/import-status")) return json({ pending: 0 });
    if (endpoint.endsWith("/nodes")) return json({
      nodes: [], total: 0,
      summary: { total: 0, available: 0, pending: 0, expiring: 0, error: 0, disabled: 0 },
    });
    if (endpoint.endsWith("/events")) return json({ events: [] });
    if (endpoint === "/proxy-discovery/sources") return json({ sources: [{
      id: 10, provider: "quake", name: "fixture", enabled: true, authorized_scope: "192.0.2.0/24",
      verify_url: "https://example.invalid/", query: "fixture", fetch_limit: 1, has_credentials: false,
    }] });
    if (endpoint.includes("/latest-run")) return json({ latest: null });
    if (endpoint === "/proxy-discovery/results") return json({
      results: [result], total: 1, page: 1, page_size: 20,
      summary: { total: 1, pending: 0, verified: 1, invalid: 0, approved: 0, ignored: 0 },
    });
    return json({});
  });
  const page = await context.newPage();
  page.on("pageerror", (error) => errors.push(error.message));
  page.on("dialog", (dialog) => {
    assert.match(dialog.message(), /123sec.*#2/);
    return dialog.accept();
  });
  await page.goto(`${base}/system/proxy-pools`, { waitUntil: "domcontentloaded", timeout: 120000 });
  await page.locator("#active-pool").selectOption("2");
  await page.getByRole("tab", { name: "FOFA / Quake 发现" }).click();
  const target = page.getByLabel("目标代理池");
  await target.waitFor();
  await page.waitForFunction(() => document.querySelector('select[aria-label="目标代理池"]')?.value === "2");
  assert.equal(await target.inputValue(), "2");
  assert.equal(await target.locator('option[value="1"]').isDisabled(), true);
  await page.getByRole("button", { name: "入池并启用", exact: true }).last().click();
  await page.waitForFunction(() => document.querySelector('select[aria-label="目标代理池"]')?.value === "2");
  assert.deepEqual(requests, [{ endpoint: "/proxy-discovery/results/41/approve", body: { pool_id: 2, target: "production" } }]);
  await page.getByRole("tab", { name: "代理 IP" }).click();
  await page.locator("#active-pool").selectOption("1");
  await page.getByRole("tab", { name: "FOFA / Quake 发现" }).click();
  await page.waitForFunction(() => document.querySelector('select[aria-label="目标代理池"]')?.value === "1");
  await page.getByRole("alert").getByText(/目标代理池「Old pool」\(#1\) 凭据不可解密/).waitFor();
  assert.equal(await page.getByRole("button", { name: "入池并启用", exact: true }).last().isDisabled(), true);
  assert.equal(requests.length, 1);
  assert.deepEqual(errors, []);
  console.log(JSON.stringify({ passed: true, request: requests[0], errors }));
} finally {
  await browser.close();
}
