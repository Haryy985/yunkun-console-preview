# Active-Led Execution

## 1. 定义

clown21 的默认执行方式是主动验证主导。被动侦察不是独立阶段，而是下一次主动动作的输入。

```text
supporting observation
-> attackable boundary
-> falsifiable hypothesis
-> baseline
-> one-variable variant
-> negative control
-> impact decision
-> cleanup
```

没有进入这个链条的信息收集，最多连续做两步。

## 2. 15 分钟冲刺模板

```text
00:00-01:30  target lock、scope、单次 baseline、识别入口和认证现实
01:30-04:00  主动探针 1：最高价值认证/对象/状态边界
04:00-07:00  主动探针 2：baseline/variant/negative-control 差分
07:00-10:00  主动探针 3：换一条独立分支，避免锚定
10:00-12:30  对最强信号做 impact validation
12:30-14:00  cleanup、复测、证据完整性检查
14:00-15:00  cycle 收口；到点即 timebox-expired，不扩 scope 补故事
```

短周期默认不允许把一半时间花在重复读 HTML、重复跑 cycle/report 或继续补指纹。

## 3. Web/API 主动优先级

根据现场形态选最短高价值边界：

| 现场信号 | 第一主动动作 | 负对照 |
|---|---|---|
| 登录/注册/重置 | 改一个身份、token、步骤或必填字段 | 随机身份或过期 token |
| list/detail | 当前对象和另一个对象做单 ID 差分 | 不存在 ID |
| export/download | 当前 actor 请求一个明确对象 | 无权限对象或随机对象 |
| workflow | 跳一个状态、重放一次动作 | 合法顺序请求 |
| file/path | 单次归一化或对象权限差分 | 不存在文件/合法路径 |
| URL/callback/import | 单次受控外连或重定向差分 | 不可达受控地址 |
| admin/control plane | 未登录或低权访问一个明确操作 | 正常公开页面 |
| SPA | 从已加载 JS 抽一个真实 API 并立即请求 | 随机不存在路由 |

## 4. 主动动作记录

每次 cycle 必须尽量提供：

```bash
python3 ~/.claude/hooks/clown21_controller.py cycle \
  --activity-kind active-validation \
  --probe-goal "验证订单对象是否校验当前用户所有权" \
  --negative-control completed \
  --artifact-path ./artifacts/<run-id>/evidence/http/order-idor-diff.http
```

原始 evidence 至少包含：

- 完整方法、URL、非敏感请求头和脱敏认证引用
- 请求体或参数
- 完整状态行、响应头和响应体
- baseline、variant、negative control 的关联 ID
- 时间、actor、对象和实际状态变化

## 5. 何时允许批量化

同时满足才允许：

1. scope 明确；
2. 单点原语已复现；
3. 目标能承受预定速率；
4. WAF/ban budget 正常；
5. 批量结果仍逐项保留证据；
6. 用户没有限制为单请求、单对象或短周期。

批量化用于扩大已确认原语的覆盖，不用于替代假设。

## 6. 禁止的伪主动

- 为清空 blindspot 创建没有测试用途的账户。
- 用两个同角色账户冒充 cross-role coverage。
- 手写 session pool、summary、coverage 或 findings 让 gate 通过。
- 把连续 GET 页面、指纹和目录列表都计成 impact validation。
- 成功状态变化后不保存响应、不 cleanup。
- 时间盒到期后继续扩 scope，只为了生成一份绿色报告。

## 7. 成功标准

主动执行不是“请求数量更多”，而是单位时间内得到更多决定性事实：

- 一个假设被证实或证伪；
- 一个 auth/object/state 边界被明确；
- 一个 lead 被升级或丢弃；
- 一个持久状态被完整清理；
- 一个分支停止消耗预算并切换到更高价值分支。
