<div align="center">

<img src="web/public/yunkun-logo.png" alt="云鲲安全" width="76">

# 云鲲自动化渗透控制台

面向授权安全测试的 Agent 工作台

对话与任务 · 执行流水 · RAG 知识库 · 代理资源 · HTTP 证据

Rowan / 云鲲安全

</div>

> 本项目仅用于合法授权的安全测试。请在隔离、低权限环境中运行，不要将控制台或数据库直接暴露到公网。

## 功能

| 模块 | 能力 |
| --- | --- |
| 对话与任务 | Agent 对话、任务规划、Worker、模板、暂停、归档和批量操作 |
| 执行流水 | 工具调用、模型请求、RAG 检索、HTTP 记录、审批和异常事件 |
| RAG 知识库 | 文档导入、切片检索、引用追踪、任务访问限制和人工审核 |
| 代理资源 | 固定代理、第三方来源、FOFA / Quake 发现、验证、启停和出口审计 |
| 漏洞与报告 | Finding、请求响应、证据快照和 HTTP 请求包复现材料 |
| 扩展配置 | 模型、工具、MCP、Skills 和系统偏好 |

RAG 知识库和代理池是云鲲安全在原始源码基础上后续新增的业务模块。发现一个知识切片或代理节点，不等于它已经被模型使用、获得授权或验证可用；实际使用仍需按任务范围核验。

## 快速开始

### Windows 一键启动

不需要 Docker，也不要求预先安装 Go、Node.js 或 PostgreSQL。

1. 将完整目录解压到当前账号可写的短路径，例如 `F:\yunkun`。
2. 双击 `setup-and-start.bat`。
3. 等待依赖下载、编译、数据库初始化和就绪检查完成。
4. 打开 `http://127.0.0.1:5173/`。
5. 使用账号 `yunkun`，首次自行设置密码。

停止服务运行 `stop-local.bat`。停止前请确认没有需要继续运行的任务。

| 项目 | 默认值 |
| --- | --- |
| 控制台 | `127.0.0.1:5173` |
| 独立 PostgreSQL | `127.0.0.1:5433` |
| 私有数据 | `.yunkun-local/` |
| 默认管理员密码 | 没有统一默认密码，首次自行设置 |

### 手动开发

需要 Go、Node.js / npm 和 PostgreSQL。版本以 `go.mod` 与 `web/package.json` 为准。

```powershell
# 后端
go run ./cmd/yunkun -addr 127.0.0.1:8787 -proxy=
```

```powershell
# 前端
cd web
npm ci
npm run dev -- -p 5173
```

手动部署、Windows 依赖和数据迁移见 [`docs/`](docs/README.md)。

## 界面预览

以下图片用于展示界面与模块布局，RAG 和代理池图片使用演示数据，不代表真实联网验证结果。

![仪表盘](screenshots/dashboard.png)

| 模型配置 | 系统配置 |
| :---: | :---: |
| ![模型配置](screenshots/llm.png) | ![系统配置](screenshots/settings-dark.png) |

| RAG 知识库 | 代理池 |
| :---: | :---: |
| ![RAG 知识库](screenshots/rag.png) | ![代理池](screenshots/proxy-pools.png) |

## 交付边界

模型服务、第三方平台凭据、测试工具和目标授权由部署方自行准备。原生 Agent 使用运行账号的本机权限，不提供容器隔离；代理配置也不等于整机网络隔离。

当前交付包包含源码、前端、启动脚本、安装器和测试代码，不包含真实模型密钥、代理凭据、数据库数据、运行日志或本地缓存。

## 来源与许可证

本项目基于既有开源项目进行二次开发，保留原项目来源、许可证和第三方声明。RAG 知识库、代理池、FOFA / Quake 发现、HTTP 请求包报告、任务可观测性和云鲲品牌界面属于后续开发内容。

来源与授权说明见 [`NOTICE.md`](NOTICE.md)，许可证见 [`LICENSE`](LICENSE)。

