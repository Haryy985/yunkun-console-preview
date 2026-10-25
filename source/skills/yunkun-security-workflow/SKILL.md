---
name: yunkun-security-workflow
description: Authorized security validation, evidence quality, HTTP packet reports and reviewed knowledge proposals in Yunkun. Load for task planning, verification or reporting.
---

# 云鲲验证与交付工作流

沿用当前 Agent、模型、任务调度和工具授权。此资料不指定模型，不自动启动
外部 MCP、Python、hooks 或其他协调器。按需阅读 references 下的专项资料，
每次只取与当前假设有关的章节，不一次载入整套资料。

1. 确认任务范围、账号角色、预算、排除项和停止条件。搜索结果不是授权。
   本项目不开展 CORS、IDOR 或 BOLA 专项探测；已有观察作为线索保留。
   这是项目排除项，不是这些漏洞普遍无危害的判断。不得改名、隐瞒类型或
   抬高严重等级来绕过排除规则。
2. 用 get_target_profile 按需读取已有画像。区分观察、推断和未验证候选。
3. 优先被动信息。目标 HTTP 访问沿用现有受管工具；analyze_web_surface
   只分析已经取得的 HTML/JavaScript，不负责联网。外部情报工具须先获绑定，
   返回值只作候选，不自动扩范围、入资产或开始探测。
4. 每个意图选择一个假设。记录正常基线、一次受控变异、实际响应差异、
   对象和身份归属、安全影响、清理及最终状态。失败或无变化也保留证据。
5. 版本、路由名、扫描器告警或单个状态码只能形成线索。report_finding
   需要 validation 和本任务真实成功步骤或已核实流量引用；证据不足用
   record_fact 说明待验证项，不编造结果或抬高严重等级。
6. 按漏洞类型记录复现次数和成功率，受授权预算约束。正式交付须通过人工
   质量审核；新发现和历史发现都不会自动 PASS，证据改变后重新审核。
   报告门槛是授权范围内可重复的实际安全影响；线索、单一状态码、猜测权限和无差异响应不得升级为漏洞。
7. 报告包含标题、目标 URL、等级、描述、已证实危害、接口清单、步骤和
   修复建议。HTTP 复现使用捕获的请求行、Host、头、空行、完整文本正文；
   不用 curl 替代，不从 URL 或记忆补造包。缺失、二进制、过长或未读完
   时标记证据缺口并引用原始附件。保密包中的捕获凭据仅经受限审计路径输出。
8. 已验证且可复用的方法通过 report_finding.learning 或
   propose_rag_change 提交到已授权知识库，包含适用条件、局限及来源。
   不提交凭据、个人数据或目标原始请求。人工批准前不进入知识检索。

交接使用已有 finding_id、intent_id、步骤和流量 ID；不得猜测 ID，
不得把未执行的想法写成执行记录。遇阻说明原因和最小下一步，不无限重试。
