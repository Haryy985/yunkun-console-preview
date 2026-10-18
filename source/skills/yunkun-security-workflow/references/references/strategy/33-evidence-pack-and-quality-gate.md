# 33 · SRC 证据包与质量门禁

## 证据包最小集合

- `scope proof`
- `actor proof`
- `object ownership proof`
- `baseline request/response`
- `variant request/response`
- `negative control`
- `final business state`
- `impact evidence`
- `replay steps`
- `remediation pointer`

## 质量门禁

- `PASS`
- `NEEDS_MORE`
- `DOWNGRADE`
- `DUPLICATE_RISK`
- `BLOCKED`

## 强制降级

以下情况必须降级或阻塞：

- 只能证明接口可达
- 不能证明对象归属
- 没有最终状态证据
- 只有截图没有原始请求
- 只有差异没有影响
- 影响描述大于已验证能力

## 原始证据纪律

- 非 observation 记录必须指向真实 raw artifact
- 不用“关键突破”代替状态
- 不做过度脱敏，除非平台或用户明确要求
