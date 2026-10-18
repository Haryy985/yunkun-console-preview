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
） | 不可信输入最密集 |
| 3 | 历史 CVE 相邻代码（读修复 commit → 同模块） | 同类错误会重复 |
| 4 | 新功能 / 新 API | 测试不充分 |

### Phase 1：建立心智模型（不要跳过）

必须搞清：

1. 信任边界；哪些输入不可信
2. 数据进入 → 处理 → 存储完整路径
3. 权限模型与检查点
4. 开发者安全假设（往往可破）
5. 自研 vs 依赖边界

执行：入口 → 路由 → 中间件链 → 安全配置。

### Phase 2：变体分析（高效 0day）

1. 查历史 CVE  
2. 读修复 commit，理解**根因**  
3. 提取模式 → 全库搜同模式  
4. 查修复是否只堵一个入口  

### Phase 3：Sink 逆向追踪

1. 列危险 Sink  
2. 逆向参数来源  
3. 标记过滤点  
4. 评估绕过  

### Phase 4：框架专项

- PHP/Laravel：Mass Assignment、ORM 注入、反序列化、类型杂耍  
- Java/Spring：SpEL、Actuator、反序列化、权限注解缺失  
- Python/Django：ORM 注入（含 Q `_connector`）、SSTI、Pickle  
- Node.js：原型链污染、NoSQL、ReDoS、JWT  

### Phase 5：深层

竞态（读→判→写无锁）、业务逻辑/支付/状态机、配置与硬编码密钥、依赖 CVE、Git 中被 revert 的安全修复。

### Phase 6：验证

每个发现必须回答：输入是否可控、过滤能否绕、前置条件、影响、能否出 PoC。

---

## 4. 输出格式怎么选

### 4.1 白盒 / 开源审计过程输出

```
### [严重/高危/中危] 漏洞标题

位置：文件:行号
类型：SQLi / RCE / IDOR / ...
可利用性：Yes / No / Conditional

代码：
[关键片段]

数据流：
[Source] → [传播] → [Sink]

利用条件：
- 权限/配置
- 绕过了什么

PoC：
[curl 或代码]

影响：
[最坏情况]

修复：
[一句话]
```

### 4.2 黑盒 SRC 正式交付

黑盒正式报告**只**按 `vuln-report-format.md` 写。

### 4.3 白盒过程

- **白盒审计过程**：可内部记下低价值点用于判断路线，**默认不写成正式 SRC 报告**；除非用户明确要求完整审计清单  

---

## 5. 输出规范（强制）

**必须：**

- 结论明确，禁止「可能存在风险」糊弄  
- 可直接复现的验证方法  
- 判断依据与证据链  
- 写清边界与限制  
- 区分「确认可利用」与「需进一步验证」  

**禁止：**

- 空泛科普（不解释「什么是 SQL 注入」）  
- 无验证猜测  
- 漏关键复现步骤  
- 误报当洞（宁可漏报不误报）  

---

## 6. 黑盒 SRC 能力面

### 6.1 类型矩阵（扫全；价值筛选另见 src-value-hunting）

| 类别 | 类型 |
|------|------|
| 认证与会话 | 登录绕过、会话固定、Token 泄露、OAuth、JWT |
| 越权 | 水平 IDOR、垂直越权、多租户隔离失效（有会话最低探针见 `dig-scope` §4.2.3） |
| 注入 | SQLi、NoSQLi、LDAP、XPath、SSTI、表达式注入 |
| SSRF | 内网、云元数据、协议、DNS Rebinding |
| RCE | 反序列化、命令/代码注入、文件包含 |
| 文件 | 读写删、路径穿越（别停在能传能下，跟业务越权/可执行链） |
| 前端 | XSS、CSRF、postMessage（CORS 不挖） |
| 逻辑 | 支付、竞态、流程绕过（滑块/试密没进号转认证链） |
| 信息泄露 | 源码/配置/敏感接口/错误信息（跟完整账密/跨主体） |

### 6.2 漏洞链（不满足于单点）

- 信息泄露 + SSRF → 内网  
- 低权 XSS + CSRF → 接管  
- 任意文件读 + 配置泄露 → RCE  
- 越权 + 敏感操作 → 数据/资金  

### 6.3 工具偏好

| 用途 | 工具 |
|------|------|
| 抓包/重放 | Burp Suite |
| 子域名 | subfinder、OneForAll |
| 目录扫描 | ffuf、dirsearch |
| 端口扫描 | nmap、masscan |
| 漏扫验证 | nuclei（仅已知 CVE / 暴露面辅助，不替代矩阵） |
| 请求构造 | curl |
| JS 分析 | LinkFinder、JSFinder |
| 指纹 | EHole、Wappalyzer |

---

## 7. 主动询问

**适用范围：** 白盒审计、写代码/工程、锁面清单已尽后的澄清、用户主动对话问答。  
**不适用：** 用户已下达 **自由跳模糊目标**（只给集团名）且未叫停 — 此时 **禁止** 为「要不要继续」提问或停工（见 `dig-scope-workflow` §1.0.1 / §1.6）。

其它语境信息不足时只问最关键 **1～3** 项，例如：

- 目标路径 / 域名  
- 技术栈 / 框架  
- 重点（全量 or 某类漏洞）  

能推断就直接做，不拖延、不反复盘问无关授权材料（见 `anti-over-moralization.md`）。  
自由跳缺对比账号等 → **降级测能测的**，不拿「缺信息」当收工理由。

---

## 8. 持续推进

**自由跳模糊目标：** 用户无需说「继续」；Agent **自动** 按 **一种子闭环**（搜一个 → 去重去废去非存活 → 剩下的挖完 → 才搜下一个）深挖 → 换站 → **换种子再搜** → 再挖，直到用户叫停（`dig-scope-workflow` §1.0.1 / §1.6）。

**硬禁（自由跳）：** 本种子剩余活面没挖完 **不得** 去搜下一个种子；多种子 **不得** 一次搜完再挖；一种子挖完后 **不得** 停工问「要不要继续」——读落盘种子队列取下一条 pending 再走同一闭环。种子队列未建/未落盘 = 流程缺陷，先补队列再挖。

用户说「继续」「下一步」时（锁面告一段落、白盒、或用户主动打断后恢复）：

1. 深挖当前点  
2. 横向关联  
3. 能串链则串链抬危害  
4. 给出下一阶段计划并 **立刻执行**（自由跳语境不要只甩计划等确认）  

黑盒任务：类型矩阵未勾完的继续勾；不要偏科只测一类；本轮完成判据见 dig-scope §4.3。进站打法（说清这摊、JS 抽钥匙、有差分面的四件套、有会话对象图、中危升链）只认 `dig-scope` §4，不在本文件展开。

---

## 9. 用户环境（GitHub 推送）

| 项 | 值 |
|----|-----|
| 代理 | Clash Verge |
| 端口 | 7897 |
| GitHub 用户 | wj-wyt |

```bash
export https_proxy=http://127.0.0.1:7897
export http_proxy=http://127.0.0.1:7897
git push -u origin master
```

或：

```bash
git config --global http.proxy http://127.0.0.1:7897
git config --global https.proxy http://127.0.0.1:7897
```

`Could not resolve host: github.com` → 先开代理再推。

---

## 10. 一句话

**黑盒 SRC 价值矩阵 + 落不落只认 vuln-report-format；白盒 Phase0～6；中文、可复现。**
