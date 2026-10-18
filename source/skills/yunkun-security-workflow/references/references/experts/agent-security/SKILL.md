---
name: agent-security
description: |
  AI Agent 系统安全测试execution-framework。当目标系统使用 AI Agent 执行工具调用、多 Agent 协作、
  或自主决策时触发。覆盖 OWASP Agentic AI Security Top 10 (ASI01-ASI10):
  目标劫持、工具滥用、身份权限、供应链、代码执行、记忆投毒、多 Agent 通信、
  级联故障、人机信任利用、失控 Agent。
metadata:
  tags: agent security, ai agent, agentic ai, owasp asi, tool abuse, agent hijack, multi-agent, 智能体安全, agent 劫持, 工具滥用, 权限越界, 级联故障, kill switch, agent蠕虫, morris ii
  category: ai-security
---

# AI Agent 安全测试execution-framework

## 概述

AI Agent 与普通 LLM 应用的本质区别在于**自主行动能力**——Agent 拥有工具调用、代码执行、持久记忆和多步规划能力，这意味着一次成功的注入不再只是生成错误文本，而是可以触发真实的系统操作。攻击面从"模型输出"扩展到了"工具执行链、Agent 间通信、持久化状态"的全栈。

## 深入参考

- 各 ASI 编号的详细攻击场景、Payload 与测试方法 → [references/agent-attack-scenarios.md](references/agent-attack-scenarios.md)

---

## OWASP ASI01-ASI10 风险速查

| 编号 | 风险名称 | 核心测试要点 |
|------|----------|-------------|
| ASI01 | 目标劫持 | 直接/间接注入能否改变 Agent 执行目标 |
| ASI02 | 工具滥用 | 未授权工具调用、参数注入、描述投毒 |
| ASI03 | 身份与权限 | Agent 权限是否遵循最小权限、凭据隔离 |
| ASI04 | 供应链 | 第三方 Skill/Plugin/MCP Server 是否可信 |
| ASI05 | 代码执行 | 沙箱逃逸、任意命令注入 |
| ASI06 | 记忆投毒 | 持久化上下文/历史对话是否可被污染 |
| ASI07 | 多 Agent 通信 | Agent 间消息伪造、信任传递链漏洞 |
| ASI08 | 级联故障 | 单 Agent 失败是否引发链式崩溃 |
| ASI09 | 人机信任 | 用户是否无条件信任 Agent 输出并执行 |
| ASI10 | 失控 Agent | 是否存在有效的终止/回滚机制 |

---

## execution-framework概览

### Phase 0: Agent 架构侦察
- 识别 Agent 类型（单体 ReAct / 多 Agent 编排 / 代码执行型）和底层框架（LangChain/AutoGen/CrewAI 等）
- 枚举 Agent 可用工具集、权限边界、通信拓扑（Hub-Spoke / Mesh / 层级）
- 确认人类监督节点（Human-in-the-loop）的位置和覆盖范围

### Phase 1: 目标劫持测试 (ASI01)
- 通过直接对话和间接数据源（文件、网页、API 返回）注入偏离指令，验证 Agent 是否执行非预期操作
- 关键判断：Agent 的目标锁定机制是否能抵抗上下文窗口中的对抗性指令

### Phase 2: 工具滥用测试 (ASI02)
- 构造恶意工具参数（路径遍历、命令拼接），测试 Agent 是否盲目传递用户可控输入到工具调用
- 测试工具描述投毒——恶意描述能否改变 Agent 对工具的使用方式（详见 `mcp-security`）

### Phase 3: 供应链与代码执行 (ASI04/ASI05)
- 审查第三方 Skill/Plugin/Rules 文件是否包含隐藏指令或后门
- 代码执行型 Agent：测试沙箱隔离强度，尝试文件系统访问、网络外联、进程逃逸

### Phase 4: 记忆投毒与持久化 (ASI06)
- 向 Agent 的对话历史或长期记忆注入恶意上下文，验证是否在后续会话中被激活
- 测试记忆的写入/读取权限控制——非特权交互能否修改系统级记忆

### Phase 5: 多 Agent 通信安全 (ASI07)
- 在多 Agent 系统中伪造来自其他 Agent 的消息，测试接收方是否验证消息来源
- 检查信任传递链：若 Agent A 信任 Agent B，攻击 B 能否间接控制 A

### Phase 6: 监控与 Kill Switch (ASI08/ASI10)
- 验证级联故障：单个 Agent 异常输出是否被下游 Agent 无条件信任并放大
- 测试终止机制（Kill Switch）的有效性——Agent 在失控状态下能否被可靠中断

---

## 实战检测清单

```
1. [ ] Agent 类型与架构已识别？工具集、权限模型、通信拓扑已枚举？
2. [ ] 目标劫持：间接数据源注入能否改变 Agent 执行流？
3. [ ] 工具滥用：用户输入是否直接拼接到工具参数？
4. [ ] 权限边界：Agent 是否运行在最小权限下？凭据是否隔离？
5. [ ] 供应链：第三方组件是否经过安全审计？
6. [ ] 代码执行：沙箱是否能阻止文件/网络/进程访问？
7. [ ] 记忆系统：是否有写入权限控制？历史是否可被投毒？
8. [ ] 多 Agent：消息来源是否验证？信任传递链是否可被利用？
9. [ ] 级联故障：异常是否被隔离？是否存在全局 Kill Switch？
```

---

## 参考资源

- [OWASP Agentic AI Security Top 10](https://owasp.org/www-project-agentic-ai-security/)
- [Morris II: Agent Worm — Self-Replicating Prompt Injection](https://sites.google.com/view/compromising-genai-agents/)
- [GAARM Framework — Agent Attack Taxonomy](https://github.com/ArcadeAI/GAARM)
- [Claude Code CVE-2025-54795 — Command Injection via Malicious Rule Files](https://www.cve.org/CVERecord?id=CVE-2025-54795)
