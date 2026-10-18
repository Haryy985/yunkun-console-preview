# Lane · WebVPN / Proxy / Internal Pivot

## 适用信号

- `webvpn` / `vpn` / `proxy` / `gateway`
- 下游系统通过网关跳转或代理访问
- 页面、cookie、header、service 参数表现出“壳 + 下游”的结构

## 先回答

1. 当前看到的是网关壳，还是下游系统真实页面
2. 下游 host 是真实业务资产，还是网关虚拟映射
3. 当前请求中哪些头、cookie、token、service 参数在控制下游访问
4. 新出现的 host / service 是否由当前跳转、运行时请求或响应明确引入为直接依赖

## 优先顺序

1. `scripts/web_target_preflight.py`
2. `scripts/service_probe_matrix.py`
3. `scripts/session_observer.py`
4. `scripts/branch_manager.py`
5. 单步验证一个下游对象或一个网关信任点

## 高价值分支

- `gateway-downstream-diff`
- `proxy-header-trust`
- `downstream-object-boundary`
- `direct-dependency-evidence`

## 默认不做

- 不把搜索、字典或证书发现的下游资产自动并入当前认证链
- 不先对整段内网开扫
- 不先把网关返回当成下游真实结果
- 不先把一次代理成功当成持久访问能力
