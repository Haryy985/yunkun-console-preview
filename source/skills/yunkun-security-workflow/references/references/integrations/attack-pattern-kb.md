# Attack Pattern KB

## 用途

不是漏洞库，而是“哪类地方最容易错”的本地模式库。

## 条目格式

```md
signal:
context:
suspect:
why_error_prone:
historical_mistakes:
priority:
next_minimal_probe:
```

## 典型模式

### export / download / report

- context: admin / user / order / data aggregation
- suspect: authorization flaw / data exposure / field overreach
- why_error_prone: 导出常复用后台查询，权限和字段过滤容易漏
- historical_mistakes:
  1. 忘记做对象级授权
  2. 导出范围比页面大
  3. 历史数据或隐藏字段一起下发
- priority: HIGH
- next_minimal_probe: 只验证“谁能导出谁的数据、导出字段是否超出前端可见范围”

### upload / preview / convert

- context: file flow / attachment / CDN / parser / thumbnail
- suspect: object boundary / parser abuse / stored content exposure
- why_error_prone: 上传链路经常分离为存储、预览、转换、回源四段
- historical_mistakes:
  1. 上传后访问权限继承错误
  2. 预览接口绕过对象归属
  3. 转换/解析组件暴露敏感内容
- priority: HIGH
- next_minimal_probe: 只验证“上传对象是否仍受原权限控制、预览/转换是否越界”

### share / invite / token

- context: file share / workspace invite / external access
- suspect: token lifecycle / binding / reuse
- why_error_prone: 分享链路容易把“方便访问”做成“过宽授权”
- historical_mistakes:
  1. token 可预测
  2. token 可重复使用
  3. token 绑定对象错误
- priority: HIGH
- next_minimal_probe: 只验证“token 是否绑定对象、身份、时效、次数”

### callback / webhook / queue

- context: async / producer-consumer / notification
- suspect: replay / trust / cross-tenant effect
- why_error_prone: 异步边界把验证和执行拆开，最易丢归属
- historical_mistakes:
  1. 回调来源未校验
  2. 任务重放影响他人
  3. 结果状态可被伪造
- priority: HIGH
- next_minimal_probe: 只验证“谁发起、谁消费、谁看到结果”

### api-key / gateway / upstream

- context: machine identity / service-to-service
- suspect: scope / audience / replay / upstream trust
- why_error_prone: 网关常把“能过签”误当“能做一切”
- historical_mistakes:
  1. scope 过宽
  2. audience 未校验
  3. 过期/重放边界缺失
- priority: HIGH
- next_minimal_probe: 只验证 canonical request 和 scope 是否严格收敛

### rag / agent / tool-use

- context: retrieval / memory / tool / function call
- suspect: cross-tenant retrieval / tool abuse / memory leak
- why_error_prone: “会回答”不等于“该访问”，工具调用与检索边界最易串线
- historical_mistakes:
  1. 检索过滤失效
  2. 工具权限跟会话不绑定
  3. 记忆/上下文污染
- priority: HIGH
- next_minimal_probe: 只验证“检索、工具、记忆是否与 tenant/session 绑定”

