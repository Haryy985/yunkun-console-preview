# MCP 工具集成 — jshookmcp

> 本文档是 clown21 skill 调用本地 MCP 服务器的工具地图。

clown21 是渗透测试实战 skill，默认假设你只有一个 URL，没有源码、没有内部信息。jshookmcp 把浏览器自动化、CDP 调试、网络拦截、JS hook、反混淆、Frida 内存取证、WASM 逆向、Source map 重构整合到一个 MCP server 中，直接服务于渗透测试阶段的"看代码 / 跑 payload / 拦数据 / 反调试 / 反混淆"五件事。

工具引用使用 `mcp__jshook__<tool>` 完整 MCP 协议命名。所有工具名以 `.omc/tool-manifest.json`(jshookmcp 0.3.0 实际 `search_tools` 返回值)为准，**不臆造**。


## 36 域索引

jshookmcp 把 134+ 工具分布在 36 个 capability domain 中。下表标注每域核心用途与代表工具(BARE 名，实际调用时加 `mcp__jshook__` 前缀)。

| 域名 | 何时用 | 代表工具 |
|---|---|---|
| `adb-bridge` | Android 设备 ADB 桥接 / APK 分析 / WebView 远调 | `adb_apk_analyze` / `adb_webview_attach` / `adb_webview_list` |
| `antidebug` | 绕过 debugger / timing / headless 指纹检测 | `antidebug_bypass` |
| `binary-instrument` | Frida 脚本生成 / Ghidra 反编译 / hook 模板导出 | `frida_run_script` / `generate_hooks` / `ghidra_decompile` / `frida_generate_script` |
| `boringssl-inspector` | TLS 握手解析 / SSL pinning 绕过 / SSLKEYLOGFILE | `tls_cert_pin_bypass_frida` / `tls_keylog_enable` / `tls_probe_endpoint` |
| `browser` | 浏览器自动化 / CDP 评估 / 反检测 stealth | `browser_evaluate_cdp_target` / `page_evaluate` / `stealth_inject` |
| `canvas` | Canvas / WebGL 引擎指纹与对象拾取 | `canvas_engine_fingerprint` / `canvas_pick_object_at_point` |
| `coordination` | 跨任务 handoff / 页面快照 / session insight | `save_page_snapshot` / `create_task_handoff` / `get_task_context` |
| `core` | 代码采集 / 反混淆主管线 / hook 管理 / 加密检测 | `collect_code` / `deobfuscate` / `js_deobfuscate_pipeline` / `detect_crypto` |
| `cross-domain` | 跨域证据图聚合 / 工作流建议 | `cross_domain_correlate_all` / `cross_domain_suggest_workflow` |
| `debugger` | JS 断点 / 单步 / 表达式求值 / blackbox | `debugger_evaluate` / `debugger_pause` / `debugger_step` / `blackbox_add_common` |
| `encoding` | base64 / protobuf / 二进制编解码 | `binary_encode` / `binary_decode` / `protobuf_decode_raw` |
| `evidence` | 反向证据图查询 / 导出 / evidence_trace 链 | `evidence_query` / `evidence_export` / `evidence_chain` |
| `extension-registry` | webhook 端点管理(外部回调) | `webhook` |
| `graphql` | GraphQL introspection / 查询提取 / 重放 | `graphql_introspect` / `graphql_extract_queries` / `graphql_replay` |
| `hooks` | JS 运行时 hook preset(eval/atob/Reflect 等 20+) | `hook_preset` / `ai_hook` |
| `instrumentation` | 仪表化 session 内的 hook / 网络重放 / artifact | `instrumentation_hook_preset` / `instrumentation_network_replay` |
| `macro` | 录制 / 重放工具调用序列 | `run_macro` / `list_macros` |
| `maintenance` | 环境健康检查 / bridge 端点诊断 | `doctor_environment` |
| `memory` | 进程内存 patch 回滚(配合 Frida) | `memory_patch_undo` |
| `mojo-ipc` | Chromium Mojo IPC 监听 / 解码 | `mojo_monitor` / `mojo_messages_get` / `mojo_decode_message` |
| `network` | 请求拦截 / 重放 / HAR / HTTP2 / 性能指标 | `network_intercept` / `network_replay_request` / `http2_probe` / `network_extract_auth` |
| `platform` | Electron / ASAR 应用分析 / fuse 状态 | `electron_inspect_app` / `electron_ipc_sniff` / `asar_search` |
| `process` | 进程级 CDP 附加(Electron 等) | `electron_attach` |
| `protocol-analysis` | 协议状态机推断 / ICMP / 模式可视化 | `proto_infer_state_machine` / `icmp_echo_build` / `proto_visualize_state` |
| `proxy` | 本地代理启停 / CA 管理 / Android 接入 | `proxy_setup_adb_device` / `proxy_status` / `proxy_stop` |
| `sandbox` | QuickJS WASM 沙箱内执行 JS | `execute_sandbox_script` |
| `shared-state-board` | 多 agent 共享状态板(观察 + IO) | `state_board` / `state_board_watch` / `state_board_io` |
| `skia-capture` | Skia 场景树提取 / 渲染器探测 / 节点关联 | `skia_extract_scene` / `skia_detect_renderer` |
| `sourcemap` | source map 抓取 / 解析 / 源码树重建 | `sourcemap_fetch_and_parse` / `sourcemap_reconstruct_tree` / `sourcemap_parse_v4` |
| `streaming` | WebSocket 连接与帧捕获 | `ws_monitor` / `ws_get_connections` |
| `syscall-hook` | 进程级 syscall 监听(ETW / strace / dtrace) | `syscall_start_monitor` / `syscall_get_stats` |
| `trace` | SQLite trace 录制 / 网络流追踪 / Chrome Trace 导出 | `trace_recording` / `trace_get_network_flow` / `export_trace` |
| `transform` | AST 改写 / 加密函数提取 / 实现对比 | `ast_transform_apply` / `crypto_extract_standalone` / `crypto_compare` |
| `v8-inspector` | V8 字节码提取 / 版本探测 | `v8_bytecode_extract` / `v8_version_detect` |
| `wasm` | WebAssembly disasm / decompile / 内存 / 反混淆探测 | `wasm_disassemble` / `wasm_decompile` / `wasm_dump` / `wasm_detect_obfuscation` |
| `workflow` | 扩展工作流 / API 批探测 / 远 bundle 搜索 | `js_bundle_search` / `api_probe_batch` / `run_extension_workflow` |

---

## 场景 → 工具映射表

这是本文档的核心。把渗透测试阶段常见动作对应到 jshook 工具，标注关联 playbook 与调用时机。

| 场景 | 漏洞类型 | 推荐工具(`mcp__jshook__*`) | 关联 playbook | 调用时机 |
|---|---|---|---|---|
| 拦截外发请求 / 观察 SSRF | SSRF | `network_intercept` + `network_get_requests` | ssrf-cache-host/00-index.md | 探测阶段被动观察 |
| 构造 HTTP/2 帧探测内网 | SSRF | `http2_probe` + `http_request_build` | ssrf-cache-host/00-index.md | 主动绕过过滤 |
| 重放修改 SSRF 请求 | SSRF | `network_replay_request` | ssrf-cache-host/00-index.md | 验证不同协议 / 主机头 |
| 浏览器执行 XSS payload | XSS | `browser_evaluate_cdp_target` + `page_evaluate` | xss/00-index.md | 验证盲打 / DOM XSS |
| XSS payload 注入页面 | XSS | `page_inject_script` | xss/00-index.md | 持久化注入 |
| 反混淆混淆 JS / AST 改写 | XSS / RCE | `ast_transform_apply` + `deobfuscate` | xss/00-index.md / rce/00-index.md | 分析 obfuscated 业务代码 |
| JSVMP / VM 保护 JS 反混淆 | XSS / RCE | `js_deobfuscate_jsvmp` + `js_deobfuscate_pipeline` | xss/00-index.md / rce/00-index.md | 高强度混淆站 |
| Source map 还原原始源码 | 信息泄露 / XSS | `sourcemap_fetch_and_parse` + `sourcemap_reconstruct_tree` | xss/00-index.md | 找 sink / 找接口 |
| Webpack bundle 模块枚举 | 信息泄露 | `webpack_enumerate` + `js_bundle_search` | xss/00-index.md | 找前端密钥 / 内部 API |
| 加密算法识别 / 提取 | OAuth / XSS / API | `detect_crypto` + `crypto_extract_standalone` | oauth-saml-jwt/00-index.md / xss/00-index.md | 还原签名逻辑 |
| eval / atob / Function preset hook | XSS / RCE | `hook_preset` | xss/00-index.md / rce/00-index.md | 找运行时反序列化 sink |
| 设置 DOM 断点 / 单步追 sink | XSS | `debugger_pause` + `debugger_step` + `get_call_stack` | xss/00-index.md | DOM XSS 数据流追踪 |
| WASM 模块抓取与反汇编 | RCE | `wasm_dump` + `wasm_disassemble` + `wasm_decompile` | rce/00-index.md | WASM 业务逻辑逆向 |
| WASM 反混淆探测 / 转 C | RCE | `wasm_detect_obfuscation` + `wasm_to_c` | rce/00-index.md | 加密 / 风控核心 |
| Frida 脚本生成与注入 | RCE | `generate_hooks` + `frida_run_script` | rce/00-index.md | 验证 RCE 落点 |
| Frida hook 导出可运行脚本 | RCE | `export_hook_script` | rce/00-index.md | 离线复测 |
| Ghidra 函数反编译 | RCE | `ghidra_decompile` | rce/00-index.md | 二进制反序列化场景 |
| 反调试 / debugger 检测绕过 | XSS / RCE / Mobile | `antidebug_bypass` | xss/00-index.md / rce/00-index.md / mobile.md | 目标主动反调试 |
| Android APK 信息提取 | Mobile | `adb_apk_analyze` | mobile.md | 静态分析前置 |
| Android WebView 远调 | Mobile | `adb_webview_attach` + `adb_webview_list` | mobile.md | App 内嵌 H5 |
| SSL pinning 绕过 (Frida) | Mobile | `tls_cert_pin_bypass_frida` + `tls_cert_pin_bypass` | mobile.md | APK 拦截前置 |
| SSLKEYLOGFILE 抓密钥 | Mobile / API | `tls_keylog_enable` + `tls_keylog_parse` | mobile.md / oauth-saml-jwt/00-index.md | 对接 Wireshark 解密 |
| 安卓代理接入 | Mobile | `proxy_setup_adb_device` + `proxy_status` | mobile.md | 流量观察前置 |
| JWT / token 抽取 | OAuth/SAML/JWT | `network_extract_auth` | oauth-saml-jwt/00-index.md | 自动找 Authorization / cookie |
| JWT base64 编解码 | OAuth/SAML/JWT | `binary_encode` + `binary_decode` | oauth-saml-jwt/00-index.md | 篡改 header / payload |
| redirect_uri 链路调试 | OAuth | `debugger_evaluate` + `network_replay_request` | oauth-saml-jwt/00-index.md | 找 open redirect |
| GraphQL introspection | API REST | `graphql_introspect` | api-rest/00-index.md | 资产展开 |
| GraphQL 历史查询提取 | API REST | `graphql_extract_queries` + `graphql_replay` | api-rest/00-index.md | 重放业务请求 |
| REST 批量接口探测 | API REST | `api_probe_batch` | api-rest/00-index.md | 批量 BOLA / mass-assignment |
| WebSocket 帧捕获 | API REST | `ws_monitor` + `ws_get_connections` | api-rest/00-index.md | 实时业务 / 推送 |
| 文件上传 polyglot 编码 | File Upload | `binary_encode` + `binary_decode` | file-upload/00-index.md | 构造图片+脚本混合 |
| 文件上传 AST 改写绕过过滤 | File Upload | `ast_transform_apply` + `ast_transform_preview` | file-upload/00-index.md | 改 magic byte / 修复 polyglot |
| 文件上传 multipart 边界改 | File Upload | `http_plain_request` + `network_replay_request` | file-upload/00-index.md | 绕过 MIME 校验 |
| Protobuf 二进制盲解 | API REST / 信息泄露 | `protobuf_decode_raw` | api-rest/00-index.md | 无 schema 抓包分析 |
| Electron 应用静态结构 | RCE / 信息泄露 | `electron_inspect_app` + `asar_search` | rce/00-index.md | 桌面端目标 |
| Electron IPC 监听 | RCE | `electron_ipc_sniff` | rce/00-index.md | renderer ↔ main IPC 漏洞 |
| Chromium Mojo IPC 监听 | RCE | `mojo_monitor` + `mojo_messages_get` | rce/00-index.md | 浏览器内核漏洞研究 |
| 进程 syscall 监听 | RCE | `syscall_start_monitor` + `syscall_get_stats` | rce/00-index.md | 验证 RCE 后行为 |
| 协议状态机推断 | API / 信息泄露 | `proto_infer_state_machine` + `proto_visualize_state` | api-rest/00-index.md | 自定义协议逆向 |
| 全流量 trace 持久化 | 调查 / 报告 | `trace_recording` + `export_trace` | 所有 playbook | 留证 / 时间线复盘 |
| 反检测 stealth 注入 | 长期挂测 | `stealth_inject` + `stealth_verify` | xss/00-index.md / api-rest/00-index.md | 风控站长期观察 |
| 跨域证据聚合 | 调查 | `cross_domain_correlate_all` + `evidence_export` | 所有 playbook | 多源对齐 / 出报告 |

---

## 推荐 profile

jshookmcp 提供三档 profile，通过环境变量 `JSHOOK_BASE_PROFILE` 或工具调用切换。**默认 `search` 模式**，符合 clown21 上下文经济原则。

| Profile | 上下文成本(token) | 适用场景 | 启用方式 |
|---|---|---|---|
| `search` (**默认**) | ~3K | 按需激活，单点调用，常态渗透测试 | `JSHOOK_BASE_PROFILE=search`(已是默认值) |
| `workflow` | 中等 | 连续编排，跨域协作，1 个 session 多次复用同类工具 | 调 `mcp__jshook__boost_profile workflow`(若可用) |
| `full` | 40K+ | 已知会用 50%+ 工具时，如大型批量任务 | 调 `mcp__jshook__boost_profile full` |

**默认推荐工作流**：

1. `mcp__jshook__search_tools <关键词>`(BM25 检索，按关键词分桶)
2. 取 top-3 结果，`mcp__jshook__activate_tools <工具名 list>` 激活
3. 调用激活的工具
4. 跨域协作时：`mcp__jshook__activate_domain <域名>` 批量激活整域

**反模式**：不要为单个工具直接 `boost_profile`，会一次性加载 40K+ token 严重浪费上下文。只在已知后续要密集复用某一族工具时升档。

---

## 内置 Burp Suite Bridge

⚠️ **本机实测说明**：`mcp__jshook__search_tools burp` 在 jshookmcp 0.3.0 本机版本上**未返回**任何 `burp_*` 原子工具，仅命中：

- `doctor_environment`(maintenance 域，描述中提到"bridge endpoints"健康检查)
- 通用 `proxy_*` 工具(proxy 域，启停 / 状态 / CA 管理)

jshookmcp 上游 README 声明内置 Burp / Ghidra / IDA Pro bridges，但 0.3.0 当前版本未将 Burp 桥接拆为独立原子工具暴露。

**实用替代路径(fallback)**：

1. **激活 proxy + cross-domain 两域**：
   ```
   mcp__jshook__activate_domain proxy
   mcp__jshook__activate_domain cross-domain
   ```
2. **用 `proxy_*` 启动本地代理 + CA**，把 Burp 配置为上游 proxy 链或反向接入。
3. **用 `mcp__jshook__doctor_environment` 检查 bridge 端点状态**，若 jshook 后续版本暴露 Burp 桥接 API，会在此显现。
4. **用 `network_*` + `instrumentation_network_replay` 替代 Burp Repeater 自动化**：`network_replay_request` 已能完成大多数 Repeater 场景的批量改包。
5. **结合 `evidence_*` / `cross_domain_correlate_all`**：把 jshook 抓的请求与外部 Burp 数据通过 evidence 图人工合并(导出 JSON 后再聚合)。

**何时仍需独立 burp-mcp-server**：

- 需要 Burp Scanner 主动扫描结果直接进 LLM
- 需要 Burp Extender 写的自定义 scan check 触发
- Repeater 复杂 macro / session handling rule 联动

如上述场景需要，可在 `~/.claude.json` 单独注册 burp 官方 MCP(out of scope of this doc)，并在 `references/tools/mcp-burp.md`(未来)中文档化。

---

## 相关 playbook

下表是本 MCP 在 clown21 19 个 playbook 中的角色分布。**本次主选 7 个高关联 playbook 加反向锚点**，其余 12 个后续迭代加入。

| Playbook | jshook 主要域 | 本 MCP 角色 |
|---|---|---|
| [xss/00-index.md](../playbooks/xss/00-index.md) | browser / debugger / transform / hooks / sourcemap / core | 浏览器执行 + AST 反混淆 + Sink 断点 + Source map 还原 |
| [rce/](../playbooks/rce/00-index.md) | wasm / antidebug / binary-instrument / memory / platform / mojo-ipc / syscall-hook | WASM 逆向 + Frida 内存验证 + 反调试 + Electron / Chromium IPC |
| [ssrf-cache-host/00-index.md](../playbooks/ssrf-cache-host/00-index.md) | network / proxy / protocol-analysis | 网络拦截 + HTTP/2 构造 + 协议状态机推断 |
| [mobile.md](../playbooks/mobile.md) | adb-bridge / boringssl-inspector / proxy / binary-instrument | SSL pinning 绕过 + APK 信息 + WebView 远调 + Frida hook |
| [oauth-saml-jwt/00-index.md](../playbooks/oauth-saml-jwt/00-index.md) | network / encoding / debugger / core | JWT 篡改 + redirect_uri 调试 + 加密算法识别 |
| [api-rest/00-index.md](../playbooks/api-rest/00-index.md) | graphql / network / workflow / streaming / protocol-analysis | introspection + 批量 API + WebSocket 帧 + 协议盲解 |
| [file-upload/00-index.md](../playbooks/file-upload/00-index.md) | encoding / transform / network | polyglot 编码 + AST 改写 + multipart 改包 |

**未覆盖 playbook**(后续迭代加入)：sqli / path-traversal / graphql / arbitrary-x-authz / logic-flaws / unauth-access / info-disclosure / http-smuggling / race-conditions / dos / llm-prompt-injection / intranet-postexp。

---
