# knowledge-base索引（src-hunt-lane 实战杀链库）

> 来源：clown-src-6k-skill knowledge-base（实战验证版）。进站先读 `shortlist.md`（87 行杀链索引：认什么→打哪→出什么算成→假点），对得上再打开对应模块看细节。
> 规则映射：原 `~/.grok/rules/` 已移植——挖什么/力气分配 → 本 lane `references/value-matrix.md`；范围/节奏 → `references/scope-seed-loop.md` + `anti-idle-gates.md`；写不写报告 → `src-report-paradigm` skill（clown21）。
> 与 clown21 execution-framework skill 的关系：本目录是**实例级杀链**（具体形态的具体打法），clown21 专项 skill 是**execution-framework级**；SRC 挖洞先查本目录，pentest 上下文优先 clown21 专项 skill。

## 使用约定

- 进站先读 `shortlist.md`；对得上再打开对应模块看细节。文件不长就整篇开；超长篇可先开点名节，不够就继续开。禁止每站通读本目录
- 磁盘有 `*src经验.md` 才开专篇，没有不算缺。开 `SKILL.md` 不会再带集团日记
- 短表和「注入/SSRF/XSS/RCE」都不是上限。本站过全类型矩阵；四件套打在有差分面上（防空窗），不是只测这四类，也不是每个 path 喷 `'`。有会话时越权/逻辑与四件套同硬（site-entry-playbook §4.2.3）
- 方便和能力优先；省 token 是顺带，不挡开模块
- 短表点名的手法用标题搜。有指针的肥篇只留实战中文 + 指针段；禁开/几乎不交的篇已收成一行。手法行不删、算成不改矮
- 篇内跳转已改成本目录真实文件（`idor-test.md` 一类）；不要再跟 `../xxx/SKILL.md`
- 冲突裁定：挖什么 → value-matrix.md；CORS 不挖（SRC lane 内）；写不写 → src-report-paradigm
- **`cors-test.md` / `llm-security-test.md`：不挖/禁开越狱。** `401-403-bypass.md` 不磨登录 HTML。
- 正式 SRC 报告：`src-report-paradigm` skill（clown21 skills 目录）

## 文件清单
| uoyi-getconfig-unauth.md | RuoYi 系 getConfig 未授权配置通杀（configKey 字典 + 微信 40164 实锤） |
| waf-bypass-mssql-blind.md | WAF 黑名单下 MSSQL 盲注提取框架（STR/CHARINDEX/RIGHT/LEFT + 封锁画像） |
| shx-unauth-combo.md | ASP.NET ashx 未授权组合面（DTcms 指纹 + 摆设参数差分法） |

| 文件 | 说明 |
|------|------|
| `shortlist.md` | 挖洞手法索引（一行/指针；正文仍在各模块） |
| `401-403-bypass.md` | **禁开磨登录 HTML**（已收成一行）；业务 API 401 现场自己打 |
| `api-gateway-test.md` | API 网关 |
| `agent-tool-exec-test.md` | 对话口工具真执行（不是越狱、不是云 IDE RPC） |
| `authbypass-test.md` | 认证绕过（未登录改密/IDaaS + 短表指针；英文字典已砍） |
| `cache-poisoning-test.md` | 缓存投毒/欺骗（原有+补充） |
| `clickjacking-test.md` | 缺头不写（已收成一行） |
| `cloud-ide-codex-rce-chain.md` | 云 IDE/Codex 系：弱口令→RPC RCE→集群/API Key 链（短表有指针） |
| `cors-test.md` | **不挖勿开**（已收成一行） |
| `crlf-injection-test.md` | 几乎不交（已收成一行） |
| `csp-bypass-test.md` | 几乎不交（已收成一行）；XSS 走 `xss-test.md` |
| `csrf-test.md` | 专题知识（clown21-specialized-library 导入或融合） |
| `csv-formula-injection-test.md` | 几乎不交（已收成一行） |
| `dangling-markup-test.md` | 几乎不交（已收成一行） |
| `dependency-confusion-test.md` | 几乎不交（已收成一行） |
| `deserialization-test.md` | 专题知识（clown21-specialized-library 导入或融合） |
| `dns-rebinding-test.md` | 几乎不交（已收成一行）；SSRF 走 `ssrf-test.md` |
| `el-injection-test.md` | 专题知识（clown21-specialized-library 导入或融合） |
| `email-header-injection-test.md` | 几乎不交（已收成一行） |
| `file-upload-test.md` | 文件上传（STS/列桶/分享鉴权等指针） |
| `ghost-bits-cast-test.md` | Ghost Bits 原理+常用字+公式；逐字节两套表已砍 |
| `graphql-test.md` | GraphQL（原有+补充） |
| `hpp-test.md` | 几乎不交（已收成一行） |
| `http-host-header-test.md` | 专题知识（clown21-specialized-library 导入或融合） |
| `http-smuggling-test.md` | 请求走私（原有+补充） |
| `http2-attacks-test.md` | 几乎不交（已收成一行）；走私走 `http-smuggling-test.md` |
| `idor-test.md` | 越权（中文主线 + 短表指针） |
| `info-leak-test.md` | 信息泄露 |
| `injection-test.md` | 注入（OR+total / 邮件订阅 iframe / SSTI 探测；英文百科已砍） |
| `insecure-scm-test.md` | 专题知识（clown21-specialized-library 导入或融合） |
| `jndi-injection-test.md` | 专题知识（clown21-specialized-library 导入或融合） |
| `js-reverse-guide.md` | JS 逆向 |
| `llm-security-test.md` | **禁开越狱教材**（已收成一行）；对话工具走 `agent-tool-exec-test.md` |
| `logic-test.md` | 业务逻辑（支付/流程 + 商家促销绑定） |
| `oauth-jwt-test.md` | OAuth/JWT/SAML/OIDC（原有+多源补充） |
| `open-redirect-test.md` | 专题知识（clown21-specialized-library 导入或融合） |
| `path-traversal-lfi-test.md` | 专题知识（clown21-specialized-library 导入或融合） |
| `prototype-pollution-test.md` | 专题知识（clown21-specialized-library 导入或融合） |
| `race-condition-test.md` | 竞态（原有+补充） |
| `recon-methodology.md` | 侦察execution-framework |
| `ssrf-test.md` | SSRF（IMDS 路径差 / GOPROXY / 对象存储回源） |
| `subdomain-takeover-test.md` | 专题知识（clown21-specialized-library 导入或融合） |
| `type-juggling-test.md` | 专题知识（clown21-specialized-library 导入或融合） |
| `waf-bypass.md` | WAF 绕过 |
| `websocket-test.md` | WebSocket（原有+补充） |
| `xslt-injection-test.md` | 几乎不交（已收成一行） |
| `xss-test.md` | XSS（中文开场 + 冷门事件 + XSS→RCE / 自定义协议） |
| `xxe-test.md` | 专题知识（clown21-specialized-library 导入或融合） |

**合计：48 个知识文件**（不含本 README）。SRC 报告版式不在本库：见 `~/.grok/rules/vuln-report-format.md`。定级只认 format，本库不定级。
