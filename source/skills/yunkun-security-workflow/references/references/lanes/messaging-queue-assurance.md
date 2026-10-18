# Lane · 消息队列与异步处理

## 适用信号

- RabbitMQ、Kafka、RocketMQ、topic、queue、exchange、consumer、producer、delayed job、event id、callback、webhook、任务状态轮询、导入导出后台处理、日志中出现消息投递或消费痕迹。

## 先建模什么

- `event chain`: `actor -> producer endpoint -> topic/queue -> consumer -> object/state/effect`。
- `message identity`: event id、trace id、tenant、owner、object id、幂等 key、签名/校验字段。
- `state transition`: queued、running、done、failed、retry、cancelled。
- `visibility`: 任务状态、结果文件、日志、回调、通知。

## 最小验证链

1. 先从真实业务入口触发或观察一个任务 baseline；不要直接枚举 broker。
2. 只改一个变量：object id、tenant、callback、event id、幂等 key、状态查询 id 或结果文件 id。
3. negative control：不存在任务、他人任务、错误 tenant、错误签名或已完成任务。
4. 观察最终状态：结果是否跨对象/跨租户可见，任务是否越权执行，回调是否被错误信任，重试/取消是否影响他人任务。
5. 没有 consumer 侧效果或结果可见性证据时，只能保持 `lead`。

## 高价值分支

- 导入/导出：结果文件归属、下载 token、过期时间、任务 id 可预测性。
- Webhook/callback：签名、来源、重放、tenant 与对象绑定。
- 延迟/重试：幂等 key、取消/重放、状态竞争是否越过 owner。
- MQ 管理面：只有证明与当前应用链路并且越过访问边界时才测试；登录页/banner 不是漏洞。

## 证据与停止

- 不向未知 broker 直接投递消息，不枚举 topic，不批量触发任务。
- 状态改变必须有 before/after 和 cleanup；无法清理时记录 `cleanup_required`。
- 连续两次只有正常排队/失败/认证阻断，冻结该任务族。

