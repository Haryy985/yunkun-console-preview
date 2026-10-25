import assert from "node:assert/strict";
import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";

const require = createRequire(import.meta.url);
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || "playwright");
const root = fileURLToPath(new URL("../out/", import.meta.url));
const output = process.env.YUNKUN_UPDATE_UI_OUTPUT;
if (!output) throw new Error("Specify an isolated screenshot output directory.");
fs.mkdirSync(output, { recursive: true });
const server = http.createServer((req, res) => {
  let file = path.join(root, decodeURIComponent(new URL(req.url, "http://localhost").pathname));
  if (!file.startsWith(root)) { res.writeHead(403).end(); return; }
  if (fs.existsSync(file) && fs.statSync(file).isDirectory()) file = path.join(file, "index.html");
  if (!fs.existsSync(file)) { res.writeHead(404).end(); return; }
  const ext = path.extname(file);
  const types = { ".js": "application/javascript", ".css": "text/css", ".html": "text/html", ".png": "image/png" };
  res.setHeader("Content-Type", types[ext] || "application/octet-stream");
  fs.createReadStream(file).pipe(res);
});
await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
const base = `http://127.0.0.1:${server.address().port}`;
const browser = await chromium.launch({ headless: true, executablePath: process.env.YUNKUN_UPDATE_TEST_BROWSER });
try {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  await context.addInitScript(() => {
    localStorage.setItem("yunkun_token", "fixture-token");
    localStorage.setItem("yunkun-update-dismissed", "v1.1.1-offline");
  });
  let supported = true;
  let state = { phase: "idle" };
  let applyCount = 0;
  let offline = false;
  await context.route("**/api/**", async (route) => {
    const p = new URL(route.request().url()).pathname;
    const json = (value, status = 200) => route.fulfill({ json: value, status });
    if (p === "/api/update/check") return json({
      current: "v1.1.0-offline", latest: "v1.1.1-offline", has_update: true, comparable: true,
      asset_available: true, asset: "yunkun-1.1.1-offline-windows-amd64.zip", os: "windows", arch: "amd64",
      auto_update_supported: supported, auto_update_reason: supported ? "" : "外部数据库请使用手动升级",
      notes: "隔离测试发布说明", size: 108000000,
    });
    if (p === "/api/update/status") {
      if (offline) return route.abort("connectionrefused");
      return json(state);
    }
    if (p === "/api/update/apply") {
      assert.deepEqual(route.request().postDataJSON(), { version: "v1.1.1-offline" });
      applyCount++;
      state = { phase: "downloading", percent: 25, version: "v1.1.1-offline" };
      return json({ ok: true, target: "v1.1.1-offline" }, 202);
    }
    if (p === "/api/auth/me") return json({ id: 1, username: "yunkun", role: "admin" });
    if (p === "/api/auth/status") return json({ initialized: true });
    if (p === "/api/health") return json({ ok: true, ready: true, service: "yunkun", version: "v1.1.0-offline" });
    if (p === "/api/settings") return json({ workers: 3 });
    return json([]);
  });
  const page = await context.newPage();
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto(`${base}/system/settings/`);
  await page.getByRole("button", { name: "一键更新", exact: true }).click();
  await page.getByRole("button", { name: "取消", exact: true }).click();
  assert.equal(applyCount, 0, "cancel must not submit");
  await page.getByRole("button", { name: "一键更新", exact: true }).click();
  await page.getByRole("button", { name: "备份并更新", exact: true }).click();
  await page.getByText("下载更新包", { exact: true }).waitFor();
  assert.equal(applyCount, 1);
  assert.equal(context.pages().length, 1, "one-click update must not open GitHub");
  await page.screenshot({ path: path.join(output, "desktop.png"), fullPage: true });
  offline = true;
  await page.getByText("服务重启中，正在重新连接…", { exact: true }).waitFor();
  offline = false;
  state = { phase: "completed", backup: "C:\\Yunkun\\.yunkun-local\\updates\\fixture\\backup" };
  await page.getByText("更新完成", { exact: true }).waitFor();
  await page.reload();
  await page.getByText("更新完成", { exact: true }).waitFor();
  state = { phase: "rolled_back", error: "隔离测试：新版启动失败，已恢复数据" };
  await page.getByText("更新失败，已恢复旧版和数据", { exact: true }).waitFor();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({ path: path.join(output, "mobile.png"), fullPage: true });
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  supported = false;
  await page.getByRole("button", { name: "检查更新", exact: true }).click();
  await page.getByText("外部数据库请使用手动升级", { exact: true }).waitFor();
  assert.equal(await page.getByRole("button", { name: "一键更新", exact: true }).isDisabled(), true);
  assert.deepEqual(errors, []);
  fs.writeFileSync(path.join(output, "result.json"), JSON.stringify({
    confirmation: true, cancellation: true, targetVersion: true, noGitHubRedirect: true,
    progress: true, reconnect: true, persistedResult: true, rollback: true, unsupportedDisabled: true,
    mobileNoOverflow: true, pageErrors: errors,
  }, null, 2));
  console.log("PASS: one-click update UI, progress, reconnect, rollback, desktop/mobile.");
} finally {
  await browser.close();
  await new Promise((resolve) => server.close(resolve));
}
