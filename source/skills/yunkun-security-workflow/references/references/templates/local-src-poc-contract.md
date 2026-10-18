# 本地 SRC / BP PoC 交付约定

默认交付面向漏洞平台或 Burp 复现阅读者，而不是管理型长报告。先读同一风格的本地参考：

- `D:\ctftools\src\MYSELF\东华理工大学_eHall_funauth_人员目录泄露_BP漏洞复现.md`
- `D:\ctftools\src\MYSELF\新疆\报告2-未授权文件下载IDOR.md`
- `D:\ctftools\src\MYSELF\sssq_PDF存储型XSS_完整报告.md`

只复用结构和证据表达，不复用目标、数据、结论、PoC 或未经当前 run 证实的影响。

## 默认结构

```markdown
# <漏洞标题>

> 目标：<精确 URL / 资产>
> 类型：<漏洞类型>
> 复现角色与前提：<已验证的测试身份、对象、状态；无则写“无认证”>
> 结论状态：impact-confirmed

## 漏洞概述
一句话说明谁能越过什么边界，造成什么已验证结果。

## 影响与边界
- 已确认：<实际读到的数据 / 实际完成的受控状态变化 / 已验证的第二角色影响>
- 未证明：<明确列出未测试或未闭环的范围，不作推断>
- 影响对象：<最小已验证范围，不把样本外推成全量>

## Burp / 请求复现
1. 前置状态与 baseline 原始请求。
2. 只改变一个变量的 variant 原始请求。
3. negative control 原始请求。
4. 可观察的业务成功或 before/after 结果。

## 差分证明
简短列出 baseline / variant / negative control 的状态码、关键字段和业务结果。

## 证据索引
- `evidence/<raw-request-or-response>`
- `evidence/<impact-proof>`

## 清理
说明未产生持久状态，或列出 `cleanup.jsonl` 的已验证回滚记录。

## 修复建议
给出对应根因的最短可执行修复。
```

## 写作硬规则

- 只有 `state/delivery-findings.json` 中的 `delivery_findings` 才能使用“漏洞”、风险等级和编号。
- `lead`、`primitive-confirmed`、`conditional-impact` 与 `false-positive` 只能写入“未闭环线索 / 验证缺口”，不计入漏洞总数，不给最终风险等级。
- 不把单端点可达、函数输出、HTTP `200`、500/405、静态源码推断或理论攻击场景写成已验证影响。
- 不用大表格重复叙述。请求/响应、差分和未证明范围优先；每个结论紧邻其原始证据路径。
