# Lane · 数据平台 / 分析平台

## 适用信号

- Jupyter Notebook/Lab、Superset、Airflow、Elasticsearch、Kibana、Grafana-like dashboard、`/notebooks`、`/lab`、`/superset`、`/airflow`、`/_cat`、`/_search`、`/kibana`、dataset、index、DAG、connection、export、report、可视化面板。

## 先建模什么

- `workspace`: notebook workspace、dashboard workspace、Airflow DAG scope、ES index/pattern。
- `identity`: 匿名、业务用户、分析用户、管理员、service token。
- `data object`: dataset、index、connection、query、notebook、DAG、job、export file。
- `sensitivity`: 是否属于公开展示、测试数据、业务敏感数据、跨租户数据或凭证配置。
- `side effect`: 查询、导出、任务执行、连接测试、调度、写入/删除。

## 最小验证链

1. 证明平台入口与当前业务存在真实链接或授权范围。
2. baseline：当前 actor 可见的一个公开/自有 dashboard、index、DAG 或 notebook。
3. 单变量：workspace、dataset/index、DAG id、owner、export format、query filter。
4. negative control：不存在对象、无认证、相邻 workspace、错误角色。
5. 只有跨 workspace/tenant 读取、未授权导出、连接配置泄露、任务越权执行或状态改变成功，才升级。

## 高价值分支

- Jupyter：token/会话边界、workspace 文件读写、kernel 权限与业务数据路径；登录页或空 workspace 不是漏洞。
- Superset：dashboard/dataset/chart/RLS/SQL Lab 权限边界；公开 dashboard 不是漏洞。
- Airflow：DAG 查看、触发、变量/connection、日志与任务参数边界；只读首页不是漏洞。
- Elasticsearch/Kibana：index pattern、query、snapshot、saved object、租户空间边界；`cluster health` 或版本号只是 observation。

## 证据与停止

- 所有数据样本只取最小必要字段并脱敏。
- 没有证明数据敏感性或越界 actor 时，只能停在 `lead/conditional-impact`。
- 连续两次只得到公开面板、空数据或认证墙，冻结该平台分支。

