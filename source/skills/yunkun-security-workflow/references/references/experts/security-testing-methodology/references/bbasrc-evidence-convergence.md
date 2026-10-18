# BBASRC 证据收敛实战案例（2026-07-12）

> 对应 `references/stm-九-证据收敛与报告定稿流程.md`（正文案例节指针）

## 目标环境

- 厂商：华晨宝马 (BMW Brilliance Automotive)
- 平台：漏洞盒子 BBASRC
- 资产范围：`*.bmw-brilliance.cn`, `*.bba-app.biz`, `*.lingyue-digital.com` 等22个域名
- 阶段：资产探测 → 漏洞线索发现 → 证据收敛 → 提交决策

## 收敛前状态（7条线索）

| # | 发现 | 初始判断 |
|---|------|---------|
| 1 | Countly `/i` 端点接受任意数据 | 高风险（未鉴权写入） |
| 2 | gateway-int Gateway 公网可达 | 中风险（内部网关暴露） |
| 3 | careersite.tupu360.com positionTotal API | 低风险（未鉴权API） |
| 4 | test.bmw-brilliance.cn 暴露 B2B API 地址 | 低风险（信息泄露） |
| 5 | www.bmw-brilliance.cn robots.txt 泄露路径 | 低风险（信息泄露） |
| 6 | miniuc.bmw-emall.cn /api/user 503 | 信息型 |
| 7 | m-admin-* 管理后台公网可达 | 信息型 |

## 收敛工具链

### 1. 证据矩阵（Evidence Matrix）

对每条发现填写了7维矩阵（已确认事实/未确认推测/当前证据/证据缺口/可能评级/建议提交/厂商驳回理由）。

**关键决策节点**:
- Countly: JSON解析逻辑确认后端在处理数据，但无法确认是否持久化 → 标记为"需厂商协助确认"
- gateway-int: 全部微服务要求JWT token → 标记为"暂不提交"
- careersite: 公开招聘站点职位数量是非敏感信息 → "放弃"

### 2. 降噪判定

| 发现 | 降噪结论 | 判定依据 |
|------|---------|---------|
| robots.txt 路径 | **放弃** | 被禁止路径当前全部404 |
| 前端API地址 | **放弃** | API地址可能是公开信息 |
| 招聘系统positionTotal | **放弃** | 公开职位数量非敏感 |
| 管理后台公网可达 | **放弃** | WAF 403拦截全部请求 |
| /api/user 503 | **放弃** | 503不构成安全缺陷 |

### 3. 产出文件

| 文件 | 内容 |
|------|------|
| `evidence-matrix.md` | 7条发现的逐项证据矩阵（含厂商驳回理由预判） |
| `countly-integrity-report.md` | Countly 数据注入完整报告（可直接提交漏洞盒子） |
| `gateway-exposure-assessment.md` | gateway-int 公网暴露评估（结论: 暂不提交） |
| `final-submission-plan.md` | 全局提交计划 |

### 4. 最终决策

| 状态 | 数量 | 项目 |
|------|------|------|
| 立即可提交 | 0 | — |
| 需厂商协助确认 | 1 | Countly `/i` 端点（中危） |
| 需补充证据 | 4 | gateway-int 等待恢复、test 环境公开性、Swagger 绕过、.env 绕过 |
| 暂不提交 | 2 | gateway-int 网关暴露、m-admin 管理后台 |
| 建议放弃 | 5 | robots.txt、招聘接口、miniuc 503、whispers、bba 静态页 |

## 关键经验

1. **"公网可达 ≠ 漏洞"** — gateway-int 虽然通过 Istio Envoy 暴露了内部微服务路由，但全部要求 valid JWT token
2. **"盲Success ≠ 盲代理"** — Countly /i 端点的 Content-Type 区分和 JSON 解析行为证明后端在处理数据，盲返回不能解释400 vs 200
3. **"503 不能推断上线后行为"** — 503 Service Unavailable 不构成安全缺陷
4. **"放弃不是失败"** — 5条放弃意味着避免了5次被厂商驳回的低质量提交

## 复现材料清单

- 所有原始 curl 请求和响应已保存至 `/root/workspace/bbasrc/`
- Countly 唯一测试标识: `security_test_` 前缀的 app_key
- 未执行的操作：爆破、批量注入、认证绕过、数据读取、业务操作
