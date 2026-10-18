# Lane · SPA / JS / API Mining

## 适用信号

- `<div id="app">` / `<div id="root">`
- `main.js` / `chunk` / `remoteEntry` / Module Federation
- 真实业务 API 藏在前端资源和异步请求里

## 先回答

1. 当前响应是前端壳、统一拦截页，还是真实后端响应
2. JS 暴露的是 API path、对象键、tenant 信息，还是仅仅是前端常量
3. 哪些接口是列表、详情、导出、上传、配置、审批
4. 哪个对象键最适合做单步对照

## 优先顺序

1. `scripts/session_observer.py`
2. `scripts/unknown_surface_router.py`
3. 正常抓一轮 XHR / fetch
4. 抽 API base / path / object key
5. `scripts/branch_manager.py`
6. `scripts/hypothesis_ledger.py`
7. `scripts/minimal_verify_gate.py`

## 高价值分支

- `js-api-topology`
- `shell-vs-backend-diff`
- `object-authz-diff`
- `config-or-admin-api-check`

## 默认不做

- 不先对每个 JS 路径都深挖
- 不先把整棵 API 树批量 fuzz
- 不先把单个 key 暴露直接当高危
