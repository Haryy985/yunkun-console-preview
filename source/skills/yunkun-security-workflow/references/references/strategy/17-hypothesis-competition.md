# Hypothesis Competition

## 目标
让 Hunter 不只是“列候选”，而是明确回答：

- 为什么现在先测 A，不先测 B？
- 如果 A 失败，谁接班？
- 什么证据会让排序发生变化？

## 最小结构

```json
{
  "target": "/api/order/detail?id=",
  "hypothesis_pool": [
    {
      "name": "BOLA",
      "score": 87,
      "reason": ["object identifier", "user-owned resource", "read operation"],
      "cost": "low",
      "status": "active",
      "disconfirming_evidence": ["same actor only", "strict object scope", "negative control matches"]
    }
  ]
}
```

## 评分

基础分：

`Hypothesis Score = Exploitability × Business Impact × Exposure × Confidence ÷ Validation Cost`

调度分：

`final_score = base_score - dead_attempt_penalty - blocked_penalty + evidence_bonus + chain_bonus`

## 规则

- 同一轮只允许 `1` 个 `active_hypothesis`
- 连续失败要降权，不允许空转霸榜
- 新证据、攻击链价值、低成本高收益路径优先
- 没有 `disconfirming_evidence` 的候选不得进入验证器
