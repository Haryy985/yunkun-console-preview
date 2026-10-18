---
name: ai-identity-security
description: |
  AI 系统身份与权限安全测试execution-framework。当目标系统涉及 Agent 身份认证、多 Agent 权限管理、
  角色设定安全、会话管理、或 MCP/API 凭据管控时触发。
  覆盖: 角色逃逸(假定场景/假定角色/遗忘法/目标劫持)、权限失控(Action 越权/MCP 未授权资源获取)、
  多 Agent 身份伪造、会话劫持、凭据泄露与滥用。
metadata:
  tags: ai identity, role escape, 角色逃逸, agent impersonation, 身份伪造, action permission, 权限失控, session hijack, 会话劫持, credential abuse, mcp unauthorized, 凭据泄露, multi-agent identity, 越权访问
  category: ai-security
---

# AI 身份与权限安全测试execution-framework

## 概述

AI 身份安全关注的是**应用层角色设定与权限边界**的可靠性。与 prompt-jailbreak（针对模型安全对齐）不同，身份安全测试的目标是验证业务赋予 AI 的角色限制、权限分配、会话隔离和凭据管控是否可被绕过。当 Agent 获得工具调用和外部系统访问能力后，身份失控的影响从"说了不该说的话"升级为"做了不该做的事"。

## 攻击面总览

| 攻击类别 | 子攻击 | GAARM 编号 |
|----------|--------|-----------|
| 角色逃逸 | 假定场景 / 假定角色 / 遗忘法 / 目标劫持 | 0052.001-004 |
| 权限失控 | Action 模块越权 / MCP 未授权资源获取 / 账户越权 | 0053/0057/0058 |
| 身份伪造 | 会话令牌窃取 / Agent 身份伪装 / 信任链欺骗 | 0059 |
| 会话与凭据 | 会话劫持 / 缓存欺骗 / API Key 泄露 | 0055/0056 |

## 深入参考

- 各攻击类别的详细测试 Payload、攻击链与检测方法 → [references/identity-attack-techniques.md](references/identity-attack-techniques.md)

---

## Phase 0: 身份架构侦察

- 识别 Agent 认证方式（API Key / OAuth / Session Token / 无认证）
- 枚举权限模型（RBAC / ABAC / 无显式权限控制），确认角色设定的实现方式（system prompt / 代码逻辑 / 配置文件）
- 探测会话管理机制（会话 ID 生成方式、过期策略、跨会话隔离程度）

## Phase 1: 角色逃逸测试 (GAARM.0052)

- **假定场景逃逸**：构造混淆场景使模型偏离工作目标，结合关键字定位尝试泄露系统设定
- **假定角色逃逸**：要求模型扮演特定角色（DAN / 开发者模式），使其忽略应用层角色限制
- **遗忘法逃逸**：诱导模型"忘记"先前的角色指令，重置为无约束状态
- **目标劫持**：通过操纵使 AI 偏离原始角色设定去执行攻击者期望的任务

角色逃逸与越狱的边界：jailbreak 攻击模型的安全对齐（让模型生成有害内容），角色逃逸攻击应用的业务角色设定（让客服 Bot 执行管理操作）。两者可组合使用但测试目标不同。

## Phase 2: 权限失控测试 (GAARM.0053/0057/0058)

- **Action 模块越权**：通过 Prompt 注入或参数篡改，触发 Agent 执行超出授权范围的 Action（如将普通查询操作替换为管理操作）
- **MCP 未授权资源获取**：测试 MCP Server 权限验证是否存在缺陷，恶意 Server 能否绕过权限检查访问系统资源（详见 `mcp-security`）
- **账户越权**：验证 Agent 在多租户场景下是否能跨账户访问数据

## Phase 3: 多 Agent 身份伪造 (GAARM.0059)

- **会话令牌窃取**：通过 Prompt 注入诱导 Agent 输出会话凭据或将其发送到外部端点
- **Agent 身份伪装**：在多 Agent 系统中伪造来自高权限 Agent 的消息，欺骗目标 Agent 执行越权操作
- **信任链欺骗**：利用 Agent 间的隐式信任关系，通过攻陷低权限 Agent 间接控制高权限 Agent

## Phase 4: 会话与凭据安全 (GAARM.0055/0056)

- **会话劫持**：测试会话 ID 的可预测性、固定性和传输安全性
- **缓存欺骗**：验证 CDN/代理缓存是否可能泄露包含认证信息的响应
- **API Key 泄露**：检查 Agent 配置中的凭据是否可通过 Prompt 注入被诱导输出

---

## 检测清单

```
1. [ ] 身份架构已侦察？认证方式、权限模型、会话管理机制已明确？
2. [ ] 角色逃逸：假定场景/角色/遗忘法/目标劫持是否能突破角色设定？
3. [ ] 权限失控：Action 参数是否可被篡改？MCP 权限验证是否完整？
4. [ ] 身份伪造：多 Agent 间消息来源是否经过验证？信任链是否可被利用？
5. [ ] 会话安全：会话 ID 是否安全生成？是否存在固定会话攻击面？
6. [ ] 凭据管控：API Key 和 Token 是否可通过注入被泄露？
7. [ ] 跨租户隔离：多租户场景下数据访问边界是否可靠？
```

---

## 参考资源

- [GAARM Framework — AI Agent 攻击分类](https://github.com/ArcadeAI/GAARM)
- [OWASP Agentic AI Security — ASI03 Identity & Access](https://owasp.org/www-project-agentic-ai-security/)
- [MCP-Remote 高危漏洞分析](https://invariantlabs.ai/blog/mcp-remote-vulnerability)
- [CVE-2025-49596 — MCP 浏览器命令执行](https://www.cve.org/CVERecord?id=CVE-2025-49596)
