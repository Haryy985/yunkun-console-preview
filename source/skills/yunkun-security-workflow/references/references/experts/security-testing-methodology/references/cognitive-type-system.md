# Cognitive Type System & Experiment Design

## Core Principle

Every knowledge object in Hermes carries a mandatory cognitive type. Types cannot be skipped — an OBSERVATION cannot become a CONCLUSION directly.

## Type Definitions

```
OBSERVATION: 工具或人工直接观察到的原始现象
  → 例如：HTTP 200、响应中包含 "error" 字符串
  → 不可跨级升级为 HYPOTHESIS

FACT: 被可靠证据支持的事实
  → 需要至少两个独立来源或可重复验证
  → 例如：经过响应内容检查确认的"接口允许匿名访问"

ASSUMPTION: 暂时采用但未验证的前提
  → 必须明确标记为 ASSUMPTION，不能混入 FACT
  → 例如：默认用户没有特殊权限

HYPOTHESIS: 可以通过实验验证或推翻的猜想
  → 必须包含可推翻条件（falsifiable_condition）
  → 例如：如果替换 order_id 为其他用户的值，系统会返回该用户数据

INFERENCE: 基于多个事实进行的推理
  → 必须列出推理链和依赖的事实
  → 例如：因为 A、B、C 三个事实，推断出 D

CONCLUSION: 通过审查后形成的阶段性结论
  → 需经过逻辑/证据/范围三重审查
  → 状态：TENTATIVE / VALIDATED / REJECTED
```

## Experiment Template

```yaml
experiment_id: EXP-NNNN
question: "研究问题"
competing_explanations:
  A: "解释A"
  B: "解释B"
  C: "解释C"
independent_variable: "唯一改变的变量"
controlled_variables:
  - "变量1"
  - "变量2"
expected_outcomes:
  A: "如果A成立，预期结果"
  B: "如果B成立，预期结果"
safety_level: LOW / MEDIUM / HIGH
stop_conditions:
  - "成功区分了竞争解释"
  - "服务端开始返回异常"
```

## Ablation Verification

For confirmed findings, run ablation to confirm which condition actually triggers the vulnerability:

1. Full conditions → vulnerability reproduced
2. Remove condition A → behavior changes?
3. Remove condition B → behavior changes?
4. Restore condition A → behavior returns?
5. Change identity → still works?
6. Change object → still works?

Purpose: confirm the *exact* trigger condition, not accidental success.

## Integration with Pentest Pipeline

```
Phase 1 (Recon):      OBSERVATIONs → FACTS
Phase 2 (Analysis):   FACTS → HYPOTHESES + INFERENCES
Phase 3 (Validation): HYPOTHESES → EXPERIMENT → RESULT
Phase 4 (Review):     RESULTS → TRIPLE REVIEW → CONCLUSION
Phase 5 (Report):     CONCLUSIONS → FINDINGS
```
