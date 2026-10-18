# insecure-scm

# Insecure Source Code Management

## 0. QUICK START

High-value paths to probe first (GET or HEAD, respect rate limits):

```http
/.git/HEAD
/.git/config
/.svn/entries
/.svn/wc.db
/.hg/requires
/.bzr/README
/.DS_Store
/.env
```

**Routing note**: quickly probe these paths first; for full recon workflow, load methodology from `recon-methodology.md` before deeper testing.

---

## 1. GIT EXPOSURE

### Detection

- **`/.git/HEAD`** — valid repo often returns plain text like:

```text
ref: refs/heads/main
```

- **`/.git/config`** — may expose `remote.origin.url`, user identity, or embedded credentials.
- **`/.git/index`**, **`/.git/objects/`** — partial object store access enables reconstruction with the right tools.

### 403 vs 404

- **`404`** — path likely absent or fully blocked at the edge.
- **`403` on `/.git/`** — directory may **exist** but listing is denied; still try direct file URLs:

```http
/.git/HEAD
/.git/config
/.git/logs/HEAD
/.git/refs/heads/main
```

A **403 on the directory** plus **200 on `HEAD`** strongly indicates exposure.

### Recovery tools (open source)

- **`arthaud/git-dumper`** — dumps reachable `.git` tree when individual files are fetchable.
- **`internetwache/GitTools`** — Dumper, Extractor, Finder modules for partial/corrupt dumps.
- **`WangYihang/GitHacker`** — alternative recovery when standard dumpers miss edge cases.

### Key files to prioritize

| Path | Why it matters |
|------|----------------|
| `.git/config` | Remotes, credentials, hooks paths |
| `.git/logs/HEAD` | Commit history, reflog-style leakage |
| `.git/refs/heads/*` | Branch tips, commit SHAs |
| `.git/packed-refs` | Packed branch/tag refs |
| `.git/objects/**` | Object blobs for reconstruction |

---

## 2. SVN EXPOSURE

### Detection

- **SVN before 1.7**: **`/.svn/entries`** — XML or text metadata listing paths and revisions.
- **SVN ≥ 1.7**: **`/.svn/wc.db`** — SQLite working copy database (`PRAGMA table_info` after download).

Example probe:

```http
GET /.svn/entries HTTP/1.1
GET /.svn/wc.db HTTP/1.1
```

### Recovery

- **`anantshri/svn-extractor`** — automated extraction from exposed `.svn`.
- **Manual**: download `wc.db`, query with `sqlite3` for file paths and checksums, then request **`/.svn/pristine/`** blobs if exposed.

---

## 3. MERCURIAL EXPOSURE

### Detection

- **`/.hg/requires`** — small text file listing repository features; confirms Mercurial metadata.

```http
GET /.hg/requires HTTP/1.1
GET /.hg/store/ HTTP/1.1
```

### Recovery

- **`sahildhar/mercurial_source_code_dumper`** — dumps repository when store paths are reachable.

---

## 4. OTHER LEAKS

### Bazaar (Bzr)

- Probe **`/.bzr/README`** and **`/.bzr/branch-format`** for Bazaar metadata.

### macOS `.DS_Store`

- **`/.DS_Store`** can encode directory and filename listings.
- Tools: **`gehaxelt/ds-store`**, **`lijiejie/ds_store_exp`** — parse `.DS_Store` offline.

### Backup and config artifacts

Probe (adjust for app root and naming conventions):

```text
/.env
/backup.zip
/backup.tar.gz
/wwwroot.rar
/backup.sql
/config.php.bak
/.config.php.swp
```

### Web server misconfiguration signal (example: NGINX)

- **`location /.git { deny all; }`** — may return **403** for `/.git/` while still allowing or denying specific subpaths depending on rules.
- **403 on a protected location** can **confirm the route exists**; always distinguish from **404** on non-existent paths.

---

## 5. DECISION TREE

1. **Probe `/.git/HEAD`** → `ref: refs/heads/` pattern? → run **git-dumper / GitTools / GitHacker**; review `config` and `logs/HEAD` for secrets.
2. **Else probe `/.svn/wc.db` or `entries`** → success? → **svn-extractor** or manual `wc.db` + pristine recovery.
3. **Else probe `/.hg/requires`** → success? → **mercurial dumper**.
4. **Else probe `/.bzr/README`** → Bazaar tooling or manual path walk.
5. **Parallel**: fetch **`/.DS_Store`**, **`/.env`**, common **backup extensions** on app root and parent paths.
6. **Interpret status codes**: **403 on directory** + **200 on specific files** → treat as **high priority** for file-by-file extraction.
7. **已经进了 Git 产品网页**（不是扫 `/.git` 的阶段）：README / 议题当 HTML 渲、组员只填数字 ID、克隆列表不隔离 → 接到 `xss-test.md` §8 特权上下文，**不替代**上面的路径泄露探针。

---

## 6. 活的 Git 产品（和 §1 路径泄露并列，不互相取代）

`/.git/HEAD` 是仓库文件漏到公网。进了工蜂 / GitLab / 自研 Git 网页之后，面还在业务里：

- README、Wiki、议题、评论：按存储 XSS 打；有桌面客户端就再看它是不是把同一段 HTML 当特权页渲（`xss-test.md` §8）
- 加组员 / 邀请：顺序 `user_id` + 批量写（`idor-test.md` §4 / §8），不是新洞种
- 克隆 / 仓库列表不按人隔离：你的仓出现在对方客户端里，只是把上面那条 XSS 送过去

没前端、只摸到裸 `.git` → 仍走 §1～§5，别为了客户端空等。
