# Authority Propagation Model

## 目标
分析权限如何在多段链路中传播、放大或解绑。

## 最小字段

```yaml
user_identity:
agent_identity:
tool_identity:
backend_identity:
effective_privilege:
authority_escalation_point:
```

## 核心问题

- 用户是谁？
- Agent 以谁的身份执行？
- Tool 调用用谁的身份？
- Backend 最终认谁？
- 哪一层发生了权限提升或默认高权？

## 适用范围

- AI / Agent
- OAuth delegated access
- STS / service account
- API gateway -> upstream
- webhook / worker / callback delegated execution
