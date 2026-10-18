# Lane · Generic Business Logic

## 适用信号

- 审批、核销、领取、支付、试用、报名、库存、资格、验证码
- 明显状态流、一锤子买卖、异步任务
- 提交 / 撤回 / 审核 / 重试 / 并发相关动作

## 先回答

1. 核心对象是什么
2. 状态如何流转
3. 哪个动作是一次性动作
4. 单次 replay 能不能先出结果
5. 角色边界和状态边界哪一个更像主问题

## 优先顺序

1. `scripts/session_observer.py`
2. `scripts/unknown_surface_router.py`
3. `scripts/branch_manager.py`
4. `scripts/hypothesis_ledger.py`
5. `scripts/minimal_verify_gate.py`
6. 先做最小状态差分，再考虑 race / replay / parallel

## 高价值分支

- `state-transition-gap`
- `actor-boundary-gap`
- `job-ownership-check`
- `callback-result-boundary`

## 默认不做

- 不先并发
- 不先爆量
- 不先把“返回 success=true”当漏洞
- 不先把二阶段动作和一次性动作混在一起看
