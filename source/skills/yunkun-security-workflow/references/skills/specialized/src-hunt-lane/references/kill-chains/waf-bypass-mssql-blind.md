# WAF 下 MSSQL 盲注提取框架（黑名单绕过技术栈）

> 源：zcode memory/sqli-waf-bypass-mssql-blind.md（tjise WebVPN 网关 WAF 实战，2026-08-29 全量提取 @@SERVERNAME 成功）。
> 适用：ASP.NET+ashx+WebVPN/云 WAF 目标。先探测黑名单边界，再套此框架。

## 认什么

- WAF 按"函数名 token"精确黑名单匹配（AND/OR/SELECT/CONVERT/@@version/db_name/LEN/SUBSTRING/CASE/IIF/SYSTEM/WAITFOR/sleep/information_schema）
- 但冷门函数几乎总放行：**STR / CHARINDEX / RIGHT / LEFT / ABS / CURRENT_USER / @@SERVERNAME / @@DBTS / 引号逃逸('+') / % _ 通配符 / PUBLIC 实体**
- 先发探测 payload 确认放行边界，别在黑名单词上死磕

## 提取框架（全部非黑名单，五步）

1. **逃逸拼接**：`kw=%25%27%2B<expr>%2B%27` → LIKE '%'+expr+'%'（注意：裸 `+` 传输被 IIS 解码为空格破坏拼接，**必须编码 %2B**）
2. **定位字符**：`RIGHT(LEFT(x,N),1)` 替代 SUBSTRING（N 超长返回末字符，无空串陷阱）
3. **集合判断**：`CHARINDEX(ch,'charset')` → 0 或位置
4. **读数**：`STR(1/CHARINDEX(...))` → 命中=200，未命中=除零=302 aspxerrorpath（报错差异布尔）
5. **二分**：charset 递归拆半，每字符 log2(|charset|) 发

**DB_ID() 非黑名单可用**——提取库 ID 可证多租户批发建库（如 ID=188 → 180+ 个库）。
**上下文函数**（CURRENT_USER/@@SERVERNAME/@@DBTS）在 SELECT 被拦时是唯一信息源；@@SERVERNAME 完整值注意 maxlen 截断（10 位值可能是截断，预留 15+）。

## ORDER BY 注入通道（参数直拼场景）

- 命中常量('w' in 'w') → aspxerrorpath 错误页；未命中(1/0=NULL, ARITHABORT OFF) → 正常 JSON
- **读数可能对调**（ERR=命中 / OK=NULL）：先做已知明文对照校准
- 陷阱：上下文函数在 ORDER BY 上下文可能全 NULL（不集成于提取框架）；ERR 页有假阳性（payload 构造 bug 的 SQL 异常也是 ERR）——**同时发"已知命中"+"已知未命中"+"已知明文期望值"三对照**

## WAF 行为与封锁画像（网关层）

| 现象 | 定性 | 处置 |
|------|------|------|
| 000 波动 | 多为波动非拦截 | 重试 1-2 次+同批对照再定性 |
| 000 持续 + 历史 baseline 也 000 | IP 级累积冷却（3-5 分钟起步，当日 100+ 请求后 15-30 分钟级） | 冷却期禁硬打（换直连 IP 子域也被拦=全局封锁），纯 sleep 等待 |
| 老好 URL 200 / 新 URL 000（TCP 立即断开非超时） | URL Reputation 模式（新 URL/新参数组合全断） | 需 24h 级完全冷却或换 IP 重新"养" URL 热度 |
| api_xxx 全方法 TCP 断开（time_total=0），其他 api 正常 | 应用池 RFP 停止（异常风暴触发 IIS Rapid-Fail Protection） | 立即停止该端点全部测试，报备服务异常，验证换端点 |
| h2/chunked 在 ALPN 层断连（curl-cffi chrome 断、curl.exe h1 通） | 协议层 h1-only 门控 | 降级 HTTP/1.1 |
| libcurl 指纹全局标记 | 指纹级封锁 | curl_cffi impersonate=chrome + CurlHttpVersion.V1_1 |

## 教训（实证）

- 黑名单按 token 精确匹配：db_name 用空格/注释断开无效；冷门函数也可能在列（original_db_name 被拉黑）
- ABS(1) 302 不是拦截而是 int+varchar 转换异常——**302 也能当执行证明**
- 引号类拦截有临时黑名单（冷却约 3-5 分钟恢复）
- db_name 三变体（标准/注释拼接/空格）全被 WAF 子串级拉黑 → 库名字面值在此类 WAF 下可能不可得，换 DB_ID()/SERVERNAME 类上下文值
