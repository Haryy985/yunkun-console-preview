# Lane · 机器身份 / API Key / AK-SK / HMAC

## 适用信号

- `x-api-key`、`Authorization: ApiKey/Bearer`、`accessKey`、`secretKey`、`signature`、`sign`、`timestamp`、`nonce`、`canonical request`、`HMAC`、`STS`、`service account`、`client_id/client_secret`、网关签名 SDK、前端直传凭证或服务间调用凭证。

## 先建模什么

- `credential-shape`: header、query、body、cookie、临时 token、STS、双 token。
- `canonical request`: method、path、query、headers、body hash、timestamp、nonce 的签名范围。
- `scope`: action、resource、prefix、tenant、audience、环境、过期时间。
- `replay boundary`: 时间窗、nonce 重放、body/path/header 改动后的验签行为。
- `owner`: 该凭证来自用户提供测试身份、运行时临时下发、公开 JS 配置，还是文档示例。

## 最小验证链

1. 只使用操作者提供或运行时正常下发的测试凭证做 baseline；凭证只脱敏记录。
2. 单变量验证 scope：只改 path、object、method、tenant、body 或时间参数中的一个。
3. 加 negative control：无签名、过期时间、错误 nonce、错误对象或错误 action。
4. 判定业务边界是否被越过：跨资源、跨租户、越权 action、超时效、可重放或服务间 trust 混淆。
5. 在 `adversary-emulation` 且 ROE 明确允许时，可对 in-scope 的候选 key/签名材料做有界验证；不能把 key 外观或单次 `200` 当 finding。

## 高价值分支

- 前端暴露临时凭证：验证是否被限制在测试前缀、动作、时效和租户内。
- API 网关签名：验证 path/method/query/header 是否都进入 canonical request。
- M2M token：验证 audience、issuer、tenant、role、service scope 是否被实际校验。
- 回调/直传：验证 callback URL、object key、content-type、policy 条件与业务取回边界。

## 证据与停止

- 原始证据必须脱敏：只保留 key 前后缀、hash、过期时间和 scope 摘要。
- 明文 credential 可在隔离运行时短暂用于验证；报告、命令行、共享日志和 finding 只保留脱敏引用，禁止外部导出。
- 连续两次只证明“签名错误被拒绝 / 正常签名可用”，冻结；这说明边界正常，不是漏洞。
