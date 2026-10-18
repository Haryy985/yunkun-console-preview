# clown21 execution-framework入口

> 视角：渗透测试（黑盒/灰盒/白盒，按授权范围执行）
> 假定：有明确目标和授权，目的是深度突破而非广泛搜索

---

## 这套execution-framework怎么用

渗透测试的execution-framework切成 5 段，其中 6 段对应本目录的文件，另有 3 个阶段工作流文件提供具体执行手法 + 2 个新引擎：

```
[01] 选靶——攻击面地图出来后，决定先打哪里（攻击优先级）
        │
        ▼
[06] 推导——攻击链自动推导：入口→攻击类型→权限跃迁→横向展开（攻击链推导引擎）
        │
        ▼
[13] 情报——版本指纹 → CVE/NVD/CNVD/Exploit-DB 自动关联（CVE 情报引擎）
        │
        ▼
[02] 起手——构造 payload，带 bypass 矩阵（绕过工具箱）
        │
        ▼
[04] 控制缺口（Control Gap）狩猎——把"敏感操作 ↔ 应有控制"翻译成探测策略
        │
        ▼
[03] 收尾——证据纪律：抓包、回显、带外、复现率，不写"我猜的"漏洞

[10-12] 阶段工作流——被动侦察 / 主动扫描 / 利用与后渗透的具体执行手册
```

另有 `05-timebox-priority.md` 作为时间盒优先级参考。

### execution-framework文件（策略与决策）

| 文件 | 作用 | 读它的时机 |
|------|------|-----------|
| `01-attack-priority.md` | 决定 RCE > 文件写 > 鉴权绕过 > 注入 > 信息泄露的攻击顺序，量化 P0–P3 评分 | 选目标 / 排攻击优先级时 |
| `02-bypass-toolkit.md` | SQLi、XSS、命令、路径、SSRF、WAF 的通用绕过决策树 + 编码字典 | 任何 payload 被拦截时 |
| `03-evidence-discipline.md` | 证据纪律：HTTP 包、回显、DNSLog、Diff、复现率；如何避免"我以为"漏洞 | 写报告之前 |
| `04-control-gap-hunting.md` | 把 9 类敏感操作（数据修改/批量/权限/资金/SSRF/文件/命令/认证/越权）翻译成"哪些控制应该存在、如何探测它缺失" | 拿到一个新功能、不知道从哪入手时 |
| `06-attack-chain-derivation.md` | 🆕 攻击链自动推导引擎：入口→攻击类型映射 → 权限跃迁递推 → 横向移动展开 → 加权排序。从攻击面地图自动推荐完整攻击路径 | Phase 2c 攻击面地图生成时（MUST Read） |
| `13-cve-intelligence.md` | 🆕 CVE 情报自动关联：版本指纹 → 多渠道查询（NVD/CNVD/Exploit-DB/GitHub）→ 过滤矩阵 → CVE 情报卡。有 PoC 的直接进 Phase 3 队列 | Phase 2b 识别出版本号时（自动触发） |
| `14-shannon-hybrid-scanning.md` | 🆕 Shannon 混合扫描：从源码/JS/OpenAPI 抽 API、变量名、Source→Sink 和漏洞面 checklist，再生成最小黑盒探针；scanner 结果只作线索，运行时证据决定是否升洞 | 有源码、前端资源、接口文档、source map、移动端包，或需要把自动化探针转成 clown21 分支时 |

### 阶段工作流文件（具体执行手册）

| 文件 | 对应阶段 | 内容 | 读它的时机 |
|------|---------|------|-----------|
| `10-passive-recon.md` | Phase 2a | 子域名枚举（crt.sh/subfinder/amass）、DNS 全记录类型分析、OSINT（Google/GitHub/Shodan/FOFA/Censys dorks）、历史 URL（gau/Wayback）、ASN/IP 归属、泄露数据库查询 | Phase 2a 被动信息收集时（MUST Read） |
| `11-active-scanning.md` | Phase 2b | Nmap 扫描策略（6 种发现 + 5 种扫描 + 速度控制 + NSE + 7 套命令模板）、Web 指纹（whatweb/httpx/WAF）、目录枚举（feroxbuster/gobuster + 敏感路径清单）、服务特定探测 | Phase 2b 主动探测时（MUST Read） |
| `12-exploit-postexp.md` | Phase 4 | 利用前评估、PoC 规范、Linux/Windows 提权、横向移动（PtH/PtT/Kerberoasting/域攻击）、清理流程 | Phase 4 漏洞利用与后渗透时（MUST Read） |

---

## 渗透测试的价值排序

按**攻击链深度和业务影响**排序——渗透测试看的是能走多深、影响多大：

| 等级 | 漏洞类型 | 渗透价值 |
|------|---------|---------|
| 🔴 P0 | 未授权 RCE / SSRF→RCE / 反序列化 → getshell | 直接获取服务器控制权，后渗透起点 |
| 🔴 P0 | 域控沦陷 / 核心数据库接管 | 全域/全库控制，最高影响 |
| 🟠 P1 | 鉴权绕过 / 公开 IDOR 大面积数据 / 任意文件读 | 可作为攻击链跳板，获取凭据/配置 |
| 🟠 P1 | 内网横向 / 凭据复用 / 提权 | 扩大攻击面，从单点到全网 |
| 🟡 P2 | SQLi / 越权 / 文件上传（受限） | 数据泄露或受限代码执行 |
| 🟡 P2 | XSS / 信息泄露（含敏感数据） | 辅助社工或获取 session/token |
| ⚪ P3 | 配置缺陷 / 低危信息泄露 | 风险较低但体现安全基线问题 |

> 这个顺序就是 `01-attack-priority.md` 里"攻击路径最短原则"的体现。渗透测试追求的是**从入口到最深控制权的完整攻击链**。

---

## 与流程阶段的对应关系

```
Phase 2a · Passive Recon → 10-passive-recon（具体手法）→ 01-attack-priority（排优先级）
Phase 2b · Active Scan   → 11-active-scanning（具体手法）→ 端口/服务/指纹矩阵 → 13-cve-intelligence（CVE 自动关联）→ 14-shannon-hybrid-scanning（代码/JS/API 线索驱动探针）
Phase 2c · Attack Surface → 06-attack-chain-derivation（攻击链自动推导）→ 输出推荐攻击路径
Phase 3  · Discover       → 04-control-gap（找入口）→ 14-shannon-hybrid-scanning（上下文探针）→ playbooks/（具体探测）→ 02-bypass（被拦时）
Phase 4  · Exploit        → 12-exploit-postexp（提权/横移/清理）→ playbooks/intranet-postexp/（内网）
Phase 5  · Report         → 03-evidence（证据纪律）→ templates/report-pentest.md（报告模板）
```

---

## 配套 playbook 目录

漏洞类型分类详见 `../playbooks/00-index.md`。
