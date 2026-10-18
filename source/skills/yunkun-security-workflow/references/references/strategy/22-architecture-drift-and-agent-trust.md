# Architecture Drift Detector And Agent Trust Boundary Model

## Architecture Drift Detector

### 目标
发现“系统模型之外的新形态”，解决 Unknown Unknown。

### 典型漂移信号
- `callback_url`
- `notify`
- `webhook`
- `consumer`
- `worker`
- `presign`
- `sts`
- `tool`
- `memory`
- `redirect_uri`
- 未知外连 host / 新协议 / 新异步边

### 规则
- 漂移一旦出现，先视为新拓扑或新信任边界
- 先补建模，再决定是否加载新 playbook
- 不允许把 `webhook`、`worker`、`tool`、`memory` 只当普通字段略过

## Agent Trust Boundary Model

### 目标
AI / Agent 系统重点不是 prompt 本身，而是：

`模型 -> 工具 -> 数据 -> 动作`

### 最小建模字段

```yaml
model_identity:
effective_privilege:
tool_scope:
memory_scope:
retrieval_scope:
cross_session_risk:
```

### 重点问题
- 模型是否拥有超出用户权限的能力？
- tool 权限是否和用户/租户绑定？
- memory 是否持久并可能串线？
- retrieval 是否跨越文档、租户或会话边界？
