# Lane · AI / RAG / Agent 集成架构

## 适用信号

- Chatbot、RAG、knowledge-base、向量数据库、embedding、retrieval、agent、tool-use、function call、plugin、memory、system prompt、conversation、assistant API、模型网关、quota、workspace knowledge、document upload。

## 先建模什么

- `chain`: `actor -> prompt/document -> retrieval -> model -> tool/function -> side effect`。
- `data boundary`: tenant、workspace、knowledge base、document owner、memory/session。
- `tool boundary`: 工具名称、参数来源、授权身份、可产生的副作用。
- `resource boundary`: quota、billing、rate、模型能力、文件大小、任务时长。
- `evidence type`: 模型输出、检索片段、工具调用日志、业务状态变化、下载/导出结果。

## 最小验证链

1. baseline：正常用户在自有会话/knowledge-base/工具权限下得到的预期结果。
2. 单变量：文档、workspace、session、tenant、tool 参数、检索查询或 quota 边界。
3. negative control：不存在文档、无权限 workspace、错误 session、禁用工具或无效参数。
4. 只有出现跨租户检索、非授权工具调用、会话/记忆串线、可测量 quota/成本越界或业务副作用，才升级。
5. Prompt injection、模型复述公开records、输出系统风格文本、一次 `200`，默认不算漏洞。

## 高价值分支

- RAG：文档归属、索引隔离、检索过滤、引用来源、删除后残留。
- Agent tool-use：工具授权身份、参数绑定、confirm/side-effect 边界、只读/写入隔离。
- Memory/session：跨会话、跨用户、跨 workspace 记忆污染或泄露。
- Public LLM/API：quota、tenant、billing、uid、key scope；必须可量化，不外推“无限”。

## 证据与停止

- 不把提示词文本本身当系统事实；以工具日志、业务状态、检索来源和原始响应为准。
- 涉及个人数据、文档内容或凭证时只取最小样本并脱敏。
- 两次只有模型拒答、公开内容复述或无副作用输出，冻结 AI 分支。

