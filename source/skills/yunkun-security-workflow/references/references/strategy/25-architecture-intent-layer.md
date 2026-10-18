# Architecture Intent Layer

## 目标
在判断“哪条边界可能坏”之前，先理解：

- 这个组件为什么存在？
- 它按设计本应如何工作？
- 它依赖什么信任假设？
- 最可能以什么方式失效？

## 最小字段

```yaml
component:
purpose:
expected_behavior:
trust_assumption:
failure_mode:
```

## 例子

### MQ
- purpose: 削峰 / 解耦 / 延迟任务
- expected_behavior: 异步消费但不改变原授权语义
- trust_assumption: consumer 正确继承任务身份或作用域
- failure_mode:
  - 重放
  - 重复消费
  - 状态不一致
  - 权限继承错误

### Agent Tool Chain
- purpose: 帮用户间接访问数据或执行动作
- expected_behavior: tool 权限不超过用户权限
- trust_assumption: Agent/Tool/Backend 权限传播正确绑定
- failure_mode:
  - 默认高权
  - tool 身份解绑
  - 跨会话数据串线
