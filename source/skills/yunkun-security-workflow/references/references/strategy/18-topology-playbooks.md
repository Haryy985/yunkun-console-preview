# Topology Playbooks

## 卡片格式

```yaml
Topology:
Signals:
Common failure:
Hunter hints:
Minimal probes:
Stop:
Not findings:
```

## 第一批

### REST API
- Signals: `/api/`、JSON、对象 `id`
- Common failure: BOLA、字段过曝、状态机遗漏
- Minimal probes: `own / other / nonexistent`
- Not findings: 单个 `200`、公开只读接口

### OAuth / OIDC
- Signals: `authorize`、`callback`、`state`、`redirect_uri`
- Common failure: redirect trust、state 绑定、token audience
- Minimal probes: callback 差分、state 绑定验证
- Not findings: 登录页存在、公开 metadata

### 对象存储
- Signals: `bucket/oss/cos/s3/obs/sts/presign/callback`
- Common failure: 非公开对象读取、列举、预签名范围过宽、STS 过宽
- Minimal probes: 小样本对象/前缀边界差分
- Not findings: 公开图片、CDN 命中、桶名暴露

### 文件流
- Signals: upload / preview / convert / download
- Common failure: 权限继承错误、预览越权、转换链泄露
- Minimal probes: 上传对象的访问控制与预览/下载差分
- Not findings: 上传成功本身

### 微服务 / MQ
- Signals: webhook / queue / topic / worker / async
- Common failure: 信任来源错误、重放、消费侧越权
- Minimal probes: `actor -> producer -> queue -> worker -> effect`
- Not findings: topic 名称、worker banner

### RAG / Agent
- Signals: prompt / retrieval / memory / tool / function call
- Common failure: 跨租户检索、tool 越权、memory 污染
- Minimal probes: retrieval、tool、memory 与 tenant/session 绑定验证
- Not findings: prompt injection 成功但未越权、系统风格文本、公开内容复述
