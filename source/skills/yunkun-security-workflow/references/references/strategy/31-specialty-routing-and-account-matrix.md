# 31 · SRC 专项分流与账号矩阵

## 专项分流

不要按漏洞名平均覆盖。按四项打分后选择一个主专项：

- `signal`
- `impact`
- `reproducibility`
- `reportability`

优先命中的专项：

- auth / sso
- bola / idor / tenant
- payment / coupon / refund
- export / report / batch
- upload / preview / convert
- graphql / api gateway
- object storage / sts / ak-sk
- webhook / queue / worker
- ai / rag / agent

## 矩阵先于结论

命中以下信号时，先建矩阵再下判断：

- `userId / tenantId / orgId / role / fileId / orderId / export / approval`

## 最小矩阵

```text
actor_a / actor_b
role_a / role_b
tenant_a / tenant_b
object_a / object_b
baseline / variant / negative
```

## 升级条件

- 只改一个维度
- 能证明对象归属
- 能证明最终业务状态
- 能区分 `read / write / export / state-change`

缺任一项，只能停在 `lead` 或 `primitive-confirmed`。
