# Lane · File Flow

## 适用信号

- upload / import / attachment / preview / download / export
- 文件列表、附件详情、预览页、导出任务、归档包
- 明显的 `fileId` / `attachmentId` / `taskId`

## 先回答

1. 这是上传、预览、下载、导出、导入，还是多段拼成的链
2. 文件对象边界按 owner、tenant、project、folder 还是 task 切
3. 文件在存储前、解析时、回显时分别经过了哪些系统
4. 当前问题更像对象越权、校验绕过、解析链还是任务归属错误

## 优先顺序

1. `scripts/session_observer.py`
2. `scripts/unknown_surface_router.py`
3. `scripts/branch_manager.py`
4. `scripts/hypothesis_ledger.py`
5. `scripts/minimal_verify_gate.py`
6. 单步 replay 文件链中的一个关键环节

## 高价值分支

- `file-object-boundary`
- `upload-parse-chain`
- `export-scope-diff`
- `export-job-ownership`

## 默认不做

- 不先把“上传成功”写成漏洞
- 不先把“文件可下载”写成越权
- 不先批量导出/下载
- 不先忽略预览、缩略图、打包、归档这些二阶段动作
