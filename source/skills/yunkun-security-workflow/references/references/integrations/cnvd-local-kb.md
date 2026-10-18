# CNVD 本地knowledge-base

## 来源

- `D:\黄鹤题目`
- `D:\黄鹤题目\cnnvd`
- `C:\Users\32237\Documents\Codex\2026-07-30\n-h\work\bundle_459\CNVD-SharedVulData-XML`

## 产物

- `C:\Users\32237\Documents\Codex\2026-07-16\d\artifacts\cnvd-local-kb\cnvd-search.jsonl`
- `C:\Users\32237\Documents\Codex\2026-07-16\d\artifacts\cnvd-local-kb\cnvd-index.jsonl`
- `C:\Users\32237\Documents\Codex\2026-07-16\d\artifacts\cnvd-local-kb\cnvd-summary.md`
- `C:\Users\32237\Documents\Codex\2026-07-16\d\artifacts\cnvd-local-kb\lookup_cnvd.py`

## 什么时候读

- 目标产品 / 版本 / CVE / CNVD 号已知，想找已知漏洞、补丁、参考链接或相似案例时。
- 目标指纹像已公开产品，但现场证据还不够时，用它做候选面归并。
- 需要把“版本命中 / 产品命中 / 补丁链接”压成 lead 时。

## 怎么用

```bash
python C:\Users\32237\Documents\Codex\2026-07-16\d\artifacts\cnvd-local-kb\lookup_cnvd.py "WebSphere Application Server"
python C:\Users\32237\Documents\Codex\2026-07-16\d\artifacts\cnvd-local-kb\lookup_cnvd.py "CVE-2026-63301"
python C:\Users\32237\Documents\Codex\2026-07-16\d\artifacts\cnvd-local-kb\lookup_cnvd.py "Jumpserver"
```

## 使用纪律

- 只把它当候选knowledge-base，不当现场证据。
- 漏洞标题、版本范围、补丁链接、参考链接都默认只是 `observation/lead`。
- 只有结合运行时请求、响应、对象边界、状态变化或可重放影响，才允许升层。
- 不把“产品名像、版本像、补丁像”直接写成 finding。

