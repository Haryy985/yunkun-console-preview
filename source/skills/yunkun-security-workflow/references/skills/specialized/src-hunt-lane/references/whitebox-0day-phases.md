# 白盒 0day 审计流程（researcher-blackbox-whitebox 提炼）

> 源：clown researcher-blackbox-whitebox.md。用户给出项目路径/源码时走本流程；黑盒 SRC 走 SKILL.md §3。

## Phase 0：优先级排序

| 优先 | 目标 | 为什么 |
|------|------|--------|
| 1 | 处理不可信输入的边界代码 | 直接攻击面 |
| 2 | 自研业务逻辑（非框架代码） | 认知盲区最多 |
| 3 | 历史 CVE 相邻代码（读修复 commit → 同模块） | 同类错误会重复 |
| 4 | 新功能/新 API | 测试不充分 |

## Phase 1：建立心智模型（不要跳过）

必须搞清：① 信任边界（哪些输入不可信）② 数据进入→处理→存储完整路径 ③ 权限模型与检查点 ④ 开发者安全假设（往往可破）⑤ 自研 vs 依赖边界。
执行：入口 → 路由 → 中间件链 → 安全配置。

## Phase 2：变体分析（高效 0day）

1. 查历史 CVE → 2. 读修复 commit 理解**根因** → 3. 提取模式全库搜同模式 → 4. 查修复是否只堵一个入口。

## Phase 3：Sink 逆向追踪

列危险 Sink → 逆向参数来源 → 标记过滤点 → 评估绕过。

## Phase 4：框架专项

- PHP/Laravel：Mass Assignment、ORM 注入、反序列化、类型杂耍
- Java/Spring：SpEL、Actuator、反序列化、权限注解缺失
- Python/Django：ORM 注入（含 Q `_connector`）、SSTI、Pickle
- Node.js：原型链污染、NoSQL、ReDoS、JWT

## Phase 5：深层

竞态（读→判→写无锁）、业务逻辑/支付/状态机、配置与硬编码密钥、依赖 CVE、Git 中被 revert 的安全修复。

## Phase 6：验证

每个发现必须回答：输入是否可控、过滤能否绕、前置条件、影响、能否出 PoC。

## 输出格式（白盒过程报告）

```
### [严重/高危/中危] 漏洞标题
位置：文件:行号
类型：SQLi / RCE / IDOR / ...
可利用性：Yes / No / Conditional
代码：[关键片段]
数据流：[Source] → [传播] → [Sink]
利用条件：权限/配置、绕过了什么
PoC：[curl 或代码]
影响：[最坏情况]
修复：[一句话]
```

白盒过程可内部记低价值点用于判断路线，**默认不写成正式 SRC 报告**（除非用户明确要求完整审计清单）。黑盒正式交付只认 `src-report-paradigm`。

## 输出规范（强制）

必须：结论明确（禁「可能存在风险」糊弄）、可直接复现的验证方法、判断依据与证据链、写清边界与限制、区分「确认可利用」与「需进一步验证」。
禁止：空泛科普、无验证猜测、漏关键复现步骤、误报当洞（宁可漏报不误报）。

## 漏洞链思维（黑盒白盒通用）

- 信息泄露 + SSRF → 内网
- 低权 XSS + CSRF → 接管
- 任意文件读 + 配置泄露 → RCE
- 越权 + 敏感操作 → 数据/资金

白盒审计路由到 clown21 专项 skill：`java-audit-pipeline`/`php-audit-pipeline`/`java-route-mapper`/`php-route-mapper` 等全链路审计工具。
