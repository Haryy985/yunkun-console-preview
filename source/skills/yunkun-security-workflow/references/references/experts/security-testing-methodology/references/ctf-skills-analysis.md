# CTF-Skills (ljagiello) 外部技能集合参考

> 来源: github.com/ljagiello/ctf-skills (Agent Skills 格式)
> 分析时间: 2026-07-07
> 技能总数: 9 大类, ~90+ .md 文件, 500+ 技术点

## 覆盖领域

| 分类 | 文件数 | 技术点 | 价值评估 |
|------|:------:|:------:|:--------:|
| ctf-pwn | 15 | 100+ 二进制利用技术 | ★★★★★ — 最全面的 PWN 技能库之一 |
| ctf-web | 16 | 150+ Web 攻击技术 | ★★★★★ — 独特的客户端攻击前沿技术 |
| ctf-crypto | 13 | 30+ RSA/ECC/Lattice/PRNG 攻击 | ★★★★ — 补充 Lattice/LWE execution-framework |
| ctf-reverse | 14 | 200+ 逆向工程模式 | ★★★★ — 全平台覆盖 (Go/Rust/Flutter/WASM) |
| ctf-forensics | 13 | 磁盘/内存/网络/隐写/硬件信号 | ★★★★ |
| ctf-ai-ml | 3 | Model attack, adversarial, LoRA | ★★★ |
| ctf-osint | 3 | 社交媒体/地理定位/Web DNS | ★★★ |
| ctf-misc | 10 | Pyjail/Bashjail/编码/游戏VM | ★★★ |
| ctf-malware | 3 | 样本分析 | ★★★ |

## 高价值独特技术（我们缺失的）

### CTF-Pwn

| 技术 | 说明 | 优先级 |
|------|------|:------:|
| **House of Apple 2** | glibc 2.34+ FSOP via _IO_wfile_jumps | 🔴 极高 |
| **leakless libc via multi-fgets** | 零泄露下构造伪造stdout泄露libc | 🔴 极高 |
| **TLS destructor hijack** | glibc 2.34后hook移除的新攻击面 | 🔴 高 |
| **musl libc heap** | musl的meta指针+atexit劫持 | 🟡 中 |
| **ret2vdso** | 静态链接二进制用内核vDSO gadgets | 🟡 中 |
| **io_uring UAF SQE injection** | 前沿Linux异步I/O利用 | 🟡 中 |
| **SROP with UTF-8 constraints** | Rust payload全UTF-8合法 | 🟢 低 |

### CTF-Web

| 技术 | 说明 | 优先级 |
|------|------|:------:|
| **CSS font glyph + container query exfiltration** | 纯CSS从inline script窃取数据 | 🔴 极高 |
| **React Server Components Flight RCE** | Next.js Flight协议反序列化 | 🔴 高 |
| **Unicode case folding XSS** | U+017F ſ绕过ASCII regex | 🔴 高 |
| **Hyperscript/Alpine.js CDN CSP bypass** | 声明式JS框架绕过CSP | 🔴 高 |
| **Go rune/byte mismatch** | 表情符导致rune检查与byte操作不一致 | 🟡 中 |
| **flatnest circular reference (CVE-2023-26135)** | 原型链污染新路径 | 🟡 中 |
| **JWE token forgery** | JWE加密≠认证，公钥即可伪造 | 🟡 中 |
| **Castor XML xsi:type deserialization** | 无gadget chain的XML反序列化RCE | 🟡 中 |

### CTF-Crypto

| 技术 | 说明 | 优先级 |
|------|------|:------:|
| Lattice/LWE 完整execution-framework (LLL/BKZ/Babai) | 补充我们的格攻击空白 | 🔴 高 |
| ECC 攻击深度 (invalid curve/Smart's attack) | 超出我们现有覆盖 | 🟡 中 |
| ZKP/Groth16/Schnorr 攻击 | 零知识证明利用 | 🟡 中 |
| LFSR (Berlekamp-Massey) 流密码攻击 | 新领域 | 🟡 中 |

## 与现有技能重叠 (不需重复)

- 基础 XSS/SQLi/SSTI/SSRF — 我们已有更好的框架型技能
- 基础 ret2libc/ROP/格式串 — 我们已有 `pwn-exploitation` 等
- 基础 RSA 攻击 (small e/Wiener/Fermat) — 已有 `ctf-crypto-techniques`
- glibc < 2.34 hooks (__free_hook) — 已过时

## 整合建议

建议在以下场景加载此技能集作为补充参考：
- CTF 竞赛中遇到新奇的 Web 客户端攻击（CSS exfiltration, XS-Leak 等）
- glibc 2.34+ 的现代堆利用（House of Apple 2 等）
- Lattice/LWE 密码学攻击
- 全平台逆向 (Go/Rust/Flutter) 的execution-framework参考
