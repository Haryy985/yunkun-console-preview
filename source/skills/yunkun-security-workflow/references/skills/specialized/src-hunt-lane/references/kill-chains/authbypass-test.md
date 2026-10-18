# authbypass-test

打开是登录页 / SSO → 表单壳听 `dig-scope` §4.1.1：找业务面；别按本文件从头跑字典 / 验证码 / 无限试密。  
发会话、重置、改绑、换票、2FA 按本文件 + `dig-scope` §4.2.2 探针打，不要因为 §4.1.1 整摊跳过。表是每站下限，不是只准打这几枪。  
中间件裸默认口可一眼。滑块 / 发码 / 没进号的试密 → 转认证链，别停半截。写不写只认 `vuln-report-format.md`。
英文字典/验证码 20 法/重置矩阵已砍；短表指针用标题搜。Host 毒重置见 `http-host-header-test.md`。扫码登录 CSRF 见 `csrf-test.md` §18。

# Authentication Bypass

### 未登录改密口

认：改密/首次设密/重置最后一步，未登录也能打到。body 常见旧密或验码字段（`old_pwd` / `sms_code` 一类）+ 新密 + 身份 id。官方页写着要短信/旧密，接口仍可能吃空串。

打：不登录。旧密/验码置空或省略，新密过复杂度。对照：填错验码应拦；不存在的身份 id 应查空。同一密再打若报「与历史密码重复」=已经写进库，不是空成功。过了立刻改回；改不回就停，不要再换别人、不要登进去。

出：未登录 Success，改掉别人的密。

假点：只回 0 没写库；必须真旧密/真短信；Success 但登录走另一套 IdP、密没跟过去。

### IDaaS 未占用密保题

认：IDaaS 忘记密码。匿名提交用户名就签发 JWT，且 `scope` 是 `_`（不是 reset）。密保题库 id 可枚举，已绑在该号上的 id 写会 409。常见 IDaaS 皮 的 path 是 `forget_password/v2/sq`，同形态换皮照打。

打：

1. 不登录 POST sq，body `{"username":"admin"}`（或存在性口先确认的号）。
2. 带 token GET question，记下已绑 id。
3. POST `update_question` 写**未占用** id 的答案（已绑 id 不要硬写）。
4. `verifyquestions?type=RESET_PASSWORD` 带自写的三道题，换 `self.password.expired.reset`。
5. `set_password`：过了立刻改回；改不回停在回包，不要把管理员密改掉留下。策略拒且密保没写上 → 半条，不按打穿进表。

出：密保答案写到别人号上（改绑），或用该令牌真改掉别人的密。

假点：sq token 直接 set_password 报 `scope is not expected`；只能写已占用题 409；verify 错答；策略拒且密保没写上。单站没中不删短表这行。


### 未登录培训绑定口（短表有指针）

认：培训 / 学院 H5 网关把账号绑定、查询 RPC 放进未登录的 client 前缀（`/spapi/v2/client/` 一类）。body 吃 C 端 uid（`mtUserId`）+ 商家号（`bAccountId`），不要 Cookie。

打（不登录）：

1. POST bind：uid 换成别人、商家号换成自编  
2. POST querybind：只带 uid 或只带商家号  

算成：出对方手机，或绑定成功改绑。

假点：只回未登录/参数异常；uid 一律未绑手机且写不进。单站没中不删短表这行。密钥实值、某次手机号不进库。

### 联合登录只吃 uin（短表有指针）

认：云活动 / 黑客松 / 伙伴门户的签发口 body 只有 `provider` + 云账号 `uin`（或同类 uid），不验 OAuth `code` / `ticket` / `id_token`。公开列表或 by-code 能问出别人的 UIN。

打（不登录）：

1. 从公开伙伴/用户列表抄 `uin` 或先打 by-code  
2. `POST` 签发口，只带 `provider` + 对方 `uin`  
3. 拿 JWT 打 `me` / 用户信息  

算成：me 是**对方** UIN，能当这个号用。只出游客空号 → 没成。

假点：必须真 OAuth code；uin 未注册直接拒；票发了但 me 仍是自己。单站没中不删短表这行。密钥实值、某次 JWT 不进库。

### 活动实验室 login 不校验互联票（短表有指针）

认：活动页 login 吃互联 `openid`+`acctype`+`access_token`，服务端**不去互联校验**这张票。对照闸：同一套参数 `acctype=wx` 或 `access_token` 留空会登陆失败；`acctype=qq`/`qc`/`pt` 加假 token 仍发票。不是空密/omit 通用枪，也不是云活动只吃 uin 那条。前端 `fx_act_helper.js` / Milo 登录助手是常见皮，没有这名仍打。

打（不登录）：

1. POST `/api/login`（活动前缀跟页面，不要只认某一条 path），`access_token` 填假值、`acctype` 填 qq（qc/pt 再各一枪），`openid` 填可枚举号  
2. 拿 JWT 打 getRoles（按区服）/ bindRole 一类角色口。对照：不带头应要登录；wx/空票应失败  
3. 角色列表空别当没洞，换区服、换邻号；绑口跟到能绑上再停。过了能改回的改回  

算成：票里是对方 openid，角色列表出现他名下角色名/`charac_no`，或能绑上。

假点：只签发空号且角色列表全空、绑不上游戏角色；必须真互联票。密钥实值、某次 JWT 不进库。单站没中不删短表这行。

### 运营配置深链里的会话票（短表有指针）

认：未登录就能打的首页/运营配置 JSON，banner 跳转 URL（skipPath / jumpUrl / schema）query 里带着 `token` / `apptoken` / `access_token` 这类能当会话的串。对照闸：不带这串打 me 应登录过期。

打（不登录）：

1. 抄配置里整段跳转 URL，把 query 里的票放到 Token / Authorization 头  
2. 打 me / user info / 钱包  
3. 对照：同一口不带这串应过期  

算成：me 是别人的手机/角色，会话能当这个号用。

假点：票过期或占位串登不进去；配置只出运营文案没有票。密钥实值不进库。单站没中不删短表这行。

### 验签失败 302 回显算出的签（短表有指针）

认：支付/开放网关信封有 `sign_data`/`sign` + 商户号。假签时 **GET / DELETE / OPTIONS / HEAD 都可能 302**，Location 或 `msg` 里带服务端刚算的合法签（`calculateSign is:` 一类）。有的网关 GET 只 HTML/ILLEGAL_SIGN，换 METHOD 才漏；有的网关 **GET 自己就会 302 漏签**，不要只认 DELETE/OPTIONS。

打（不登录）：

1. 假签打签约/下单口，**GET 先看 Location** 有没有 `calculateSign`  
2. GET 没带签再换 DELETE/OPTIONS/HEAD，从 Location URL 解码抄签  
3. 填回 `sign_data` 打查询/订单。对照：假签仍失败，真签出他商户单  

算成：过签查出**他商户**未公开订单/账户正文（金额、关单原因、卡种）。

假点：所有 METHOD 都只报验签失败、没有算出的签；抄回去仍拒签。密钥实值不进库。单站没中不删短表这行。

### 身份供应商代调报错回显 token（短表有指针）

认：业务口帮前端调身份供应商。常见三皮：① 小程序码/跳转码口，`path`/`page`/`pagepath` 你能填，非法页时 message **原样带回** `access_token=`；② QQ/OAuth 换票口把 `client_secret` 拼进上游 URL，`redirect_uri` 改外域让下游 HTTP 5xx，报错 URL 把 `client_secret=` 吐给前端；③ 未登录业务口 `GET /api/business/access-token`（或同类领票 path）直接 200 出票，不必等报错回显。假码打本域回调往往只回业务错码、不带钥，**别停**。清单里有领票口，别只打签发码口。

打（不登录）：

1. 清单有领票口先 GET，200 且 `data` 是 `access_token` 就当已经抄到，不必再走非法 pagepath  
2. 没有领票口：path 填外站或明显非法页；换票口先假码+本域 `redirect_uri` 对照  
3. 换票口再把 `redirect_uri` 改外域，从报错 URL/message 抄 `access_token=` 或 `client_secret=`  
4. 身份票打 `身份供应商 API/cgi-bin/account/getaccountbasicinfo`，再 `/wxa/generatescheme`；QQ AppSecret 拿授权码换该应用 access_token / 打本站换票  

算成：能问出官方号主体/appid，或签发官方正式跳转码；或完整 AppSecret 能换成该应用 access_token。

假点：报错只有 ErrCode 没有 token/secret；假码本域回调不带钥；token 调身份接口无效；secret 换不了票。密钥实值不进库。单站没中不删短表这行。

### 开通页 RPC 下发 signKey（短表有指针）

认：支付 UISDK/开通页。未登录打配置或 thrift 口，body 只有可枚举 `appId`。死应用 `APP_NOT_FOUND` 且无钥，活应用回完整 `signKey`/`signMethod`。不是文档/webpack 写死（那枪见下一节）。

打（不登录）：

1. 换 `appId`。对照死号应无钥  
2. 抄活号 `signKey`，按回的 `signMethod`（常见 MD5，参按字典序拼 `&key=`）现签  
3. 打支付查询。对照假钥 `SIGN_ERROR`，真钥过签  
4. 过签别停在查询。打改结算卡 `change/card`：支行号差分出 `changeId`，原值报未变更才算写上。SUCCESS 但 `changeId=null` / 费率没变不算。过了立刻改回  

算成：完整 AppSecret 且真钥过生产验签；或查出他商户身份证/卡/挂单；或建出虚拟门店 poiId；或改掉他商户结算卡/开户行（`changeId` 有值）。

假点：回的是占位钥过不了验签；死活都 `APP_NOT_FOUND` 且无钥。密钥实值不进库。单站没中不删短表这行。

### 调试文档写死 AppSecret（短表有指针）

认：文档 / Demo / 官方包 / 接入 HTML / npm 历史包写死**完整** AppSecret（appId+appSecret、signKey、商户 pfx、SDK 钥），不是 `your-secret` / `YOUR_ACCESS_KEY` 占位。**wiki 把钥打成星号，同一套官方 zip / Demo / HTML 示例仍明文，别停。npm 旧版别当下架。** 常见皮：调试指南 `config.js`、FAQ 链到的 GitHub `properties`、文档操作截图 PNG、SDK zip 里 `prod/config.properties` / Demo Java `baseInfo`、接入 HTML 的 `MSDKConfig.json`、公网 npm `index.js`。旁边有换票口或按文档现签的生产口。算法在下面打哪，不钉某一家产品。

打（不登录）：

1. 从文档、FAQ 链过去的 GitHub、**或文档页操作截图**抄 appId/accessKey、appSecret/secretKey（实值只进报告，不进本库）。截图 OCR/看图即可，别停在 config.js
1b. 企业 IM/开放通讯录：换票后 `user/get` 或 `user/list` 的 mobile 是空串**别停**，打 `linkedcorp/user/get`（body `userid`）。对照通讯录口无手机、互联企业口出 11 位手机才算升链
2. 换票口：code 填假串。对照：secret 改成等长假值应「业务参数非法」；真密钥变成「授权码过期/无效 code」。本站没有 `/proxy`、换票口在另一棵业务域不打时，同产品独立 oauth `/oauth/v2/token` 直接 POST `grant_type=client_credentials`（对照假钥 `invalid_client`）
3. 分销/开放查询口：按文档 HMAC（key 小写字典序拼接，有 `test=test` 一类联调参就带上）。对照：假 secret 签名失败，真钥出该应用下的供给名单
4. 官方 CLI 改调试包/正式包口（`package/changeDebugVersion`、`changeVersion` 一类）。对照：假钥「密钥错误」，真钥过鉴权变成「游戏包资源不存在」也算钥活了，不要停在没上传真包。**改包口不要只打 upload/换版本**，WASM 分包 `packageBind` / `build` / `queryBuildResult` 也打；假钥密钥错误、真钥能绑到该应用名下算出写上
5. 网关写了服务器 IP 白名单：试 `X-Forwarded-For: 127.0.0.1`。无头应无权，加上后真钥出供给
6. 供给查询过签后别停在名单：打下单前校验（check），回真卖价/结算价再 booking。对照假钥签名失败。下出订单号立刻取消；改不回停在回包，禁止批量真下
7. GitHub/SDK 公开演示 RSA+3DES：生产 XML pay-gate 仍认时，现签 `POST /service/query` 解开返回密文。对照假钥验签失败。邻号共用同一把演示 RSA 时回业务错（订单号有误）不是验签失败，钥仍活。禁止真下单/退款
8. 开放文档对象存储超长预签名 SDK zip：解开 `prod/config.properties`（及国密配置）+ pfx，用配置密码开证书，抄商户号/signKey 打**生产查单**。对照假钥 SIGN_ERROR。只查单，禁止真转账
9. 游戏 MSDK：wiki encode=2 时 `sig=md5(msdkKey+timestamp)`（不绑 path/body、无时间窗）现签生产/测试 `/auth/*`。对照假签 `sig error`；真签过闸转到身份供应商 `access_token expired` / 手Q `0x711` 一类业务错也算钥活，别当没洞。V3 `/auth/guest_register`：`reqid` 明文 uuid 会 `decrypt faild`，用 msdkKey **前 16 个 ASCII** 做 QQ TEA（16 轮大端 `oi_symmetry_encrypt2`）再 hex；开出 guestid 再打 `/auth/guest_check_token`。没有玩家有效社交 token 就停，禁止为进号磨登录
10. 官方接入 HTML 示例 `MSDKConfig.json` 明文 `MSDK_SDK_KEY`：`source=0` 时 `sig=md5(path+"?"+除 sig 外字典序 query+body+钥)`；decrypt 用 `md5(ts+密文+钥)`（文档示例 ts/密文/签能对上再拿到生产打）。对照假签 `invalid sig`；真签过闸后打游客登录，`channel_info` 只放自编 uuid 仍签发 openid/token/jwt 就算用户票。禁止为进别人号磨渠道 code
11. npm 历史包写死 `developerId`+`signKey`：合作中心网关假签活号回「签名错误」、死号回「开发者数据异常」，可枚举活号。真签 `sign=SHA1(signKey+按 key 排序的 key+value)`。隐私号批量拉取口不带门店 `appAuthToken` 也能 `OP_SUCCESS`；换票口真签过闸转到「入驻状态不正确」也算钥活。空数组不是假点（当前没降级单）。**别停**：同套更高版本 npm 把 `appAuthToken` 写死别当联调丢掉，带令牌现签 `queryPoiInfo`，换 `ePoiId` 出他商户店名+11 位手机。禁止真下单/磨授权码
12. 充值/企业支付 Demo HTML（`enterprise_client` 一类）输入框写死 AppId+AppKey：别当联调占位。不登录抄出来，HMAC-SHA1 源串 `GET&urlencode(path)&urlencode(按 key 排序的 kv)`，密钥 `AppKey+'&'`，Base64 得 sig。对照假签 `sig error`。真签打生产 `/v1/r/{appid}/open_order` 出 `token_id`。只下未支付单，禁止走收银台付款/退款。实值只进报告
13. 登录页 JS 把钉钉 ISV `suiteKey`+`suiteSecret` 写成 `clientId`/`clientSecret`（值 `suited` 开头）：别当 OAuth 占位。不登录 POST `https://oapi.dingtalk.com/service/get_suite_token`，`suite_ticket` 填假值。对照假 secret 回「不合法的套件key或secret」。真钥出 `suite_access_token`。实值只进报告
14. 控制台 webpack/Vuex 默认地图 key 别当占位。假钥对照后不要只打逆地理：打地点云/图层 `table/list`，有表再 `data/list`。只逆地理通、表是空的 → 假点。实值只进报告
15. 官方文档/接入 HTML 示例 curl 写死 Gamekey：别当占位。不登录抄 AppID+Gamekey，`sign=md5(mod,func,appid,time,postdata,key)` 现签生产排队网关 `getZoneListCount`。对照假钥 `req sign error`。真签 `ret=0` 出区服在线。只查排队/在线，禁止往队列插人、禁止把人退出排队。实值只进报告

算成：钥是活的，能换成该应用的用户票，或业务查询出该应用下的供给名单，或给该应用绑上 WASM 分包版本，或 booking 下出该应用订单号，或解开**他商户**未公开支付订单持卡人；或假签验签失败、真签过生产闸转到下游身份供应商业务错（证明生产仍认这把完整钥）；或生产 `open_order` 出未支付 `token_id`；或钉钉 `get_suite_token` 出 `suite_access_token`；或生产排队 `getZoneListCount` `ret=0` 出区服在线。

假点：文档是占位符；真假密钥同一句错；钥过期调不通；联调钥只能打测试环境、生产拒；wiki 打码就当 zip/Demo 也打码。密钥实值不进库。单站没中不删短表这行。

### 门户 CMS 写死站点钥（短表有指针）

认：门户/开放平台前端（不是调试文档 zip）写死 门户 CMS 的 `accessKey`+`secretKey`。有 `GET /api/v1/cms/login`，回 JWT，头名常见 `cms-token`。首页可能只是 CMS 欢迎页，钥在另一个门户 JS 里。

打（不登录）：

1. 抄 AK/SK 打 login（实值只进报告）
2. 带票打 `channel/list`、`content/list|detail|search`、`attachment/list`
3. 不要停在官网轮播频道。跟开发者/测试/后台频道，GET `attachments.url`

算成：内部测试报告/未对公开展示频道的稿件正文（xlsx 表字段也算）。

假点：只有公开运营 Banner/客户端下载地址。密钥实值不进库。单站没中不删短表这行。

### 发签 nonce 是私钥（短表有指针）

认：企业 IM/业务 **unlogin** 发签口（`queryAppSignature` / `queryCorpSignature` 一类）。其它业务口请登录，这条不要 Cookie。回包 `nonceStr`/`nonce` 以 `MIIE` 或 `-----BEGIN` 开头，能当 PKCS8 私钥 load。常见前面还有未登录渠道/活码口漏 `corpId`。

打（不登录）：

1. 未登录渠道/活码漏 `corpId` 就抄  
2. POST 发签口带 url + corpId  
3. 把 `nonceStr` 当私钥 load（Python `load_der_private_key` / openssl）。对照：假 corpId 应业务错、没有 PEM  

算成：能 load 成 RSA 私钥。

假点：nonce 只是短随机串；假 corpId 一直业务错。密钥实值不进库。单站没中不删短表这行。

### 未登录发 IM/体验票（短表有指针）

认：未登录 `im/getConfig` 下发 TIM `userSig` 且 `identifier=null`；或云产品体验中心/apaas 发签口按请求里的**已有 userId** 发票（对照游客 `none_auth` 会新开号）；或同一后台其它业务口请登录，发签口只要自定义 trace 头 + 客户端传入的 userId 就发 IM/RTC userSig；或自建 IM 网关发签口只吃 appkey，票里没 userId，接入口 body 的 uId 才是身份；或 JS 写的 v2 inner 发签被 IP 白名单；或官方音media/IM Web Demo 前端写死发签口和固定 pwd，identifier 跟请求走，空密也给已有用户发票。

打（不登录）：用该 sig 登 `IM 接入域`：`openim/login`、`getmsg`、`friend_get`、`get_joined_group_list`、群records/消息。体验页再打 login_token 看 `data.phone`。对照闸：其它业务口应请登录；发签口只加 trace 头、userId 换成 interviewer_/candidate_ 一类；拿 authorization/signature 打群历史/进房。匿名 appkey 发票后，access/get 把 uId 换成 interviewer_/admin，看回包 userID。v2 `by_app_name` 回 IP not allowed **别停**，改打 v3 同 path，只要 `appName`。官方 Demo：抄 JS 里的 UserSigService URL 和 pwd，identifier 填已有用户编号；pwd 空串再打一枪。拿 userSig 打 friend_get，对照游客空号。

算成：拉到**他人**会话/群成员/好友，或明文手机。

假点：只能登游客 `null`、空会话、只有自己建的空群；换别人 UserID=70013；`none_auth` 只新开空号；演示号没绑手机不算。单站没中不删短表这行。

### 写死产品号发共享 JWT（短表有指针）

认：AIGC/小工具 H5 或落地页把产品号 / `qbid` 写死在脚本里。发签口（`account/qb` 一类）不登录就给**共享产品号** JWT，票里 name 是产品名不是游客。JS 往往只有 create / GET by id，没有 list。

打（不登录）：

1. 抄页面写死的产品号打发签口  
2. 带着票 POST 同源 `.../task/list`（或 workflow/history 一类 JS 没写的 list）。对照：不带票应未登录；空包 list 仍出 total  
3. 跟 `userImages` / 原图 CDN，确认真下到人脸/证件照  

算成：list total 海量且带**他人**原图/证件照。

假点：票只能建空任务；list total=0 或只有自己刚传的。密钥实值、某次 JWT 不进库。单站没中不删短表这行。

### 官方客服链签发外站（短表有指针）

认：企业客服 H5 签发链；内联有 `getXcxLink`/`get_wx_open_link`。不是通用跳转页。

打（不登录）：POST `queryStr=caUrl=外站&urlType=2`。打开返回的 `客服落地链`。

算成：落地页正式名是官方客服号，`base64Decode` 出的 query 是你填的网址。

假点：只签发本站 `/ca/`；落地页不带 query；必须登录。单站没中不删短表这行。

### 演示号领云钥（短表有指针）

认：云厂商产品 Demo / 体验页 / 控制台代理 / 供应商后台；领 STS、联合身份、建任务只要业务 id 或空 body，不要 Cookie。领 STS 口缺参只报字段校验不是登录闸，不一定有演示号。不是对象存储 STS 通配覆盖（那枪见 `file-upload-test.md`）。

打（不登录）：有演示号抄客户编号再打领钥 / 建任务 / 联合身份。没有演示号也打领钥口：缺参报字段校验别当登录闸。空 body 报缺 `AppId`/`Uin` 也别当没口，带能解析的数字 Uin 就可能发联邦 STS；bucket+file_name 仍打。领钥口可能在独立 demo CDN 的 `/openapi/`（页面 JS 跟过去）。分享域前端 HMAC 过网关后，对照其它口「token 不合法 / Token校验失败」，`/share/token/info` 一类领 STS 口不带分享页 token 仍打。换 bucket 用 PUT/NoSuchBucket 探活桶。钥拿去 GetCallerIdentity 对主账号；有启动、检索一类 Action 再签（余量不够别停，看能不能问出账号或出账单）。Epaas 过 HMAC `SC-SIGN` 后对照名单仍 UserNotLogin，专找 GetUploadURL/FileName 不校验 token（过签≠出数）。

算成：临时钥匙能问出 AccountId/角色名；同一身份下列表出现刚建的任务；或检索出口径出账单/实例/他主体业务字段；或活桶 PUT 200。

假点：钥匙调业务 API 全 Unauthorized；领钥口请登录/401；create 200 但列表没有。没有演示号/客户编号不是假点。单站没中不删短表这行。密钥实值不进库。

### 未登录签发合作方登录链（短表有指针）

认：电子合同 / 供应商门户人打开是登录页，框里只填可枚举合作方数字 id（partnerId / supplierId 一类），没有密码。未登录 `getUrl`（或同类签发登录链）直接回 `partnerId`+`generate`+`code`。旁边有 `setCookies` / 换票口吃这三项下会话。

打（不登录）：

1. 登录页 JS 抄签发口，合作方 id 用页上提示的数字或邻号  
2. 把回包三项 POST 给 setCookies / 换票口，抄 Set-Cookie  
3. 带会话打合同列表 / 支付进件 / 身份口。对照：不带 Cookie 应未登录；不存在的合作方 id 应查空  

算成：身份口登录成功，或能当这个号读未公开合同。

假点：签发口要已登录；code 必须从邮件点开；setCookies 不下会话；只出公开招商页。单站没中不删短表这行。密钥实值、某次 Cookie 不进库。

### 未登录合作方直连配置查询（短表有指针）

认：运营台 / 供应链前端把供应商直连配置查询挂成未登录。body 只有可枚举合作方数字 id（partnerId 一类），不要 Cookie。回包 `partnerInfos`（或同类）里带 clientId+clientSecret。

打（不登录）：

1. 运营台 JS 抄查询口（getV2 / tech/partner 一类）
2. partnerId 用邻号。对照：未配置的 id 应回查空/未配置，不是登录闸
3. 算出是完整 clientSecret，不是空成功码

算成：回包里是完整 clientId+clientSecret，能当这把直连钥用。

假点：接口已下线；clientSecret 空或占位；必须登录；只出公司名没有钥。单站没中不删短表这行。密钥实值不进库。

### 未登录签发回跳外域（短表有指针）

认：未登录签发 SSO / 回跳；callback 只判断字符串里有没有官方 host，或不校验、任意外域也能签。

打（不登录）：callback/redirect 填外域：夹官方 host（query/子域/userinfo）和**不夹**都试。回包有票/签再交给自家 SSO。通行证页看会不会 `location.replace(回跳+"?"+token名+"="+session)`。对照：不含官方字应被改空或拦。必须真机登录才出票且前端不会把 token 拼进回跳 → 半条，停在签单，不算打穿。

算成：SSO 成功且回跳仍是外域；或登完通行证出现在外域 query。

假点：callback 被改空；SSO 缺回调参数。单站没中不删短表这行。

### 缺参字段改头换票（短表有指针）

认：未登录业务口 JSON 报「缺少 xxx」；query/body 带了仍报缺少。或 Spring 400 HTML/`message` 写 `Required String parameter 'os' is not present` 一类，点名的是 **Cookie 名** 不是 query。

打：把这个字段放到 **HTTP 头**再打 SSO/换票/login。头没吃别停，改打 Cookie（设备 `uuid`/`os`/`platform` 一类自己编）。未登录写口（绑店/改状态）同样试，不要只打换票。对照：只放 query 应仍报缺少。

算成：出别人的登录票/姓名；或写上他人门店/对象。

假点：query 有该字段仍报缺少（头和 Cookie 都没吃）不算过；只出自己的票。单站没中不删短表这行。

### SSO 壳后面的账密登录口（短表有指针）

认：管理后台人打开只跳 SSO / 统一认证，页面没有账密框；后端仍暴露账密登录 API。别把 SSO 壳当成整摊没登录口。常见后台骨架 `/admin-api/system/auth/login` 是常见皮，没有这名仍打。

打（不登录）：

1. POST `/admin-api/system/auth/login`，body `{"username":"admin","password":"123456"}`（不要去磨 SSO 验证码）
2. 把 `data.accessToken` 放到 `Authorization: Bearer` 打 `/admin-api/system/tenant/page` 或 `/system/user/page`
3. 对照：错密应失败；默认口出 `userId=1` 一类平台票

算成：进了平台管理员号，且租户/用户列表出他人手机/姓名。

假点：登录口 404 或默认口已改；票只能进空租户没有联系人。密钥实值、某次 Token 不进库。单站没中不删短表这行。

### 刷新票空包发管理员票（短表有指针）

认：JSON 网关有刷新票口（`/api/auth/refresh` 一类）。不登录、空包 `{}` 就发票，票里身份是 `admin`（不是游客）。管理后台和前台走同一套云开发/网关。

打（不登录）：POST 刷新口；把 `data.token` 放到 `Authorization: Bearer` 打 `/api/admin/me/permissions` 和申请/用户名单。对照：不带这张票应未登录。

算成：me 是超级管理员（权限 `*`），或名单出现**他人**手机/姓名。

假点：只出游客/过期票；refresh 必须带旧 refresh token；me 仍是自己。密钥实值不进库。单站没中不删短表这行。

### 空 openId 进已有号（短表有指针）

认：小程序 / H5 的 GET 登录口吃 `openId`（`loginByOpenId` 一类）。缺参报缺字段，假 openId 登录失败，空串仍 200 出**已有商家号**和手机，不是游客空号。

打（不登录）：

1. 缺参一枪，确认这是登录口  
2. 假值一枪，应失败  
3. `openId=` 空串再打；GET query 没吃再试 POST JSON `{"openId":""}`  
4. 对照：空串出的店名/登录手机，和假值失败、缺参报缺，不是同一套游客号  

算成：进已有商家号且出手机（能当这个号用）。

假点：空串只出游客空号。密钥实值、某次 Cookie 不进库。单站没中不删短表这行。

