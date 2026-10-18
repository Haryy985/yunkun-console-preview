# Lane · CAS / SSO / Service Ticket

## 适用信号

- `cas/login` / `sso/login` / `oauth/authorize`
- `service=` / `ticket=` / `execution=`
- 明显的网关页、票据体系、下游 service 跳转

## 先回答

1. 当前系统是认证网关本体，还是被网关保护的下游系统
2. `service -> gateway -> service参数` 的关系是什么
3. 票据、state、remember-me、cookie、localStorage 哪些在参与
4. 有没有新 host / 新 system 因跳转被引入，它是否是当前认证链的直接运行时依赖

## 优先顺序

1. `scripts/web_target_preflight.py`
2. `scripts/service_probe_matrix.py`
3. `scripts/session_observer.py`
4. `scripts/branch_manager.py`
5. 如明确是标准 CAS REST，再考虑 `cas_rest_login.py` / `cas_service_ticket.py`

## 高价值分支

- `auth-boundary-map`
- `ticket-state-replay`
- `gateway-native-surface`
- `downstream-service-diff`

## 默认不做

- 不先把登录页当普通业务页 fuzz
- 不把搜索、字典或证书发现的下游 host 当作当前认证链；只继续当前跳转、运行时请求或认证参数明确引入的直接依赖
- 不先暴力重放整个登录流程
- 不先把一次跳转成功当成拿到下游访问权
