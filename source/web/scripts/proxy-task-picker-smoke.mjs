import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || "playwright");
const root = path.resolve(process.env.PROXY_PREVIEW_STATIC || "out");
const output = path.resolve(process.env.PROXY_PREVIEW_OUTPUT || ".proxy-picker-smoke");
const base = "http://127.0.0.1:3028";
fs.mkdirSync(output, { recursive: true });
const browser = await chromium.launch({ headless: true, executablePath: process.env.PROXY_PREVIEW_BROWSER });
try {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  await context.addCookies([{ name: "yunkun_token", value: "fixture-token", url: base }]);
  await context.addInitScript(() => localStorage.setItem("yunkun_token", "fixture-token"));
  const errors = [];
  const writes = [];
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
      assert.equal(endpoint, "/tasks");
      const payload = req.postDataJSON();
      writes.push(payload);
      return route.fulfill({ status: 409, contentType: "application/json", body: JSON.stringify({ error: "fixture: no task started" }) });
    }
    if (endpoint === "/auth/me") return json({ role: "admin", username: "fixture" });
    if (endpoint === "/health") return json({ service: "yunkun", version: "fixture", ready: true });
    if (endpoint === "/tasks") return json({ tasks: [], active: "" });
    if (endpoint === "/proxy-pools") return json({ pools: [1, 2].map((id) => ({
      id, credentials_available: true, config: { enabled: true, kind: "fixed", name: `Fixture ${id}` },
    })) });
    if (endpoint === "/proxy-pools/settings") return json({ enabled: false });
    if (/^\/proxy-pools\/[12]\/nodes$/.test(endpoint)) {
      const id = Number(endpoint.split("/")[2]);
      return json({ nodes: [{ id: id * 10, enabled: true, address: `http://192.0.2.${id}:8080` }] });
    }
    if (endpoint === "/companies") return json([]);
    return json({});
  });
  const page = await context.newPage();
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(`${base}/function/tasks/`, { waitUntil: "domcontentloaded" });
  await page.getByRole("button", { name: "新建任务", exact: true }).click();
  await page.getByLabel("描述", { exact: true }).fill("synthetic fixture only");
  await page.getByLabel("目标", { exact: true }).fill("no network activity");
  const pool = page.locator("#task-proxy-pool");
  const mode = page.getByLabel("节点分配", { exact: true });
  const node = page.getByLabel("固定代理节点", { exact: true });
  const create = page.getByRole("button", { name: "创建", exact: true });
  await pool.selectOption("1");
  await mode.selectOption("manual");
  await page.getByText("请选择当前池的可用固定节点", { exact: true }).waitFor();
  assert.equal(await create.isDisabled(), true, "manual mode without node must block creation");
  await node.selectOption("10");
  await page.waitForFunction(() => ![...document.querySelectorAll("button")].find((b) => b.textContent.trim() === "创建")?.disabled);
  assert.equal(await create.isDisabled(), false);
  await pool.selectOption("2");
  assert.equal(await node.inputValue(), "", "old pool node must be cleared");
  assert.equal(await create.isDisabled(), true);
  await node.selectOption("20");
  await create.click();
  await page.waitForTimeout(100);
  assert.equal(writes.length, 1);
  assert.equal(writes[0].proxy_pool_id, 2);
  assert.equal(writes[0].proxy_node_id, 20);
  await mode.selectOption("auto");
  await create.click();
  await page.waitForTimeout(100);
  assert.equal(writes.length, 2);
  assert.equal(writes[1].proxy_pool_id, 2);
  assert.equal(writes[1].proxy_node_id, undefined);
  await page.setViewportSize({ width: 390, height: 844 });
  await mode.selectOption("manual");
  await node.scrollIntoViewIfNeeded();
  await page.screenshot({ path: path.join(output, "task-picker-mobile.png"), fullPage: true });
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  assert.deepEqual(errors, []);
  console.log(JSON.stringify({ passed: true, writes, errors, output }));
} finally {
  await browser.close();
}
