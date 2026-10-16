# 文档

| 文档 | 说明 |
| --- | --- |
| [`OFFLINE_UPDATE.md`](OFFLINE_UPDATE.md) | Windows 离线版首次安装、更新、回滚和数据保留 |
| [`agent-reliability-2026-10-16.md`](agent-reliability-2026-10-16.md) | 本次 Agent 稳定性修复及验证边界 |
| [`DEPLOYMENT.md`](DEPLOYMENT.md) | 源码自行编译的手动部署说明，非离线包启动步骤 |
| [`WINDOWS.md`](WINDOWS.md) | 在线源码安装说明，非离线包启动步骤 |
| [`../migration/README.md`](../migration/README.md) | 已有数据与旧环境迁移说明 |
| [`../NOTICE.md`](../NOTICE.md) | 来源、授权和分发说明 |

离线版使用 `setup-and-start-offline.bat`，由启动器在 `.yunkun-local/` 内生成配置和数据库。仅在源码手动部署时才从 `config.example.json` 创建配置。不要把数据库密码、模型密钥、代理凭据、Cookie、Token 或 `.yunkun-local/` 上传到公开仓库。
