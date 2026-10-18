# 主动探测工作流

> 视角：渗透测试，向目标发包探测端口/服务/指纹
> 加载时机：Phase 2b 开始时
> 前提：已获授权，并已完成不超过 90 秒的最小 baseline。Phase 2a 不再是必须完整跑完的长阶段。

## 零、clown21 默认：主动验证先于覆盖率

单目标 Web/API 默认顺序：

```text
baseline request
-> 认证/对象/状态/文件/URL 边界中选一个最高价值点
-> one-variable variant
-> negative control
-> 保存完整证据
-> 再决定是否扩大扫描覆盖
```

端口扫描、目录枚举和模板扫描是地图工具，不是默认第一主线。只有用户授权范围包含资产面盘点，或当前目标没有可交互业务面时，才按后文的网络扫描流程展开。

短周期中禁止先跑全端口、全目录、全模板再开始业务验证。至少一半时间必须留给差分、影响确认和 cleanup。

---

## 一、Nmap 扫描策略

### 1.1 主机发现

| 参数 | 原理 | 适用场景 |
|------|------|---------|
| `-sn` | Ping 扫描，不扫端口 | 快速盘点网段存活主机 |
| `-Pn` | 跳过主机发现，视目标全部存活 | 云服务器/防火墙屏蔽 ICMP |
| `-PS<端口>` | TCP SYN Ping | 穿透 ICMP 过滤的防火墙 |
| `-PA<端口>` | TCP ACK Ping | 突破 SYN 过滤 |
| `-PU<端口>` | UDP Ping | DNS/SNMP/游戏等 UDP 设备 |
| `-PR` | ARP Ping | 局域网专用，最准最快 |

```bash
# 内网快速发现（ARP）
sudo nmap -sn --send-eth 192.168.1.0/24

# 外网/云环境
nmap -Pn -n 10.0.0.1-50

# 穿透防火墙
sudo nmap -sn -PS80,443 -PA21,22,25 -PU53 10.10.0.0/24

# 生成存活列表
nmap -sn 192.168.0.0/24 -oG - | grep "Up" | awk '{print $2}' > alive_hosts.txt
```

### 1.2 端口扫描类型

| 类型 | 参数 | 需 Root | 特点 | 场景 |
|------|------|--------|------|------|
| **SYN** | `-sS` | ✅ | 半开放，较隐蔽 | **首选** |
| TCP 全连接 | `-sT` | ❌ | 完整握手，留日志 | 无 root 时替代 |
| UDP | `-sU` | ✅ | 慢，发现 DNS/SNMP/DHCP | 完整扫描必做 |
| ACK | `-sA` | ✅ | 判断防火墙规则 | 防火墙探测 |
| FIN/NULL/Xmas | `-sF/-sN/-sX` | ✅ | 规避简单状态防火墙 | 特定绕过 |

### 1.3 扫描范围选择

| 范围 | 参数 | 端口数 | 耗时（单机） | 场景 |
|------|------|--------|-------------|------|
| Fast | `-F` | 100 | ~10秒 | 初步快速侦察 |
| Top 1000 | 默认 | 1000 | ~30秒 | 标准评估第一阶段 |
| 全端口 TCP | `-p-` | 65535 | 3-20分钟 | 全面评估 |
| UDP Top 200 | `-sU --top-ports 200` | 200 | 15-60分钟 | 重要目标 |
| 常见 Web | `-p 80,443,8080,8443,8888,3000,8000` | 7 | ~3秒 | Web 专项 |

### 1.4 速度控制

| 模板 | 参数 | IDS 风险 | 场景 |
|------|------|---------|------|
| Paranoid | `-T0` | 极低 | 极度隐蔽 |
| Sneaky | `-T1` | 低 | 有 IDS 环境 |
| Polite | `-T2` | 较低 | 避免影响目标性能 |
| **Normal** | **`-T3`** | 中 | **默认选择** |
| Aggressive | `-T4` | 较高 | 实验室/靶机/CTF |
| Insane | `-T5` | 极高 | 内网快速，可能漏报 |

```bash
# 精细化速率控制
nmap --min-rate 100 --max-rate 500 target
sudo nmap -sS -T2 --max-retries 1 --max-rate 100 -Pn target  # 隐蔽
```

### 1.5 服务识别

```bash
nmap -sV target                          # 基础
nmap -sV --version-intensity 0 target    # 仅 Banner，最快
nmap -sV --version-intensity 5 target    # 平衡（推荐）
nmap -sV --version-all target            # 最全面
```

### 1.6 OS 探测

```bash
sudo nmap -O --osscan-guess target
sudo nmap -A target   # = -sV -O --script=default --traceroute
```

### 1.7 NSE 脚本引擎

```bash
# 默认安全脚本
nmap -sC target

# 发现类
nmap --script=dns-brute,http-headers,http-robots.txt,smb-enum-shares target

# 认证类
nmap --script=ssh-auth-methods,ftp-anon,smtp-open-relay target

# 漏洞类
nmap --script=vuln target
nmap --script=smb-vuln-ms17-010 -p 445 target     # EternalBlue
nmap --script=ssl-heartbleed -p 443 target          # Heartbleed
nmap --script=http-shellshock target                # Shellshock

# 服务专项组合
nmap -p 139,445 --script "smb-enum-shares,smb-enum-users,smb-protocols,smb-vuln-ms17-010" target
nmap -p 3306 --script "mysql-info,mysql-databases,mysql-empty-password" target
nmap -p 6379 --script redis-info target
nmap -p 27017 --script "mongodb-info,mongodb-databases" target
```

### 1.8 输出格式

```bash
# 推荐：全部格式
nmap ... -oA /path/to/scan_$(date +%Y%m%d_%H%M%S)
# -oN（文本） -oX（XML，导入 Metasploit） -oG（grepable）
```

---

## 二、Nmap 命令模板库

```bash
# 模板 1：单 IP 快速评估
sudo nmap -sS -sV -sC -O -T4 -p- --min-rate 5000 -oA scan TARGET_IP

# 模板 2：单 IP 全面评估（推荐）
# 步骤1：TCP 全端口
sudo nmap -sS -p- --min-rate 5000 -T4 -oA tcp_all TARGET_IP
# 步骤2：对开放端口做详细扫描
sudo nmap -sS -sV -sC -O -A -p <open_ports> -oA tcp_detail TARGET_IP
# 步骤3：UDP Top 200
sudo nmap -sU --top-ports 200 -T4 -oA udp_top TARGET_IP

# 模板 3：C 段网络扫描
sudo nmap -sn -T4 192.168.1.0/24 -oG alive.gnmap
grep "Up" alive.gnmap | awk '{print $2}' > hosts.txt
nmap -sS -T4 --top-ports 1000 -iL hosts.txt -oA subnet_scan

# 模板 4：大规模网络扫描（/16 以上）
masscan -p1-65535 10.0.0.0/16 --rate=10000 -oL masscan_results.txt
awk '/open/{print $4}' masscan_results.txt | sort -u > interesting.txt
nmap -sV -sC -iL interesting.txt -oA detailed_scan

# 模板 5：隐蔽扫描（规避 IDS）
sudo nmap -sS -T1 -f --data-length 25 --randomize-hosts \
  -D RND:10 --source-port 53 -Pn -n -p 80,443,22 TARGET_IP

# 模板 6：Web 应用专项
nmap -sS -sV -p 80,443,8080,8443,8888,3000,8000,8008,9090,9200 \
  --script "http-headers,http-methods,http-server-header,http-robots.txt,http-waf-fingerprint" \
  TARGET -oA web_scan
```

---

## 三、Web 应用指纹识别

### 3.1 whatweb

```bash
whatweb -v http://target.com
whatweb --aggression 3 http://target.com   # 更激进
whatweb -i hosts.txt --log-brief=results.txt  # 批量
```

### 3.2 httpx（ProjectDiscovery）

```bash
httpx -u http://target.com -tech-detect -status-code -title -follow-redirects
httpx -l hosts.txt -tech-detect -status-code -title -web-server -ip -o results.txt
httpx -l hosts.txt -json -o results.json   # JSON 输出
```

### 3.3 HTTP 响应头分析

```bash
curl -I -L http://target.com
```

**关键响应头**：

| 响应头 | 情报价值 |
|--------|---------|
| `Server: Apache/2.4.51` | 服务器类型+版本 |
| `X-Powered-By: PHP/7.4.3` | 后端语言版本 |
| `Set-Cookie: PHPSESSID` | PHP 应用 |
| `Set-Cookie: JSESSIONID` | Java/Tomcat |
| `X-AspNet-Version: 4.0` | ASP.NET |
| `CF-Ray` | Cloudflare CDN |
| `Access-Control-Allow-Origin: *` | CORS 宽松（注意！）|

### 3.4 Favicon Hash 指纹

```python
import requests, mmh3, base64
response = requests.get('http://target.com/favicon.ico', verify=False)
hash_val = mmh3.hash(base64.encodebytes(response.content))
print(f'Shodan: http.favicon.hash:{hash_val}')
```

### 3.5 WAF 检测

```bash
wafw00f http://target.com
wafw00f -a http://target.com   # 暴力模式
nmap --script http-waf-fingerprint,http-waf-detect -p 80,443 target
```

**常见 WAF 特征**：Cloudflare（`cf-ray`头）、AWS WAF（`x-amzn-requestid`）、ModSecurity（403+"Mod_Security"）、F5（`BIGipServer` Cookie）、Akamai（`AkamaiGHost`）、Imperva（`incap_ses` Cookie）

---

## 四、目录/路径枚举

### 4.1 工具选择

| 工具 | 强项 | 推荐场景 |
|------|------|---------|
| feroxbuster | **原生递归**，Rust 写的快 | 深度枚举（首选） |
| gobuster | 多模式（dir/dns/vhost） | 通用 |
| ffuf | 最快，支持 POST/参数 fuzz | API/参数枚举 |
| dirsearch | Python，不需 root | 兼容性好 |

### 4.2 命令示例

```bash
# feroxbuster（推荐）
feroxbuster -u http://target.com \
  -w /usr/share/seclists/Discovery/Web-Content/raft-medium-directories.txt \
  -d 3 -t 50 -x php,html,js --rate-limit 50

# gobuster
gobuster dir -u http://target.com \
  -w /usr/share/seclists/Discovery/Web-Content/common.txt \
  -x php,html,txt,bak -t 50

# gobuster vhost 枚举
gobuster vhost -u http://target.com \
  -w /usr/share/seclists/Discovery/DNS/subdomains-top1million-5000.txt --append-domain
```

### 4.3 字典选择（SecLists）

| 用途 | 字典 | 条目数 |
|------|------|--------|
| 快速初探 | `common.txt` | ~4700 |
| 标准评估 | `raft-medium-directories.txt` | ~30000 |
| 深度挖掘 | `directory-list-2.3-big.txt` | ~1270000 |
| API 路径 | `api/api-endpoints.txt` | API 专用 |
| 框架专用 | `spring-boot.txt` / `apache.txt` | 对应框架 |

### 4.4 高价值敏感路径必查清单

```
# 版本控制/代码泄露
/.git/  /.git/config  /.svn/  /.DS_Store  /.env  /.env.production

# 配置文件
/config.php  /application.properties  /web.config  /appsettings.json  /wp-config.php

# 备份文件
/backup.zip  /backup.tar.gz  /backup.sql  /db.sql  /www.tar.gz

# 管理界面
/admin/  /wp-admin/  /phpmyadmin/  /adminer.php  /manager/  /console/

# 框架端点
/actuator/  /actuator/env  /actuator/heapdump
/swagger-ui.html  /swagger-ui/  /v2/api-docs  /openapi.json
/graphql  /graphiql

# 调试
/phpinfo.php  /info.php  /debug/

# 常规
/robots.txt  /sitemap.xml  /.well-known/security.txt  /server-status
```

---

## 五、服务特定探测

### 5.1 数据库服务

```bash
# MySQL (3306)
nmap -sV -p 3306 --script mysql-info,mysql-databases,mysql-empty-password target

# PostgreSQL (5432)
nmap -sV -p 5432 --script pgsql-brute target

# MSSQL (1433)
nmap -sV -p 1433 --script ms-sql-info,ms-sql-config,ms-sql-empty-password,ms-sql-ntlm-info target

# Redis (6379)
nmap -sV -p 6379 --script redis-info target
redis-cli -h target info server

# MongoDB (27017)
nmap -sV -p 27017 --script mongodb-info,mongodb-databases target

# Elasticsearch (9200)
curl http://target:9200/
curl http://target:9200/_cat/indices?v

# 统一快速探测
nmap -sV -p 3306,5432,1433,6379,27017,9200,9300,5984,11211 \
  --script "banner,(mongodb* or mysql* or redis* or ms-sql*) and not intrusive" target
```

### 5.2 远程管理服务

```bash
# SSH (22)
nmap -sV -p 22 --script ssh-auth-methods,ssh2-enum-algos target

# RDP (3389)
nmap -sV -p 3389 --script rdp-enum-encryption,rdp-vuln-ms12-020 target

# WinRM (5985/5986)
nmap -sV -p 5985,5986 target

# VNC (5900)
nmap -sV -p 5900 --script vnc-info target
```

### 5.3 文件共享服务

```bash
# SMB (445/139)
nmap -sV -p 139,445 --script smb-os-discovery,smb-security-mode,smb-enum-shares,smb-enum-users target
nmap -p 445 --script smb-vuln-ms17-010,smb-vuln-ms08-067 target
smbclient -L //target -N
enum4linux -a target

# FTP (21)
nmap -sV -p 21 --script ftp-anon,ftp-syst target

# NFS (2049)
nmap -sV -p 111,2049 --script nfs-ls,nfs-showmount target
showmount -e target
```

### 5.4 邮件服务

```bash
# SMTP (25/587)
nmap -sV -p 25,587,465 --script smtp-commands,smtp-open-relay,smtp-enum-users target

# POP3 (110) / IMAP (143)
nmap -sV -p 110,143,993,995 --script pop3-capabilities,imap-capabilities target
```

---

## 六、完整执行顺序

```
1. 主机发现      → nmap -sn / masscan ping sweep → alive_hosts.txt
2. TCP 端口扫描   → Top 1000 快速（全体）→ 全端口（重点目标）
3. UDP 端口扫描   → Top 200（重点目标）
4. 服务识别       → nmap -sV -sC 对开放端口
5. Web 指纹       → whatweb / httpx 对 Web 端口
6. WAF 检测       → wafw00f
7. 目录枚举       → feroxbuster / gobuster + 敏感路径清单
8. 服务特定枚举   → SMB/FTP/数据库/邮件/远程管理
9. 漏洞预扫       → nmap --script vuln + 特定 CVE
```

---

## 七、MCP 工具降级对照

| 操作 | 有 kali-mcp | 无工具降级 |
|------|-----------|----------|
| Nmap 扫描 | 直接调用 | 输出命令让用户在 Kali 中执行，粘贴结果回来 |
| Web 指纹 | `whatweb` / `httpx` | `curl -I` 手动分析响应头 |
| WAF 检测 | `wafw00f` | 发 XSS payload 观察 403 响应 |
| 目录枚举 | `feroxbuster` / `gobuster` | 手动检查敏感路径清单 |
| SMB 枚举 | `enum4linux` / `smbclient` | `nmap --script smb-*` |
| 服务探测 | 各服务专用工具 | `nmap -sV --script` 替代 |
