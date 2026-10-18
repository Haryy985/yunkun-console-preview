# 字节跳动 SRC 信息收集与攻击面测绘

> 生成于 2026-07-12，基于一次完整的 ByteSRC 信息收集会话。
> execution-framework可迁移到其他大型互联网公司 SRC 场景。

## 核心资产域

| 产品 | 域名 | 子域数量 | 特点 |
|------|------|---------|------|
| 抖音 | douyin.com | ~264 | Tengine/volc-dcdn |
| 今日头条 | toutiao.com | ~154 | 开发者门户、开放平台 |
| 西瓜media | ixigua.com | ~296 | 大量 CDN/直播节点 |
| 飞书 | feishu.cn | 大量(含租户域) | 98% 是客户租户，存活率低 |
| 火山引擎 | volcengine.com | — | Goofy Node SSR 框架 |
| 剪映/CapCut | capcut.com | ~219 | 国际化部署 |
| 番茄小说 | fanqienovel.com | ~7 | 小规模 |
| 主域 | bytedance.com | ~2410 | 大量 bytespider 爬虫节点需过滤 |

**关键过滤规则**: bytedance.com 子域中 `bytespider-*` 开头的均为爬虫节点，需过滤掉

## SSO 单点登录系统 (`sso.bytedance.com`)

### OIDC 配置
- **Well-Known**: `https://sso.bytedance.com/.well-known/openid-configuration`
- **JWKS**: `https://sso.bytedance.com/oidc/v1/keys` (2 个 RSA key)
- **授权端点**: `https://sso.bytedance.com/oidc/v1/authorize`
- **Token 端点**: `https://sso.bytedance.com/oidc/v1/token`
- **Userinfo**: `https://sso.bytedance.com/oidc/v1/userinfo` (始终返回 200 但空 body)
- **Logout**: `https://sso.bytedance.com/oidc/v1/logout`

### 关键安全发现
- **HS256 + RS256 同时支持** — 经典 JWT 算法混淆攻击面
- **暴露的管理 Scope**: `*`, `ListUsers`, `DeleteUser`, `CreateUser`, `UpdateUser`, `ProvisionBytedUser`, `ciam.*`, `ssf.*`
- **支持 client_secret_post 认证方法**
- **支持 HS256** for id_token signing

### 内部 API 端点（从前端 JS 提取）
| 端点 | 方法 | 响应 |
|------|------|------|
| `/api/v1/` | OPTIONS | 200 `{"response":"ok","ret":0}` |
| `/api/v1/health` | GET | 200 `{"response":"ok","ret":0}` |
| `/api/v1/apps` | GET/POST | 403 `{"code":4003,"message":"权限不足"}` |
| `/api/v1/apps/list` | GET | 403 "权限不足" |
| `/api/v1/apps/me` | GET | 403 "权限不足" |
| `/api/v1/users/me` | GET | 401 (无auth) |
| `/api/v1/be/user` | GET | 401 (无auth) |
| `/api/v1/login/scode` | GET | 400 "session expired" |
| `/api/v1/lark/oauth` | GET | 307 → open.feishu.cn OAuth |

### SSO 前端认证端点
- `/api/v1/login/fido2/begin_discoverable_login?redirect_uri=`
- `/api/v1/login/fido2/finish_discoverable_login`
- `/api/v1/security/fido2/begin_registration`
- `/api/v1/security/fido2/finish_registration`

## 火山引擎 (`console.volcengine.com`)

### API 网关特征
- 所有 `/api/` 前缀路径被 volc-dcdn WAF 拦截 → 403 `{"Code":"Forbidden","Message":"Method forbidden"}`
- 后端 Goofy Node SSR (`x-gateway-scm-name`, `x-gw-dst-psm`)
- 返回结构化 JSON 含 RequestId/Service/Error

### WAF 绕过发现
| 方法 | 结果 |
|------|------|
| `/API/v1/` (大写) | `302` → 绕过 WAF，到达 SSR 登录层 |
| `/api%09/v1/` (控制字符) | `500` → WAF 未拦截，后端崩溃 |
| `X-Forwarded-Host: 127.0.0.1` | `404` → 影响路由选择 |
| `api.volcengine.com` | 独立 API 域名，不受 console WAF 覆盖 |

### 子域/内部服务
| 域名 | 来源 |
|------|------|
| `ngcc.bytedance.com` | main.js 域名配置 `ngccDomain` — 内部 NGCC 服务 |
| `console.partner.volcengine.com` | 合作伙伴控制台 |
| `partner.volcengine.com` | 合作伙伴门户 |
| `developer.volcengine.com` | 开发者门户 |
| `market.volcengine.com` | 市场 |
| `api.volcengine.com` | 独立 API 域名 |
| `activity.volcengine.com` | 活动平台 |
| `ark.volcengine.com` | 方舟平台 |
| `exp.volcengine.com` | 体验平台 |

### 前端 API 端点（需 credentials:include + x-csrf-token）
- `/api/passport/*` — account/getUser, login/logout, security/encCerts, googlePromptCallback
- `/api/search/*` — suggestion, hotWord, openSuggestion, hotCard
- `/api/siteLead/*` — advisory, advisoryByLogin, sendSmsVerifyCode
- `/api/account/bindInvitation`
- `/api/activity/CheckIdGroupInV2`, `checkReferralCode`
- `/api/bee/getJson`
- `/api/location/getIPInfo`
- `/api/info/getCompanyNames`

## 飞书/文档

### 飞书账号认证端点 (docs.feishu.cn)
- `/accounts/auth/`, `/accounts/config`, `/accounts/csrf`
- `/accounts/idp/auth_url`, `/accounts/page/create`
- `/accounts/page/login/`, `/accounts/retrieve/`

### 暴露的 app_id
- `app_id=18` — 飞书文档 OAuth 应用
- `app_id=3013` — 另一个 OAuth 应用
- 飞书 OAuth 流程: `open.feishu.cn/open-apis/authen/v1/index`

### 内部环境
| 域名 | 说明 |
|------|------|
| `hc.feishu-boe.cn` | BOE 内部帮助中心 |
| `www.feishu-staging.cn` | Staging 环境 |
| `docs.feishu-staging.cn` | 文档 Staging |
| `sf-unpkg-src.bytedance.net` | 内部 npm 组件注册源 |

## 技术栈指纹

| 组件 | 字节使用 |
|------|---------|
| Web Server | Tengine / TLB |
| CDN | volc-dcdn / bytednsdoc |
| SSR 框架 | Goofy Node (字节自研) |
| UI 库 | Arco Design |
| 前端框架 | Modern.js (火山), Hera (飞书) |
| HTTP 客户端 | @edenx/plugin-fetch (axios 定制) |
| 监控 | Slardar |
| 埋点 | Rangers SDK |
| OIDC 实现 | 字节自研 (TLB + Go) |

## 推荐攻击链优先级

1. **SSO OIDC** — 找有效 client_id → 利用暴露的管理 scope (ListUsers/DeleteUser)
2. **ngcc.bytedance.com** — 内部 NGCC 服务，未探索
3. **partner 控制台** — console.partner.volcengine.com 认证可能更弱
4. **大小写 WAF 绕过** — `/API/v1/` + 有效 session → 可能访问到实际 API
5. **飞书 Staging** — docs.feishu-staging.cn 可能配置更弱
