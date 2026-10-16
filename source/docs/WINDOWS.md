# Windows 离线运行

## 环境

Windows x64、可写的本地目录。不需要安装 Go、Node.js、npm、Docker，也不需要联网。

建议使用较短路径，例如：

```text
F:\yunkun-offline
```

不要把目录放在受系统保护的路径、OneDrive、同步盘或临时目录中。

## 安装与启动

1. 解压完整离线包。
2. 双击根目录的 `setup-and-start-offline.bat`。
3. 等待本机 PostgreSQL 初始化和应用就绪检查完成。
4. 打开 `http://127.0.0.1:5173/`。
5. 使用管理员账号 `yunkun`，首次自行设置密码。

停止服务时双击 `stop-offline.bat`，并在提示时输入 `STOP`。停止不会删除本机数据库。

## 默认端口

| 服务 | 地址 |
| --- | --- |
| 控制台 | `127.0.0.1:5173` |
| 独立 PostgreSQL | `127.0.0.1:5433` |

启动器会核验端口占用和进程路径，不会抢占未知进程，也不会修改防火墙规则。数据库端口冲突时可以使用：

```powershell
.\setup-and-start-offline.ps1 -DatabasePort 55433 -NoBrowser
```

## 私有文件

以下内容只保存在本机，不应上传或提交：

```text
.yunkun-local/
config.json
运行日志
数据库数据
模型密钥
代理凭据
Cookie 和 Token
```

离线包首次启动会自动创建随机数据库凭据和空数据库，不会读取交付目录外的配置。

## 故障排查

登录页提示服务未就绪时，先检查 `.yunkun-local\logs\` 中的第一条错误。不要为了重启删除 `.yunkun-local/`，否则可能造成数据库和本机凭据丢失。

忘记管理员密码时，先备份本机数据库，再使用维护脚本或控制台内的改密功能。已有业务数据不要直接覆盖离线包目录。
