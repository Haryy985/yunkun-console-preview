import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || "playwright");
const root = path.resolve(process.env.PROXY_PREVIEW_STATIC || "out");
const output = path.resolve(process.env.PROXY_PREVIEW_OUTPUT || ".proxy-routing-smoke");
const base = "http://127.0.0.1:3027";
fs.mkdirSync(output, { recursive: true });
const browser = await chromium.launch({ headless: true, executablePath: process.env.PROXY_PREVIEW_BROWSER });
try {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  await context.addCookies([{ name: "yunkun_token", value: "fixture-token", url: base }]);
  await context.addInitScript(() => localStorage.setItem("yunkun_token", "fixture-token"));
  const errors = [];
  const writes = [];
  let strategy = "round_robin";
  let nodeID = 41;
  let paused = true;
  await context.route("**/*", async (route) => {
    const url = new URL(route.request().url());
    if (url.origin !== base) return route.abort();
    const target = path.resolve(root, `.${decodeURIComponent(url.pathname)}`);
    if (target !== root && !target.startsWith(root + path.sep)) return route.abort();
    const file = [target, `${target}.html`, path.join(target, "index.html")].find((candidate) =>
      fs.existsSync(candidate) && fs.statSync(candidate).isFile());
    if (!file) return route.fulfill({ status: 404, body: "fixture not found" });
    const mime = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".txt": "text/plain",
      ".png": "image/png", ".svg": "image/svg+xml", ".woff2": "font/woff2" }[path.extname(file)];
    return route.fulfill({ status: 200, contentType: mime || "application/octet-stream", body: fs.readFileSync(file) });
  });
  await context.route("**/api/**", async (route) => {
    const req = route.request();
    const endpoint = new URL(req.url()).pathname.slice(4);
    const json = (value) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(value) });
    if (req.method() !== "GET") {
      assert.equal(endpoint, "/tasks/1/network/routing");
      assert.equal(req.method(), "PUT");
      const payload = req.postDataJSON();
      assert.ok(paused, "attempted change while task running");
      strategy = payload.strategy;
      if (strategy === "fixed") nodeID = payload.node_id;
      writes.push(payload);
      return json({ ok: true });
    }
    if (endpoint === "/auth/me") return json({ role: "admin", username: "fixture" });
    if (endpoint === "/health") return json({ service: "yunkun", version: "isolated-ui-fixture", ready: true });
    if (endpoint === "/tasks/1") return json({ id: "1", name: "Routing fixture", title: "Routing fixture", status: "running",
      paused, created_at: new Date().toISOString(), in_flight: 0, goals_total: 0, goals_met: 0, llm_profile_ids: [] });
    if (endpoint === "/tasks/1/network") return json({ bound: true, enforced: false, pool_name: "Fixture pool",
      routing_strategy: strategy, binding: { task_id: 1, pool_id: 1, node_id: nodeID, mode: "configured", strategy },
      node: { id: nodeID, address: "http://192.0.2.10:8080", enabled: true }, events: [] });
    if (endpoint.includes("profiles") || endpoint.includes("intercept")) return json([]);
    return json({});
  });
  const page = await context.newPage();
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(`${base}/function/tasks/detail?id=1&tab=network`, { waitUntil: "domcontentloaded" });
  const select = page.getByLabel("代理调度策略", { exact: true });
  await select.waitFor();
  assert.equal(await select.inputValue(), "round_robin");
  await select.selectOption("fixed");
  await page.getByLabel("固定节点 ID", { exact: true }).fill("42");
  await page.getByRole("button", { name: "保存策略", exact: true }).click();
  await page.getByText("代理调度策略已保存", { exact: true }).waitFor();
  assert.deepEqual(writes[0], { strategy: "fixed", node_id: 42 });
  await page.screenshot({ path: path.join(output, "routing-fixed-desktop.png"), fullPage: true });
  await select.selectOption("round_robin");
  await page.getByRole("button", { name: "保存策略", exact: true }).click();
  await page.waitForFunction(() => !document.querySelector('input[aria-label="固定节点 ID"]'));
  assert.deepEqual(writes[1], { strategy: "round_robin" });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({ path: path.join(output, "routing-mobile.png"), fullPage: true });
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  paused = false;
  await page.reload({ waitUntil: "domcontentloaded" });
  await select.waitFor();
  assert.equal(await select.isDisabled(), true);
  assert.equal(await page.getByRole("button", { name: "保存策略", exact: true }).isDisabled(), true);
  assert.equal(writes.length, 2);
  assert.deepEqual(errors, []);
  console.log(JSON.stringify({ passed: true, writes, errors, output }));
} finally {
  await browser.close();
}
