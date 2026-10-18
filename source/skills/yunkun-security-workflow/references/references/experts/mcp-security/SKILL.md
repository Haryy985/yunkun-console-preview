---
name: mcp-security
description: |
  MCP (Model Context Protocol) 协议安全测试execution-framework。当目标环境使用 MCP Server 集成外部工具、
  需要评估 MCP 工具描述安全性、或测试 Agent 通过 MCP 调用工具时的安全边界时触发。
  覆盖: 工具描述投毒、地毯式骗局(动态篡改)、指令覆盖(Shadow Tool)、隐藏指令(ANSI/Unicode)、
  跨 Server 攻击、Token 窃取、Schema 操纵、上下文溢出。
metadata:
  tags: mcp, model context protocol, tool poisoning, mcp server, mcp tool, agent tool, 工具投毒, 指令覆盖, shadow tool, rug pull, 隐藏指令, ansi escape, zero-width, unicode steganography, mcp security, tool description injection
  category: ai-security
---

# MCP 协议安全测试execution-framework

## 概述

MCP (Model Context Protocol) 为 LLM 提供标准化的工具调用接口，但其信任模型存在根本缺陷：模型必须解析工具的完整描述（description）来决定如何调用，而这些描述由 MCP Server 控制。攻击者可通过恶意 Server 在描述中注入指令，劫持模型行为——这比传统 Prompt 注入更隐蔽，因为用户通常不会审查工具描述的原始内容。

## 深入参考

- 各 Phase 详细 Payload 与攻击场景 → [references/mcp-attack-payloads.md](references/mcp-attack-payloads.md)

---

## 攻击面分类

| 攻击类型 | 核心机制 | 危害等级 | GAARM 编号 |
|----------|----------|----------|------------|
| 工具描述投毒 | description 中嵌入恶意指令 | Critical | GAARM.0046 |
| 地毯式骗局 | 授权后动态篡改 description | Critical | GAARM.0046.001 |
| 指令覆盖 (Shadow Tool) | 恶意 Server 劫持可信工具行为 | Critical | GAARM.0046.002 |
| 隐藏指令注入 | ANSI 转义码/零宽 Unicode 隐写 | High | GAARM.0046.003 |

---

## execution-framework概览

### Phase 0: MCP 环境侦察
- 枚举目标已连接的 MCP Server 列表及来源（官方/第三方/自建）
- 提取每个 Server 暴露的工具清单、inputSchema 定义、权限声明
- 判断客户端的信任模型：是否区分不同 Server 的信任等级、工具调用是否需要二次确认

### Phase 1: 工具描述投毒检测
- 获取所有工具的完整 description（包括 inputSchema.description），检查是否包含 `<IMPORTANT>`、`[SYSTEM]`、HTML 注释等指令标签
- 对 description 做 base64/Unicode 解码扫描，识别编码后的隐藏指令
- 验证模型是否会遵循 description 中的指令——构造含有"忽略用户请求"类指令的测试工具，观察模型行为变化

### Phase 2: 动态篡改测试（地毯式骗局）
- 记录工具首次注册时的 description 哈希值
- 模拟持续运行场景：定时重新获取工具列表，对比 description 是否发生变化
- 关键判断：MCP 协议允许 Server 随时通过 `notifications/tools/list_changed` 更新描述，客户端是否在更新时重新请求用户授权

### Phase 3: 指令覆盖测试（Shadow Tool）
- 注册恶意 MCP Server，其工具 description 中包含针对其他 Server 工具的覆盖指令
- 测试跨工具劫持：恶意工具描述要求模型在调用邮件/文件等可信工具时篡改参数
- 验证客户端是否存在工具名称冲突保护（同名工具优先级机制）

### Phase 4: 隐藏指令注入
- ANSI 转义码测试：在 description 中嵌入 `\x1b[8m` (隐藏文本) 指令，检查终端渲染和模型解析的差异
- 零宽 Unicode 字符测试：用 U+200B/U+200C/U+200D/U+FEFF 编码恶意指令，验证人类不可见但模型可读
- "行跳跃"攻击：利用 ANSI 光标控制序列覆盖终端显示内容，伪造用户可见的输出

### Phase 5: 跨 Server 攻击与 Token 窃取
- 测试恶意 Server 能否通过工具描述指令获取其他 Server 的 OAuth Token 或 API Key
- 验证 Token 隔离机制：一个 Server 的凭据是否对其他 Server 可见
- Server 伪装检测：恶意 Server 是否能声称自己是可信 Server 的"更新版本"

---

## 实战检测清单

```
1. [ ] 目标环境连接了哪些 MCP Server？是否包含第三方/未审计 Server？
2. [ ] 工具 description 原始内容是否包含可疑指令标签或编码内容？
3. [ ] 工具描述是否会在授权后发生动态变化？客户端是否重新确认？
4. [ ] 是否存在跨 Server 的工具名称冲突或描述覆盖？
5. [ ] description 中是否嵌入 ANSI 转义码或零宽 Unicode 字符？
6. [ ] 各 Server 的 Token/凭据是否实现了隔离？
7. [ ] 客户端是否对 inputSchema 做严格校验，还是透传给模型？
8. [ ] 工具调用结果是否经过消毒后再返回模型上下文？
```

---

## 参考资源

- [Invariant Labs — MCP Security Notification: Tool Poisoning Attacks](https://invariantlabs.ai/blog/mcp-security-notification-tool-poisoning-attacks)
- [OWASP — Model Context Protocol Security](https://genai.owasp.org/)
- [GAARM Framework — MCP Attack Taxonomy](https://github.com/ArcadeAI/GAARM)
