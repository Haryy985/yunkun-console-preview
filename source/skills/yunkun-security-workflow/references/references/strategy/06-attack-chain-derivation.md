# 攻击链自动推导引擎

> 视角：攻击面地图生成后，自动推导从入口到最深控制点的完整攻击链
> 输入：攻击面地图（目标 → 端口 → 服务 → 版本 → 技术栈 → 入口点）
> 输出：优先级排序的攻击路径建议（含每一步的工具/payload/预期结果）

---

## 1. 一句话原则

**不等人逐个挑目标——引擎根据信号组合自动推荐"最可能打穿"的攻击路径。**

攻击链 = 入口点 + 漏洞类型 + 权限跃迁 + 横向跳板 + 最终目标。
引擎沿这条路自动匹配 playbook、payload 和 bypass 策略。

---

## 2. 攻击链推导矩阵

### 2.1 入口点 → 首层攻击类型（自动映射）

| 入口信号 | 自动推导攻击类型 | 匹配 Playbook | 预期首层收益 |
|---------|----------------|---------------|-------------|
| 开放 6379/27017/3306/1433/5432 端口 | 未授权数据库访问 | `playbooks/unauth-access.md` | 数据读取 / 写文件 RCE |
| 开放 22/3389/5900 + 弱口令 | 远程登录爆破 | `playbooks/unauth-access.md` §远程管理 | Shell 权限 |
| `/actuator` / `/swagger` / `/druid` 暴露 | 未授权 API 信息泄露 | `playbooks/unauth-access.md` §Actuator | 配置泄露 / env 凭据 |
| `.git` / `.svn` / `.env` / `backup.zip` 可访问 | 源码/配置泄露 | `playbooks/info-disclosure.md` | 源码审计 / 硬编码凭据 |
| 登录页 + 无验证码/频率限制 | 账号爆破 / 凭据喷洒 | `playbooks/unauth-access.md` §默认凭据 | 合法账号 |
| 302 → `cas/login` / `sso/login` | SSO 认证流攻击 | SKILL.md 协议 2（SSO/CAS） | 认证绕过 / 凭据规则提取 |
| SPA (`<div id="app">`) + JS bundle | JS 静态分析 API 挖掘 | SKILL.md 协议 3（JS API 挖掘） | 隐藏 API / 硬编码凭据 |
| `?id=` / `?uid=` / `/user/{id}` 参数 | IDOR / 越权 | `playbooks/arbitrary-x-authz.md` | 水平/垂直越权 |
| `?q=` / `?search=` / `?keyword=` 参数 | SQLi | `playbooks/sqli.md` | 数据库访问 |
| `?url=` / `?path=` / `?file=` 参数 | SSRF / LFI / 路径穿越 | `playbooks/ssrf-cache-host/` 或 `path-traversal/` | 内网探测 / 文件读取 |
| 文件上传点 (multipart/form-data) | 上传绕过 → Webshell | `playbooks/file-upload/00-index.md` | RCE |
| OAuth/SAML/JWT 认证 | 认证协议攻击 | `playbooks/oauth-saml-jwt/00-index.md` | 认证绕过 / 伪造 |
| GraphQL endpoint | GraphQL 攻击 | `playbooks/graphql.md` | 数据暴露 / 注入 |
| 支付/订单/优惠券/提现功能 | 业务逻辑攻击 | `playbooks/logic-flaws/00-index.md` | 金额篡改 / 越权操作 |
| 验证码/短信/邮件发送接口 | 竞态 / 爆破 | `playbooks/race-conditions.md` | 绕过限制 |

### 2.2 首层突破 → 权限跃迁链（自动递推）

拿到首层后，引擎根据当前权限自动推荐下一步：

| 当前权限 | 自动推荐下一步 | 匹配资源 |
|---------|--------------|---------|
| **无权限（仅信息泄露）** | 凭据喷洒 → 默认凭据 → 弱口令 | `dictionaries/default-credentials-cn.md` |
| **低权限 Web 用户** | IDOR → 越权 → 管理功能 | `playbooks/arbitrary-x-authz.md` |
| **普通用户（有 session）** | 参数污染 → Mass Assignment → 角色提升 | `playbooks/api-rest/00-index.md` |
| **文件读取（任意文件）** | 读配置 → 读源码 → 读 /proc/self/environ → 拿凭据 | `playbooks/path-traversal/00-index.md` |
| **SSRF（任意 URL）** | 内网探测 → 云元数据 → Redis/Memcached → RCE | `playbooks/ssrf-cache-host/10-ssrf-core.md` |
| **SQLi（有数据库权限）** | 读表 → 拿 admin hash → 破解/改密 → 后台上传 | `playbooks/sqli.md` |
| **文件上传（受限）** | 绕过检测 → 解析漏洞 → Webshell | `playbooks/file-upload/00-index.md` |
| **Webshell (www-data / IIS APPPOOL)** | 提权枚举 → SUID/Sudo/Cron/内核 | `references/strategy/12-exploit-postexp.md` §Linux |
| **Webshell (NT AUTHORITY\NETWORK SERVICE)** | 提权枚举 → 服务/令牌/UAC | `references/strategy/12-exploit-postexp.md` §Windows |
| **root / SYSTEM** | 凭据收集 → 横向移动 → 域攻击 | `playbooks/intranet-postexp/00-index.md` |

### 2.3 横向移动链（内网突破后自动展开）

| 当前内网位置 | 自动推荐横向目标 | 匹配资源 |
|-------------|----------------|---------|
| **Web 服务器（DMZ）** | 数据库服务器、缓存服务器、配置中心 | `12-exploit-postexp.md` §内网探测 |
| **数据库服务器** | 应用服务器、报表服务器、备份服务器 | 凭据复用 + 配置文件分析 |
| **跳板机 / 运维终端** | 全内网可达、运维通道、堡垒机 | PtH/PtT/Kerberoasting |
| **域成员机器** | 域控、Exchange、文件服务器 | `intranet-postexp/00-index.md` §域攻击 |
| **云环境（AK/SK 泄露）** | ECS/RDS/OSS 全资源 | `playbooks/ssrf-cache-host/11-cloud.md` |

---

## 3. 自动攻击链生成模板

Phase 2c 出攻击面地图后，引擎为每个 P0/P1 目标生成以下格式的攻击链建议：

```
┌─────────────────────────────────────────────────────────┐
│  攻击链 #01 · 自动推导 · 置信度: HIGH                      │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  [入口] https://target.com/.git/HEAD 可访问               │
│    │  信号: 信息泄露（源码/配置）                           │
│    │  Playbook: playbooks/info-disclosure.md §.git       │
│    │                                                    │
│    ▼                                                    │
│  [Step 1] git clone 源码 → grep 硬编码凭据                 │
│    │  工具: git-dumper / dvcs-ripper                      │
│    │  预期: 获取 DB 密码 / AK/SK / JWT secret              │
│    │                                                    │
│    ▼                                                    │
│  [Step 2] 凭据登录 DB / 云服务                             │
│    │  预期: DBA 权限 / ECS 控制台                          │
│    │                                                    │
│    ▼                                                    │
│  [Step 3] 数据库写文件 → Webshell / UDF 提权              │
│    │  预期: 服务器 Shell                                  │
│    │                                                    │
│    ▼                                                    │
│  [最深] 内网横向 → 域控（如适用）                            │
│                                                         │
│  估计用时: 2-4h | 关键依赖: .git 可完整克隆                  │
└─────────────────────────────────────────────────────────┘
```

---

## 4. 攻击链优先级自动排序规则

同时有多条攻击链时，按以下规则自动排序：

### 排序算法（加权打分）

```
总得分 = 入口可达性(0-3) × 2 + 权限跃迁跨度(0-3) × 3 + 利用确定性(0-3) × 2 + 横向潜力(0-3) × 1
```

| 维度 | 3 分 | 2 分 | 1 分 | 0 分 |
|------|------|------|------|------|
| **入口可达性** | 无需认证，直接可访问 | 需注册普通账号 | 需特权账号 | 需物理接触/内网 |
| **权限跃迁跨度** | 无权限 → root/SYSTEM | 普通用户 → 管理员 | 管理员 → 更高（域控） | 同级横向 |
| **利用确定性** | 已知 CVE + PoC 公开 | 通用漏洞类型（如 SQLi） | 需绕过 WAF/定制 | 需 0day |
| **横向潜力** | 内网可达 + 域环境 | 内网可达，无域 | 隔离环境 | 仅当前主机 |

**排序输出**：得分从高到低排列，Top 3 打 ⭐ 建议优先执行。

---

## 5. 引擎输出格式（嵌入攻击面地图）

在 Phase 2c 的攻击面地图中，新增"推荐攻击链"列：

```
目标资产 | 服务/技术栈 | 入口点 | 认证状态 | 攻击优先级 | 匹配信号 | 推荐攻击链
---------|------------|--------|---------|-----------|---------|-----------
xxx.com  | Tomcat 9   | /.git  | 无       | P0        | 源码泄露 | ⭐ 链#01: .git→凭据→DB→Webshell→内网
xxx.com  | SpringBoot | /actuator/env | 无 | P0   | 配置泄露 | ⭐ 链#02: env→AK/SK→云控制台→全资源
api.xxx  | Nginx+Flask| /api/user/{id} | 普通用户 | P1 | IDOR    | 链#03: IDOR→读全用户数据→找管理员→后台
```

---

## 6. 引擎自我修正（Monitor 反馈回路）

每执行完一条攻击链的一个 Step 后，Monitor 自动评估：

- **Step 成功**：继续链中下一步，更新"实际用时 vs 估计用时"
- **Step 部分成功**：调整后续 Step 的预期，必要时切换到备选链
- **Step 完全失败**：标记该链为"已堵死"，自动推荐备选链（从攻击面地图中选得分次高的）
- **连续 2 条链失败**：触发 Reflector → 建议重新收集信息或调整测试策略

---

## 7. 与现有流程的集成点

```
Phase 2c 攻击面地图
    │
    ├── 攻击链自动推导（本文件）── 输出推荐攻击链
    │
    ▼
Phase 3 Discover（按推荐攻击链顺序逐条测试）
    │
    ├── Step 成功 → 继续链中下一步
    ├── Step 失败 → Monitor 评估 → 备选链
    │
    ▼
Phase 4 Exploit（沿攻击链深度利用 + 横向）
```

> 本文件与 `01-attack-priority.md` 互补：前者定"单个漏洞打哪个"，本文件定"整条攻击链走哪条"。
