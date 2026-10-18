# Lane · Admin Control Plane

## 适用信号

- `/admin` / `/console` / `/manage` / `/system`
- 角色管理、租户管理、配置中心、审计中心、运维面板
- 高权限按钮、危险开关、维护接口、批量操作

## 先回答

1. 这是普通业务后台，还是系统控制面
2. 当前身份理论上拥有哪些角色和动作
3. 危险动作是读、写、发版、导出，还是用户/角色变更
4. 权限边界按 role、tenant、dept、project 还是 environment 切

## 优先顺序

1. `scripts/session_observer.py`
2. `scripts/unknown_surface_router.py`
3. `scripts/branch_manager.py`
4. `scripts/hypothesis_ledger.py`
5. `scripts/minimal_verify_gate.py`
6. 单步验证角色边界或配置写路径

## 高价值分支

- `role-boundary-check`
- `config-write-path`
- `maintenance-endpoint-check`
- `tenant-scope-diff`

## 默认不做

- 不先全后台爆枚
- 不先跑高危配置修改
- 不先把“能看见按钮”当成“能执行动作”
- 不先忽略二次确认、审批链和审计字段
