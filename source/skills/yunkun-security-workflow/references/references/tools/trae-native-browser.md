# Trae CN / Trae SOLO 原生浏览器适配

适用条件：当前执行面是 Trae CN 或 TRAE SOLO CN，且 `TRAE-browseruse` 或项目的 integrated browser MCP 可用。

## 定位

Trae 原生浏览器是 `browser-required` 页面、SPA、SSO、前端加密、动态 API 和运行时业务流的**首选观察面**。它补足 WSL Playwright 的运行时可见性，不替代 clown21 的证据分层、认证 owner、状态变更守卫或 WAF 冻结规则。

## 最小运行时采集序列

进入一个新页面或完成一个真实业务步骤后，优先在一个短序列中完成：

1. `browser_tabs`：确认当前 tab 与 URL；
2. `browser_snapshot`：记录实际 DOM、路由与可交互节点；
3. `browser_console_messages`：提取运行时错误、路由/API 线索；
4. `browser_network_requests`：提取真实 XHR/fetch/GraphQL/WebSocket 请求；
5. 仅当上述证据提出具体假设时，使用 `browser_evaluate` 读取与该假设直接相关的公开运行时状态或 DOM 数据。

每个序列只服务一个模块或一个假设；不做轮询、不把多页流程塞入长脚本。网络记录按 `host + path-prefix + method + auth-shape` 归并后进入 API inventory，静态 JS 字符串只作为候选。

## 证据与会话纪律

- 保存决定性 network/console/snapshot 原文到当前 run 的 `evidence/`；记录来源页面、时间、tab 与触发动作。
- 认证完成后，Trae 浏览器是该认证状态机的唯一 owner。不得复制 Cookie、Authorization、CSRF 或 session 到 curl、脚本、其他浏览器上下文或报告。
- 需要前端行为差分时，在同一 tab 的 baseline/variant/negative-control 间只改变一个变量；截图仅用于 canvas、复杂视觉状态或最终可读证据，默认优先 snapshot 与网络原文。
- `browser_evaluate` 只用于当前假设的读取、DOM/source-to-sink 追踪或安全标记验证；会留下业务状态的调用仍须先通过 `mutation_guard.py`。

## 执行面选择

| 场景 | 首选 | 降级 |
|---|---|---|
| SPA、SSO、XHR、前端路由、前端加密 | `TRAE-browseruse` + network/console | 已保存的 HAR/JS 离线分析 |
| Canvas、原生弹窗、非标准控件 | `TRAE-computer-use` 仅完成必要 UI 动作 | 浏览器 snapshot/截图保留证据 |
| 本地源码的运行时问题 | `TRAE-debugger`，仅对本地受控项目 | clown21 source-audit + 运行时请求关联 |
| 黑盒 EduSRC/远程目标 | clown21 + Trae browser | 不使用 Debugger 插桩或修改目标代码 |

`TRAE-computer-use` 不拥有网络证据，不得作为认证/API 测试主执行面；`TRAE-debugger` 的创建调试文件与插桩流程不适用于远程黑盒测试。

## WAF 与阻塞

首个明确 challenge、自动化指纹拒绝、异常 reset、`429` 或正常 baseline 的异常 `403` 触发后，先检查当前 profile。若 `waf_bypass.mode=allowed-with-budget`，进入串行、限额、单变量的 WAF 验证分支，并保存 snapshot、console、network、JS/HAR；若未启用该模式，则按 `waf-blocked` 冻结 origin。无论哪种模式，都禁止无上限轮询、第二个浏览器 agent、破坏性动作或未批准的指纹/出口轮换。
