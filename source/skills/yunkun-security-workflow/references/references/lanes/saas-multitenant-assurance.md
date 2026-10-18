# Lane · SaaS Multitenant Assurance

## 适用信号

- tenant、workspace、organization、team、project、member、department、environment 或跨组织邀请。

## 先画隔离边界

记录身份所属 tenant/workspace、对象所属 tenant、切换上下文的位置、请求内的 tenant/project 字段以及导出、任务、附件和订阅的最终归属。切换 UI 或 cookie 本身不是越权证据。

## 高价值分支

- `tenant-object-consistency`: 列表、详情、搜索、导出、附件和异步结果是否同样校验租户归属。
- `membership-lifecycle-boundary`: 邀请、加入、移除、角色变更后旧会话和令牌是否及时收敛。
- `cross-workspace-token-scope`: API token、下载链接、webhook、订阅和分享链接是否限制到正确 workspace。
- `admin-delegation-boundary`: 组织管理员、项目管理员与普通成员的配置和数据权限是否一致。

## 最小验证

有两个已授权测试 tenant 时，使用同一接口的 own / other / nonexistent 对照；没有第二 tenant 时，不把单租户正常行为写成隔离已验证。
