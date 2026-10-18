# Playbooks 总目录

> 每个 playbook 提供完整的探测→绕过→利用流程，适用于黑盒/灰盒渗透测试

---

## 阅读建议

按**攻击链深度和突破价值**排序，优先测试能直接获取控制权的入口：

| 优先级 | Playbook | 渗透价值 |
|-------|---------|---------|
| 🔴 P0 | `unauth-access.md` | 默认凭据 / Redis-Mongo-ES / Actuator / Swagger / .git——低成本直接进入 |
| 🔴 P0 | `rce/00-index.md` | Log4Shell / Spring4Shell / Fastjson / Shiro / Jackson / Struts2 / WebLogic / 命令注入——直接 getshell（子目录: framework / cmd-inj / deserialize / file-chain / ssti / xxe / supply / proto-pollution） |
| 🔴 P0 | `file-upload/00-index.md` | 解析漏洞 + 编辑器漏洞 + 截断绕过 + Java上传框架指纹——落地 webshell |
| 🔴 P0 | `path-traversal/00-index.md` | `../etc/passwd` 加 6 种编码 + WEB-INF / web.config / Spring Boot配置——读取配置/凭据 |
| 🟠 P1 | `info-disclosure.md` | .git / .svn / 备份文件 / phpinfo / 日志 / OSS bucket——获取源码和凭据 |
| 🟠 P1 | `logic-flaws/00-index.md` | 密码重置 4 模式 / IDOR / 越权 / 验证码 / 支付——接管账户或操纵业务 |
| 🟠 P1 | `arbitrary-x-authz.md` | 任意 X 子授权（任意账号 86.4% / 任意操作 72.5%）——越权控制 |
| 🟠 P1 | `oauth-saml-jwt/00-index.md` | redirect_uri / state / JWT alg / kid / SAML 包裹 / Token 预测 / Shiro&Spring Security URI绕过——认证绕过 |
| 🟠 P1 | `sqli.md` | 27,732 个真实案例提炼，含高频参数频率表——数据泄露/提权到 RCE |
| 🟠 P1 | `ssrf-cache-host/00-index.md` | 内网探测 + 云元数据 + Host header / 缓存投毒——内网跳板 |
| 🟡 P2 | `api-rest/00-index.md` | BOLA / Mass Assignment / 速率 / CORS / HPP / API 语义 Fuzz——API 层面突破 |
| 🟡 P2 | `graphql.md` | Introspection / 嵌套 IDOR / DoS——GraphQL 特有攻击面 |
| 🟡 P2 | `race-conditions.md` | 优惠券双花 / 余额超扣 / 限额绕过——业务逻辑利用 |
| 🟡 P2 | `logic-flaws/11-business-logic.md` | 整数溢出 / 订阅混淆 / 状态操纵 / Zip 炸弹——深层业务逻辑 |
| 🟡 P2 | `xss/00-index.md` | 7,532 真实案例，上下文绕过表——窃取凭据/辅助攻击链 |
| 🟡 P2 | `http-smuggling.md` | CL.TE / TE.CL / H2→H1 / CRLF 注入——协议层绕过 |
| 🟡 P2 | `mobile.md` | 安卓导出组件 / Intent / WebView / Pinning / iOS Objection / Frida / Biometric——移动端突破口 |
| 🟡 P2 | `llm-prompt-injection/00-index.md` | Prompt 注入 / RAG 投毒 / Agent 工具 / OWASP ASI / CoT注入 / 跨域攻击链 / AI基础设施——AI 系统渗透 |
| 🔴 P0 | `intranet-postexp/00-index.md` | 内网渗透 / 横向移动 / 域控攻击——**Phase 4 后渗透核心** |

---

## 每个 playbook 的统一结构

```
1. 一句话说清是什么 + 渗透测试中的价值定位
2. 高频入口点（参数名 / 路径 / Header），引用统计数字
3. 探测手法（Probe）—— payload + 响应特征 + 带外/延时/差异
4. Bypass 矩阵（编码、混淆、WAF、过滤器绕过）
5. 利用提权 / 横向（从触发到攻击链升级）
6. 真实案例指纹（CVE/wooyun ID + 1 句版本特征 + 检测 payload）
7. 复现/证据要点（HTTP 包、CVSS 关键 vector、影响段写法）
8. 不要做的事（合规边界 / 数据保护）
```

---

## 与execution-framework的关系

```
strategy/01-attack-priority   →  决定先打哪个 playbook
strategy/04-control-gap        →  端点归类 → 翻到对应 playbook
strategy/02-bypass-toolkit     →  payload 被拦时的通用绕过
strategy/03-evidence            →  写报告前的证据纪律
playbooks/<type>.md                →  具体类型的探测/利用细节
templates/report-pentest.md        →  渗透测试报告模板
```
