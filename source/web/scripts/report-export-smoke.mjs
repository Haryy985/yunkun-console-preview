import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || "playwright");
const root = path.resolve(process.env.PROXY_PREVIEW_STATIC || "out");
const output = path.resolve(process.env.PROXY_PREVIEW_OUTPUT || ".report-export-smoke");
const base = "http://127.0.0.1:3029";
fs.mkdirSync(output, { recursive: true });
const browser = await chromium.launch({ headless: true, executablePath: process.env.PROXY_PREVIEW_BROWSER });
try {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, acceptDownloads: true });
  await context.addCookies([{ name: "yunkun_token", value: "fixture-token", url: base }]);
  await context.addInitScript(() => localStorage.setItem("yunkun_token", "fixture-token"));
  const errors = [];
  let reportCalls = 0;
  let exportCalls = 0;
  let downloads = 0;
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
    assert.equal(req.method(), "GET", "report fixtures must not mutate data");
    const endpoint = new URL(req.url()).pathname.slice(4);
    const json = (value, status = 200, headers = {}) => route.fulfill({
      status, contentType: "application/json", headers, body: JSON.stringify(value),
    });
    if (endpoint === "/auth/me") return json({ role: "admin", username: "fixture" });
    if (endpoint === "/health") return json({ service: "yunkun", version: "fixture", ready: true });
    if (endpoint === "/report") {
      reportCalls++;
      if (reportCalls === 1) return json({ error: "fixture gateway limit" }, 429, { "Retry-After": "1" });
      if (reportCalls === 2) return json({ error: "quality fixture", blockers: ["#70:NEEDS_MORE"] }, 409);
      return route.fulfill({ contentType: "text/markdown", body: "# Verified task fixture" });
    }
    if (endpoint === "/exploration/findings/export") {
      exportCalls++;
      // Give a duplicate click time to run before fulfilling the first request.
      await new Promise((resolve) => setTimeout(resolve, 250));
      if (exportCalls === 1) return route.fulfill({
        status: 429, contentType: "text/html", headers: { "Retry-After": "1" }, body: "<html>gateway rate limit</html>",
      });
      if (exportCalls === 2) return json({
        error: "quality fixture", blockers: ["#8:REPORT_MISSING", "#9:REPORT_EVIDENCE_STALE"],
      }, 409);
      return route.fulfill({ contentType: "text/markdown", headers: {
        "Content-Disposition": 'attachment; filename="verified-fixture.md"',
      }, body: "# Verified export fixture\n" });
    }
    if (endpoint === "/tasks/1") return json({
      id: "1", name: "Report fixture", status: "done", paused: true, llm_profile_ids: [],
    });
    if (endpoint === "/tasks") return json({ tasks: [], active: "" });
    if (endpoint === "/exploration/findings/stats") return json({
      total: 0, pending: 0, critical: 0, high: 0, medium: 0, low: 0, vulnclasses: [], tasks: [],
    });
    if (endpoint === "/exploration/findings") return json({ items: [], total: 0, page: 1, page_size: 20 });
    if (endpoint.endsWith("/asset-tree")) return json({ nodes: [], total: 0 });
    if (endpoint.includes("profiles") || endpoint.includes("intercept")) return json([]);
    return json({});
  });
  const page = await context.newPage();
  page.on("pageerror", (error) => errors.push(error.message));
  page.on("download", () => downloads++);
  await page.goto(`${base}/function/tasks/detail?id=1&tab=report`, { waitUntil: "domcontentloaded" });
  const reportError = page.getByRole("alert").filter({ hasText: "HTTP 429" });
  await reportError.waitFor();
  assert.match(await reportError.innerText(), /fixture gateway limit/);
  assert.equal(await page.getByRole("button", { name: "复制", exact: true }).count(), 0);
  await page.waitForTimeout(1300);
  assert.equal(reportCalls, 1, "must not automatically retry limited requests");
  await page.getByRole("button", { name: "重试", exact: true }).click();
  await page.getByRole("alert").filter({ hasText: "#70:NEEDS_MORE" }).waitFor();
  assert.match(await page.getByRole("alert").filter({ hasText: "#70:NEEDS_MORE" }).innerText(), /HTTP 409/);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({ path: path.join(output, "task-report-409-mobile.png"), fullPage: true });
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  await page.getByRole("button", { name: "重试", exact: true }).click();
  await page.getByText("Verified task fixture", { exact: true }).waitFor();
  assert.equal(reportCalls, 3);

  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto(`${base}/function/findings/`, { waitUntil: "domcontentloaded" });
  await page.waitForTimeout(500);
  if (errors.length) throw new Error(`Fixture page errors: ${JSON.stringify(errors)}`);
  await page.screenshot({ path: path.join(output, "findings-before-export.png"), fullPage: true });
  await page.getByRole("button", { name: "导出", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "导出发现" });
  const exportButton = dialog.getByRole("button", { name: "导出", exact: true });
  await exportButton.evaluate((button) => { button.click(); button.click(); });
  await dialog.getByRole("alert").filter({ hasText: "HTTP 429" }).waitFor();
  assert.equal(exportCalls, 1, "double click sent more than one export");
  assert.equal(downloads, 0, "429 was downloaded as a report");
  await page.waitForTimeout(1300);
  assert.equal(exportCalls, 1, "export must not auto-retry");
  await exportButton.click();
  await dialog.getByRole("alert").filter({ hasText: "#8:REPORT_MISSING" }).waitFor();
  assert.match(await dialog.getByRole("alert").innerText(), /HTTP 409/);
  assert.match(await dialog.getByRole("alert").innerText(), /#9:REPORT_EVIDENCE_STALE/);
  assert.equal(downloads, 0, "quality failure was downloaded as a report");
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({ path: path.join(output, "finding-export-409-mobile.png"), fullPage: true });
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  const pending = page.waitForEvent("download");
  await exportButton.click();
  const download = await pending;
  assert.equal(download.suggestedFilename(), "verified-fixture.md");
  assert.equal(fs.readFileSync(await download.path(), "utf8"), "# Verified export fixture\n");
  assert.equal(exportCalls, 3);
  assert.equal(downloads, 1);
  assert.deepEqual(errors, []);
  console.log(JSON.stringify({ passed: true, reportCalls, exportCalls, downloads, errors, output }));
} catch (error) {
  const page = browser.contexts()[0]?.pages()[0];
  if (page) console.error((await page.locator("body").innerText()).slice(0, 4000));
  throw error;
} finally {
  await browser.close();
}
