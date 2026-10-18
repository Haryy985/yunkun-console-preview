# GAARM 风险矩阵与 Flask 业务链优先级（外迁明细）

> 本文件为 `security-testing-methodology` §三 决策树的完整明细（P3 治理 2026-08-14 外迁）。
> 正文只保留摘要与指针；规划 AI 安全测试与 Flask 业务链测试时按本文执行。

## GAARM 风险矩阵（AI 安全域覆盖）

| 安全域 | 训练阶段 | 部署阶段 | 应用阶段 |
|--------|----------|----------|----------|
| **AI应用安全** | 不安全输出处理/框架漏洞/第三方组件 | API管理不当/源代码投毒 | Prompt注入/CoT注入/MCP攻击/Agent利用 |
| **AI模型安全** | 模型后门/对齐不足/投毒 | 参数篡改/文件窃取 | 越狱/幻觉/对抗样本/功能滥用 |
| **AI数据安全** | 训练数据投毒/泄露/偏见 | 存储攻击/传输劫持 | 隐私窃取/Prompt泄露/推断攻击 |
| **AI身份安全** | 权限设计缺陷/环境认证 | 未授权访问/凭据滥用 | 角色逃逸/会话劫持/Agent伪造 |
| **AI基座安全** | 开发工具漏洞/环境隔离 | 容器漏洞/云平台/供应链 | 容器逃逸/拒绝服务/代码执行逃逸 |
| **AI合规治理** | 数据合规/隐私保护法规 | 部署审计/合规检查 | 内容合规/版权/偏见歧视 |

## Flask 业务链优先级与降级信号

核心原则：不平均用力，按攻击链可能性排序；主路不通立即 pivot。优先找「不需要 is_admin=true 也能碰到 admin 数据」的业务链，而非纯权限提升。

| # | 攻击链 | 关键测试点 | 放弃信号 |
|---|--------|-----------|---------|
| 1 | PDF/下载/导出 IDOR | 鉴权是否独立于详情页（`Expense.query.get(id)` 无 user_id 检查即通杀）；枚举 `/{expense(s)}/{id}/{pdf\|download\|export}`、`/admin/expense(s)/{id}/pdf` | 路由均 302 ⇒ 有鉴权，非 IDOR |
| 2 | ReportLab/PDF 生成链 | 测 Paragraph markup sink（`<b>`、`<font color>`、`<br/>`）；被解析 → CVE-2023-33733（toColor/rl_safe_eval）；`abc) Tj 0 0 1 rg (INJECT` 测 PDF injection | `canvas.drawString()`+`(...)Tj` 字面渲染 ⇒ 无 sink |
| 3 | 模板 owner 反向污染 | 无鉴权模板编辑时把 admin 模板 owner 改为当前用户；查权限判断是否 `template.owner_id == current_user.id`（而非 `user_id`） | — |
| 4 | 草稿状态绕过 + Method Override | POST + `_method=PUT/PATCH/GET`、`X-HTTP-Method-Override: PUT`、`status=approved`、`action=approve/confirm=1` | — |
| 5 | 用户路由 IDOR | user_id 取 URL/表单而非 session：`/profile\|user\|users\|account\|settings/{id}`、`/change_password`、`/reset_password` | — |
| 6 | 辅助路由 IDOR | `/expense/{id}/{copy\|clone\|duplicate\|receipt\|attachment\|print\|withdraw}`——复制报销单可能把 admin 报销复制成你的 | 所有 copy/clone 路由 404 ⇒ 无此功能 |
| 7 | 弱口令枚举（登录页有提示时） | admin/employee/manager/finance/auditor；密码：用户名自身/123456/admin/password/888888 | session secret 词典爆破不中 ⇒ 非主线 |
| 8 | 源码/配置/数据库泄露 | `/instance/config.py`、`/config.py`、`/.env`、`/database.db`、`/static/../app.py`、`/source.zip`、`/.git/config` | — |
| 9 | owner_user_id 类型异常 | 边缘值 `0/-1/999/''/null/None/'admin'/'string'` → 期望 500/模板消失/owner 异常/自动审批异常 | 所有值均显示"未指定(ID: X)" ⇒ 无类型异常 |

## 关键思维转折信号（>15 分钟无进展时自检）

① 在找"拿 admin"还是"不拿 admin 也能碰数据"？后者优先；② 该能力有无未试过的副作用（如列表/导出按 owner 关联泄露）？③ 是否只测主路由漏了副功能？先测所有 `/{resource}/{id}/{action}` 组合。
