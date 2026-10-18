# JS 逆向配合接口挖掘指南

> 进站强制步骤见 `dig-scope` §4.1：**不只抽 `/api/` path**。盐、密文 id 公钥、hidden/admin 路由、写死的演示号有就进清单，没有写「无」。演示号当钥匙，不是登录框字典。

## 使用场景

- 页面接口有加密参数，无法直接用 curl 重放
- 需要从前端 JS 发现隐藏 API 接口
- 需要了解签名/token 生成逻辑以构造任意请求
- 路由表里 hidden/admin、webpack 异步 chunk、写死演示号/测试租户

---

## 流程一：接口发现

### 1. 查看网络请求

使用 js-reverse MCP 工具（浏览器已打开目标页面时）：

```
操作: list_network_requests()
筛选: resourceTypes=["xhr", "fetch"]
关注: 含用户数据的接口（/user/, /api/, /order/, /account/）
```

### 2. 从 JS 源码批量提取接口

```javascript
// 在 evaluate_script 中执行，提取页面所有 XHR 路径
() => {
  const scripts = Array.from(document.querySelectorAll('script[src]'))
    .map(s => s.src);
  return scripts;
}
```

```bash
# 下载所有 JS 文件，grep 接口路径
for url in $(cat js_files.txt); do
  curl -s "$url" | grep -oP '"(/api/[^"]+)"' | tr -d '"'
done | sort -u > discovered_apis.txt

# 关键词搜索
grep -E "(userId|uid|token|sign|order|payment)" discovered_apis.txt
```

### 3. 使用 search_in_sources 搜索

```
search_in_sources("userId")          // 找用户ID相关接口
search_in_sources("/api/")           // 找所有 API 路径
search_in_sources("Authorization")   // 找 token 设置位置
search_in_sources("signature")       // 找签名参数
```

---

## 流程二：加密参数分析

适用于请求中有 `sign`/`_token`/`x-sign` 等加密参数。

### Step 1: XHR 断点定位

```
1. break_on_xhr("/api/target-endpoint")
2. 在页面触发对应操作
3. 执行暂停后: get_paused_info()
4. 查看调用栈，找到设置加密参数的函数
```

### Step 2: 分析调用栈

```
get_paused_info() 返回示例:
  Frame 0: setRequestHeader (XMLHttpRequest)
  Frame 1: signRequest (utils.js:342)     ← 关注这里
  Frame 2: sendApiRequest (api.js:89)
  Frame 3: onClick (page.js:234)
```

定位到 Frame 1，读取源码：

```
get_script_source(url="utils.js", startLine=335, endLine=355)
```

### Step 3: 提取签名逻辑

常见签名算法模式：

```javascript
// 模式 1: 参数排序 + MD5
function signRequest(params) {
  const sorted = Object.keys(params).sort().map(k => `${k}=${params[k]}`).join('&');
  return md5(sorted + SECRET_KEY);
}

// 模式 2: timestamp + nonce + HMAC
function sign(data) {
  const ts = Date.now();
  const nonce = Math.random().toString(36).substr(2);
  return hmacSha256(ts + nonce + JSON.stringify(data), APP_SECRET);
}

// 模式 3: 固定 salt 拼接
const sign = md5(userId + ':' + timestamp + ':' + SALT);
```

### Step 4: 在浏览器中执行签名函数

```javascript
// evaluate_script 直接调用页面内的签名函数
() => {
  // 如果函数在全局作用域
  return window.signRequest({userId: "victim_id", action: "getInfo"});
}
```

### Step 5: Python 复现签名

```python
import hashlib
import hmac
import time
import random
import string

# MD5 签名复现
def sign_request(params: dict, secret_key: str) -> str:
    sorted_str = '&'.join(f"{k}={params[k]}" for k in sorted(params.keys()))
    return hashlib.md5((sorted_str + secret_key).encode()).hexdigest()

# HMAC-SHA256 签名复现
def sign_hmac(data: str, app_secret: str) -> str:
    ts = str(int(time.time() * 1000))
    nonce = ''.join(random.choices(string.ascii_lowercase, k=8))
    msg = ts + nonce + data
    return hmac.new(app_secret.encode(), msg.encode(), hashlib.sha256).hexdigest()

# 验证：Python 结果应与浏览器 JS 执行结果一致
```

---

## 流程三：隐藏接口发现

### 从 Webpack chunk 中提取

```bash
# 找 chunk 文件
curl -s "https://target.com" | grep -oP 'chunk\.[a-z0-9]+\.js'

# 下载所有 chunk
for chunk in $(curl -s "https://target.com" | grep -oP '"/static/js/[^"]+\.js"' | tr -d '"'); do
  curl -s "https://target.com$chunk" >> all_js.txt
done

# 提取路径
grep -oP '"(/[a-z]+){1,5}"' all_js.txt | sort -u | grep -v node_modules
```

### 从路由配置提取

```bash
# Vue/React 路由配置
grep -oP 'path:\s*["\x27][^"'\'']+' all_js.txt
grep -oP '"route":\s*["\x27][^"'\'']+' all_js.txt

# Axios 基础 URL
grep -oP 'baseURL:\s*["\x27][^"'\'']+' all_js.txt
grep -oP 'BASE_API\s*=\s*["\x27][^"'\'']+' all_js.txt
```

---

## 流程四：API 参数枚举

发现接口后，枚举参数找越权/注入点：

```python
import requests

# 对发现的接口逐个测试
discovered_apis = [
    "/api/v1/user/info",
    "/api/v1/order/list",
    "/api/v2/account/profile",
]

session = requests.Session()
session.headers.update({"Authorization": "Bearer YOUR_TOKEN"})

for api in discovered_apis:
    r = session.get(f"https://target.com{api}")
    print(f"[{r.status_code}] {api} - {len(r.text)} bytes")
    if r.status_code == 200:
        # 记录响应中的 ID 字段，用于后续越权测试
        data = r.json()
        print(f"  响应字段: {list(data.keys()) if isinstance(data, dict) else 'array'}")
```
