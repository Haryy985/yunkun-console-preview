# Semgrep Adapter Contract

## 目的

将 Semgrep 作为本地源码的候选信号生成器，而不是漏洞判定器或远程扫描器。

## 允许输入

- 当前项目工作区内已确认的源码目录。
- 明确版本化的规则集与输出文件路径。

## 执行纪律

1. 先完成 `source-audit.md` 的入口、依赖和信任边界建模。
2. 记录 Semgrep 版本、规则集标识、排除目录、命令、JSON 输出路径和扫描时间。
3. 规则命中只建候选 branch；沿真实调用链确认 source、transform、sink 和防护条件后才可升级。
4. 不自动安装依赖、执行项目构建、运行测试钩子或下载未经登记的规则包。
5. 只读扫描失败时，记录 `tool-unavailable` 或 `scan-incomplete`，继续人工代码追踪；不要把扫描失败写成“未发现漏洞”。

## 输出字段

`rule_id / file / line / snippet_hash / source_path / sink_path / guard / reachability / finding_status / raw_artifact_path`
