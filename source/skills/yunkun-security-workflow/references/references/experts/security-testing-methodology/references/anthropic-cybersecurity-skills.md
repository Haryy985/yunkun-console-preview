# Anthropic Cybersecurity Skills 1.1.0 — 外部技能集合参考

> 仓库: https://github.com/mukul975/Anthropic-Cybersecurity-Skills
> 版本: v1.1.0 | 技能数: 753 | 格式: agentskills.io 标准

## 概述

Anthropic Cybersecurity Skills 是一个大型开源网络安全技能集合（Apache 2.0），包含 753 个标准化技能，覆盖 26 个安全领域。每个技能包含 YAML frontmatter（name/description/tags）、Markdown 步骤式工作流、参考文件和 Python 辅助脚本。

**与我们技能的关系**: 完美互补。

| 我们的技能 | Anthropic 技能 |
|-----------|---------------|
| 思维模型层（Why + How with原理） | 操作手册层（What 命令 + How 步骤） |
| CTF/0day/代码审计深度 | 企业安全/合规/OT/治理广度 |
| 攻击链思维+决策树 | 工具参数+验证checklist |

## 覆盖的强领域（我们弱/空白）

| 领域 | Anthropic 技能数 | 说明 |
|------|-----------------|------|
| **OT/ICS 工控安全** | 28 | Stuxnet检测/S7comm/Modbus/DNP3/IEC 62443/SCADA |
| **合规治理** | 4+ | PCI DSS 4.0 / SOC 2 / GDPR / ISO 27001 / NIST CSF |
| **零信任架构** | 17 | HashiCorp Boundary / BeyondCorp / Zscaler / Cloudflare Access |
| **云原生安全** | 29+ | Cilium Tetragon/eBPF/Binary Authorization/Image Lineage |
| **SOC运营** | 33 | Splunk SPL / QRadar / Elastic SIEM / SOAR Playbook |
| **威胁情报** | 50 | Diamond Model / MISP / STIX/TAXII / OpenCTI |
| **IAM/PAM** | 33 | Vault / CyberArk / Okta / SailPoint / AD Tiered Model |
| **供应链安全** | 3 | in-toto / Sigstore / SLSA / SBOM |
| **DevSecOps** | 17 | SAST/DAST集成 / IaC扫描 / AFL++ CI Fuzzing |
| **密码学实操** | 13 | TLS审计 / 证书管理 / PQC迁移 / HSM |
| **端点取证** | 37 | Autopsy/Sleuth Kit/Plaso/Timesketch/Volatility3全套 |

## 格式结构

```
skills/<skill-name>/
├── SKILL.md           # YAML frontmatter + Markdown workflow
├── references/        # 工具API参考/标准文档/流程文档
├── scripts/           # agent.py 自动化辅助脚本
```

每个 SKILL.md 统一包含: When to Use, Prerequisites, Workflow (分步骤), Verification (Checklist)

## 使用方式

当遇到 Anthropic 覆盖的领域（OT/ICS、合规、零信任、SOC运营等）时，优先考虑引用其对应 skill 中的:
1. **具体工具命令** — 比我们思维模型中的命令更完整
2. **分步骤工作流** — 适合新手或标准化场景
3. **Verification checklist** — 可用于我们的验证步骤
4. **agent.py 脚本** — 可直接复用为自动化辅助

## 不覆盖的领域（用我们自己的技能）

- CTF 挑战（PWN/RE/Crypto/Web3 完全缺失）
- 代码审计（Java/Python/PHP/Go/Rust/C# execution-framework）
- 高级 Exploit 开发（ROP/堆/格式串/沙箱逃逸）
- Web3/区块链（仅1个Ethereum + slither分析）
- AI Agent 安全（仅2个基础skill）

## 安装方式

```bash
# 方法1: npx (推荐)
npx skills add mukul975/Anthropic-Cybersecurity-Skills

# 方法2: git clone
git clone https://github.com/mukul975/Anthropic-Cybersecurity-Skills.git
```
