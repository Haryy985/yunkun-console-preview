import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import os from "node:os";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || "playwright");
const base = process.env.PROXY_PREVIEW_URL || "http://127.0.0.1:3019";
const staticRoot = process.env.PROXY_PREVIEW_STATIC ? path.resolve(process.env.PROXY_PREVIEW_STATIC) : null;
const output = process.env.PROXY_PREVIEW_OUTPUT || path.join(os.tmpdir(), "yunkun-credentials-smoke");
const pool = {
  id: 1,
  credentials_available: false,
  has_credentials: false,
  quota: {},
  config: {
    name: "Credential recovery fixture",
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
};
let failing = false;
let polls = 0;
const writes = [];
const errors = [];
fs.mkdirSync(output, { recursive: true });
const browser = await chromium.launch({ headless: true, executablePath: process.env.PROXY_PREVIEW_BROWSER });
try {
  const context = await browser.newContext({ viewport: { width: 1600, height: 1000 }, locale: "zh-CN" });
  await context.addCookies([{ name: "yunkun_token", value: "isolated-test-token", url: base }]);
  await context.addInitScript(() => localStorage.setItem("yunkun_token", "isolated-test-token"));
  await context.route("**/*", async (route) => {
    const url = new URL(route.request().url());
    if (url.origin !== new URL(base).origin) return route.abort();
    if (!staticRoot) return route.continue();
    // Serve only the generated export in-memory; no service or production API.
    const target = path.resolve(staticRoot, `.${decodeURIComponent(url.pathname)}`);
    if (target !== staticRoot && !target.startsWith(staticRoot + path.sep)) return route.abort();
    const candidates = [target, `${target}.html`, path.join(target, "index.html")];
    const file = candidates.find((name) => fs.existsSync(name) && fs.statSync(name).isFile());
    if (!file) return route.fulfill({ status: 404, body: "Fixture asset not found" });
    const contentType = {
      ".html": "text/html", ".js": "text/javascript", ".css": "text/css",
      ".json": "application/json", ".txt": "text/plain", ".png": "image/png",
      ".svg": "image/svg+xml", ".ico": "image/x-icon", ".woff2": "font/woff2", ".woff": "font/woff",
    }[path.extname(file)] || "application/octet-stream";
    return route.fulfill({ status: 200, contentType, body: fs.readFileSync(file) });
  });
  await context.route("**/api/**", async (route) => {
    const request = route.request();
    const endpoint = new URL(request.url()).pathname.slice(4);
    const json = (value, status = 200) =>
      route.fulfill({
        status,
        contentType: "application/json",
        body: JSON.stringify(value),
      });
    if (request.method() !== "GET") {
      writes.push(endpoint);
      assert.equal(endpoint, "/proxy-pools/1/credentials/recover", "unexpected outbound or configuration operation");
      assert.equal(request.method(), "POST");
      assert.equal(request.postDataJSON().confirm, true);
      assert.equal(request.postDataJSON().no_auth, false);
      assert.deepEqual(request.postDataJSON().secrets, {
        proxy_username: "fixture-user",
        proxy_password: "fixture-password",
      });
      pool.credentials_available = true;
      pool.has_credentials = true;
      return json({ ok: true, requires_reverification: true });
    }
    if (endpoint === "/auth/me") return json({ id: 1, username: "fixture", role: "admin" });
    if (endpoint === "/health") return json({ ok: true, version: "credential-fixture" });
    if (endpoint === "/proxy-settings") return json({ enabled: true, default_pool_id: 1 });
    if (endpoint === "/proxy-pools") return json({ pools: [pool] });
    if (endpoint === "/proxy-pools/1/nodes/import-status") {
      polls++;
      return failing ? json({ error: "fixture status unavailable" }, 503) : json({ pending: 0 });
    }
    if (endpoint === "/proxy-pools/1/nodes") {
      if (failing) return json({ error: "fixture inventory unavailable" }, 503);
      return json({
        nodes: [
          {
            id: 41,
            pool_id: 1,
            address: "http://192.0.2.10:8080",
            enabled: false,
            origin: "quake",
            success_count: 0,
            failure_count: 0,
          },
        ],
        total: 1,
        summary: { total: 1, available: 0, pending: 0, expiring: 0, error: 0, disabled: 1 },
      });
    }
    if (endpoint === "/proxy-pools/1/events") return json({ events: [] });
    if (endpoint === "/proxy-discovery/results")
      return json({
        results: [],
        total: 0,
        summary: { total: 0, unapproved: 0, approved: 0 },
      });
    return json({});
  });
  const page = await context.newPage();
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(`${base}/system/proxy-pools`, { waitUntil: "domcontentloaded", timeout: 120000 });
  await page.getByText("当前代理池凭据不可解密。", { exact: false }).waitFor();
  const table = page.getByRole("table", { name: "代理节点列表" });
  await table.getByText("192.0.2.10", { exact: true }).waitFor();
  assert.equal(await table.locator("tbody tr").count(), 1);
  assert.equal(await page.getByRole("button", { name: "验证本页", exact: true }).isDisabled(), true);
  assert.equal(await page.getByRole("button", { name: "导入并验证", exact: true }).isDisabled(), true);
  assert.equal(await page.getByRole("button", { name: "导出节点", exact: true }).isEnabled(), true);
  await page.screenshot({ path: path.join(output, "credential-error-desktop.png"), fullPage: true });
  await page.getByRole("button", { name: "编辑代理池凭据", exact: true }).click();
  await page.getByRole("button", { name: "确认恢复凭据", exact: true }).click();
  await page.getByText("原代理池凭据不可读，请先恢复原密钥，或勾选替换整组凭据", { exact: true }).waitFor();
  assert.equal(writes.length, 0, "implicit credential replacement was submitted");
  await page.getByLabel("替换整组凭据（空字段会清除）", { exact: true }).check();
  await page.locator("#secret-proxy_username").fill("fixture-user");
  await page.locator("#secret-proxy_password").fill("fixture-password");
  await page.getByRole("button", { name: "确认恢复凭据", exact: true }).click();
  await page.getByRole("button", { name: "编辑代理池凭据", exact: true }).waitFor({ state: "hidden" });
  assert.equal(writes.length, 1);
  await table.getByText("192.0.2.10", { exact: true }).waitFor();
  failing = true;
  polls = 0;
  await page.reload({ waitUntil: "domcontentloaded" });
  await page.getByText("节点读取失败，数量未知", { exact: true }).waitFor();
  assert.equal(await page.getByText("没有匹配的节点", { exact: true }).count(), 0);
  assert.equal(await page.getByText("共 0 条", { exact: false }).count(), 0);
  await page.waitForTimeout(9000);
  assert.ok(polls <= 3, `failed status polled too often: ${polls}`);
  await page.screenshot({ path: path.join(output, "read-error-desktop.png"), fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({ path: path.join(output, "read-error-mobile.png"), fullPage: true });
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  assert.deepEqual(errors, []);
  console.log(JSON.stringify({ passed: true, writes: writes.length, failedStatusPolls: polls, errors, output }));
} finally {
  await browser.close();
}
