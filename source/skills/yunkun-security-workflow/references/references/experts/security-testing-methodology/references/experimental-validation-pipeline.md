# 9-Phase Experimental Validation Pipeline

用于将任何安全概念从理论（REFERENCE）验证到可复现实验（CANDIDATE）的通用execution-framework。

## 适用场景

将一个 REFERENCE 技能升级为有实验证据支持的 CANDIDATE 时需要经过这 9 个阶段。禁止跳过实验直接创建 Skill。

## 阶段

### 1. 提取安全不变量

确定该漏洞或技术的核心安全不变量。例如 SSTI 的不变量：模板引擎将用户输入作为代码解释执行，存在输入到代码的边界穿越。

### 2. 阅读可靠records

按优先级依次使用：历史 Case Replay、官方文档与标准、官方测试项目、修复 Commit、权威研究、成熟开源实现。单一博客不能作为能力晋升依据。至少覆盖 2 个来源。

### 3. 搭建最小隔离实验

只允许使用以下环境：本地容器、隔离 VM、CTF、公开实验平台、开源项目、公开修复版本、脱敏日志、历史 Case Replay、明确授权环境。禁止随机选择外部真实系统。

### 4. 执行 4 种场景

- 正常路径 (Happy Path)：普通操作正常。
- 目标条件 (Target Condition)：漏洞/技术工作正常。
- 失败模式 (Failure Mode)：明确什么时候失败、为什么失败。
- 误判检查 (False Positive)：什么情况下会被误认为存在或不存在。

### 5. 修改变量

至少修改 2 个不同于原始上下文的变量。例如：从 GET 改为 POST 注入、从 Undefined 改为 DebugUndefined、从默认沙箱改为自定义沙箱。

### 6. Teacher/Student 隔离

Teacher 环境（有已知答案的实验脚本）与 Student 环境（无答案的考核环境）物理分离。Student 的测试文件中不得包含已知答案或参考 Teacher 的路径。

### 7. 闭卷迁移

Student 在零参考 Teacher 答案的环境下独立完成同类任务。如果靠记忆能直接默写出 payload 并成功，说明真正掌握了。如果需要回去翻文档再试，说明理解不透彻。

### 8. Reviewer 考核

Reviewer 验证 4 项内容：
- 人工制品完整性：实验环境、测试脚本齐全
- 证据链：场景结果、变量修饰、闭卷结果有记录
- 管道完整性：9 个阶段全部完成
- 能力等级：是否有虚假宣称

Reviewer 可判定 HARD_FAIL (拒绝)、CONFLICTED (有矛盾)、PASS (通过)。

### 9. 定级与沉淀

- 只阅读无实验：REFERENCE，不创建 Skill
- 完成实验：CANDIDATE，可创建 Skill
- 通过闭卷迁移+负面测试：申请 ACTIVE
- 任务经验（不可迁移）：Case Episodic Record，不创建 Skill
- 稳定方法：沉淀到现有 Skill
- 环境/工具状态：Environment Manifest
- 任何情况不自动写入 Memory

## 停止条件

满足任一停止：达到目标等级、当前阻塞解决、无法获得必要环境、连续两次实验无新信息、成本超过复用价值、主题只有单目标价值、无法设计真实考核、已有 Skill 足以解决。

## 参考

NetStr1ke 自主学习策略 LEARNING_POLICY.yaml (config/LEARNING_POLICY.yaml) 完整定义了节奏控制、触发源白名单、能力等级、训练材料安全等全局规则。
