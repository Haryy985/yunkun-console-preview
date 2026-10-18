# Lane · API 与微服务拓扑

## 适用信号

- 响应、JS、HAR、OpenAPI/Swagger/Knife4j、GraphQL endpoint、`.proto`、gRPC-Web、HTTP/2、`application/grpc`、`x-grpc-web`、`/api/`、`/rpc/`、`/graphql`、`/v1/`、`/v2/`、网关路由或服务名前缀。
- 目标主要由接口驱动，页面只是 SPA 壳、移动端/小程序壳或管理前端。

## 先建模什么

- `transport`: REST / JSON-RPC / GraphQL / gRPC / gRPC-Web / WebSocket / SSE。
- `route family`: `host + path-prefix + method + auth-shape + object-shape`。
- `actor/object/state`: 当前调用者、对象标识、状态字段、业务动作、返回敏感度。
- `schema source`: 运行时请求优先，其次 OpenAPI/schema/descriptor，最后 JS/源码候选。
- `gateway boundary`: 网关是否统一鉴权、是否有内部服务前缀、是否存在多版本或多租户 header。

## 最小验证链

1. 取一条运行时真实调用作为 baseline，保存原始请求/响应。
2. 只改一个变量：对象 ID、方法、版本、租户 header、字段选择、分页范围或状态参数。
3. 加 negative control：不存在对象、无权限对象、错误类型字段或空认证。
4. 比较业务码、认证状态、对象归属、状态变化和数据敏感度，而不是只比较 HTTP status。
5. 若只有 schema/descriptor 暴露，先记 observation；只有真实越过授权、对象、字段、方法或状态边界才升级。

## 高价值分支

- REST/OpenAPI：影子端点、调试字段、对象详情/导出/批量接口、版本差异、`admin/internal` 前缀是否仍受同一授权边界保护。
- GraphQL：字段级授权、query/mutation 权限差异、嵌套对象越权、批量别名导致的对象边界混淆；Introspection 暴露本身仅 observation。
- gRPC/gRPC-Web：方法级授权、metadata 中身份/租户/签名边界、proto 字段默认值与业务状态机差异；能列出方法不等于漏洞。
- HTTP/2 / 网关：协议转换后的 header、path、method、host 和 tenant 一致性；只验证运行时相关族，不逐条打全量路由。

## 证据与停止

- 每条候选必须落：baseline、variant、negative-control、原始响应、差异解释。
- 同一路由族连续两次只有认证失败、公开数据或无业务差异，冻结该族。
- 无法关联到真实业务状态机时，保持 `observation/lead`，不要写成“API 漏洞”。

