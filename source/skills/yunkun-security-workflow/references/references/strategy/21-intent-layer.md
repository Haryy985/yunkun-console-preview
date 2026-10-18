# Intent Layer

## 目标
Hunter 不只回答“这里像什么漏洞”，还要先回答：

- 攻击者为什么要走这条路？
- 为了达成目标，需要什么能力？
- 哪类信任边界最可能失效？
- 哪些接口/资源最值得先看？

## 结构

```yaml
attacker_goal:
required_capability:
possible_boundary_failure:
candidate_interfaces:
```

## 例子

### 读取他人数据
- attacker_goal: 获取超出自身权限的数据
- required_capability: 对象访问控制失败 / 数据范围过滤失败
- possible_boundary_failure: `对象边界 / 数据范围边界`
- candidate_interfaces: `detail / export / download / search`

### 提升操作权限
- attacker_goal: 执行本不该允许的状态变更
- required_capability: 身份边界失效 / 状态机缺口 / 服务信任错误
- possible_boundary_failure: `身份边界 / 状态边界 / 服务信任边界`
- candidate_interfaces: `approve / callback / worker / admin action`

## 规则

- Intent Layer 先于 Hypothesis Generator
- 没有 `attacker_goal` 的假设默认降权
- “参数存在”不是目标；“越过哪条边界以达成什么效果”才是目标
