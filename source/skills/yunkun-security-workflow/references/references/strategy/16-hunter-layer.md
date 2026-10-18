# Hunter Layer · 信号排序与攻击路径图

## 目标

把“看起来可疑的地方”快速变成可验证候选，而不是平均铺开测试面。

## 输入

- 运行时请求/响应、JS/HAR、路由表、对象流、文件流、导出流、消息流、产品名、版本号、CVE/CNVD、控制面、AI/Agent 链路。

## 输出

1. `signal`
2. `why_error_prone`
3. `ranked_hypotheses`
4. `attack_path_graph`
5. `next_minimal_probe`

## 排序维度

- 对象密度
- 权限密度
- 状态密度
- 聚合出口
- 异步链
- 导出链
- 分享链
- 预览链
- 转换链
- 回调链
- 机器身份链
- 跨租户链

## 反向推理

先从业务目标反推开发者设计，再反推常见失误：

`业务目标 -> 实现路径 -> 可能复用点 -> 可能遗漏点 -> 最小验证`

## 攻击路径图

模板：

`actor -> entry -> transform -> storage/queue/parser -> exposure/effect`

示例候选边：

- `upload -> storage -> preview`
- `share -> token -> download`
- `export -> aggregation -> file`
- `invite -> code -> membership`
- `api-key -> gateway -> upstream`

## 纪律

- Hunter 不验证，不写结论，只做候选排序。
- 每个候选都要给出“为什么容易出错”和“什么证据会推翻它”。
- 前 3 个候选进入验证器；其余保持冻结。

