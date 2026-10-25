import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || "playwright");

const base = process.env.PROXY_PREVIEW_URL || "http://127.0.0.1:3017";
const output = process.env.PROXY_PREVIEW_OUTPUT || path.join(os.tmpdir(), "yunkun-proxy-pools-smoke");
const nodes = Array.from({ length: 551 }, (_, index) => ({
  id: index + 1, pool_id: 1, address: `socks5://192.0.2.${index % 250 + 1}:${20001 + index}`,
  enabled: index % 7 !== 0, origin: index % 2 ? "quake" : "fofa",
  success_count: 0, failure_count: 0,
}));
const pools = [{ id: 1, config: {
  name: "代理池验收样例", kind: "fixed", enabled: true, protocol: "socks5", addresses: [],
  ttl_seconds: 120, fetch_count: 1, safety_seconds: 15, quota_supported: false,
  low_quota: 100, verify_url: "", expiry_warning_seconds: 3600,
}, has_credentials: false, quota: {} }];
const requests = [];
const errors = [];
let active = 0;
let maxActive = 0;
let delay = 150;
let importPending = 0;
let imported = false;

async function main() {
  fs.mkdirSync(output, { recursive: true });
  const browser = await chromium.launch({ headless: true, executablePath: process.env.PROXY_PREVIEW_BROWSER });
  try {
    const context = await browser.newContext({ viewport: { width: 1600, height: 1000 }, locale: "zh-CN" });
    await context.addCookies([{ name: "yunkun_token", value: "isolated-ui-fixture", url: base }]);
    await context.addInitScript(() => {
      localStorage.setItem("yunkun_token", "isolated-ui-fixture");
    });
    // All API calls remain inside this browser fixture; no user database or
    // discovered third-party proxies participate in UI acceptance checks.
    await context.route("**/api/**", async (route) => {
      const request = route.request();
      const url = new URL(request.url());
      const endpoint = url.pathname.slice(4);
      const json = (data, status = 200) => route.fulfill({ status, contentType: "application/json", body: JSON.stringify(data) });
      if (endpoint === "/auth/me") return json({ id: 1, username: "preview", role: "admin" });
      if (endpoint === "/health") return json({ ok: true, version: "proxy-preview" });
      if (endpoint === "/proxy-settings") return json({ enabled: true, default_pool_id: 1 });
      if (endpoint === "/proxy-pools") return json({ pools, enforced_available: false });
      if (endpoint === "/proxy-pools/1/nodes/import-status") return json({ pending: importPending });
      if (endpoint === "/proxy-pools/1/nodes/import" && request.method() === "POST") {
        assert.ok(request.headers()["content-type"].startsWith("multipart/form-data;"));
        assert.ok(request.postData().includes("http://192.0.2.200:18080"));
        importPending = 1;
        imported = true;
        nodes.push({ id: 552, pool_id: 1, address: "http://192.0.2.200:18080", enabled: false, origin: "manual", success_count: 0, failure_count: 0 });
        setTimeout(() => {
          Object.assign(nodes[551], { enabled: true, last_success_at: new Date().toISOString(), success_count: 1,
            last_verify_mode: "connectivity", last_http_status: 200 });
          importPending = 0;
        }, 2500);
        return json({ node_ids: [552], imported: 1, skipped: 1 });
      }
      if (endpoint === "/proxy-pools/1/nodes/export") {
        return route.fulfill({ status: 200, contentType: "text/csv; charset=utf-8",
          headers: { "Content-Disposition": "attachment; filename=proxy-pool-1.csv" },
          body: "address,enabled\n" + nodes.map((n) => `${n.address},${n.enabled}`).join("\n") });
      }
      if (endpoint === "/proxy-discovery/results") return json({ results: [], total: 0, summary: { total: 0, unapproved: 0, approved: 0 } });
      if (endpoint.endsWith("/events")) return json({ events: [] });
      if (endpoint === "/proxy-pools/1/nodes") {
        const page = Number(url.searchParams.get("page") || 1);
        const size = Number(url.searchParams.get("page_size") || 20);
        const q = (url.searchParams.get("q") || "").toLowerCase();
        const status = url.searchParams.get("status") || "";
        let filtered = [...nodes].reverse().filter((node) => !q || node.address.includes(q));
        if (status) filtered = filtered.filter((node) => ({
          disabled: !node.enabled, available: node.enabled && node.last_success_at && !node.last_error,
          pending: node.enabled && !node.last_success_at && !node.last_error, error: node.enabled && node.last_error,
        })[status]);
        if (url.searchParams.get("protocol") === "http") filtered = [];
        if (url.searchParams.get("sort") === "latency") filtered.sort((a, b) => (a.latency_ms ?? Infinity) - (b.latency_ms ?? Infinity));
        return json({ nodes: filtered.slice((page - 1) * size, page * size), total: filtered.length, summary: {
          total: nodes.length, available: nodes.filter((n) => n.enabled && n.last_success_at && !n.last_error).length,
          pending: nodes.filter((n) => n.enabled && !n.last_success_at && !n.last_error).length,
          disabled: nodes.filter((n) => !n.enabled).length, expiring: 0, error: nodes.filter((n) => n.enabled && n.last_error).length,
        } });
      }
      const verify = endpoint.match(/^\/proxy-pools\/1\/nodes\/(\d+)\/verify$/);
      if (verify && request.method() === "POST") {
        const node = nodes.find((n) => n.id === Number(verify[1]));
        const body = request.postDataJSON();
        requests.push({ id: node.id, mode: body.mode });
        active++;
        maxActive = Math.max(maxActive, active);
        await new Promise((resolve) => setTimeout(resolve, delay));
        active--;
        const failed = node.id % 5 === 0;
        Object.assign(node, { last_checked_at: new Date().toISOString(), latency_ms: 120 + node.id % 40,
          last_verify_mode: body.mode, last_verify_target: body.mode === "connectivity" ? "https://www.baidu.com" : "https://api.ipify.org",
          last_http_status: failed ? 503 : 200,
          last_error: failed ? "[proxy_verify:response:http_status] endpoint returned HTTP 503" : "",
        });
        if (failed) node.failure_count++;
        else {
          node.success_count++;
          node.last_success_at = node.last_checked_at;
          if (body.mode === "exit_ip") { node.verified_ip = "198.51.100.42"; node.verified_at = node.last_checked_at; }
        }
        return json(failed ? { error: node.last_error } : { mode: body.mode, target: node.last_verify_target,
          ip: body.mode === "exit_ip" ? node.verified_ip : "", http_status: 200, latency_ms: node.latency_ms, checked_at: node.last_checked_at }, failed ? 502 : 200);
      }
      if (request.method() !== "GET") throw new Error(`Unexpected fixture write: ${endpoint}`);
      return json({});
    });
    const page = await context.newPage();
    page.on("pageerror", (error) => errors.push(error.message));
    await page.goto(`${base}/system/proxy-pools`, { waitUntil: "networkidle", timeout: 120000 });
    const table = page.getByRole("table", { name: "代理节点列表" });
    await table.waitFor();
    assert.equal(await page.getByLabel("验证目标", { exact: true }).inputValue(), "connectivity");
    assert.equal(await table.locator("tbody tr").count(), 20);
    await page.getByRole("button", { name: "28", exact: true }).click();
    await page.getByRole("button", { name: "验证节点 1", exact: true }).waitFor();
    assert.equal(await table.locator("tbody tr").count(), 11);
    await page.getByRole("button", { name: "1", exact: true }).click();
    await page.getByRole("button", { name: "验证节点 551", exact: true }).waitFor();

    const enablement = nodes.map((node) => node.enabled);
    await page.getByRole("button", { name: "验证本页", exact: true }).click();
    await page.getByText("验证完成 20 / 20", { exact: true }).waitFor();
    assert.equal(requests.length, 20);
    assert.ok(requests.every((request) => request.mode === "connectivity"));
    assert.ok(maxActive <= 4, `batch concurrency ${maxActive} exceeds four`);
    assert.deepEqual(nodes.map((node) => node.enabled), enablement);
    assert.equal(nodes[550].verified_ip, undefined);
    assert.ok((await table.textContent()).includes("百度连通性 · HTTP 200"));
    assert.ok((await table.textContent()).includes("HTTP 503"));
    assert.ok((await table.textContent()).includes("未检测出口 IP"));

    await page.getByLabel("验证目标", { exact: true }).selectOption("exit_ip");
    await page.getByRole("button", { name: "验证节点 551", exact: true }).click();
    await page.getByText("验证完成 1 / 1", { exact: true }).waitFor();
    assert.equal(nodes[550].verified_ip, "198.51.100.42");
    const verifiedAt = nodes[550].verified_at;
    await page.getByLabel("验证目标", { exact: true }).selectOption("connectivity");
    await page.getByRole("button", { name: "验证节点 551", exact: true }).click();
    await page.getByText("验证完成 1 / 1", { exact: true }).waitFor();
    assert.equal(nodes[550].verified_at, verifiedAt);
    assert.equal(nodes[550].verified_ip, "198.51.100.42");

    await page.getByRole("checkbox", { name: "选择节点 551", exact: true }).check();
    await page.getByRole("checkbox", { name: "选择节点 550", exact: true }).check();
    await page.getByRole("button", { name: "验证所选（2）", exact: true }).click();
    await page.getByText("验证完成 2 / 2", { exact: true }).waitFor();
    await page.getByRole("button", { name: "清除选择", exact: true }).click();

    await page.getByLabel("搜索代理节点", { exact: true }).fill(":20551");
    await page.waitForFunction(() => document.querySelectorAll('table[aria-label="代理节点列表"] tbody tr').length === 1);
    await page.getByLabel("搜索代理节点", { exact: true }).fill("");
    await page.waitForFunction(() => document.querySelectorAll('table[aria-label="代理节点列表"] tbody tr').length === 20);
    await page.getByLabel("筛选协议", { exact: true }).selectOption("http");
    await page.getByText("没有匹配的节点", { exact: true }).waitFor();
    await page.getByLabel("筛选协议", { exact: true }).selectOption("");
    await page.getByRole("button", { name: "验证节点 551", exact: true }).waitFor();
    delay = 1000;
    const beforeCancel = requests.length;
    await page.getByRole("button", { name: "验证本页", exact: true }).click();
    await page.getByRole("button", { name: "停止验证", exact: true }).click();
    await page.getByText(/已停止.*\/ 20/).waitFor();
    await page.waitForTimeout(1200);
    assert.ok(requests.length - beforeCancel <= 4, "cancel continued starting queued requests");
    await page.getByRole("button", { name: "刷新代理来源", exact: true }).click();
    await page.getByRole("button", { name: "验证节点 551", exact: true }).waitFor();
    delay = 150;
    await page.getByRole("button", { name: "验证本页", exact: true }).click();
    await page.getByText("验证完成 20 / 20", { exact: true }).waitFor();
    const beforeImport = requests.length;
    await page.locator('input[type="file"]').setInputFiles({
      name: "import.txt", mimeType: "text/plain",
      buffer: Buffer.from("http://192.0.2.200:18080\nhttp://192.0.2.200:18080\n"),
    });
    await page.getByText(/新增 1 个节点，跳过 1 个重复节点/).waitFor();
    assert.ok(imported);
    await page.reload({ waitUntil: "networkidle" });
    await page.getByRole("button", { name: "验证节点 552", exact: true }).waitFor();
    await page.waitForFunction(() => !document.body.textContent.includes("导入验证中：剩余"));
    assert.equal(nodes[551].enabled, true);
    assert.equal(requests.length, beforeImport, "import verification must not depend on browser requests");
    const [download] = await Promise.all([
      page.waitForEvent("download"),
      page.getByRole("button", { name: "导出节点", exact: true }).click(),
    ]);
    assert.equal(download.suggestedFilename(), "proxy-pool-1.csv");
    const exported = path.join(output, "proxy-pool-fixture.csv");
    await download.saveAs(exported);
    assert.ok(fs.readFileSync(exported, "utf8").includes("http://192.0.2.200:18080"));
    await page.addStyleTag({ content: "nextjs-portal { display: none; }" });

    for (const [name, viewport] of [["desktop", { width: 1600, height: 1000 }], ["laptop", { width: 1366, height: 900 }], ["mobile", { width: 390, height: 844 }]]) {
      await page.setViewportSize(viewport);
      await page.waitForTimeout(200);
      const overflow = await page.evaluate(() => ({ width: innerWidth, body: document.body.scrollWidth, html: document.documentElement.scrollWidth }));
      assert.ok(overflow.body <= overflow.width + 1 && overflow.html <= overflow.width + 1, `${name}: ${JSON.stringify(overflow)}`);
      await page.screenshot({ path: path.join(output, `proxy-pools-${name}.png`), fullPage: true });
      await page.screenshot({ path: path.join(output, `proxy-pools-${name}-viewport.png`) });
    }
    assert.deepEqual(errors, []);
    const result = { passed: true, checks: ["default Baidu target", "551-node backend pagination", "four-worker batch", "single verification", "selected verification", "cancellation", "search and protocol filter", "IP evidence retained", "enablement retained", "multipart import", "background validation across refresh", "CSV export", "desktop/laptop/mobile layout"], requests: requests.length, maxActive, errors };
    fs.writeFileSync(path.join(output, "ui-results.json"), JSON.stringify(result, null, 2));
    console.log(JSON.stringify(result));
  } finally { await browser.close(); }
}
main().catch((error) => { console.error(error); process.exitCode = 1; });
