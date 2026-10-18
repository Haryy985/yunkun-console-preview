# Lane · 云原生与服务治理控制面

## 适用信号

- Nacos、Eureka、Consul、Actuator、K8s API、Docker Registry、Harbor、Envoy、Kong、APISIX、Ingress、Service Mesh、`/actuator`、`/v2/_catalog`、`/nacos`、`/eureka`、`/consul`、`/apisix`、`/k8s`、`/.well-known` 配置或网关错误页。

## 先建模什么

- `control-plane role`: 注册中心、配置中心、健康检查、镜像仓库、API 网关、服务网格、管理面。
- `application linkage`: 当前应用是否通过运行时请求、JS、配置、跳转、错误页、证书、网关 header 或操作者测试链路关联到该控制面。
- `identity`: 匿名、用户会话、管理员、service account、mTLS、网关 token。
- `asset boundary`: namespace、tenant、service、route、upstream、registry project、environment。

## 最小验证链

1. 先证明与当前应用的真实链路；只有 banner/端口/标题时只记 observation。
2. 只做低噪声只读 baseline：健康检查、元信息、公开路由或当前服务相关配置。
3. 单变量验证：namespace、service、route、project、environment 或认证状态。
4. negative control：不存在服务、无认证、错误 namespace、错误项目。
5. 只有读到非公开配置、越过 namespace/tenant、访问受保护路由、修改路由/配置或影响服务状态时，才升级。

## 高价值分支

- Nacos/Consul/Eureka：服务列表是否暴露内部服务拓扑、配置是否越租户/越环境读取。
- Actuator：是否暴露敏感 env/config/heapdump/logfile 或管理操作；`health/info` 默认 observation。
- K8s/Docker Registry：namespace/project、镜像 tag、token scope、仓库列举边界。
- Envoy/Kong/APISIX：路由、upstream、plugin、consumer、credential、admin API 与业务网关边界。

## 证据与停止

- 不把公网可达、版本号、登录页、默认 401/403、健康页当漏洞。
- 不横向扫同单位控制面；只围绕当前应用链路或操作者给定范围。
- 涉及配置修改、路由变更、镜像写入时必须先过状态变更守卫并有 cleanup；缺失即只读。

