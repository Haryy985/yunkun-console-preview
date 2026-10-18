---
name: security-testing-methodology
description: "Use when planning systematic security testing (L1-L4 thinking pyramid, WooYun formula, OWASP/WSTG mapping)."
auto_load: true
status: active
category: methodology
tags: [security-testing, methodology, src, web, ai]
version: 1.0.0
related_skills: [pentest-master-entry, hypothesis-falsification-and-pivot, pwn-exploitation, php-filter-chain-exploitation, response-style-clean-chinese]
---

# 安全测试execution-framework

## 一、触发条件（When to Use）

- 需要为 Web/API/移动端/云服务/AI Agent/LLM 制定**系统化安全测试计划**，而非单点 payload
- 需要 L1-L4 金字塔、WooYun 公式、GAARM 矩阵、OWASP LLM/ASI/WSTG 映射指导规划
- 准备提交 SRC，需执行**提交前证据审计**（候选分类/证据矩阵/对照测试/严重程度判定）
- 遇到带权限控制的 **Flask 应用**，需按业务链优先级组织测试
- 持有产品级 POC（通杀包），需在**批量资产**中验证漏洞存在性
- 需区分结论可信度（已确认事实/合理推测/尚未验证），执行认知类型系统

## 二、适用范围与不适用范围（Applicability）

- **覆盖**：Web 与 AI 应用全链路系统化测试；SRC 提交前证据审计与收敛；Flask 业务链测试；批量 POC 验证；认知类型系统与实验设计。
- **框架映射**：OWASP LLM Top 10 (2025)、OWASP ASI Top 10 (2026)、OWASP WSTG。
- **不覆盖**：单点 payload 构造与具体漏洞利用 → `pwn-exploitation`、`php-filter-chain-exploitation`、`web-exploit-werkzeug-debugger` 等叶技能；特定平台/靶场操作；单一实例题解（→ `references/cases.md`）；报告话术 → `response-style-clean-chinese`。

## 三、决策树（Decision Tree）

### L1-L4 思维层级选择

```
L4: 防御反推  ← 从补丁/过滤规则/安全机制反推绕过
L3: 边界探索  ← 在已知攻击面上寻找corner case
L2: 假设验证  ← 构建推理链条，逐步验证假设
L1: 攻击面识别 ← 寻找数据与指令不分离的接口
```

- 攻击面未摸清 → L1；有推理链未验证 → L2；已知攻击面找角点 → L3；有补丁/过滤/安全机制 → L4 防御反推

**跨领域核心公式**（漏洞判定依据）：

| 领域 | 公式 |
|------|------|
| 通用 | 漏洞 = 边界失控 + 状态不一致 + 信任假设违背 |
| 代码审计 | 漏洞 = Source可达Sink && 无有效Sanitizer |
| 二进制 | 利用 = 信息泄露 + 原语构造 + 控制流劫持 |
| AI应用 | 漏洞 = Prompt可控 + 输出无过滤 + 工具权限过大 |

**六大元思考原则**：假设-验证循环 / 边界条件思维 / 防御反推 / 链式思维 / 版本敏感 / 语义差异。

### WooYun 漏洞本质判定

```
漏洞 = 预期行为 - 实际行为 = 开发者假设 ⊕ 攻击者输入 → 意外状态
```

核心问题链：数据从哪来（输入源）→ 到哪去（数据流）→ 在哪被信任（信任边界）→ 如何被处理（处理逻辑）→ 处理后去哪（输出点）。攻击面三层模型：输入层 → 处理层 → 输出层。

### GAARM 风险矩阵（AI 安全域覆盖）

覆盖 6 个 AI 安全域（AI 应用/模型/数据/身份/基座/合规治理）× 3 阶段（训练/部署/应用）的威胁面枚举。**完整矩阵表**：`references/gaarm-and-flask-chain-priority.md`。

### 统一安全测试决策循环（详见 §四）

```
1.目标分析 → 2.信息收集 → 3.漏洞假设 → 4.验证利用 → 5.报告迭代 →(回 1)
```

### Flask 业务链优先级与降级信号

核心原则：不平均用力，按攻击链可能性排序；主路不通立即 pivot。优先找「不需要 is_admin=true 也能碰到 admin 数据」的业务链，而非纯权限提升。

9 条攻击链按序测试：① PDF/下载/导出 IDOR → ② ReportLab/PDF 生成链 → ③ 模板 owner 反向污染 → ④ 草稿状态绕过 + Method Override → ⑤ 用户路由 IDOR → ⑥ 辅助路由 IDOR → ⑦ 弱口令枚举 → ⑧ 源码/配置/数据库泄露 → ⑨ owner_user_id 类型异常。每条链含关键测试点与放弃信号，**完整明细表**：`references/gaarm-and-flask-chain-priority.md`。

**关键思维转折信号**（>15 分钟无进展时自检）：① 在找"拿 admin"还是"不拿 admin 也能碰数据"？后者优先；② 该能力有无未试过的副作用（如列表/导出按 owner 关联泄露）？③ 是否只测主路由漏了副功能？先测所有 `/{resource}/{id}/{action}` 组合。

### 批量 POC 分层决策（Tiered Approach）

| 层 | 类型 | 优先级 | 风险等级 | 说明 |
|----|------|--------|---------|------|
| Tier 1 | 信息泄露 | 1st | 低 | 无副作用探路，确认漏洞存在 |
| Tier 2 | 文件读取 | 1st | 中-高 | 读配置/凭据确认影响范围 |
| Tier 3 | RCE探测 | 2nd | 高 | fastjson/反序列化/上传，先 dnslog |
| Tier 4 | SQL注入 | 3rd | 中 | 保底漏洞，确认注入点 |

## 四、步骤（Steps）

### 统一安全测试决策循环

1. **目标分析**：确定目标类型（Web/API/移动端/云/AI Agent/LLM）、入口点（URL/接口/客户端/配置文件/Prompt）、测试范围（功能/角色/数据流）。**完成标准**：三项齐备。
2. **信息收集**：被动（响应头/错误页/源码注释/JS → 技术栈/版本/路径）；主动（目录/端口/版本探测 → 攻击面）；引擎（Google Hacking/FOFA/Hunter → 泄露资产）。**完成标准**：攻击面清单产出。
3. **漏洞假设**：按攻击面构建假设列表，分类（Web/业务逻辑/AI/部署配置），高频入口（登录/SQL/文件操作）优先。**完成标准**：每条假设可验证、可推翻（HYP-xxx）。
4. **验证利用**：构造 Payload → 发送 → 判断响应；无回显用 OOB（DNSLog/HTTPLog）；确认后构造完整利用链。**完成标准**：每条假设有实验结果或明确排除。
5. **报告迭代**：记录漏洞类型/路径/Payload/影响；上一轮结果迭代为新一轮假设。**完成标准**：结论记录完整，新假设入列。

**流水线集成约定**：启动前检查 TARGET_WORKSPACE（未初始化先走 `pentest-master-entry`）；Step 1-3 产出写入 assets/、js/、models/；假设用 HYP-xxx 编号；验证写入 tests/；迭代对应流水线循环调度。

### 科学实验框架（假设验证）

假设验证前必须设计实验，而非直接打 payload：`competing_explanations`（竞争解释集）、`independent_variable`（唯一改变量）、`controlled_variables`（保持变量）、`expected_outcomes`（各解释预期结果）。实验成功 → 确定竞争解释之一；失败 → 排除至少一项；均算知识增量，禁止无假设的探索性请求。

### 批量 POC 验证 5 步漏斗

```
CSV资产表 → 关键词过滤 → 存活探针 → 并发POC扫描 → 深度验证
```

1. **资产筛选**：从目标 CSV 提取特定产品 URL，关键词优先级：产品名 > 厂商缩写 > 特征端口 > 功能子域名（计分：直接匹配+3，功能子域+2，缩写+1）；无关键词时按端口或 body 特征筛选。**完成标准**：URL 清单过滤完成。
2. **存活探针**：GET/HEAD（5s 超时），200/401/403 均算存活，检查 body 产品指纹，匹配才进入全量扫描（节省约 80% 时间）。**完成标准**：存活+指纹匹配目标列表产出。
3. **并发 POC 扫描**：ThreadPoolExecutor——本地 8-16 路、公网 4-8 路（避免 WAF/速率限制）、每 POC 间 1 秒延迟；禁止顺序扫描。**完成标准**：N×M 扫描完成，命中列表产出。
4. **分层验证**：按 Tier 1→4（信息泄露→文件读取→RCE→SQL），先低风险探路再高风险利用；POC 设计：无副作用优先（泄露>读取>盲注>RCE）、可重复、不依赖目标状态变化。**完成标准**：各层结果分级记录。
5. **深度验证**：命中目标全链路扫描（打开所有探测路径），测业务特性组合漏洞，利用信息泄露二次扩大攻击面。**完成标准**：利用链/影响范围确认。

**关键技术（Tomcat 语义差异，属 L4 防御反推）**：模式1 URL 编码路径穿越 `/<app>/service/.%2e/user/...` 等价 `/<app>/user/...`，绕过路径前缀鉴权；模式2 分号截断 `/<app>/login.jsp/..;/rest/...`，绕过 URL 后缀鉴权。本质：Tomcat 与鉴权中间件 URL 解析不一致。

## 五、坑位表（Pitfalls）

### POC 批量验证陷阱

| 症状 | 修复 |
|------|------|
| 用 Pipe 解析 CSV 只读到 0 条 | 先 `head -1` 看真实分隔符 |
| 顺序扫描超时（N 目标 × M POC 串行耗时爆炸） | ThreadPoolExecutor 并发：本地 8-16 路、公网 4-8 路 |
| 存活判定模糊，误报"无漏洞"但目标有 WAF | 先发健康检查确认 HTTP 200 + 产品指纹 |
| 文件读取返回 200 但内容是 404 页 | 检查 Content-Length 与 body 内容，勿误信单一 POC 结果 |

### Flask 链测试陷阱

| 症状 | 修复 |
|------|------|
| PDF 路由均 302 跳转即判"有鉴权" | 确认两路径是否到达同一应用、绕过后是否获得数据/功能，而非仅触发登录跳转 |
| 只测主路由，漏副功能 | 先测所有 `/{resource}/{id}/{action}` 组合 |
| 在普通路径上消耗超过 15 分钟无进展 | 自检：在找"拿 admin"还是"不拿 admin 也能碰数据"？后者优先 |

### SRC 审计陷阱

| 症状 | 修复 |
|------|------|
| 有效/无效参数返回相同错误即判"无枚举" | 设计对照组（有效/无效/空/无参数）后再判定 |
| 响应差异仅含通用文本即判信息泄露 | 差值含敏感数据才升级为泄露候选，否则为无损枚举，影响极低 |
| 依据品牌/系统名称提高漏洞等级 | 严格按证据矩阵与严重程度判定规则评级 |

## 六、验证清单（Verification）

### SRC 漏洞证据审计框架

提交前每个候选项必须走：**候选分类**（公网暴露/权限泄露/WAF绕过/信息枚举/状态码差异 五类）→ **证据矩阵**（13 字段：资产/SRC范围/安全边界/已确认事实/漏洞假设/对照组/最小验证步骤/实际结果/预期结果/安全影响/是否可提交/缺失证据/停止条件）→ **控制组对照测试** → **严重程度判定**（信息/低危、中危候选、高危候选三档）→ **三部分输出**（结论摘要 / 最强漏洞证据包 / 下一步只读任务）。

**完整明细表**（候选分类验证协议、13 字段矩阵、严重程度判定条件、输出结构模板）：`references/src-evidence-audit.md`。

### 认知类型系统（结论可信度验证）

L1-L4 金字塔之上是认知类型系统——规定每层信息应如何分类、升级和验证，防止结论跳过必要前置条件。完整展开见 `references/cognitive-type-system.md`。

六种强制区分的认知类型：**OBSERVATION**（原始现象，不可直接标记为漏洞）→ **FACT**（可靠证据支持，不可仅凭单次观察确认）→ **ASSUMPTION**（未验证前提，不可作结论支撑）→ **HYPOTHESIS**（可验证猜想，必须定义可推翻条件）→ **INFERENCE**（多事实推理，必须列出推理链）→ **CONCLUSION**（阶段性结论，需逻辑/证据/范围三重审查）。

升级路线：`OBSERVATION →(重复验证+多源确认)→ FACT →(与事实矛盾？设计实验)→ HYPOTHESIS/INFERENCE →(实验验证+三重审查)→ CONCLUSION →(一致→合并，不一致→矛盾记录)→ FINDING`。

**证据哈希链**：原始证据 → 分析 → 认知升级 → 三重 Reviewer → Finding；每步产生 SHA-256 哈希，最终生成 Merkle Root，任何证据变化导致验证失败。

### 勾选清单

- [ ] L1-L4 层级选择明确
- [ ] 决策循环五步完成标准逐项达成
- [ ] Flask 链按优先级执行并记录降级信号
- [ ] POC 漏斗五步完成、各层无副作用
- [ ] 每个 SRC 候选填完证据矩阵，结论区分已确认/推测/未验证
- [ ] 认知结论经逻辑/证据/范围三重审查

## 七、关联技能（Related）

- `pentest-master-entry`：前置初始化（先于本技能执行）
- `hypothesis-falsification-and-pivot`：假设验证与转向（互补）
- `pwn-exploitation`：二进制利用（互补，L1-L4 金字塔的二进制实例）
- `php-filter-chain-exploitation` / `web-exploit-werkzeug-debugger`：Web/Flask 具体技术（后于本技能执行）
- `response-style-clean-chinese`：报告表达规范（后于本技能执行）

## 八、案例（Cases）

- 实战案例索引（SRC 审计、批量 POC、证据收敛）：`references/cases.md`
- SRC 证据收敛与报告定稿流程/模板：`references/stm-九-证据收敛与报告定稿流程.md`
- SRC 四候选证据审计案例：`references/session-byte-src-audit.md`
