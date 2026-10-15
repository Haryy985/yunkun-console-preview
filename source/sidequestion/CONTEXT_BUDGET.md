# 旁路长对话与上下文预算

本模块继承此前的预算修复：原实现将请求 JSON 的字符数直接视为 token，并继承主任务的 32K 输出预留，因此 HTML、JS 和工具结果较多时会提前拒绝正常旁路问题。当前仓库为 [Haryy985/yunkunsec](https://github.com/Haryy985/yunkunsec)。

## 开源实现核对

- [Grok CLI 旁路上下文](https://github.com/superagent-ai/grok-cli/blob/fb97af83f06dca873281d60168430f06c8de6324/src/agent/agent.ts#L739)：从最近的用户、助手文本中摘取片段，字符预算约 2000，每条至多截取 400 字符。未在此路径维护连续旁路问答历史。
- [Grok CLI 独立请求](https://github.com/superagent-ai/grok-cli/blob/fb97af83f06dca873281d60168430f06c8de6324/src/utils/side-question.ts)：独立取消信号；模型支持时输出上限为 2048 tokens；不提供工具。
- [Grok CLI 主会话压缩](https://github.com/superagent-ai/grok-cli/blob/fb97af83f06dca873281d60168430f06c8de6324/src/agent/compaction.ts)：估算 token、保留近期内容、把新内容更新进旧摘要，并处理跨回合截断。
- [OpenCode 会话压缩](https://github.com/anomalyco/opencode/blob/b3f1a96c6dd7adeb28b36dd11add1998fc84d67b/packages/core/src/session/compaction.ts)：完整请求估算、输出/缓冲预留、近期内容加滚动摘要、无工具摘要请求；该实现默认近期预算 8000、摘要输出上限 4096 tokens。
- [OpenCode 溢出恢复](https://github.com/anomalyco/opencode/blob/b3f1a96c6dd7adeb28b36dd11add1998fc84d67b/packages/core/src/session/runner/llm.ts)：尚未开始助手输出时才尝试溢出恢复，恢复后的调用不再进入同一溢出恢复路径。

云鲲沿用独立输出预算、近期内容与滚动摘要、有限恢复的做法。保持 go.mod 锁定版本的 norma 的结构化消息与工具配对，不照搬 Grok 的文本摘录；不把 OpenCode 的主会话压缩事件写入主 transcript。

## 请求预算与执行

- 消息沿用 norma 的按内容块 UTF-8 字节估算及 4/3 余量；额外计入系统提示、工具 schema 和消息封装开销。估算不是模型精确 token 计数。
- 旁路输出默认最多 8192 tokens，也不超过主配置已设的输出上限。可用服务环境变量 `YUNKUN_BTW_MAX_OUTPUT_TOKENS` 设置 256–32768 的上限；不会修改产品默认模型或主任务参数。
- 输入预算为上下文窗口减去输出上限和安全余量；未知窗口使用平台默认 200K。安全余量为窗口的 5%，最小 128、最大 8192 tokens。
- 成功问答按递增序号每批最多加载 20 组。最多保留 20 组原文，其 token 预算最多为输入预算的 1/4，且不超过 16K。
- 超额问答更新到滚动摘要。摘要带历史来源与上下文时间；历史助手回答不等同于新的工具证据，冲突时优先依据最新主快照。
- 主上下文仍过长时，仅摘要副本中的旧消息，近期最多保留 8K tokens；截取点不会拆开工具调用与结果。过大的单组会整体进入摘要。
- 摘要输入按实际剩余窗口进行 UTF-8 安全分块，输出上限 2048 tokens；空摘要、截断、工具调用或超出摘要预算均不写缓存。单次旁路最多 12 次摘要调用，并受同一 120 秒超时限制；达到上限会明确失败，不无限循环。
- 若模型首次返回上下文超限且尚未输出文本或工具调用，进一步缩减后最多重试一次；若估算大小没有下降，立即停止恢复。其他模型错误和部分流式输出不触发该恢复。
- 所有已取得用量，包括摘要、失败尝试和取消时的用量，累加到同一旁路请求。Provider 不返回用量时仍只能记录零，不能把估算伪装成实际用量。

## 持久化与界面

`side_question_sessions.memory` 保存旧问答摘要、覆盖的 ordinal，以及按快照身份缓存的主上下文摘要。`side_question_requests.context_info` 保存准备阶段、实际回放组数、摘要使用情况和预算估算。

摘要只在原请求仍运行且清理版本匹配时保存；清空同时清除缓存，迟到写入不会恢复已清理数据。新快照不复用旧快照摘要。摘要字段随 v3 任务归档保存；恢复旧 v3 缺失字段时补空对象，v1/v2 继续兼容。

POST 先接纳并返回请求，准备与压缩在后台执行，不持有准入锁或数据库事务。SSE/历史显示准备、整理问答、压缩副本、回答阶段；压缩失败作为该旁路请求的失败终态保存。前端保留错误并恢复本次失败问题的草稿，不用悬浮通知遮挡输入框；历史轮询不再清掉提交错误。

## 验证方法

重点检查回放分页、滚动摘要、UTF-8 分块、工具配对、快照与缓存失效、取消和清空竞争、摘要失败、恢复次数及部分流输出。数据库用例另需隔离 PostgreSQL；模拟或历史通过记录不能替代当前真实模型验收。本轮实际结果统一记在 [品牌审计](../YUNKUN_BRAND_AUDIT.md)。

以下为当前命令示例，仅使用独立测试库；竞态检查还需要本机可用的 race 工具链：

```sh
go test -race ./sidequestion ./db ./server -run 'TestSide|TestCheckpoint|TestSnapshot|TestBuildRequest|TestService|TestMainSide|TestTaskArchive' -count=1
go build ./cmd/yunkun
npx tsc --noEmit
npm run build:static
```

Go 命令在项目根目录执行，前端命令在 `web/` 执行。构建使用独立工作区；部署前保留数据库、密钥、数据目录与旧程序的备份。本手册不提供历史机器路径，也不将编译成功视为服务已经切换。
