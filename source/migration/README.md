# 云鲲旧安装一次性迁移

仓库：https://github.com/Haryy985/yunkunsec。

本目录独立于生产运行代码。这里保留旧格式的明文标识，以便读取历史数据；
生产后端和前端只使用新标识。本工具默认只检查，尚未对本机业务数据执行迁移。
新安装无需迁移。旧安装不能直接用新版本覆盖启动。

## 切换顺序

1. 暂停任务，停止应用和所有 Worker；备份数据库、原密钥目录、transcripts 和原运行版本。
2. 迁移环境变量；检查实际启动进程的环境。保留现有 JSON 配置的连接值，不替换真实数据库名称。
3. 对同一数据库检查并迁移两个代理来源表的密文，以及 RAG 文档 MIME。
4. 检查并迁移现有 transcripts；保存原文件备份。
5. 在原页面 origin 迁移浏览器偏好，清除旧登录态。新管理员用户名为 `yunkun`，密码不变；数字 ID 的多用户账号不改名。
6. 用新版本启动后检查来源解密、历史文档、Worker 会话、登录和业务流程，再开放任务。

数据库和文件迁移是两个独立操作，不保证跨资源原子提交。失败时保持服务停止，
查明是否部分完成；检查操作可重复执行。回退旧版本必须同步还原数据库字段和
transcripts，不能让旧程序读取已改成新格式的代理密文。

## 数据库：代理凭据与文档类型

在本目录构建：`go build -trimpath -o yunkun-migrate.exe .`。
仅通过环境变量 `YUNKUN_MIGRATION_DSN` 提供待迁移连接，避免将密码放进命令行。
务必使用原有的 `proxy-pool.key`；工具不会生成密钥。通常密钥位于应用私有密钥目录，
实际路径以原安装为准。

```powershell
# 以下路径为示例，先替换为实际私有目录；默认只检查。
.\yunkun-migrate.exe -mode database -key C:\Private\proxy-pool.key
.\yunkun-migrate.exe -mode database -key C:\Private\proxy-pool.key -apply -service-stopped -backup C:\Private\db-brand-before.json
```

检查 `proxy_pools` 和 `proxy_discovery_sources` 的所有非空 credentials。
旧 AES-GCM 密文用原密钥解密，以新 AAD 和随机 nonce 重新加密；保留原 JSON
载荷和未知字段。已是新格式则跳过。错误密钥、损坏密文或缺表会在写入前失败。
备份记录前后密文和文档类型，不含解密凭据，但仍应作为敏感数据保管。
Windows 权限以私有目录 ACL 为准，Go 的 0600 不能代替 ACL。

全部更新在一个可串行化事务内完成，按 ID 和旧值条件更新；出现冲突则回滚。
备份使用新文件独占创建，写入失败不会更改数据库。提交失败时先检查实际值再重试。
备份不替代完整数据库备份。回退时在停机状态按备份的 table/column/id 恢复 before
值，并核对实际值仍等于 after；发生其他业务改动时不得盲目覆盖。

只修改旧抽取 MIME 的类型部分，保留参数；不改文档正文、切片、向量和任务证据。
数据库原有 updated_at 触发器可能更新行的修改时间。

## Worker 会话文件

```powershell
.\yunkun-migrate.exe -mode transcripts -root C:\Private\data\transcripts
.\yunkun-migrate.exe -mode transcripts -root C:\Private\data\transcripts -apply -service-stopped -backup C:\Private\transcripts-brand-before
```

只处理 `.jsonl` 文件中 user 消息的顶层 text 块里的有效 Worker 标记。
不替换 assistant 文本、嵌套工具结果、证据正文或其他任意品牌字样。
保留未知 JSON 字段、记录 ID 和用量；改动记录的 JSON 排版可能变化。
未改动记录逐字节保留。单行限额为 64 MiB，畸形 JSON 和符号链接会拒绝。
全批次校验并备份后逐文件临时写入和替换；文件批次不具备整体事务。
中途失败可能部分完成，但原文件均已备份。备份目录须新建且在源目录之外。

本工具不会修改归档 ZIP、导出的证据和离线历史副本。恢复旧任务归档到新版环境时，
恢复完成后、启动任务前，需对恢复出来的 transcripts 再做检查与迁移。

## 浏览器与环境变量

`browser-preferences.js` 是离线辅助脚本，不注入生产页面。由维护者在原控制台
origin 的浏览器开发者工具载入后运行 `yunkunBrandMigration()` 检查；
运行 `yunkunBrandMigration({apply: true})` 才迁移六类偏好并清理登录态。
已有新偏好优先；存储失败保留源值。脚本不打印偏好值或令牌、不伪造新 JWT。
不同 origin（协议、主机、端口不同）无法直接读取旧存储。

环境脚本默认只检查 Process 范围，`-Apply` 才写入，`-RemoveLegacy` 才删除旧变量。
已有非空新变量优先，输出不含变量值。User/Machine 修改后重开启动终端；
Machine 写入需已有管理员权限。Process 模式需在原启动 PowerShell 中执行，
从另一个子进程运行无法改变父终端的环境。

```powershell
.\environment.ps1 -Scope Process
.\environment.ps1 -Scope User -Apply -RemoveLegacy
```

本脚本只覆盖产品配置变量，API_KEY 等供应商通用变量不改名。

## 复核

```text
go test ./...
node browser-preferences.test.cjs
```

Go 测试使用内存 SQLite、临时 JSONL 和测试密钥；浏览器脚本测试使用模拟存储。
这不等于真实 PostgreSQL、浏览器和业务数据切换验收。
