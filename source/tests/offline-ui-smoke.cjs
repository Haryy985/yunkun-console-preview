const { chromium } = require(process.env.PLAYWRIGHT_MODULE || "playwright");
const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");

(async () => {
  const base = process.env.YUNKUN_SMOKE_URL;
  const output = process.env.YUNKUN_SMOKE_OUTPUT;
  if (!/^http:\/\/127\.0\.0\.1:53513$/.test(base || "") || !output) {
    throw new Error("Only the isolated local installation is allowed.");
  }
  fs.mkdirSync(output, { recursive: true });
  const browser = await chromium.launch({
    headless: true,
    executablePath: process.env.YUNKUN_SMOKE_CHROMIUM || undefined,
  });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  const page = await context.newPage();
  let simulatedUpdate = false;
  await page.route("**/api/update/check**", (route) => route.fulfill({
    json: {
      current: "v1.0.3-offline", latest: simulatedUpdate ? "v1.0.4-offline" : "v1.0.3-offline",
      comparable: true, has_update: simulatedUpdate, os: "windows", arch: "amd64",
      mode: "native", notes: simulatedUpdate ? "隔离测试发布说明：更新提醒显示测试。" : "",
      asset: "yunkun-v1.0.4-offline-windows-x64.zip", asset_available: true,
      html_url: "https://github.com/Haryy985/yunkun-console-preview/releases",
    },
  }));
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  const results = [];
  try {
    const status = await context.request.get(`${base}/api/auth/status`);
    if ((await status.json()).initialized !== false) throw new Error("Smoke installation is not fresh.");
    await page.goto(`${base}/setup/`);
    await page.locator("input[type=password]").first().waitFor();
    await page.screenshot({ path: path.join(output, "setup.png"), fullPage: true });

    const init = await context.request.post(`${base}/api/auth/init`, {
      data: { password: crypto.randomBytes(24).toString("hex") },
    });
    if (!init.ok()) throw new Error(`Initialization failed: ${init.status()}`);
    const { token } = await init.json();
    const headers = { Authorization: `Bearer ${token}` };
    const get = async (url) => {
      const response = await context.request.get(base + url, { headers });
      if (!response.ok()) throw new Error(`API failed: ${url} (${response.status()})`);
      return response.json();
    };
    const health = await get("/api/health");
    if (health.version !== "v1.0.3-offline" || !health.ready) throw new Error("Wrong release binary.");
    const tasks = await get("/api/tasks");
    if (Array.isArray(tasks) ? tasks.length : tasks.tasks?.length) throw new Error("Fresh package contains tasks.");
    const knowledge = await get("/api/rag/knowledge-bases");
    if (Array.isArray(knowledge) ? knowledge.length : knowledge.knowledge_bases?.length) {
      throw new Error("Fresh package contains knowledge bases.");
    }
    const skills = await get("/api/skills");
    if (!JSON.stringify(skills).includes("yunkun-security-workflow")) throw new Error("Bundled workflow missing.");
    const tools = await get("/api/tools");
    for (const name of ["analyze_web_surface", "asset_intel_search", "threatbook_lookup", "get_target_profile"]) {
      if (!JSON.stringify(tools).includes(name)) throw new Error(`Native tool missing: ${name}`);
    }
    const agents = await get("/api/agents");
    const worker = (Array.isArray(agents) ? agents : agents.agents).find((entry) => entry.key === "worker");
    const detail = await get(`/api/agents/${worker.key}/visibility`);
    if (!JSON.stringify(detail).includes("yunkun-security-workflow")) throw new Error("Workflow not bound to worker.");

    await page.evaluate((value) => localStorage.setItem("yunkun_token", value), token);
    await context.addCookies([{ name: "yunkun_token", value: token, url: base }]);
    for (const width of [1440, 390]) {
      await page.setViewportSize({ width, height: width === 390 ? 844 : 1000 });
      for (const route of ["/dashboard/", "/system/skills/", "/system/rag/", "/system/proxy-pools/", "/system/llm/", "/system/settings/"]) {
        await page.goto(base + route);
        await page.locator(".yunkun-masthead").waitFor({ timeout: 15000 });
        if (route.includes("settings")) {
          await page.getByText("版本与更新", { exact: true }).waitFor();
          await page.getByText("当前已是最新版本。", { exact: true }).waitFor();
        }
        await page.screenshot({ path: path.join(output, `${width}-${route.split("/").filter(Boolean).join("-")}.png`), fullPage: true });
        const size = await page.evaluate(() => ({
          width: innerWidth, scroll: document.documentElement.scrollWidth,
        }));
        if (size.scroll > size.width + 2) throw new Error(`Horizontal overflow: ${route} at ${width}`);
        const broken = await page.locator("img").evaluateAll((images) =>
          images.filter((image) => image.complete && image.naturalWidth === 0).map((image) => image.src));
        if (broken.length) throw new Error(`Missing image: ${broken.join(", ")}`);
        results.push({ route, width, ok: true });
      }
    }
    simulatedUpdate = true;
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.goto(base + "/dashboard/");
    await page.getByText("发现云鲲控制台新版本", { exact: true }).waitFor();
    await page.getByText("隔离测试发布说明：更新提醒显示测试。", { exact: true }).waitFor();
    await page.screenshot({ path: path.join(output, "simulated-update-notice.png"), fullPage: true });
    await page.getByRole("button", { name: "稍后", exact: true }).click();
    await page.reload();
    await page.locator(".yunkun-masthead").waitFor();
    if (await page.getByText("发现云鲲控制台新版本", { exact: true }).isVisible()) {
      throw new Error("Dismissed update reminder repeated.");
    }
    await page.goto(base + "/system/settings/");
    await page.getByText("隔离测试发布说明：更新提醒显示测试。", { exact: true }).waitFor();
    await page.screenshot({ path: path.join(output, "simulated-update-settings.png"), fullPage: true });
    if (errors.length) throw new Error(`Browser errors: ${errors.join("; ")}`);
    fs.writeFileSync(path.join(output, "ui-smoke.json"), JSON.stringify({
      version: health.version, emptyTasks: true, emptyKnowledge: true,
      bundledWorkflow: true, nativeTools: true, workerBinding: true,
      simulatedUpdateNoticeAndNotes: true, browserErrors: errors, results,
    }, null, 2));
    console.log(`UI smoke passed: ${results.length} route/viewport combinations, fresh installation and native tool/Skill checks.`);
  } finally {
    await browser.close();
  }
})().catch((error) => { console.error(error.message); process.exitCode = 1; });
