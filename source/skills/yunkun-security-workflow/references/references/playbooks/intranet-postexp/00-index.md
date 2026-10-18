# 内网渗透 / 后渗透 — 决策索引

> ⚠️ **合规警告**：内网横向 / 后渗透必须在**明确授权范围内**进行。本目录内容默认用于**红队评估 / 授权渗透测试 / 已明确允许后渗透的 HVV**。执行前必读 [`../../compliance.md`](../../compliance.md) 确认授权边界。

---

## 子文件路由(Phase 4 / 内网后渗透阶段读哪一份?)

| 当前阶段 | 任务 | MUST Read |
|---|---|---|
| 已拿 shell,身份是 user | 找系统凭据 / 浏览器密码 / Kerberos 票据 / 哈希破解速查 / 社工字典 | `10-credentials.md` |
| 已有一组凭据,需要打第二台机器 | SMB / WMI / PsExec / WinRM / PtH / PtT / 密码喷射 / 远程执行对比 | `11-lateral.md` |
| 拿到 user,需要 root/SYSTEM | UAC bypass / sudo 提权 / 内核 / SUID / Token | `12-privesc.md` |
| 防护强,被 AV/EDR 拦 | AMSI bypass / ETW patch / DLL sideload / unhook / CS操作 / 免杀集成模式 / IOC对抗 / 磁盘反取证 | `13-evasion.md` |
| 已进域,要打 DC | ACL详细表 / Shadow Credentials / 委派决策树 / Kerberoasting / DCSync / ExtraSid/Diamond/Trust Ticket / 信任类型 / bloodyAD | `14-domain.md` |
| 内网不出网 | reGeorg / frp / Chisel / ICMP / DNS tunnel / SOCKS / ssh -D | `15-tunneling.md` |
| 进了陌生网,先摸地形 | fscan / adinfo / 被动发现 / 端口扫描 / BloodHound收集(含认证方式) / hostname | `16-recon.md` |
| 拿到 admin,要维持长期访问 | 决策树+隐蔽性评估 / COM劫持 / AdminSDHolder / DSRM / SharPersist/PowerLurk / 清理检查表 | `17-persistence.md` |
| 目标含 Exchange | 版本-CVE映射 / ProxyLogon/ProxyShell/ProxyNotShell / privexchange / GAL提取 / OWA密码喷射 | `18-exchange.md` |
| 目标含 ADCS / 证书服务 | ESC1-ESC8 + ESC9-11 / ADCS发现 / Web Enrollment / 证书认证(PKINIT/Schannel/PassTheCert) | `19-adcs.md` |
| 目标含 SharePoint | 信息收集 / SOAP / API / OneDrive 同步 | `20-sharepoint.md` |
| 在容器/Pod 内需逃逸或打 K8s | 容器逃逸决策树(13条件) / CDK / K8s SA Token / 特权Pod / CI/CD Pipeline攻击 / ArgoCD / Harbor / Concourse / iox代理 | `21-cloud-services.md` |
| 发现数据库/ES/网络服务端口 | Redis/MSSQL/PostgreSQL/MySQL/MongoDB/Oracle 攻击 + Elasticsearch + 数据库横向移动(dblink/linked server/FEDERATED) + DNS/FTP/IMAP | `22-database-services.md` |

---

## 数据规模(原文档)

| 类别 | 数量 |
|---|--:|
| 凭证窃取 | 20 + hashcat 类型表 / ccupp 社工字典 / 破解决策树 |
| 横向移动 | 16 (+密码喷射+远程执行对比) |
| 权限提升 | 15 |
| 免杀与规避 | 14 + Cobalt Strike / 免杀集成 / IOC对抗 / 磁盘反取证 |
| 域渗透攻击 | 14 (+Shadow Credentials+委派决策树+ExtraSid/Trust Ticket) |
| 隧道代理 | 13 |
| 信息收集 | 12 (+fscan+adinfo+被动发现) |
| 权限维持 | 12 (+COM劫持+AdminSDHolder+决策树+清理表) |
| Exchange攻击 | 5 (+版本CVE+ProxyNotShell+privexchange+GAL+OWA喷射) |
| ADCS攻击 | 5 (+ESC1+ESC9-11+Web Enrollment+证书认证) |
| SharePoint攻击 | 2 |
| 云原生/容器/CI/CD | 13 (逃逸条件) + CDK + K8s + PPE + ArgoCD + Harbor + Concourse + iox |
| 数据库/网络服务 | Redis/MSSQL/PG/MySQL/MongoDB/Oracle/ES + 数据库横向 + DNS/FTP/IMAP |
| **合计** | **128** |

---

## 渗透测试场景下的"价值升级链停车点"

| 你拿到了 | 授权范围内允许做的"价值证明" | 不允许做的事 |
|---|---|---|
| Webshell / RCE | `whoami` / `hostname` / `ip route` / `/etc/issue` 截图 | 反弹 shell / 写持久化 / 提权 |
| 一组 SSH key | 截图 key 文件 mode + comment(脱敏) | 用 key 登录他人机器 |
| 一组数据库凭据 | 截图凭据存在 + 标记访问权限范围 | 实际连库 / dump 数据 |
| 进入内网(SSRF 触达) | 探测 1-2 个明确受影响端口的 banner | 端口扫描全网段 / 横向 |
| AWS / GCP credentials | `aws sts get-caller-identity`(只读 identity)| `aws s3 ls` 等枚举操作 |
| 容器/Pod shell | `id` / `env | grep KUBERNETES` / SA Token 文件名截图 | 容器逃逸 / 创建 Pod / dump Secret |
| 数据库未授权连接 | `redis-cli -h IP PING` / `psql -h IP -U postgres -c "SELECT version()"` 截图 | 读表数据 / 写文件 / 横向移动 |

任何超出"价值证明"的动作 → 立即停 + 回去看 [`../../compliance.md`](../../compliance.md)。

---

## 与红线规则的关系

- 本文件子目录所有 payload **只在已获授权时使用**(红队、合同测试、HVV)
- 未获明确授权时大多数 payload 仅用作"我能做什么"的认知,不准实操
- 一旦提交 finding,价值评级写"假设具备内网持续访问,可进一步..." 而非"已演示横向"
