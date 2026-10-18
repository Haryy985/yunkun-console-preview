# 原生部署手册

> 本文用于从源码自行编译，不是 Windows 完整离线包的启动和更新流程。已下载带 `bin/yunkun.exe` 与 `runtime/pgsql-18.4/` 的包，请使用 [离线版手册](OFFLINE_UPDATE.md)。

日期：2026-10-15。本交付包不提供容器部署文件，Windows、Linux、macOS 均按原生方式构建和运行。

Windows x64 全新安装增加 `setup-and-start.bat` 双击入口，自动下载依赖、编译和启动独立 PostgreSQL。它不接管本节所述的已有配置或已有数据库，详细说明见 `ONE_CLICK_WINDOWS.md`；本手册下面的手动部署方式保持可用。

## 依赖与范围

- 编译：Go 1.26.3 或兼容工具链、Node.js 22/npm，以及下载 Go/npm 依赖所需的网络。Linux/macOS 使用 `build.sh` 时还需 Bash 和 rsync。
- 运行：PostgreSQL 服务，可以在本机或受控远程服务器；执行文件旁的配置和数据目录须可写。已内嵌前端的程序不需要 Node.js 来托管界面。
- 任务工具：Python、浏览器及相关命令行工具不是控制台启动的前提，但使用对应 Agent 工具时必须另行安装和配置。
- 本源码不含真实密钥、数据库、登录密码、业务数据或预编译程序，附带公开 Skills 工作流资料。模型、知识库、代理来源及第三方 API 需要自行配置。

原生启动不提供容器隔离。Agent 调用的本机工具继承运行账号的文件和网络权限，建议使用专用低权限账号和独立测试环境，仅对已授权目标执行任务；代理配置不等于强制网络隔离。

现有验证记录：本机 Windows 使用 Go 1.26.3、Node.js 24.19.0、PostgreSQL 18.4 完成过编译、初始化和登录。Node.js 22 为文档要求，未在此次环境重新验证；Linux/macOS 的以下步骤未在干净机器实测，不宣称全部平台验收完成。

## 一、准备 PostgreSQL

先安装并启动 PostgreSQL，确认实际监听端口。示例配置使用常见的本机 `5432`，不代表你的数据库一定使用该端口。已有安装请保留真实端口、账号及库名，不要为了适配示例修改现有业务库。

新建安装可使用 PostgreSQL 自带的管理工具，也可以以数据库管理员身份进入 psql，逐条执行：

```sql
CREATE ROLE yunkun LOGIN;
\password yunkun
CREATE DATABASE yunkun OWNER yunkun;
```

`\password` 是 psql 的交互命令，不是在其他 SQL 编辑器中执行的 SQL。自行设置独立数据库密码，不要使用示例密码。已有同名角色或数据库时不要重复创建，更不要删除现有数据。

随后用 `yunkun` 数据库角色执行 `SELECT 1;`，确认真实连接成功。角色需要在自己的数据库中创建应用表的权限。数据库仅向应用所在受控环境开放，不要直接暴露到互联网。

## 二、编译

在下载的仓库根目录执行。首次安装依赖需要网络；单独运行 `go build ./...` 不会生成可用的内嵌前端。

### Windows PowerShell

```powershell
Push-Location web
try {
    npm.cmd ci
    if ($LASTEXITCODE -ne 0) { throw 'npm ci failed' }
    npm.cmd run build:static
    if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed' }
} finally {
    Pop-Location
}

$assetDir = Join-Path (Get-Location).Path 'server\webui\current'
if (Test-Path -LiteralPath $assetDir) {
    throw 'Static asset destination already exists; preserve or clean previous generated files before retrying.'
}
New-Item -ItemType Directory -Path $assetDir -Force | Out-Null
Get-ChildItem -LiteralPath 'web\out' -Force | ForEach-Object {
    Copy-Item -LiteralPath $_.FullName -Destination $assetDir -Recurse -Force
}
go build -tags embedui -trimpath -o yunkun.exe ./cmd/yunkun
if ($LASTEXITCODE -ne 0) { throw 'Backend build failed' }
```

上述命令在发现已有内嵌资源目录时停止，避免新旧文件混用。再次构建前先确认该目录仅含生成文件，再自行留档或清理；不要删除业务数据。

### Linux / macOS

```bash
bash build.sh --no-compress
```

脚本自动安装前端锁文件依赖、导出页面、同步内嵌资源并编译当前系统程序，输出位于 `dist/yunkun-<系统>-<架构>/`。不要只复制程序而忘记后续的配置和 Skills 目录。

需要输出到仓库根目录时，可在构建前设置：

```bash
YUNKUN_OUTPUT=./yunkun bash build.sh --no-compress
```

若所用 Go 版本不足且工具链自动下载不可用，先安装兼容版本；不要擅自降低 `go.mod` 的版本要求。

## 三、配置与启动

将 `config.example.json` 复制为 `config.json`，填写真实的数据库密码、地址、端口及库名。如果文件已存在，先备份，禁止用示例覆盖。配置文件含密码，不要上传 GitHub。

示例中的 `skill_dir` 留空时，使用可执行文件旁的 `skills` 目录；该目录初始只有占位文件。也可使用 `YUNKUN_SKILL_DIR` 指定自己的路径，不依赖 `/opt/` 等固定目录。

以下命令假定程序、`config.json` 和 `skills/` 都在仓库根目录；若使用 `dist/` 内的程序，应将配置和目录放在程序旁，并从该目录启动。避免当前工作目录留有其他 `config.json`，因为当前目录配置优先于程序旁配置。

Windows：

```powershell
.\yunkun.exe -addr 127.0.0.1:5173 -proxy=
```

Linux / macOS：

```bash
./yunkun -addr 127.0.0.1:5173 -proxy=
```

打开 `http://127.0.0.1:5173/`。新库首次访问会进入密码初始化，管理员账号为 `yunkun`，没有统一默认密码。已有库保留原密码。数据库密码与控制台登录密码不是同一个配置。

此示例仅启动本机控制台入口，关闭流量捕获代理。需要捕获时，显式传入 `-proxy 127.0.0.1:8788`，并按功能要求配置工具代理及 HTTPS 信任。程序默认监听参数仍保留原值，省略参数不等于只启动 5173。

需要守护重启时，在程序旁使用 `start.bat -addr 127.0.0.1:5173 -proxy=` 或 `sh start.sh -addr 127.0.0.1:5173 -proxy=`。不要重复启动多个实例；端口占用时先核实进程身份，不要盲目结束数据库或其他服务。

## 四、上线前检查

1. 检查 `/api/health` 返回就绪状态，并完成初始化、登录、刷新及重新登录。
2. 设置自己的模型配置，验证连通性；检查工具依赖后执行一项受控授权测试任务。
3. 分别验收计划使用的知识库上传检索、代理出口、工具执行、流量和报告，不能用“页面能打开”代替业务验收。
4. 备份 PostgreSQL 及程序产生的数据、签名密钥和代理凭据密钥；不要只备份程序。
5. 对外提供服务前配置 HTTPS、访问限制及备份策略；不要直接把本机测试监听改成公网开放，或依赖前端隐藏按钮作权限控制。

## 常见问题

- 数据源不可用：先检查 PostgreSQL 是否启动，以及配置中实际端口、角色、密码和库名；运行进程存在不代表数据库就绪。
- 页面缺失：检查前端是否成功导出，资源内容是否放在 `server/webui/current`，Go 是否带 `-tags embedui` 重新编译。
- 功能缺失：确认 Skills、模型、第三方密钥和工具依赖已配置。控制台不自动提供这些外部能力。
- 忘记管理员密码：已登录时优先使用界面改密；Bash 环境可使用 `bash reset-password.sh --config config.json`，需要本机 psql，使用脚本读取 JSON 时建议安装 Python 3。脚本使用 pgcrypto 生成哈希，需要该扩展及相应权限；重置前核实目标库，勿误改业务账号。
- 升级旧安装：先读 `migration/README.md`，停机备份和迁移后再切换，不要将此手册当成覆盖升级命令。
