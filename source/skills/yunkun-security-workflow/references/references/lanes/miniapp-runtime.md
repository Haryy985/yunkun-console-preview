# 微信小程序 / Burp Proxy Runtime lane

适用信号：

- `miniapp` / `miniprogram`
- `wechat`
- `proxifier`
- `burp`
- `burpsuite`
- `burp mcp`
- `wx.request` / `wx.login`
- `AppService` / `webview`
- 抓包历史、跳转链、接口清单里持续出现新的 exact origin

核心目标：

1. 先拿到网络层真实请求链，不把小程序误判成普通 H5 或普通 SPA
2. 先拆清楚 `AppService -> webview -> backend API -> auth material`
3. 让抓包发现的新 exact origin 自动并入当前 run，然后只刷新一次该 origin 的内部 gate

推荐顺序：

1. 确认 `Proxifier -> Burp Suite -> Burp MCP` 链可用
2. 用 Burp 历史确认目标 host、路径、认证头与对象参数已真实出现
3. 从抓包历史构建 API inventory、页面/功能映射与对象矩阵
4. 先做 baseline，再做单变量 variant 与 negative control
5. 对 upload / download / preview / export / approval / callback / pay / login 形成业务链
6. 对命中的高价值对象接口执行 Hunter -> Validator 闭环
7. 只在网络面无法回答关键问题时，才补充前端运行时/源码侧证据
8. 输出证据包与回放步骤

优先观察：

- 页面路由 / tab / scene / webview 与 appservice 分层
- `wx.login`、code2Session、token refresh、静默登录
- `wx.getStorageSync` / 本地缓存 / openid / unionid / session / role
- Burp 历史里的 `cookie`、`authorization`、`x-token`、`sign`、`timestamp`、`nonce`
- `userId` / `uid` / `tenantId` / `orgId` / `fileId` / `orderId` / `recordId` / `applyId`
- 上传、下载、预览、素材、文件对象边界
- 小程序壳页面与真实业务页面的分离
- 请求、跳转、响应、JS、接口里暴露的新 exact origin

优先分支：

- `proxy-chain-and-history`
- `webview-backend-chain`
- `auth-material-trace`
- `scene-shell-vs-business-page`
- `object-matrix-and-bola`
- `file-chain-and-object-storage`

默认不要：

- 不先让用户手工做 `approval`、`scope-expand`、`cycle`
- 不先把小程序全量当普通前端站点扫
- 不先堆 JS 模板命中或泛化 fuzz
- 不先把“Burp 已启动”当成“流量已接入”
- 不先在没有真实抓包请求链的前提下下重工具结论
- 不把 WMPF / CDP / hook / offset / frida attach 作为默认执行面；它们只算备用补充，不再作为开测主路径
