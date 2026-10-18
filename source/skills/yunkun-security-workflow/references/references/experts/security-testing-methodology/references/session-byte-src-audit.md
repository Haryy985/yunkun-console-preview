# 字节跳动 SRC 漏洞证据审计案例

**日期**: 2026-07-12  
**审计框架版本**: security-testing-methodology 正文「SRC 漏洞证据审计框架」  
**结论**: 四个候选全部 NO-GO，未发现安全边界突破。

---

## 候选 A：NGCC 公网暴露

### 证据矩阵

| 字段 | 内容 |
|:-----|:------|
| 资产 | `https://ngcc.bytedance.com/login?aid=2488` |
| SRC 范围状态 | 无法确认 |
| 安全边界 | 登录认证 |
| 已确认事实 | 登录页 200 OK (1628B 纯静态 HTML)；JS bundle (888KB) 含 0 个 API 端点；`window.$employee` 全为空值；无硬编码凭据；CDN 域名 `ocic-static.com`；Passport 接口可区分有效/无效 app_id |
| 漏洞假设 | 内部应用公网暴露可导致未授权访问 |
| 对照组 | 正常→需登录；实际→静态登录页 |
| 最小验证步骤 | GET `/login?aid=2488` → 检查响应；分析 JS bundle；POST passport 对照 |
| 实际结果 | 静态页面，无业务数据，Passport = error_code 16/22 通用文本 |
| 预期结果 | 未登录用户应只能看到登录页 |
| 安全影响 | 无 |
| 是否可提交 | NO-GO |
| 缺失证据 | 需要有效登录凭证测试登录后权限 |
| 停止条件 | 确认静态页面无数据泄露 |

### 对照测试结果

```python
("有效 app_id=2488",     {"app_id": "2488"})  → error_code=16,  "该应用无权限"
("随机无效",           {"app_id": "xxxx"})    → error_code=22,  "非法应用"
("空参数",             {})                    → error_code=22,  "非法应用"
```
差值仅为通用错误文本 → 无损枚举，影响极低。

---

## 候选 B：OIDC Scope 配置

### 证据矩阵

| 字段 | 内容 |
|:-----|:------|
| 资产 | `https://sso.bytedance.com/.well-known/openid-configuration` |
| SRC 范围状态 | 无法确认 |
| 安全边界 | 客户端注册 + 客户端认证 |
| 已确认事实 | OIDC discovery 公开；scopes_supported 含 `*`, `DeleteUser`, `ListUsers`, `CreateUser` 等；JWKS 2 个 RSA 密钥；所有未注册 client_id 均被拒绝 (400)；Feishu app_id 无效；token 端点需 client_auth |
| 漏洞假设 | 高权限 Scope 可被未授权客户端获得 |
| 对照组 | 安全设计→仅授权客户端；实际→所有未注册客户端被拒绝 |
| 实际结果 | `400 invalid_client` — 无客户端可请求 |
| 安全影响 | 无 |
| 是否可提交 | NO-GO |
| 缺失证据 | 需要有效 client_id+secret 才能验证授权 |
| 停止条件 | 所有已知和推测 client_id 均被拒绝 |

---

## 候选 C：WAF 大写路径差异

### 证据矩阵

| 字段 | 内容 |
|:-----|:------|
| 资产 | `https://console.volcengine.com/api/v1/` |
| SRC 范围状态 | 明确在范围 |
| 安全边界 | WAF + 登录认证 |
| 已确认事实 | `/api/v1/` → 403；`/API/v1/` → 302 登录跳转；大写路径下所有 API 请求均返回 302 登录跳转 |
| 漏洞假设 | 大写路径绕过 WAF 后访问受保护资源 |
| 对照组 | 小写→403 (WAF)；大写→302 (SSR 要求登录) |
| 实际结果 | 大写路径未访问到任何受保护资源 |
| 安全影响 | 无 |
| 是否可提交 | NO-GO |
| 缺失证据 | 需要有效 session 验证大写路径下的 API 功能 |
| 停止条件 | 所有大写变体返回 302 登录跳转 |

---

## 候选 D：Passport app_id 枚举

### 证据矩阵

| 字段 | 内容 |
|:-----|:------|
| 资产 | `https://ngcc.bytedance.com/passport/auth` |
| SRC 范围状态 | 无法确认 |
| 安全边界 | 应用认证 |
| 已确认事实 | 有效 app_id=2488 → error_code=16；无效 → error_code=22；均为通用文本 |
| 漏洞假设 | error_code 差异可枚举有效 app_id |
| 对照组 | 有效 vs 无效 vs 空参数 |
| 实际结果 | 可区分但无敏感信息泄露 |
| 安全影响 | 极低 |
| 是否可提交 | NO-GO |
| 缺失证据 | 需要证明该枚举可导致 OAuth 攻击或授权绕过 |
| 停止条件 | 仅做对照测试，不批量枚举 |

---

## 关联案例

本案例与 DeerFlow 开源项目路径审计（`open-source-security-audit-methodology → references/deerflow-path-audit.md`）互补：
- **本案例**: Web 资产 SRC 审计 — 面向公网服务，侧重状态码/响应内容/配置差异的对照验证
- **DeerFlow 案例**: 开源项目白盒审计 — 面向本地源码，侧重调用链追踪/TOCTOU 分析/Python symlink 语义核查/防御层叠加验证
