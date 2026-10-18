# 32 · SRC 业务链模式与链路验证

## 目的

把多个弱信号串成同一条攻击链，不拆碎凑洞。

## 高频业务链

- `register -> login -> reset/bind`
- `list -> detail -> export/download`
- `upload -> storage -> preview -> convert -> download`
- `order -> pay -> callback -> entitlement`
- `submit -> approve -> state transition -> downstream access`
- `tenant switch -> object read/write -> admin effect`
- `public entry -> webhook/queue -> worker -> side effect`

## Hunter 输出

```text
signal:
attacker_goal:
candidate_path:
weakest_edge:
next_minimal_probe:
```

## 链路验证规则

- 先证明链存在，再证明哪条边失效
- 一次只打一个最弱边
- 先拿最小影响，不追求一步到位
- 链断了就回到上一个可信节点，不补脑

## 不是漏洞

- 单个公开导出入口但无越权
- 单个上传入口但无访问控制/解析/转换影响
- 单个回调参数可见但无状态影响
