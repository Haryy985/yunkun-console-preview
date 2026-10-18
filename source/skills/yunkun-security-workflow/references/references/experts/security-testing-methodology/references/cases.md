# 安全测试execution-framework — 实战案例库

> 正文按 A4.1 只保留案例指针；完整实战案例、日期化标注与实例化数据统一存放于此。

## SRC 实战审计案例

- **字节跳动 SRC 四候选证据审计**（NGCC 公网暴露 / OIDC Scope 配置 / WAF 大写路径绕过 / Passport app_id 枚举，结论全部 NO-GO）：完整记录见 `session-byte-src-audit.md`。含证据矩阵、对照测试设计（有效/无效/空/无参数四组对照）与 NO-GO 判定，是正文 §6 SRC 证据审计框架的实战应用。
- **字节跳动 SRC 信息收集与攻击面测绘**：见 `bytedance-src-recon.md`。execution-framework可迁移至其他大型互联网公司 SRC 场景。
- **BBASRC（华晨宝马）证据收敛全流程**：见 `bbasrc-evidence-convergence.md`。7 条发现经证据矩阵、降噪分析后，输出 1 条候选提交（Countly 数据注入）、2 条暂不提交、5 条放弃；含完整 evidence-matrix、Countly 报告、gateway-int 评估和最终提交计划。
- **无认证数据采集端点分析**（API 对任意参数返回 Success 时的系统性判定）：见 `unauthenticated-endpoint-analysis.md`。通过 JSON 解析验证、Content-Type 过滤、app_key 差分、请求处理假设、持久化旁证确认数据是否被实际处理/持久化。

## 批量 POC 通杀案例

- **天锐绿盾审批系统通杀批量验证**：见 `tianrui-lvdun-poc.md`。实例化数据包括：产品关键词（tipray / ldsafe / sztipray / trld / dlp / ld.）、特征端口（31001 / 8280 / 40001）、trwfe 路径指纹、Tomcat 路径穿越鉴权绕过（`.%2e` / `..;/`）与 `.git/config` 泄露扫描。正文 §4.3 POC 工作流为其execution-framework抽象。
- **HTTP 代理格式（全 URL 请求行）CTF 利用**：见 `http-proxy-format-ctf.md`。

## 其他参考

- `cognitive-type-system.md`：认知类型系统展开。
- `experimental-validation-pipeline.md`：实验验证流水线展开。
- `research-os-execution-layer.md`：研究执行层展开。
- `ctf-skills-analysis.md` / `anthropic-cybersecurity-skills.md` / `penetration-skill-v10-audit-summary.md`：外部技能集审计参考。
