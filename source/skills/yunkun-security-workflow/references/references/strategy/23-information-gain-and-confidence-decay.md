# Information Gain, Confidence Decay, And Budget

## Information Gain

预算调度不只看“要花多少时间”，还看“能带来多少信息增益”。

### 推荐排序

`Priority = Boundary Value × Expected Information Gain ÷ Validation Cost`

## Confidence Decay

高分假设不是永久高分。

### 典型触发
- 发现统一鉴权中间件
- 负对照一致
- object scope 严格生效
- role 比较结果稳定
- 同类接口已被负证据覆盖

### 结果
- 降低当前假设分数
- 回写 posterior update
- 降低未来同类接口的起始优先级

## Budget

- 默认总预算 `100`
- 连续 `3` 次无新增事实：扣预算并冻结
- 进入 `primitive-confirmed` 或获得链路证据：回补预算
- 低增益动作自动降权
