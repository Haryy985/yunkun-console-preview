# MCP 工具集成 — Playwright（WSL 路线）

适用前提：**Claude Code 直接在 WSL Ubuntu 中运行**。

## 作用

Playwright MCP 负责：

- 打开页面
- 观察 SPA 路由与前端行为
- 截图
- 表单交互
- 基础浏览器自动化
- 把“浏览器里发生了什么”补成证据

## 推荐注册方式

优先用 Claude CLI 注册：

```bash
claude mcp add --scope user playwright -- npx -y @playwright/mcp@latest
```

> `-y` 用于避免 `npx` 首次下载时进入交互确认。

## 前置依赖

| 项 | 必须 | 说明 |
|---|---|---|
| `node` | 是 | 启动 Playwright MCP |
| `npm` / `npx` | 是 | 下载并运行包 |
| Playwright 浏览器运行时 | 是 | 推荐 Chromium |
| 浏览器系统依赖 | 推荐 | 不补齐依赖时常见白屏/无法启动 |

## 推荐初始化

### 1. 安装 Chromium 依赖

```bash
sudo npx -y playwright@latest install-deps chromium
```

### 2. 安装 Chromium 运行时

```bash
npx -y playwright@latest install chromium
```

## 冒烟测试

### 包侧可启动

```bash
npx -y @playwright/mcp@latest --help
```

### Claude 侧可见

```bash
claude mcp get playwright
claude mcp list
```

## 适合的 Phase

- `recon`
- `discover`

## 典型场景

- 登录流
- OAuth / SAML / JWT 前端行为
- JS 动态加载接口
- 按钮 / 表单触发的隐藏请求
- 截图证据
- 确认 SPA 假路由和真实后端请求的差异

## 不适合的事

- 批量目录枚举
- 模板化漏洞扫描
- 端口扫描

这些应该交给 `wsl-secops`。

## 常见故障

### 1. 能注册，但打开页面失败

先补：

```bash
sudo npx -y playwright@latest install-deps chromium
```

### 2. 首次下载卡住

优先检查 WSL 代理变量是否存在：

```bash
echo $HTTP_PROXY
echo $HTTPS_PROXY
```

### 3. MCP 已注册，但 Claude 看不到

先跑：

```bash
claude mcp list
```

如果没有 `playwright`，重新执行注册命令，不要直接手改配置文件。

## 每次调用后的产物要求

- 截图放 `screenshots/`
- 关键 URL 与请求记入 `notes/`
- 如提取到可结构化结果，继续进 `normalize_findings.py`
