# Experience Replay

## 目标
遇到相似系统、相似信号、相似边界时，不再完全从零推理。

## 最小字段

```yaml
similar_case:
architecture_pattern:
signal_pattern:
recommended_first_check:
historical_outcome:
replay_confidence:
```

## 规则

- Replay 只影响先验和排序
- Replay 不直接生成 finding
- 若当前证据与历史案例冲突，以当前运行时为准

## 例子

- similar_case: SaaS export boundary failure
- signal_pattern: export / attachment / tenant_id / org_id / file_id
- recommended_first_check: object boundary first
