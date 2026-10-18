# Shannon Hybrid Scanning

> 视角：把白盒代码线索变成黑盒/灰盒探针。它不是“扫到就报”，而是用源码、前端资源、接口文档和运行时流量缩小测试面，再用最小请求证明真实边界是否被突破。

---

## 1. 一句话原则

**Source hints guide probes; runtime evidence decides.**

源码、JS、OpenAPI、日志、模板、注释都只能提供线索。最终是否成洞，必须回到运行时证据：

1. 当前请求确实命中了该路由。
2. 当前 actor 的权限/对象/状态边界被突破。
3. 响应或状态变化能稳定复现。
4. 影响通过 `reality-gates.md` 后才升级为 finding。

如果只证明“代码里看起来危险”或“payload 被反射”，最多记为 `lead` 或 `primitive-confirmed`。

---

## 2. 何时读取本方法

出现以下任一信号时读本文件：

- 有源码、前端 JS、source map、OpenAPI/Swagger、移动端包、模板、配置或日志可分析。
- 黑盒扫到了很多路径但不知道优先测哪条。
- 目标是 SPA/API，接口藏在 JS、GraphQL query、WebSocket 或移动端流量里。
- 想把 `JWT / GraphQL / HPP / CRLF / OOB / subdomain takeover / cloud bucket` 等漏洞面整理成自动化 checklist。
- 需要把不同场景打包成 OWASP、Bug bounty、Internal、CI/CD、CTF 等扫描 playbook。

---

## 3. 工作流

```text
collect source artifacts
-> extract API / params / roles / state / sinks
-> rank probes by control gap
-> send one safe runtime probe
-> compare baseline / variant / negative control
-> reality gate
-> promote to playbook branch or discard
```

每一轮只改变一个变量：一个参数、一个 header、一个 object id、一个 role 字段或一个 state 字段。

---

## 4. 白盒线索怎么抽

### 4.1 API endpoint extraction

从源码/JS/文档里抽：

- route：`GET /api/users/{id}`、`POST /order/create`
- method：GET/POST/PUT/PATCH/DELETE/WS/GraphQL mutation
- auth hint：`Authorization`、cookie、`X-User-*`、JWT、sessionStorage/localStorage
- object key：`id`、`uid`、`orderId`、`tenantId`、`fileId`
- state key：`status`、`step`、`paid`、`approved`、`verified`
- money/amount key：`price`、`amount`、`balance`、`couponCode`
- file/url key：`file`、`path`、`url`、`redirect`、`callback`

输出时不要只列路径，要写“为什么值得测”：

```text
endpoint: PATCH /api/users/{id}
shape: list-detail-crud / data modification
boundary to test: object ownership + mass assignment
first probe: A token modifies B id, no extra field changes
negative control: random non-existing id should fail
```

### 4.2 Source to sink tracing

轻量追踪即可，不要幻想完整静态分析。重点找：

| Source | Sink | 优先验证 |
|---|---|---|
| query/body/path param | SQL execute/query | SQLi / error diff / timing diff |
| URL/file/path param | fetch/read/open/include | SSRF / file read / path traversal |
| user text/comment/title | HTML render/innerHTML/template | XSS / stored reflection |
| JSON body | model auto binding | Mass assignment |
| role/userId/header/cookie | auth middleware | auth bypass / trust boundary |
| upload filename/content | storage/parser/extractor | upload chain / archive traversal |

把结果写成候选分支，而不是直接写 finding：

```text
branch: url-param-to-server-fetch
evidence: parameter `callbackUrl` reaches backend fetch helper in current source
probe: replace callbackUrl with controlled domain once
gate: OOB hit + response/state evidence required
```

### 4.3 Context-aware payload generation

payload 不要一把梭，按变量语义选最小探针：

- `id / uid / orderId`：先做对象替换和 negative id，不先做 SQLi。
- `url / callback / redirect`：先验证是否发起外连，再验证协议/重定向/内网边界。
- `file / path / filename`：先验证目录归一化和对象权限，再验证读取影响。
- `price / amount / balance`：先验证服务端是否重算，不直接进入真实支付。
- `role / isAdmin / status`：先验证字段是否被接受，再验证权限是否真的变化。
- `q / search / content`：先定位输出上下文，再按 HTML/attribute/JS/URL context 选 XSS payload。

---

## 5. 漏洞面清单怎么变成 checklist

这些模块适合做“不要漏测”的提醒，不适合直接当结论引擎。

| 面 | 线索 | 最小验证 | 升级条件 |
|---|---|---|---|
| JWT | `Authorization: Bearer eyJ...`、响应体/URL 里出现 JWT | 解 header/payload，看 alg、kid、exp、敏感字段 | 服务端接受伪造/篡改 token，或弱密钥可签出有效 token |
| GraphQL | `/graphql`、`query`、`mutation`、`__typename` | introspection、query/mutation inventory | 发现未授权字段/嵌套 IDOR/敏感 mutation 可执行 |
| HPP | 同名参数、数组参数、`_method`、`role`、`price` | baseline vs duplicate param diff | 参数污染改变鉴权、金额、对象或后端语义 |
| CRLF | redirect、download、filename、header reflection | 单个 CRLF probe，看 header/body 是否真实拆分 | 可注入 header/cookie/location，或形成缓存/会话影响 |
| OOB blind | url/fetch/import/webhook/template/cmd | 一次受控 OOB 域名探测 | OOB 命中且可关联请求来源、参数、触发路径 |
| Subdomain takeover | CNAME 指向第三方平台、404 takeover 指纹 | DNS + HTTP 指纹确认 | 可注册/声明资源并控制原子域名内容 |
| Cloud bucket | bucket 命名、OSS/S3/GCS/Azure URL | 只读 list/object HEAD | 公开读取敏感对象，或可写入/覆盖对象 |

默认输出层级：

- 只有指纹：`observation-only`
- 能触发差异：`primitive-confirmed`
- 跨边界读/写/控制：`impact-confirmed`

---

## 6. Playbook 配置思路

把扫描项按场景组合，而不是所有检查全开。

### 6.1 OWASP baseline

用于常规体检，覆盖面广但不要深爆破。

- auth/access control：unauth、IDOR、CSRF、CORS
- injection：SQLi、XSS、SSRF、XXE、deserialization
- misconfig：headers、methods、OpenAPI、DVCS leak
- crypto/session：JWT、cookie、TLS basics

适合 Phase 2b/2c 生成地图，不适合直接出高危结论。

### 6.2 Bug bounty / SRC high ROI

优先高价值、低噪声：

- BOLA/IDOR：list/detail/export/batch/object id
- auth/reset：OTP、reset token、session/JWT
- business logic：price、coupon、balance、quota、approval
- SSRF/file：url/fetch/import/download/path
- source/secret：`.git`、backup、config、AK/SK

配合 `reality-gates.md` 和 `pentest-severity` 使用，过滤安全头、普通版本号、无影响反射等低价值项。

### 6.3 Internal / intranet

授权内网场景才启用：

- service exposure：Redis/Mongo/ES/Actuator/Jenkins/GitLab
- default credentials：只在范围允许时低频验证
- pivot candidates：proxy、SSRF、file read、config leak
- lateral readiness：credential usability、network reachability

必须先过 scope gate，不把外部 public target 自动扩展成内网授权。

### 6.4 CI/CD / source audit

代码仓库、制品、流水线场景：

- secrets：cloud keys、tokens、webhook secrets、JWT secrets
- dependency/config：lockfile、Dockerfile、compose、K8s yaml
- artifact exposure：source map、build output、backup package
- pipeline abuse：runner token、deploy key、artifact permission

发现凭据后必须验证有效性。401/invalid/expired 直接降级，不写成高危。

### 6.5 CTF / sandbox helper

CTF 中可以更积极地把线索转成解题路径：

- route mining：JS/source map/OpenAPI/hidden endpoints
- transform chain：encoding、crypto、serializer、template
- challenge state：flag storage、admin-only branch、bot review flow
- controlled exploit：优先可复现、最小状态改动

仍然遵守：运行时证据优先于注释和 README。

---

## 7. 与 clown21 其他文件的衔接

| 当前线索 | 下一步读 |
|---|---|
| 不知道先测哪条接口 | `04-control-gap-hunting.md` |
| 需要排序攻击路径 | `01-attack-priority.md`、`06-attack-chain-derivation.md` |
| API/GraphQL/JWT 明显 | `../playbooks/api-rest/00-index.md`、`../playbooks/oauth-saml-jwt/00-index.md` |
| 业务金额/状态/重置流程 | `../playbooks/logic-flaws/00-index.md` |
| SSRF/cache/Host/OOB | `../playbooks/ssrf-cache-host/00-index.md` |
| 文件上传/下载/路径 | `../playbooks/file-upload/00-index.md`、`../playbooks/path-traversal/00-index.md` |
| 准备升洞/写报告 | `../reality-gates.md`、`03-evidence-discipline.md` |

---

## 8. 禁止事项

- 不把源码里的危险函数直接写成漏洞。
- 不把 scanner 的 finding 直接搬进报告。
- 不用无 baseline 的时间差、长度差、状态码差直接定高危。
- 不在没有授权的范围里做 takeover 声明、云桶写入、爆破、DoS、真实支付。
- 不因为 checklist 很长就批量打高噪声探针；每次先建模，再发最小请求。

---

## 9. 最小记录模板

```text
source hint:
  artifact:
  route / param / sink:
  confidence:

runtime baseline:
  actor:
  request:
  response:

single variable changed:
  changed field:
  expected safe behavior:
  observed behavior:

gate:
  auth boundary:
  object boundary:
  state boundary:
  data sensitivity:
  level: lead | primitive-confirmed | conditional-impact | impact-confirmed

next playbook:
  file:
  reason:
```

