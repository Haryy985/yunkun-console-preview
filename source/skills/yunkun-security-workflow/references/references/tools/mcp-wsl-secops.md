# MCP 工具集成 — `wsl-secops`（WSL 路线）

适用前提：**Claude Code 直接在 WSL Ubuntu 中运行**。

`wsl-secops` 是 clown21 的本地最小闭环 MCP server，负责把 WSL 里的 CLI 工具变成 Claude 可自动调用能力。它不是“大而全 Kali 替身”，而是优先覆盖单智能体渗透流程里最常用、最容易自动化的几类动作：

- 基础 HTTP 重放 / 差分
- 端口与服务识别
- 小范围目录枚举
- 定向模板验证
- 注入收敛验证

## 当前封装的工具

- `tool_status`
- `job_status`
- `wait_for_job`
- `http_request`
- `run_nmap`
- `run_ffuf`
- `run_nuclei`
- `run_sqlmap`

## 执行模型

从当前版本开始，`wsl-secops` 支持“启动即返回 + 边跑边落盘 + 轮询状态”。

- `http_request` 默认 `wait=true`
  适合短请求、基线重放、差分对照，默认同步返回。
- `run_nmap` / `run_ffuf` / `run_nuclei` / `run_sqlmap` 默认 `wait=false`
  适合中长任务，默认异步启动，先返回 `job_id`，再由 `job_status` 或 `wait_for_job` 继续观察。
- 同步执行现在也会实时把标准输出和标准错误写入 `stdout.txt` / `stderr.txt`
  不再等整条命令结束后才落盘。

## 推荐注册方式

### 1. 先准备 Python venv

```bash
python3 -m venv ~/.claude/venvs/wsl-secops
~/.claude/venvs/wsl-secops/bin/python -m pip install --upgrade pip
~/.claude/venvs/wsl-secops/bin/python -m pip install mcp
```

### 2. 用 Claude CLI 注册，不手改 JSON

```bash
claude mcp add --scope user wsl-secops -- \
  /home/user/.claude/venvs/wsl-secops/bin/python \
  /home/user/.claude/skills/clown21/scripts/wsl_secops_mcp.py
```

> 优先用 `claude mcp add`，因为它会写到正确作用域，少踩配置格式坑。

## 前置依赖

| 项 | 必须 | 说明 |
|---|---|---|
| Python 3 | 是 | 用于启动本地 MCP server |
| `mcp` Python 包 | 是 | `FastMCP` 运行时 |
| `curl` | 是 | `http_request` 使用 |
| `nmap` | 推荐 | 网络识别 |
| `ffuf` | 推荐 | 路径枚举 |
| `nuclei` + templates | 推荐 | 定向模板验证 |
| `sqlmap` | 可选但强烈建议 | 注入验证 |

## 工具职责边界

### `tool_status`

用途：

- 查看当前工具是否在 PATH
- 查看默认 wordlist 路径
- 查看代理环境
- 查看是否支持异步轮询
- 作为每次会话开始前的最小健康检查

### `job_status`

用途：

- 查询异步任务当前状态
- 读取 `meta.json`
- 查看 `stdout.txt` / `stderr.txt` 的尾部预览
- 判断子进程是否还活着

输入要点：

- `job` 可传 `job_id`
- 也可直接传产物目录路径
- 也可传 `meta.json` 路径

### `wait_for_job`

用途：

- 阻塞轮询已启动任务
- 适合“先异步启动，再在后续步骤等待结果”

输入要点：

- `timeout_sec` 控制最长等待时间
- `poll_interval_sec` 控制轮询间隔
- 超时会返回当前状态并带 `timed_out=true`

### `http_request`

用途：

- 基线请求
- 认证边界重放
- BOLA/IDOR 对照组请求
- 文件上传前的接口格式探测

输入要点：

- `headers` 支持两种格式：
  - JSON object：`{"Authorization":"Bearer xxx","X-Test":"1"}`
  - 多行头文本：
    ```text
    Authorization: Bearer xxx
    X-Test: 1
    ```
- `args` 默认已带 `-i -sS -k --max-time 20`
- 要做特殊请求时再额外通过 `args` 补，比如 `--path-as-is`、`-L`
- 默认 `wait=true`，如需后台执行可显式传 `wait=false`

### `run_nmap`

用途：

- scoped 端口/服务识别
- recon 阶段最小端口面确认

注意：

- 默认 `wait=false`
- 如需同步等待结果，可显式传 `wait=true`

### `run_ffuf`

用途：

- 小范围路径枚举
- 默认自带一个小字典 `assets/wordlists/web-basic.txt`

注意：

- 默认 `wait=false`
- 只适合入口已知后的收敛扫描
- 不要在未知边界上直接重速率轰目录

### `run_nuclei`

用途：

- 定向模板验证
- 指纹已知后的收敛确认

注意：

- 默认 `wait=false`
- 不应用作第一跳广撒网枚举
- 先有 signal，再跑模板

### `run_sqlmap`

用途：

- 已有高信号注入入口时做收敛验证

注意：

- 默认 `wait=false`
- 默认参数应保持保守
- 没有基线、没有可复现参数面时不要直接跑

## 典型用法

### 1. 启动长任务

```python
run_nuclei(
  target="https://target.example",
  severity="low,medium,high",
  wait=False
)
```

返回值里重点关注：

- `job_id`
- `status`，初始通常是 `queued`
- `artifact_dir`
- `poll_hint`

### 2. 查询运行状态

```python
job_status(job="20260627-150923-nuclei-target-example")
```

返回值里重点关注：

- `status`
- `stdout_preview`
- `stderr_preview`
- `stdout_size`
- `stderr_size`
- `pid_alive`
- `runner_alive`

### 3. 阻塞等待完成

```python
wait_for_job(
  job="20260627-150923-nuclei-target-example",
  timeout_sec=300,
  poll_interval_sec=2
)
```

## 状态文件与产物目录

每次调用都会自动写出：

- `stdout.txt`
- `stderr.txt`
- `meta.json`
- `job.json`（异步任务会有）

默认输出目录：

```text
~/clown21-artifacts/tool-runs/<timestamp>-<tool>-<label>/
```

`meta.json` 里的典型状态流转：

- `queued`
- `running`
- `completed`
- `failed`

常见字段：

- `job_id`
- `mode`
- `started_at`
- `finished_at`
- `return_code`
- `runner_pid`
- `worker_pid`
- `pid`

## 适合的 Phase

| Phase | 典型动作 |
|---|---|
| `recon` | `tool_status`、`http_request`、`run_nmap`、轻量 `run_ffuf` |
| `discover` | `http_request` 对照、`run_nuclei`、`run_sqlmap` |
| `exploit` | 已确认利用链后的定向请求与验证 |

## 不适合的事

- 浏览器交互
- DOM 观察
- 截图
- SPA 路由分析

这些交给 Playwright MCP。

## 冒烟测试

### 服务侧健康检查

```bash
/home/user/.claude/venvs/wsl-secops/bin/python - <<'PY'
import importlib.util
spec = importlib.util.spec_from_file_location("wsl_secops_mcp", "/home/user/.claude/skills/clown21/scripts/wsl_secops_mcp.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
print(module.tool_status())
PY
```

### 异步任务检查

```bash
/home/user/.claude/venvs/wsl-secops/bin/python - <<'PY'
import importlib.util
import time

spec = importlib.util.spec_from_file_location("wsl_secops_mcp", "/home/user/.claude/skills/clown21/scripts/wsl_secops_mcp.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

job = module.run_nmap("127.0.0.1", args="-Pn -p 22,80", wait=False)
print(job)
time.sleep(2)
print(module.job_status(job["job_id"]))
PY
```

### Claude 侧注册检查

```bash
claude mcp get wsl-secops
claude mcp list
```

之后应继续用 `normalize_findings.py` 进入统一 finding 流。
