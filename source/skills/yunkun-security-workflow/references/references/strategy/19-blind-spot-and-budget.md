# Blind Spot Detector And Exploration Budget

## Blind Spot Detector

结束前先问：

- 发现 `OAuth/OIDC`，是否检查了 `redirect_uri / state / callback trust`？
- 发现审批/流程，是否建了状态机？
- 发现上传/预览/转换，是否走完了完整链路？
- 发现 MQ/Webhook，是否检查了信任边界与重放？
- 发现 RAG/Agent，是否检查了 retrieval/tool/memory 边界？

任一答案为“否”，都标记 `coverage_gap`。

## Exploration Budget

- 默认总预算 `100`
- 模块预算按价值和成本动态分配
- 连续 `3` 次无新增事实：扣预算并冻结
- 进入 `primitive-confirmed` 或获得攻击链证据：回补预算
- 预算归零的分支进入 `frozen`

## Prerequisite Ledger

以下属于前提缺失，不得写成“系统安全”：

- 缺第二角色
- 缺第二对象
- 缺真实 callback
- 缺 worker 消费证据
- 缺 redirect 目标
- 缺审批链上下文
