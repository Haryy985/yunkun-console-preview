# Attack Path Search

## 目标
Hunter 不只“列漏洞候选”，而是主动搜索最短突破链。

## 流程

```text
signal
  -> attacker_goal
  -> candidate_paths
  -> weakest_edge
  -> minimal_probe
```

## 例子

### /api/file/upload
- attacker_goal: 获得未授权文件访问
- candidate_path:
  - user
  - upload
  - storage
  - access_control
  - download
  - disclosure
- weakest_edge: access_control
- minimal_probe: 上传后对下载/预览对象边界做小样本差分

## 规则

- 优先搜索“最短可验证突破链”
- 先找 weakest edge，再选 probe
- 路径搜索结果服务于排序与验证，不直接形成 finding
