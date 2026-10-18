# Dependency Graph

## 目标
把 `Prerequisite Ledger` 从“阻塞列表”升级成“依赖图”。

## 结构

```yaml
validation_target:
requires:
  - actor_b
  - object_b
  - permission_comparison
missing:
  - actor_b
blocked_at:
next_needed_step:
```

## 例子

### BOLA 验证
- validation_target: `BOLA verification`
- requires:
  - actor A
  - actor B
  - object A
  - object B
  - permission comparison
- missing:
  - actor B
- blocked_at: `identity-dependent validation`
- next_needed_step: `obtain controlled second actor or pivot to object-range differential`

## 规则

- 缺依赖只代表 `blocked`
- 不代表“系统安全”
- Hunter/Validator 要根据依赖图自动规划“下一步缺什么”
