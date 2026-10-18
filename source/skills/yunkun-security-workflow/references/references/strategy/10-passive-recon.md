# 被动信息收集工作流

> 视角：渗透测试，不向目标发任何包，仅利用第三方数据源
> 加载时机：Phase 2a 开始时

---

## 一、子域名枚举

### 1.1 crt.sh（证书透明度）

```bash
# 基础查询
curl -s "https://crt.sh/?q=%25.target.com&output=json" \
  | jq -r '.[].name_value' | sed 's/\*\.//g' | sort -u | tee crtsh.txt

# 直连 PostgreSQL（大批量）
psql -h crt.sh -p 5432 -U guest certwatch \
  -c "SELECT DISTINCT lower(name_value) FROM certificate_and_identities 
      WHERE plainto_tsquery('certwatch', 'target.com') @@ identities(certificate) LIMIT 5000;"
```

**注意**：仅返回有 SSL/TLS 证书的子域名，HTTP-only 不在内。

### 1.2 subfinder（ProjectDiscovery）

```bash
# 基础被动枚举
subfinder -d target.com -o subfinder.txt

# 全源 + 递归（推荐）
subfinder -d target.com -all -recursive -rl 5 -t 30 -o subfinder_full.txt

# 管道验活
subfinder -d target.com -silent | httpx -silent
```

**API 密钥配置**（`~/.config/subfinder/provider-config.yaml`）：
```yaml
binaryedge:
  - <key>
shodan:
  - <key>
censys:
  - <api_id>:<api_secret>
```

### 1.3 Amass（OWASP）

```bash
# 纯被动模式
amass enum -d target.com -passive -timeout 60 -o amass.txt
```

### 1.4 合并去重 Pipeline

```bash
cat subfinder.txt amass.txt crtsh.txt | sort -u > all_subdomains.txt
cat all_subdomains.txt | httpx -silent -status-code -title -o live_subdomains.txt
```

---

## 二、DNS 信息收集

### 2.1 全记录类型枚举

```bash
TARGET="target.com"
dig +short A $TARGET
dig +short AAAA $TARGET
dig +short MX $TARGET        # 邮件服务商
dig +short NS $TARGET        # Zone transfer 目标
dig +short TXT $TARGET       # SPF/DMARC/验证码
dig SOA $TARGET              # 区域管理员
dig CAA $TARGET              # 允许的 CA
dig SRV _sip._tcp.$TARGET   # 可能暴露内部服务
dig SRV _autodiscover._tcp.$TARGET
```

### 2.2 SPF / DMARC / DKIM 分析

```bash
dig +short TXT $TARGET | grep spf
# "v=spf1 ip4:X.X.X.X/24 ..." → 自有 IP 段

dig +short TXT _dmarc.$TARGET
# p=none → 可能邮件伪造风险

for selector in default mail google s1 s2 email k1; do
  result=$(dig +short TXT "${selector}._domainkey.$TARGET")
  [ -n "$result" ] && echo "[$selector] $result"
done
```

**情报价值**：

| 记录 | 情报价值 |
|------|---------|
| `v=spf1 ip4:X.X.X.X/24` | 揭示自有 IP 段 |
| `include:mailgun.net` | 使用 Mailgun 发信 |
| `p=none` DMARC | 可构造伪造邮件 |
| `include:_spf.salesforce.com` | 使用 Salesforce |

### 2.3 Zone Transfer 尝试

```bash
for ns in $(dig +short NS $TARGET); do
  echo "[*] Trying zone transfer from: $ns"
  dig axfr $TARGET @$ns
done
```

成功 = 重大发现，直接暴露完整 DNS 树。

### 2.4 DNS 历史

```bash
# SecurityTrails API
curl -s "https://api.securitytrails.com/v1/history/$TARGET/dns/a" \
  -H "APIKEY: YOUR_KEY" | jq '.records[].values[].ip'
```

---

## 三、搜索引擎 OSINT

### 3.1 Google Dorks

```
# 资产发现
site:target.com -www
site:target.com filetype:pdf

# 登录/管理
site:target.com inurl:login OR inurl:admin

# 敏感文件
site:target.com ext:env OR ext:config OR ext:log OR ext:sql OR ext:bak

# API 文档
site:target.com inurl:swagger OR inurl:/api/
site:target.com "api_key" OR "api-key" OR "apikey"

# 技术栈泄露
site:target.com "Exception in thread" OR "SQL syntax" OR "ORA-"
```

### 3.2 GitHub Dorks

```
org:target-company
"target.com" password OR secret OR api_key OR token
"target.com" filename:.env OR filename:config.yml OR filename:credentials
"internal.target.com" OR "staging.target.com"
```

### 3.3 Shodan 搜索

```bash
shodan search 'org:"Target Company"'
shodan search 'hostname:target.com'
shodan search 'ssl.cert.subject.cn:target.com'
shodan search 'org:"Target" http.title:"Kibana" OR http.title:"Jenkins" OR http.title:"Grafana"'
shodan search 'http.favicon.hash:<hash>'      # Favicon 指纹
shodan search 'org:"Target" has_vuln:true'     # 有已知 CVE 的资产
```

### 3.4 FOFA 搜索

```
domain="target.com"
cert="target.com"
app="Shiro" && domain="target.com"
domain="target.com" && status_code="200"
```

### 3.5 Censys 搜索

```bash
censys search 'services.tls.certificates.leaf_data.subject.common_name: target.com'
censys search 'autonomous_system.description: "TARGET COMPANY"'
```

---

## 四、历史 URL 数据

### 4.1 gau（getallurls）

```bash
gau --subs target.com | tee gau_all.txt

# 实战 Pipeline
gau --subs target.com | grep -E "\.php|\.asp|\.aspx|\.jsp" | tee dynamic_pages.txt
gau --subs target.com | grep -E "api|token|key|secret|password|login|admin" | tee juicy_urls.txt
gau --subs target.com | grep "?" | cut -d '?' -f 2 | tr '&' '\n' | cut -d '=' -f 1 | sort -u | tee params.txt
```

### 4.2 Wayback Machine

```bash
curl -s "https://web.archive.org/cdx/search/cdx?url=*.target.com&output=json&fl=original,timestamp,statuscode&collapse=urlkey" \
  | jq -r '.[] | @csv' | tee wayback.txt

# 查找 JS 文件（可能含 API 密钥）
cat wayback.txt | grep "\.js$" | sort -u | tee js_files.txt
```

---

## 五、ASN / IP 归属

```bash
# IP 查 ASN
curl -s "https://ipinfo.io/$(dig +short A target.com | head -1)/json" | jq '{ip, org, asn}'
curl -s "https://api.hackertarget.com/aslookup/?q=target.com"

# ASN 查 IP 段
curl -s "https://api.bgpview.io/asn/AS12345/prefixes" \
  | jq -r '.data.ipv4_prefixes[].prefix' | tee asn_prefixes.txt

# C 段 PTR 反查
for i in {1..254}; do
  result=$(dig +short -x "1.2.3.$i" 2>/dev/null)
  [ -n "$result" ] && echo "1.2.3.$i -> $result"
done
```

---

## 六、泄露数据库查询

```bash
# Have I Been Pwned（域名查询，需付费）
curl -s "https://haveibeenpwned.com/api/v3/breacheddomain/target.com" \
  -H "hibp-api-key: YOUR_KEY" | jq

# Pastebin 搜索（Google Dork）
site:pastebin.com "target.com" password
site:pastebin.com "@target.com"

# GitHub 泄露密钥
"target.com" "BEGIN RSA PRIVATE KEY"
"target.com" "AKIA"
```

---

## 七、社交媒体 / 招聘信息推断技术栈

```
# LinkedIn Google Dork
site:linkedin.com/jobs "target.com" "Java" OR "Spring Boot" OR "Kubernetes"

# 从招聘信息推断
# 职位: "Senior Backend Engineer"
# 要求: Java 11+, Spring Boot, Kubernetes, AWS EKS
# → 推断: AWS 部署, K8s 集群, 微服务架构
# → 攻击面: K8s API Server, AWS 元数据, Spring Boot Actuator
```

---

## 八、工具降级对照表

| 功能 | 有 MCP 工具 | 无工具降级 |
|------|-----------|----------|
| 子域名枚举 | `subfinder-mcp` / `kali-mcp` | `curl crt.sh API` |
| DNS 查询 | `kali-mcp (dig)` | 建议用户执行 `dig` 命令 |
| Shodan 搜索 | `shodan-mcp` | `curl Shodan API` |
| Wayback URLs | `fetch_url(web.archive.org)` | `gau target.com` |
| 验活 | `kali-mcp (httpx)` | 建议用户执行 `httpx` |
