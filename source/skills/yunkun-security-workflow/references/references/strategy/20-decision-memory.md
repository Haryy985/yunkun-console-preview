# Decision Memory

## 目标
失败验证也要产出知识，让 Hunter 越测越聪明。

## 最小结构

```json
{
  "signal": "export/report",
  "chosen_hypothesis": "authorization flaw",
  "rejected_hypotheses": ["SQLi"],
  "reason": ["aggregation endpoint", "high business value"],
  "outcome": "negative",
  "negative_evidence": ["auth checked", "object scope enforced", "role verified"],
  "posterior_shift": {
    "export_idor": -0.3,
    "export_field_overreach": 0.1
  }
}
```

## 规则

- 记录为什么当时选这个方向
- 记录什么证据推翻了它
- 记录下次同类信号该优先怀疑什么
