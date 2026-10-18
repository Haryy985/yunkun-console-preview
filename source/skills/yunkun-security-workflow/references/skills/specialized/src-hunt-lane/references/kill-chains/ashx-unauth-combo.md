# ASP.NET ashx 未授权组合面（DTcms 模板 + 摆设参数差分法）

> 源：zcode memory/tjise-webvpn-unauth-ashx.md（tjise run2 深挖，2026-08-29）。
> 与 waf-bypass-mssql-blind.md 同目标配套（WebVPN 网关 WAF 画像在该文件）。

## 认什么

- `*.ashx?act=` 型数字 RPC 网关（api_pdms/api_mms 等多 api 前缀）
- DTcms 模板指纹：`upload_ajax.ashx?action=UpFileAction`、`uplaod_logo` **拼写错误**（模板级指纹，可搜索同模板站）
- 旧后台 `/admin/main.aspx /welcome /menu /left` 异常重定向证明存在（登录墙后）

## 打哪 + 出什么算成

**F4 注入前置——摆设参数差分（先负对照验证参数生效性）：**
- `get_will_item_list&kw=` 直拼 LIKE：`%/_` 通配符差分（0/73 条）、单引号 302 aspxerrorpath → 参数生效且直拼
- **get_direction_list 的 kw 是摆设参数**（全量返回，注入无效）——同网关不同参数行为不同，**先做通配符差分再上引号**，别把"无回显"当"参数化安全"
- `get_manager_by_id&id=` 数字参数算术差分（11999+1）：有强转无注入 → 等值/强转/参数化三种形态用不同探针区分

**F5 未授权上传：**
- `upload_ajax.ashx?action=UpFileAction` → txt 落地 `/upload/年月日/` 公网可读
- RCE 通道被 WAF multipart 文件名白名单全拦（ashx/asmx/html/大小写/尾空格/顺序/boundary 双写）
- zip 深度检测：SYSTEM 实体拦、PUBLIC 放行 → XXE 探测用 PUBLIC 变体过 WAF

**F6 账号枚举：**
- `get_manager_by_username&user_name=` 精确查账号（等值参数化无注入，但可当用户名枚举口）
- ManagerFile 未授权目录列举（d 参数硬编码 /upload/）

## execution-framework提炼（跨站通用）

1. **摆设参数差分法**：同端点参数先验证"生效性"（改值看响应变不变），再谈注入——"参数不生效"与"参数化安全"是两回事
2. **WAF 拦截 vs 服务端解析区分**：构造"语法相关但语义无害"的对照 payload（内部实体 vs 外部实体）
3. **000 ≠ 拦截**：先重试 1-2 次再定性；同批对照必须
4. **数字参数算术差分**（id=11999+1）是免关键词的强转验证
5. **浏览器与脚本通道行为可能不同**（会话态触发前置 SSO 重定向）：复测用脚本通道（curl_cffi 无凭据），别用浏览器结论否定脚本结论
6. **抽样统计先统一口径**：只匹配目标字段；不同分页填充率分布不均（36%/12%/15%/14%），混口径会高估（8 千 → 修正 5800）

## 修复建议（报告要点）

ashx 网关统一鉴权 + kw 参数化 + 上传扩展名服务端白名单 + 旧后台下线 + 目录列举封禁。
