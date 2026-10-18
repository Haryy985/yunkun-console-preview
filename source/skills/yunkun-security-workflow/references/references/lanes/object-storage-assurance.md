# 对象存储 / 存储桶专项 lane

## 何时加载

出现以下任一信号就加载本 lane：

- 前端或小程序运行时请求、JS、HAR、上传回包、附件 URL 中出现 `oss` / `cos` / `s3` / `obs` / `bucket` / `cdn` / `sts` / `policy` / `signature` / `callback`
- API 返回临时凭证、上传 policy、预签名 URL、回源/回调地址、对象 key、下载 token
- 静态资源域名与业务域名分离，且下载、预览、导出、上传或头像/附件明显走独立 host

## 最小模型

先用一行写清：

`provider | bucket-or-host | key-prefix | access-shape(public/presign/sts/proxy/upload) | object-sensitivity | owner-surface`

若这六项还没清楚，不要进入批量验证。

## 优先证据

1. 浏览器/小程序运行时网络记录
2. 上传/下载接口的 JSON 回包
3. JS/源码中的 storage client 配置
4. 真实对象 URL 与响应头
5. 回调/回源/缩略图/预览链路

桶名、CDN 域名、logo、公开 JS/CSS/营销图片默认只是 observation。

## 验证顺序

### 1. 运行时关联确认

证明对象存储 host/bucket 确实属于当前业务，而不是第三方公共资源或文档示例。

### 2. 只读边界差分

对同一对象家族做：

- baseline：已知对象
- variant：同前缀随机对象
- negative control：相邻前缀或随机对象

目标是区分：

- 本来公开的资源
- 不存在对象
- 真实未授权读取/列举

### 3. 预签名 URL

一次只改一个变量：

- object key
- method
- expiry
- prefix
- bucket
- required headers

同一轮必须有 negative control。仅 URL 可访问自己，不构成漏洞。

### 4. STS / 临时凭证

只验证最小越界边界：

- action 是否超出预期（读 / 写 / 删 / 列举）
- prefix 是否可逃逸
- bucket 是否可切换
- region / resource ARN 限制是否真实生效

没有真实越界动作时，只保留为 lead。

### 5. 上传 / 直传 / 回调 / 托管

只能在测试前缀、测试对象或操作者明确允许的路径下做验证。

重点看：

- 能否指定任意 key / prefix
- 能否覆盖现有对象
- 能否上传被真实业务回取并执行/渲染的主动内容
- callback / 回源是否可伪造、越权或打到非预期目标

所有写动作先过 `mutation_guard.py`，并记录 cleanup。

## Promotion 规则

- bucket host / CDN 域名 / 单个公开对象 / 200 静态资源 = `observation`
- 读/列举只有在读到非公开、非当前 actor、非预期对象时，才能升层
- 上传成功只有在形成越界写入、覆盖、托管主动内容或真实业务影响时，才能升层
- STS / presign 只有在证明范围、时效、动作或租户边界突破时，才可写 finding
- CORS 只有在跨站能读取受保护对象时，才有意义；公开对象跨站可读不算 finding

## 噪声控制

- 不做大轮次对象字典爆破
- 不围绕 bucket 名称做盲猜
- 同一 bucket/prefix 两轮无新增事实就冻结
- 没有测试前缀时默认只做只读验证
