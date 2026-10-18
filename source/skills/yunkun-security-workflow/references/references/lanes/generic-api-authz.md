# Lane · Generic API AuthZ

## 适用信号

- `/api/`、JSON、GraphQL、list/detail/profile/search/export
- 明显对象参数：`userId` / `studentId` / `fileId` / `taskId` / `orderId`
- 同一对象存在“列表 → 详情 → 编辑 → 导出”多条接口

## 先回答

1. 当前核心对象是什么
2. 当前身份对该对象理论上应有什么权限
3. 这是水平越权、垂直越权，还是公开数据误判
4. owner / tenant / dept / role 哪条边界最关键

## 优先顺序

1. `scripts/session_observer.py`
2. `scripts/unknown_surface_router.py`
3. `scripts/branch_manager.py`
4. `scripts/hypothesis_ledger.py`
5. `scripts/minimal_verify_gate.py`
6. 单对象 / 单变量差分 replay
7. `scripts/finding_gate.py`

## 高价值分支

- `object-authz-diff`
- `hidden-parameter-diff`
- `export-scope-diff`
- `tenant-boundary-check`

## 默认不做

- 不先 batch 枚举
- 不先 fuzz 所有参数
- 不先把公开列表直接当 IDOR
- 不先跳过“当前身份本来就能看”的对照组
