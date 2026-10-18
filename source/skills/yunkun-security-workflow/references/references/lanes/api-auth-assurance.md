# Lane: API And Authentication Assurance

## 适用信号

- REST、GraphQL、WebSocket、移动端或小程序 API 与登录、注册、找回、SSO、JWT、OAuth、SAML、session cookie 同时出现。

## 状态机先于探针

1. 标出身份建立、挑战、凭证签发、刷新、注销、恢复与权限加载的真实顺序。
2. 为每个受保护端点记录认证工件来自哪里、由浏览器还是客户端显式携带、何时失效。
3. 认证链只保留一个执行面 owner；具体规则以 `SKILL.md` 2.19 为准。
4. 先建立一个正常业务 baseline，再验证一个最小变量和一个 negative control。

## 高价值模块顺序

1. 身份绑定与会话生命周期。
2. 对象/租户/角色授权边界。
3. 恢复、邀请、设备绑定、导出和高价值状态变更。
4. API 数据契约、隐藏字段、方法差异与协议边界。
5. CORS、缓存、错误页与速率策略的真实受保护响应。

## 证据门槛

- BOLA/IDOR 必须完成 own / other / nonexistent 三联验证，且只改对象标识。
- 认证或账户接管相关结论必须证明凭证、会话或绑定状态真的改变；页面跳转、header 差异或 token 外观不够。
- API schema、Swagger、客户端 JS 和运行时请求只产生候选端点；必须用当前身份和负对照复核。
- 遇到验证码、429、403、challenge 或 reset 时，执行 `SKILL.md` 2.19 与 2.20，不跨执行面继续重试。

## 按需加载

- REST/GraphQL/WebSocket -> `playbooks/api-rest/00-index.md`。
- OAuth/OIDC/SAML/JWT -> `playbooks/oauth-saml-jwt/00-index.md`。
- 业务状态/审批 -> `generic-business-logic.md`。
