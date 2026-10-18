> 写不写只认 `~/.grok/rules/vuln-report-format.md`。进站有会话时最低探针见 `dig-scope` §4.2.3（对象图、换 id、哨兵值）；单号用列表/回包里的他人 id，不为第二号磨注册。默认先用读/列表差分证明跨用户·跨租户。写越权仍要测，但**不是**对着别人已有数据改/删。顺序见下「写越权怎么打」。禁止批量改删、禁止真资损。
> 短表指针用标题搜。英文 BOLA 百科已砍；写越权怎么打仍在上半。

## 一、原有knowledge-base

# 越权漏洞（IDOR/权限绕过）测试手册

> **最小伤害：** 默认读/列表差分。写越权要测，按「先加自己的 → 再删自己加的」。不要把「只读」理解成「写 IDOR 不测不报」。

### 写越权怎么打（先加后清）

读差分够闭环就不要写。读不够、要证「能改别人的东西」时：

1. **先测添加。** 用自己的号（或未登录）调创建口，看能不能挂到别人名下 / 别人店 / 别人租户。成功 = 写越权已证。  
2. **再删自己刚加的那一条。** 用同一条接口或对应删除口清掉，别留脏数据。  
3. **不要**去改别人已有订单、改别人密码/角色、删别人原来的地址/券/商品。那些删不干净，也不是最小证伪。  
4. 现场没有创建口、只有改/删现成对象：改一个**自己能改回去**的测试字段，打一次就停。改密 / 改角色 / 改绑按 `dig-scope` §4.2.2 和 `authbypass-test.md` **可探**（拿掉旧验看过不过）；过了立刻改回；改不回就停在回包，不要把别人的密、角色、邮箱留下。  
5. 要证资损：只动**自己能改回去的**测试金额/状态，打一次立刻改回。不要真转走生产资金、不要清别人库存、不要把别人已有单改到收不回。**不是**支付/写 IDOR 不测。禁止批量、禁止对着生产主数据试删。本段是最小伤害，**不是**「写 IDOR / 接管 / 加字段不测」。

## 概念区分

- **水平越权**: 同权限级别，访问他人资源（A 访问 B 的订单）
- **垂直越权**: 低权限账户调用高权限接口（普通用户调管理员 API）
- **未授权访问**: 未登录直接访问需认证接口

---

## 测试流程

### Step 1: 找到用户标识参数

在以下位置寻找用户/资源标识：

```
URL 路径:    /api/user/12345/info
URL 参数:    ?userId=12345&orderId=ABC
请求体:      {"uid": 12345, "target_id": "user_abc"}
响应体中:    {"id": 12345, "created_by": 67890}  ← 收集这些 ID
```

**高价值接口特征**:
- 包含 `user`, `account`, `profile`, `order`, `bill`, `message` 的路径
- 响应包含他人的手机号、邮箱、真实姓名、身份证
- 修改类接口: `update`, `edit`, `delete`, `change`

### Step 2: 对象 id（有两号更好，没有也不磨注册）

```
有对照号：账号 A 登录拿 token，账号 B 的资源 ID 做对照。
单号 / 没号：用列表、回包、邻号、`0`/`-1`/空当他人 id（`dig-scope` §4.2.3）。禁止为凑第二号去磨注册。
```

### Step 3: 替换标识符测试

```python
# 水平越权测试示例
# 用 A 的 token 访问 B 的资源

import requests

token_a = "Bearer eyJ..."
victim_user_id = "B的用户ID"

# 原始请求（访问自己）
r1 = requests.get(
    "https://target.com/api/user/info",
    params={"userId": "A的ID"},
    headers={"Authorization": token_a}
)

# 越权请求（访问 B）
r2 = requests.get(
    "https://target.com/api/user/info",
    params={"userId": victim_user_id},
    headers={"Authorization": token_a}
)

# 对比响应
print("自己:", r1.json())
print("他人:", r2.json())
# 若 r2 成功返回 B 的数据 → 水平越权
```

### Step 4: 垂直越权测试

```bash
# 用普通用户 token 调用管理员接口
# 先在管理员账号下抓包找到管理接口
ADMIN_ENDPOINT="https://target.com/api/admin/users/list"
USER_TOKEN="普通用户的token"

curl -X GET "$ADMIN_ENDPOINT" \
  -H "Authorization: Bearer $USER_TOKEN" \
  -H "Content-Type: application/json"

# 200 成功 → 垂直越权
# 403 Forbidden → 有权限控制
```

### Step 5: 未授权访问测试

```bash
# 去掉 Authorization 头直接请求
curl -X GET "https://target.com/api/user/info?userId=12345"

# 或替换为无效 token
curl -X GET "https://target.com/api/user/info?userId=12345" \
  -H "Authorization: Bearer invalid_token_123"
```

---

## 常见绕过技巧

### ID 猜测
```
数字 ID: 尝试 ±1, ±10, 0, -1, 999999
列表上的租户/应用字段：换别人真实 ID 拦了，再试 0 / -1 / 空 / 不传（见下「哨兵租户」）
UUID: 可能从响应或 JS 中获取其他用户 UUID
手机号: 某些接口直接用手机号做标识
```

### 参数污染
```
# 同名参数多次提交
POST /api/user/info?userId=A_ID&userId=B_ID
POST body: userId=A_ID   + URL: ?userId=B_ID
```

### 路径遍历
```
/api/user/A_ID/orders  →  /api/user/B_ID/orders
/api/order/123         →  /api/order/124, 125...
```

### 编码绕过
```
userId=12345         → userId=0x3039（16进制）
userId=12345         → userId=%31%32%33%34%35（URL编码）
```

---

## PoC 证据收集

```
必须保存:
1. 请求完整内容（含 token、headers）
2. 响应完整内容（含受害者数据）
3. 对比截图（A 的正常响应 vs 越权响应）
4. 受害账号 ID 和数据的对应关系证明
```

---

### 密文 ID（短表有指针）

认：改/查地址、订单、records的请求 id 是一长串密文；回包或列表里能看到明文小数字；JS 有 `RSAUtils` / `JSEncrypt` / `security.js`，或写死 `modulus` + `exponent`。

打：密文不是墙。公钥谁都能用。

1. 先打自己的那条，对照回包里的明文 id  
2. 用同一套前端加密，把相邻数字（明文±1、再往两边走）自己加密  
3. 只换这个密文 id，会话不动  

算成：换过去之后出**别人**的姓名/手机/地址。只证明自己的密文能解回自己的明文 → 不算。

假点：服务端按会话过滤，加密对了也只回自己的；那把钥匙是验签用的，乱加密直接拒。单站没中不删短表这行。钥的实值、某站 `security.js` 链接不进库，现场从当前站 JS 抄。

### 哨兵租户（短表有指针）

认：工单 / 资质 / 会话列表带 `appId`、`tenantId`、`orgId`；回包里有附件 URL（`sign` + `file_name` + 又一个租户字段）。

打（换真实他 ID 是已有水平越权，这里多一枪）：

1. 租户字段试 `0`、`-1`、空串、不传 —— 后端常把哨兵值当成「不过滤」  
2. `total` / 条数相对本租户基线暴涨才算打穿；加大 `pageSize`、翻 `page` 只是把量拉全，**不是洞**  
3. 类型字段（`serviceType` 一类）同样试 `0` / `-1`，可能切到另一套业务单  
4. 列表里的带签下载：原样打开常 403；**`sign` / `file_name` 不动，只把 URL 里的租户改成自己的**（签名没罩住这个字段）。正文在 `file-upload-test.md` 预签名那段  

算成：列表出现他租户的工单正文 / 客服对话，或附件真下到对方证照。主体是跨租户业务读。

假点：`0` 仍只回自己的；换租户下载 403；只有文件通道、没有业务列表。哨兵值没中 → 不删这行，下一站列表过滤照样试。

### 制品库 catalog 不过滤租户（短表有指针）

认：有容器 / 云函数 / 小程序云；能 `docker login` 或看到 Registry / Harbor / `/v2/`。

打：这是列表越权，不是未授权下文件。

1. 用**自己的号**登录制品库  
2. `GET /v2/_catalog`（Harbor 还有项目列表一类接口，同一枪）  
3. 回包仓库名对得上**别人租户** → 再 `/v2/<仓库>/tags/list`，`docker pull` 其中一个  

算成：catalog 里是他租户的私有仓，并且镜像能拉下来（层里是对方的应用/配置）。只证明自己能 login、只能拉官方公开镜像 → 不算。

假点：catalog 只回自己的；列出名字但 pull 403；本来就是公开库。单站没中不删短表这行。

和匿名 fileId 下载、和 OSS 桶策略全开都不是一条：本条是 **登录后 catalog 把全站私有仓交出来**。

### 登录前缀双胞胎（短表有指针）

认：业务 H5 把登录 RPC 写在 `/fapi/d/`（或同类需登录前缀）；同网关另有 `/fapi/n/`（或 n / unlogin / guest）未登录前缀。只打 JS 里的 d 口会看起来整段请登录。

打（不登录）：

1. 对照 d 口应请登录 / 要票  
2. 把 path 里的 `d` 改成 `n`，同一 RPC、同一对象 id 再打  
3. 换邻号；回包证件照 URL 跟着打开  

算成：不登录出他人身份证 / 证件照 / 手机。

假点：n 仍请登录；n 只有公开配置；只有自己刚交的补件。单站没中不删短表这行。

### 数字 RPC 邻 cmd（短表有指针）

认：业务页 JS 调数字 RPC 网关（`/data/{数字}/forward` 一类）。页面往往只写死一个 cmd（发票/广告一类）。邻号可能是另一套内部列表/写口，不要停在写死的那一个。`数字 RPC 网关` 是常见皮，没有这名仍打。

打（不登录）：

1. 抄 JS 里的 forward 地址，把 path 里的数字换成邻号  
2. 先空 `{"req":{}}` 看列表；对照写死 cmd 应是另一套业务  
3. 列表出内部记录再带 id 打邻写口（先假 id / 已空 id）。过了能改回的改回  

算成：内部发布/操作人正文，或字段被改。

假点：邻号仍是同一套公开接口；只出公开软件目录。密钥实值不进库。单站没中不删短表这行。

### 身份域账号 CRUD（短表有指针）

认：同产品业务前端的账号 CRUD 口回登录闸（`AuthFailure.NoLogin`）；另有独立 `*-ids` 身份子域，同一套 `/api/ms-account/` 不要 Cookie。GET list 可能 404，POST 才出数。

打（不登录）：

1. 业务前端同一条 list 对照应登录闸  
2. 改打 ids 子域 `POST /api/ms-account/user/list`，空包/`Offset+Limit` 都试  
3. 通了再 `generate` → `reset-pwd` 只带 Id（不带 Id 应参数错误）→ `delete` 清探测号  

算成：名单里是手机/企业邮箱；能建号、不验旧密改密、按 Ids 删。

假点：只打了业务前端就当没洞；list 出数但 Mail/Phone 全空且写口也闸。单站没中不删短表这行。

### 列表过滤详情不闸（短表有指针）

认：公开列表只出上架/公开；详情、hidden、tab、预览/导出用同一个业务 id。或列表有可见性查询参，默认把隐藏滤掉。或列表要登录/空包，详情只要数字 id + 业务键（workid 一类）。或对外详情把联系人/手机置空，审核/approval 详情用同一个业务 id。或文档 CMS 前端写死公开 `area` / `端` / `channel`，API 不鉴权。或对外搜索口有素材类型参（`materialType` / `tab`）默认公开值。或入驻/审核 query 只带业务 id 默认空壳。或同产品全量列表口不带可见性参，行里就带着 unpublished/offline 详情正文，公开搜索和橱窗详情仍闸。或浏览页/目录写访客请登录，同站 search API 未登录仍出内部/专有文档正文。

打（不登录）：

1. 记下公开列表 id；列表没有的 id 丢给详情 / hidden / tab；详情 403 再跟预览 / 导出  
2. 列表口自己加 `status=hidden` / `all` / `private`（或同类可见性参）。对照：不带参列表空或只有公开；同一 id 打详情仍「没有权限」也别停——闸可能只套在详情上  
3. **这条列表口 401 / 要登录 / 空包别停。** 换同产品另一条 guest 查询，看回包 `hidden` / `is_public` / `isHidden`。公开 contents 404 再打 hidden/contents 同一 id。详情只要数字 id + 业务键仍打。`is_secret=1` 仍出全文别停在标题。  
4. **详情 200 且 JSON 已写尚未发布 / 要登录 / `canAnswer=false`，别停。** 看同包 `savedConfigDraft` / 同类草稿字段有没有未发布正文。拦截文案和草稿可以在同一份 JSON。  
5. **对外详情 contact / mobile / email 置空别停。** 同一 id 再打审核 / approval / audit 详情；闸可能只套在对外展示口。  
6. **作品/项目有 `period=edit|publish`（或同类状态参）别停在 publish。** 不登录打内容口 `period=edit`。对照：同一 id 的 publish 说不存在/已删除，info 挡「没权读别人的」。edit 仍出正文才算。  
7. **JSON 列表筛选项 null / 缺字段报 Unknown / 参数错误，别停。** 把该字段改成空数组 `[]` 再打。后端常把空数组当成「不过滤」，匿名出下架 / 全表；缺参或 null 反而拦。对照：`null` 或删掉该键应报错或空，`[]` 才出已下架正文。
8. **文档 CMS 前端写死公开 `area` / `端` / `channel`，别停在默认值。** 未登录把参改成内部区（常见 1 vs 2、oa vs public）。对照：默认公开列表没有的接入/加固/内网算法正文，改参后列表或详情出来才算。  
9. **对外搜索口有 `materialType`（或同类素材类型）别停在前端默认公开值。** 对照公开规范/橱窗类型，改成内部市场素材、营销规范一类。path 缺 `/v2/` 报 `auth failed` 别停，按前端替换规则补上再打。算成：SSO 墙后、`isLimitAuth=1` 的内部物料名单/文件直链。  
10. **自助入驻/审核 query 只带门店号/业务 id 出空壳别停。** 加上审核状态=已通过（`baseInfoStatus` 一类）再打。回包 `uid` 立刻跟邮箱/联系人口。对照：不带状态应空壳或没手机。  

11. **证照图 JSON 没有 idNo 别停。** 详情出商标注册证 / 执照扫描件就打开看：注册人行常把身份证号拼在姓名后面。图上对得上人就算，不要只扫 JSON 字段。  
12. **导出任务列表 `pageNum=0` 空别当没数据。** 改 `pageNum=1` 再打，常能出全站别人的导出。抄 `fileName` / key 打 download。
13. **详情 telephone / 手机字段打码别停。** 未发布详情的 `requirement` / 备注 / 自由文本里常把完整 11 位手机写进去，接口只剥了结构化字段。
14. **全量列表口不带可见性参也打。** 行里直接看 unpublished / offline / rejected 详情正文。对照：公开搜索该 id total=0、人打开的橱窗详情说不存在。别只打公开搜索和详情页就当没洞。
15. **浏览页/目录写访客请登录别停。** 同站 search API（Keyword+Page+PageSize）未登录仍可能出内部/专有文档正文和详情 URL。详情页 JSON `isLogined=false` 仍出全文别停。对照：人打开的目录页写着访客请登录才能看。
16. **详情口已补 / 项目不存在别停。** 同站 visit-log / 埋点 / doc-add-visit-log 只要数字篇号仍可能出 OA/内部全文。对照：原详情口同一业务 id 应已拦。
17. **目录写访客请登录才能看/下别停。** 同站文件列表口（`getHomeCosFileList` 一类，`solution_id`+`version_id`+`folder_id=0`）未登录仍可能出培训media/白皮书/手册名单。列表 `file_id` 丢权限口（`getFilePrivilege`）看 `PreviewUrl` / 带签下载。对照：人打开的目录页写着访客请登录才能看、才能下。
18. **内容 SPA 按 `location.host` 正则选 Color/网关域，别打错 TLD。** JS 里常见 `.*\\.jd\\.hk` → `内容网关域`；打成 `打错的内容网关域` 会当没口。打法仍是不登录楼层 queryContents + previewContentDetail。对照公开详情应不存在/1414。
19. **文档站公开 itemList / 目录只出对外产品别停。** 未登录把 `item`（或同类项目号）改成纯数字自增打详情/page。正文可能是压缩 markdown，按前端同一套解开。对照：公开 itemList 没有的「对内/未对外/不对外」库才算。
20. **收集表/问卷填报详情 JSON 的 relative / 关联表挂着答卷 sheet 别停。** 人打开的是填报页，答卷表可能另有 id。不登录打答卷表详情/opendoc。对照：填报页只是题目，答卷表才出身份证/手机单元格。
21. **内容预览口只要数字 contentId，回包 `isDelete=1` / 已删除 / 未发布别停。** 对照公开橱窗应没有这篇。出整页 H5 或编辑器 JSON 才算，不要只看标题。

算成：未公开业务正文从列表出来（不是公开橱窗标题）；或他主体手机 / 证件；或证照图注册人行印着身份证号；或未发布/已下架作品源码正文。

假点：加了可见性参仍只出公开稿；只有标题没有正文；列表 401 就当没洞；edit 和 publish 出同一份已上架正文；空数组和缺参出同一份上架稿；`materialType` 仍只出公开橱窗；只带业务 id 出空壳就当没洞、没再加审核状态；只打了公开搜索/橱窗详情就当没洞；浏览页写请登录就当没洞；数字 item 仍是那几本公开文档。打错 Color 域当没口不是假点。单站没中不删短表这行。

### 过网关无需鉴权头（短表有指针）

认：物流 / 开放网关（`lop-proxy` 一类）前端 JS 写死 `Auth-Control: no-auth` + `LOP-DN`（业务域）。其它业务口请登录，带着这组头名单/运单口仍出数。

打（不登录）：

1. 抄 JS 里的 `LOP-DN` 和 `Auth-Control: no-auth`（或同类过网关鉴权头）  
2. 空翻页打站点/名单口；再换可枚举 `siteId` 打运单/详情  
3. 对照：不带头应请登录或 Host 未注册  

算成：出他人手机 / 住址 / 站点联系人，不是公开轨迹。

假点：头过了业务仍请登录；只有公开轨迹没有 PII。单站没中不删短表这行。

### 匿名会话读报名表（短表有指针）

认：BaaS 匿名 session；列表口不验管理员。页面写死几个管理员 ID，后台跳登录。Appwrite / Weavefox / TablesDB 是常见皮，没有这名仍打。前端常见 `Client().setEndpoint`、`X-Appwrite-Project`、`POST /v1/account/sessions/anonymous`；报名/活动表走 `tablesDB.listRows`。

打（不登录、不要管理员号）：

1. 无 Cookie 先打 `GET /v1/tablesdb/{db}/tables/{table}/rows` —— 基线应是 `total=0` / 空 rows（系统其实会拦游客）  
2. `POST /v1/account/sessions/anonymous` body `{}`，头带同一 `X-Appwrite-Project`，拿 `Set-Cookie: a_session_…`  
3. 带着这条匿名会话再打同一 listRows  

算成：匿名会话后 `total` 相对空会话暴涨，rows 里是**别人**的手机 / 邮箱 / 报名正文。管理员白名单只写在浏览器里、没套到列表口。

假点：无会话已经是全表（那是完全无鉴权，另记）；匿名会话仍空或只有自己刚填的一条；DELETE 别人行 401 只说明写没开，读仍算；公开运营展示名单。单站没中不删短表这行。

和「无 Project key、连匿名会话都不用就能 CRUD」不是同一枪：本条关键是 **匿名 session 被当成已登录，且不验管理员**。

### 云开发匿名用户表（短表有指针）

认：云开发开了匿名登录；有低代码数据源函数。控制台或前端能抄到环境 id。不是短表已有的 proxy `targetUrl`，也不是 BaaS `sessions/anonymous`。

打（不登录）：

1. `POST https://{envId}.api.云开发网关/auth/v1/signin/anonymously`，头 `x-device-id`，body `{}`，拿 JWT  
2. `POST /v1/functions/lowcode-datasource`，`Authorization: Bearer` 那张票，body `dataSourceName=sys_user`、`methodName=wedaGetRecords`（`getRecords` 同源）  
3. 对照：同一网关 `GET /auth/v1/user/query` 应拦匿名；数据源名 `users` 常行权限失败，**不要停**，`sys_user` 才是系统用户表  
4. HTTP 网关（常见 `/web?env=`）回 `LOGIN_TYPE_DISABLED` **别停**：改 `POST /web?env=`，`auth.signInAnonymously` → `auth.getUserInfo` 拿 `jwt;expire`（**不要去掉分号后缀**）→ `functions.invokeFunction`，同样打 `sys_user` / `wedaGetRecords`  
5. 旧 HS256 票丢 `/v1/functions` 报 `KID_INVALID` → 这枪没成，换第 4 步网关票，不要当「数据源不存在」  
6. `sys_user` 行权限失败 **别停**：同一张 `/web` 匿名票改 `database.countDocument` / `database.queryDocument`，`collectionName=users`（互联登录落库的用户表，不是数据源名 `users`）  

算成：records 里是**他人**手机 / 邮箱 / uin / 超管标记，不是自己刚匿名建的空号。

假点：行权限拒匿名且 `/web` users 集合也空；数据源不存在；只出演示 todo/sales；旧 HS256 票 `KID_INVALID` 当没洞。HTTP 网关 `LOGIN_TYPE_DISABLED` 不是假点。数据源名 `users` 失败不算假点。单站没中不删短表这行。密钥实值、某次 JWT 不进库。

### 详情抄 openid 再打信箱（短表有指针）

认：招募/指定用户详情带 `specifyUsers` 或 openid；H5 写死 SHA256 请求签名；或邀请页地址已有 `?openid=`。

打（不登录）：

1. 调详情抄 openid  
2. 钥算 Signature 再打 `/msg/message/list/`  
3. URL 上已有 openid 则再打无签名的 `get_invite_code` / `count` / `detail`，名单 `uid` 可再填回去  

算成：换 openid 信箱或邀请码/名单变，出现对方积分过期通知或打码手机昵称。

假点：列表人人一样只是全站广播；接口要真登录/签名；假 openid 没有邀请码；公开运营名单。单站没中不删短表这行。

### records库 nodeId 未授权读全文（短表有指针）

认：发布页 conversation-data 的 artifactMap 有records库 `nodeId`；主站 `/space/d/` 要登录。

打（不登录）：打发布域 `POST /space/api/page/share/query/pagechunk`（body=`pageId=nodeId`）。对照：分享页自己的 pageId 应是「不可分享」。

算成：拉到他人文档全文（标题/角色卡/大纲，`role=editor`）。

假点：只读到已发布 HTML 快照；pagechunk 对records库也 12607；正文已在对话快照里。单站没中不删短表这行。

### 匿名 CSRF 头读详情（短表有指针）

认：匿名 CSRF/临时 token 口直接出 token；业务详情只验这个头不验登录；query 的数字 id 和回包 `data.id` 可以不是同一个号。不是助手 `GetHistoryList` 可选登录头（那枪见「助手历史未授权读他人任务」）。

打（不登录）：

1. 打 csrf/token 口拿头  
2. 详情换数字 id（1、2、13），**query id 对不上回包 id 别停**  
3. 对照：不带头应失败；不存在的 id 应「不存在」

算成：未上架/测试训练脚本或内部会话正文（cardMap/对白全文，不是标题）。

假点：只有公开广场；token 过了仍空。单站没中不删短表这行。

### 自定义身份头当会话（短表有指针）

认：管理后台 SPA 请求拦截器把自定义头（`User-Id` / `employeeId` / `X-User-Id` 一类）当登录身份，不要 Cookie。未登录名单口往往只出数字员工/BD 号，没有手机。

打（不登录）：

1. 抄 JS 拦截器里的头名  
2. 未登录打名单（空参/总部筛）。只有数字号**别停**  
3. 把头换成这个号打当前人信息口。对照：不带头或填 `1` 应查空  

算成：出他人姓名+11 位手机。

假点：头过了仍空；名单已经出手机（那是另一条）。单站没中不删短表这行。

和「匿名 CSRF 头读详情」不是一条：那条是临时 token 头，这条是**身份数字号进头**。和「入驻 H5 空参出招商通讯录」也不是一条：那条类目空着整表出手机，这条名单没手机还要再把头当身份打一枪。

### 客户详情口还吃 phone（短表有指针）

认：CRM / 企业 IM 客户详情口前端只写了外部联系人 id（`externalUserId` / `contactId`）。只带这个 id 回空壳。后端还吃 `phone`，常还要 `tagShow` / `udfShow` 才出内部字段。

打（不登录）：

1. 对照只带外部联系人 id 应空壳  
2. body 加 11 位 `phone`，并把 `tagShow` / `udfShow`（或同类展开字段）打开  
3. 空号 / 乱填应空壳；换号必须换人  

算成：出他人姓名 / 公司 / 内部 UDF，且查的就是这个完整号。

假点：phone 一律空或只出同一条测试号。身份证槽位空不算假点。单站没中不删短表这行。

### 助手历史未授权读他人任务（短表有指针）

认：助手/Agent 前端有 `GetHistoryList`；登录态只在可选头里。不是对话口工具执行（那枪见 `agent-tool-exec-test.md`）。

打（不登录）：调列表，换 guid 对照是否同一批；再把 `session_id` 丢给 `GetHistory`。翻页 `last_ts`+`direct=back`。

算成：列表/详情出现**他人**任务原文（下载、订阅、对话卡片），不是公开广场。

假点：只出广场 `GetSquareTasks`/`share_id`；换 guid 列表变空或只剩自己的。单站没中不删短表这行。

### 写死 appKey 打业务表（短表有指针）

认：前端 `AV.init` / LeanCloud 写死 `appId`+`appKey`（或 `X-LC-Id`/`X-LC-Key`）；或 nocode/supabase 落地页写死 `role=anon` JWT。

打（不登录）：带这两头打 `/1.1/classes/*`：先 `_User` 对照应 403，再扫业务表 count/limit，能写就改探测字段再删回。supabase 带 `apikey`+`Authorization: Bearer` 打 `/rest/v1/` swagger 列出的表。**rest 表 403 / 只有公开运营配置别停**：改打 `POST /storage/v1/object/{桶}/{官方前缀}`，头 `x-upsert:true`（见 `file-upload-test.md` STS 第 9 步）。

算成：业务表 `count` 海量或出现**他人**稿/邮箱/电话；PUT 改别人 `objectId` 成功。

假点：`_User` 和业务表都 403；只能读自己刚建的；只能 LIST 公开运营配置。单站没中不删短表这行。密钥实值不进库。

### 填表模型带标准答案（短表有指针）

认：问卷/测验填表模型口；前端写死业务 id 或白名单（`FORM_WHITE_LISTS` / 演示 encryptFormId 一类）；回包 schema 带 `answer` 标准答案或内部审核题干/样图。不是公开报名表标题，也不是已经交过的答卷列表。

打（不登录）：

1. 从 JS 白名单/演示 id 打 getModel / schema / getFormModel，别停在表单标题  
2. schema 里的样图 preview/imgUrl 跟着打开  
3. 管理 list/export/fillList 另打；没有填写记录不要编答卷  

算成：内部测验正文+标准答案，或证件/执照样图真下到。

假点：只有公开报名表标题；schema 没有 answer；只有自己刚填的答卷。单站没中不删短表这行。

### 未授权内部话术正文（短表有指针）

认：客服/开发者支持台 umi 有 `getKnowledgeList.json` + `getKnowledgeInfo.json`；或大厅/帮助 HTML 详情口吃数字篇号；或对外公告 JSON（bulletin / getbulletin 一类）用 `callname` + `callcontent` 当 RPC，页面只调公开菜单（`getKnowledgeByMenuId`）；或未登录 CMS `siteList` / `contentList` / `content` 能列出非官网站点，且频道名带「内部knowledge-base」；或客服/IT chatbot 未登录检索口（菜单 id + 模糊 searchText），正文在 `buttonList`/`searchList` 的 `behavior.value`，不在标题 `content`。不要只认 umi 那一套 json。

打（不登录）：对照 `queryUserInfo`/`queryFeedbackList` 应 deny。列表 `categoryId` 从 1 试，再把 `id` 丢给 Info。**没有 json 列表也打 HTML 详情**（`showKnowledgeInfo.htm?knowledgeId=` / `help_detail.htm?help_id=`），用现代 UA（IE 可能触 netd）。公告口：页面公开菜单对照条数很少时，把 `callname` 换成 `getKnowledgeList`（`callcontent` 带翻页），再 `getKnowledge` 打详情 id。CMS：先 `siteList` 抄非官网 siteId，再 `contentList` 看频道名，换 siteId 打 `content` 详情；默认官网 Banner 不是这枪。网关报 `loginMode is null` 别停，头加 `loginMode: 0`；siteList 仍缺 siteId 失败时，直接带内部 siteId 打 contentList。content 也要带 siteId，缺了会当没正文。chatbot：`chat_dir_id` 一类目录口 401 别停，改打检索口（`search_recommend` 一类），`searchText` 填常用字、`id` 填菜单号；正文看 `behavior.value`。

算成：列表 `pager.items` 上千或 count 海量，且 Info/HTML/详情/`behavior.value` 出**内部**话术/协查/短信/运营knowledge-base正文，不是公开 FAQ / 对外协议。

假点：只有公开帮助稿/错误码/对外协议/官网 Banner；Info 只要标题；工单口也放行（那是另一条）；只打了默认官网站点；dir 节点 401 就停；检索口只出标题 content。单站没中不删短表这行。

### 开放支付假签枚举 appId（短表有指针）

认：开放支付 / 进件网关 body 有 `appId`+`sign`（可再加 `random`/`merchantId`）。假签时活应用回 `MERCHANT_NOT_EXIST` 或 `SUCCESS`，死应用回 `SIGN_ERROR` / `APP_NOT_FOUND`。不是「JS 写死盐自己算」那一行，也不是文档里抄出真 AppSecret。

打（不登录）：

1. `sign` 填一串假值（32 个 `a` 一类），扫 `appId`，对照回码  
2. 活的再打商户查询（`query` / `query/v2`）换 `merchantId`（文档示例号、邻号）  
3. 同一套假签再点签订/协议口，只看回包是否接受；不要批量签、不要改结算卡  

4. **ST / 演示收银台预下单**假签或 node 代签也能进生产网关时：不登录 POST `/api/precreate` 或 `/demoapi/precreate`，sign 填假值 + 活 appId + 他人 merchantId。本站查单 404 别停，打生产 `/api/pay/query`。

算成：出他人商户身份证 / 银行卡 / 手机；或生产查单 `ORDER_NEW` 且有他商户交易号。

假点：假签一律 `SIGN_ERROR`；`SUCCESS` 但证件卡号全空；文档示例 appId 已死；只回 `channelPayerNo` 且生产查单不存在；只能给演示店挂。单站没中不删短表这行。密钥实值不进库。

### 短链 302 query 带手机（短表有指针）

认：短信/运营短链解析站；猜中的短码 302 到落地页，query 明文带 `phone` / `name` / 金额。首页可能是 OpenResty 欢迎页、302 到品牌官网、或 `index.html` 只有 Hello，**不要只看 `/` 当整站证伪**。

打（不登录）：

1. `/open` `/app` `/s/` `/www` 加短字典（`aaaaa`、`1`、`1234`）  
2. 只看 302 目标 query，不要跟到落地业务域  
3. 对照：对不上的码应失效页、没有手机号  

算成：跳转地址里是**别人**的手机/姓名。

假点：失效页；公开营销无 PII；短码要真短信才解析。单站没中不删短表这行。首页 Welcome / 302 官网 / Hello 壳不是假点。

### 隐私号失败回真实号（短表有指针）

认：订单/门店/物流 H5 调隐私号或虚拟号（AXB）口；未登录只要业务 appid + 可遍历对象号。失败时回包把真实 11 位手机当下发，文案还写获取隐私号失败、将用真实号码拨打。不要只当虚拟号口丢掉。

打（不登录）：

1. 前端抄 functionId / 隐私号 path，对象号从 0、2、邻号打  
2. 对照：非数字对象号应参数错、没有手机  
3. 换号号码变才算批量  
4. 网关空 Origin 或本站 Origin 回 `cross-origin 403` **别停**，改业务域 Origin（订单 / 购物车 / H5 域，不钉某一家 host）再打  

算成：回包是**他人**真实手机。

假点：只出虚拟号/中间号；必须登录；换号号码不变；空 Origin 403 当没口。单站没中不删短表这行。

### 入驻 H5 空参出招商通讯录（短表有指针）

认：入驻 H5 有招商电话页；同套其它 settle 口 302 未登录。JS 里按品类查 BD 的 RPC（queryBd / bdInfo 一类），industry/类目空时 body 空。页面上可能只摆 400 热线。

打（不登录）：

1. 从招商电话页 JS 抄网关 `/api` 和 functionId  
2. 类目字段空着打（`body={}`）。对照：填死数字类目号常回空数组  
3. 同套其它 settle 口 302 别当整站没口  

算成：内部 BD 姓名+11 位手机+企业邮箱整表。

假点：类目填死数字出空数组就当没口；把页面公开 400 热线当这枪。单站没中不删短表这行。

### 入驻 H5 写死 token 换 pin（短表有指针）

认：入驻 H5 / 小程序打包 JS 把 OCR / 企业信息口 token 写死。JSON 口只要 pin（或同类账号）+ 非空 token，不要 Cookie。对照：token 留空或省略只回空 data。

打（不登录）：

1. 从入驻 chunk 抄写死 token 和企业信息 path  
2. 空 token 对照应空 data  
3. 带写死 token，pin 用 `admin` / `test` / `0` / 短账号再打  
4. 写死那串过了，再填任意非空串对照（过了说明不校验 token 内容）  

算成：换 pin 出对应真实手机。

假点：空 token 也出数（完全无鉴权，另一条）；token 过了仍只出自己刚入驻的。单站没中不删短表这行。密钥实值不进库。

### 公司名/抬头自动完成（短表有指针）

认：报名/入驻/发票抬头自动完成；页上下拉只出公司名；接口走工商/邓白氏 Match 或抬头 suggest。前端可能把该口标 `auth:true`，服务端仍不验登录。

打（不登录）：

1. POST 名称+国家码，或抬头口。suggest 字段可能是 `prefix`（填 `title` 会报「关键词不能为空」像没入口）  
2. suggest 报关键词空别停：同产品还有专票/工商补全口，字段就是 `title`，前端标 `auth:true` 仍打  
3. 对照页面下拉：页上只有公司名，回包多出手机/卡/住址才算  

算成：回包出现负责人或开票手机/住址/银行账号，且对得上人。

假点：下拉和接口都只有公司名；必须登录；公开企业名录没有电话；前端标 auth 就当没洞。单站没中不删短表这行。


### Mass Assignment / 隐藏可写字段（短表有指针）

认：注册 / 改records / 建用户的 JSON 比页面控件多；Swagger、管理员「建用户」或前端注释里多出 `role` / `isAdmin` / `verified` / `tenantId` / `balance` / `permissions`。

打：

1. 对照管理员建用户 vs 自己注册/改records，差出来的字段才塞。没有对照就从 Swagger / JS 抄。
2. 只对自己的号塞一次；过了立刻改回。不要改别人已有账号的角色。
3. `__proto__` / 嵌套 `user.role` 也试；这和 PP 模板 RCE 不是一条（PP 见 `prototype-pollution-test.md`）。

算成：自己号变成高权，或余额/认证状态真变。字段吃了、权限没变 → 没成。

假点：只能改展示名；公开运营开关；服务端白名单吞掉多余键。单站没中不删短表这行。没注册/改records口不要发明字段。

## 12. QUICK IDOR CHECKLIST

```
□ 有对照号更好；单号用列表/回包/邻号，不为第二号磨注册
□ Map all API calls that contain object IDs (Burp History export filter)
□ Test all HTTP verbs on each endpoint（写：先 POST 添加，再删自己加的那条；勿删别人已有对象）
□ Test ID in all locations: path, body, header, query, cookie
□ Try sequential IDs (−1, +1 from your own）
□ 密文 id：JS 有公钥就自己加密相邻数字（见「密文 ID」）
□ 列表租户字段试 0 / -1 / 空（换真实他 ID 失败也试；见「哨兵租户」）
□ 有 Registry/`/v2/`：自己号打 `_catalog`，能否列出并 pull 他租户镜像（见「制品库 catalog」）
□ BaaS 匿名 session 后再 listRows，对照无 Cookie 的空表（见「匿名会话读报名表」）
□ 云开发匿名登录后再打 `lowcode-datasource` 的 `sys_user`/`wedaGetRecords`；HTTP 网关 `LOGIN_TYPE_DISABLED` 别停（见「云开发匿名用户表」）
□ 详情带 specifyUsers/openid：抄 openid 打信箱（见「详情抄 openid 再打信箱」）
□ 发布页 artifactMap 的 nodeId 打 pagechunk（见「records库 nodeId 未授权读全文」）
□ 助手 GetHistoryList 可选头：不登录调列表/详情（见「助手历史未授权读他人任务」）
□ 前端写死 LeanCloud/supabase anon：打业务表（见「写死 appKey 打业务表」）
□ 知识列表+详情未登录出内部话术（见「未授权内部话术正文」）
□ 开放支付进件网关假签枚举 appId，活应用再换 merchantId（见「开放支付假签枚举 appId」）
□ 短信短链首页 Welcome / 302 官网 / Hello 壳别停，猜 /open /app /s/ /www 看 302 query 手机（见「短链 302 query 带手机」）
□ 未登录隐私号/虚拟号口失败时看是不是真下发真实手机，对象号可遍历（见「隐私号失败回真实号」）
□ 报名/入驻/发票抬头自动完成：不登录打 suggest（prefix）和专票/工商补全口（title）；前端标 auth 仍打（见「公司名/抬头自动完成」）
□ 入驻 H5 招商电话页：JS 查 BD 口类目空着打，不要填死数字（见「入驻 H5 空参出招商通讯录」）
□ 后台 SPA 把头 User-Id 当身份：名单只有数字号别停，把头换成这个号打当前人信息口（见「自定义身份头当会话」）
□ 浏览页/目录写访客请登录别停，同站 search（Keyword+Page）未登录也打，跟详情 URL（见「列表过滤详情不闸」第 15 步）
□ 目录请登录才能下别停，同站文件列表口+权限口 PreviewUrl 未登录也打（见「列表过滤详情不闸」第 17 步）
□ 对外搜索口 `materialType`/`tab` 改成内部类型；path 缺 v2 报 auth failed 别停（见「列表过滤详情不闸」第 9 步）
□ 入驻/审核 query 只带业务 id 出空壳时加审核状态=已通过，回包 uid 跟邮箱口（见「列表过滤详情不闸」第 10 步）
□ 文档站公开 itemList 只有对外产品别停，item 纯数字打详情/page（见「列表过滤详情不闸」第 19 步）
□ 收集表/问卷填报详情 relative 挂答卷 sheet 别停，不登录打答卷表（见「列表过滤详情不闸」第 20 步）
□ supabase anon JWT：rest 表之外打 Storage REST + x-upsert 盖官方前缀（见 file-upload STS 第 9 步）
□ Try UUIDs/GUIDs collected from your own account data
□ Test sub-resources (attachments, comments, transactions)
□ Test admin endpoints directly (BFLA)
□ Test POST/PUT body for extra fields (mass assignment)
□ Compare JSON response field count vs documented fields (hidden fields)
□ Test state/status：只改自己的单 / 能改回去的测试字段
□ 商家绑促销/券：改 productId 挂到别人的货，C 端价掉才算（见 logic-test.md §1.4）
```

---


商家绑促销/券到 SKU：只校验券归属、不校验商品归属 → 自己的券 + 别人的 productId，C 端价掉才算。正文在 logic-test.md §1.4。
