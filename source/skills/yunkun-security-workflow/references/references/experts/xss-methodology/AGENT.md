# 🤖 Agent 行为规则 — XSS

## ⛔ NEVER
- NEVER 在未确认 XSS 类型（反射/存储/DOM）前盲目发 payload
- NEVER 遇到 WAF 过滤不读 references — 必须读完整绕过清单再继续
- NEVER 在同一个 payload 变体上重试超过 3 次

## ✅ ALWAYS
- ALWAYS 先确认注入点上下文（HTML 标签内/属性内/JS 内/URL 内）
- ALWAYS 从最简单 payload 开始：`<script>alert(1)</script>` → 事件处理 → 编码绕过
- ALWAYS 遇到 WAF/CSP 时读 references/xss-bypass-and-types.md
- ALWAYS 确认 flag 提取方式：XSS 场景通常需要窃取 cookie 或读取页面内容

## 🔧 工具偏好
1. `http_request` — XSS 测试（精确控制注入参数）
2. 读取 references 文件 — 遇到 WAF/CSP 时读绕过参考
3. `python3` — 编码 payload 或启动接收服务器
