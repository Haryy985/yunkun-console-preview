package selfupdate

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// sumsAsset 是 release.yml 生成的校验和清单，覆盖 Release 里全部 zip。
const sumsAsset = "SHA256SUMS"

// maxBinarySize 限制解压出来的二进制体积，防止畸形 zip 把磁盘写满。
const maxBinarySize = 512 << 20 // 512 MiB

// Phase 是升级过程中的阶段，直接用作 SSE 事件里的 phase 字段。
type Phase string

const (
	PhaseIdle     Phase = "idle"
	PhaseDownload Phase = "downloading"
	PhaseVerify   Phase = "verifying"
	PhaseExtract  Phase = "extracting"
	PhaseStaged   Phase = "staged"
	PhaseFailed   Phase = "failed"
)

// Progress 由调用方提供，用来把进度推给前端。pct 仅在下载阶段有意义（0-100），
// 其余阶段传 -1。
type Progress func(ph Phase, pct int, msg string)

// Stage 下载指定 Release 的当前平台发布包，校验后把新二进制暂存为 artex.new。
//
// 走的是完整 zip 而不是裸二进制，理由有两个：现有 Release 的 SHA256SUMS 本来就
// 只覆盖 zip，走 zip 不需要改 CI，也能兼容已经发布出去的历史版本；zip 里还带着
// skills/，为将来同步内置 skill 留了口子。代价只是多下载 skills 那几百 KB。
//
// 函数返回即代表暂存完成，调用方随后优雅关闭并以 ExitRestart 退出。
func Stage(ctx context.Context, c *http.Client, rel *Release, currentVersion string, prog Progress) error {
	if prog == nil {
		prog = func(Phase, int, string) {}
	}
	p, err := ResolvePaths()
	if err != nil {
		return err
	}
	if err := checkWritable(p.Dir); err != nil {
		return err
	}

	name := AssetName(rel.TagName, runtime.GOOS, runtime.GOARCH)
	asset, ok := rel.FindAsset(name)
	if !ok {
		return fmt.Errorf("该版本没有提供 %s/%s 的发布包（缺少 %s）", runtime.GOOS, runtime.GOARCH, name)
	}

	prog(PhaseDownload, 0, "获取校验和清单…")
	sums, err := fetchSums(ctx, c, rel, prog)
	if err != nil {
		return err
	}
	want, ok := sums[name]
	if !ok {
		return fmt.Errorf("%s 未收录 %s，拒绝安装未经校验的二进制", sumsAsset, name)
	}

	// 临时文件全部落在目标目录里，保证最后的 rename 是同一文件系统内的原子操作
	// （跨设备 rename 会失败，而 /tmp 常常是独立挂载点）。
	zipPath := p.New + ".zip.part"
	binPath := p.New + ".part"
	defer func() {
		_ = os.Remove(zipPath)
		_ = os.Remove(binPath)
	}()

	prog(PhaseDownload, 0, fmt.Sprintf("下载 %s（%s）…", name, humanSize(asset.Size)))
	got, err := download(ctx, c, asset, zipPath, prog)
	if err != nil {
		return err
	}

	prog(PhaseVerify, -1, "校验 SHA256…")
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("SHA256 不匹配：期望 %s，实际 %s（下载损坏或被篡改）", short(want), short(got))
	}

	prog(PhaseExtract, -1, "解压并冒烟测试…")
	if err := extractBinary(zipPath, binPath); err != nil {
		return err
	}
	if err := smokeTest(binPath); err != nil {
		return fmt.Errorf("新版本无法在当前系统上运行: %w", err)
	}

	// 暂存件自己的 sha256 单独存一份：下次启动换装前还要再校验一次，
	// 防止暂存后到重启前这段时间里文件被改动或写坏。
	binSum, err := fileSHA256(binPath)
	if err != nil {
		return fmt.Errorf("计算新二进制校验和: %w", err)
	}
	if err := os.WriteFile(p.Sum, []byte(binSum), 0o644); err != nil {
		return fmt.Errorf("写入校验和: %w", err)
	}
	if err := os.Rename(binPath, p.New); err != nil {
		_ = os.Remove(p.Sum)
		return fmt.Errorf("暂存新版本: %w", err)
	}

	if err := writeMarker(p.Marker, marker{
		From:     currentVersion,
		To:       strings.TrimPrefix(rel.TagName, "v"),
		StagedAt: time.Now().Unix(),
	}); err != nil {
		// 标记只影响自动回滚能力，暂存件本身已就位，不因此中断升级。
		prog(PhaseStaged, -1, "警告：写入升级标记失败，本次升级将没有自动回滚保护")
	}

	prog(PhaseStaged, 100, "新版本已就绪，正在重启…")
	return nil
}

// fetchSums 下载并解析 SHA256SUMS，返回 文件名 → 十六进制摘要。
func fetchSums(ctx context.Context, c *http.Client, rel *Release, prog Progress) (map[string]string, error) {
	asset, ok := rel.FindAsset(sumsAsset)
	if !ok {
		asset, ok = rel.FindAsset("SHA256SUMS.txt")
		if !ok {
			return nil, fmt.Errorf("该 Release 没有 %s，无法校验完整性，拒绝升级", sumsAsset)
		}
	}
	if asset.Size > 1<<20 {
		return nil, fmt.Errorf("%s 超过大小限制", sumsAsset)
	}
	// The API asset route avoids an extra TLS connection to github.com on
	// networks where the release-page host is unreliable. GitHub redirects
	// this route only to its release-assets host, still subject to checkURL.
	urls := []string{}
	if asset.ID > 0 {
		urls = append(urls, "https://api.github.com/repos/"+Repo+"/releases/assets/"+strconv.FormatInt(asset.ID, 10))
	}
	urls = append(urls, asset.URL)
	var lastErr error
	for _, rawURL := range urls {
		attempts := 2
		if len(urls) > 1 && rawURL == urls[len(urls)-1] {
			attempts = 1
		}
		for attempt := 1; attempt <= attempts; attempt++ {
			attemptCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			headers := http.Header{}
			if strings.HasPrefix(rawURL, "https://api.github.com/") {
				headers.Set("Accept", "application/octet-stream")
			}
			resp, err := getResponse(attemptCtx, c, rawURL, headers)
			if err == nil {
				var raw []byte
				raw, err = io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
				_ = resp.Body.Close()
				if err == nil {
					switch {
					case len(raw) > 1<<20:
						err = fmt.Errorf("%s 超过大小限制", sumsAsset)
					case asset.Size > 0 && int64(len(raw)) != asset.Size:
						err = fmt.Errorf("%s 长度不符", sumsAsset)
					case asset.Digest != "" && !strings.EqualFold(asset.Digest, "sha256:"+fmt.Sprintf("%x", sha256.Sum256(raw))):
						err = fmt.Errorf("%s 摘要不符", sumsAsset)
					default:
						if sums := parseSums(string(raw)); len(sums) > 0 {
							cancel()
							return sums, nil
						}
						err = fmt.Errorf("%s 内容为空或格式无法识别", sumsAsset)
					}
				}
			}
			cancel()
			lastErr = err
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if attempt == attempts || !retryableFetch(err) {
				break
			}
			if prog != nil {
				prog(PhaseDownload, 0, fmt.Sprintf("校验清单连接失败，正在重试（%d/%d）", attempt+1, attempts))
			}
			if err := waitRetry(ctx, attempt); err != nil {
				return nil, err
			}
		}
		if prog != nil && rawURL != urls[len(urls)-1] {
			prog(PhaseDownload, 0, "校验清单 API 不可用，尝试发布附件地址")
		}
	}
	return nil, fmt.Errorf("下载 %s 失败（已尝试 GitHub API 和发布附件，检查服务器网络或系统代理）: %w", sumsAsset, lastErr)
}

// parseSums 解析 sha256sum 风格的清单，返回 文件名 → 十六进制摘要。
//
// 第一个字段必须是 64 位十六进制才收录。只按"恰好两个字段"判断是不够的——
// 任意一行两个单词的说明文字都会被当成合法条目，把垃圾值塞进摘要表，
// 真正的资产反而可能匹配到错误的摘要。
func parseSums(raw string) map[string]string {
	out := map[string]string{}
	for line := range strings.Lines(raw) {
		// 格式为 "<sha256>  <filename>"（sha256sum 用双空格；shasum 的二进制
		// 模式会给文件名加 * 前缀）。
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 2 || !isHexSHA256(fields[0]) {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		if name == "" {
			continue
		}
		out[name] = strings.ToLower(fields[0])
	}
	return out
}

func isHexSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

// download 把资产写入 dst，同时计算 SHA256 并按 Content-Length 汇报进度。
func download(ctx context.Context, c *http.Client, a Asset, dst string, prog Progress) (string, error) {
	if prog == nil {
		prog = func(Phase, int, string) {}
	}
	if a.Size <= 0 || a.Size > maxOfflineZip {
		return "", fmt.Errorf("发布包大小无效或超限")
	}
	f, err := os.Create(dst)
	if err != nil {
		return "", fmt.Errorf("创建临时文件: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	pw := &progressWriter{total: a.Size, prog: prog, name: a.Name, last: time.Now()}
	urls := assetURLs(a)
	for attempt := 1; attempt <= 4 && pw.written < a.Size; attempt++ {
		headers := http.Header{}
		offset := pw.written
		if offset > 0 {
			headers.Set("Range", fmt.Sprintf("bytes=%d-", offset))
		}
		rawURL := urls[0]
		if attempt > 2 && len(urls) > 1 {
			rawURL = urls[1]
		}
		if strings.HasPrefix(rawURL, "https://api.github.com/") {
			headers.Set("Accept", "application/octet-stream")
		}
		resp, err := getResponse(ctx, c, rawURL, headers)
		if err != nil {
			if ctx.Err() == nil && !retryableFetch(err) && len(urls) > 1 && rawURL == urls[0] {
				attempt = 2 // The API asset endpoint is unavailable; try the browser URL.
				continue
			}
			if ctx.Err() != nil || !retryableFetch(err) || attempt == 4 {
				return "", fmt.Errorf("下载 %s: %w", a.Name, err)
			}
		} else {
			if strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") &&
				rawURL == urls[0] && len(urls) > 1 {
				resp.Body.Close()
				attempt = 2 // API served metadata instead of bytes; use browser URL.
				continue
			}
			if strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") ||
				(offset == 0 && resp.StatusCode != http.StatusOK) {
				resp.Body.Close()
				return "", fmt.Errorf("%s 返回了非完整发布包，拒绝安装", a.Name)
			}
			if offset > 0 && (resp.StatusCode != http.StatusPartialContent ||
				!validContentRange(resp.Header.Get("Content-Range"), offset, a.Size)) {
				resp.Body.Close()
				return "", fmt.Errorf("%s 的服务器未正确支持断点续传，保留旧版本；请检查网络后重试", a.Name)
			}
			// A stalled body must not hold the upgrade open for the client's
			// 30-minute whole-request timeout. Each successful write resets it.
			stall := time.AfterFunc(45*time.Second, func() { _ = resp.Body.Close() })
			pw.onWrite = func() { stall.Reset(45 * time.Second) }
			_, err = io.Copy(io.MultiWriter(f, h, pw), io.LimitReader(resp.Body, a.Size-offset+1))
			pw.onWrite = nil
			stall.Stop()
			resp.Body.Close()
			if err == nil && pw.written > a.Size {
				return "", fmt.Errorf("下载内容超过发布包大小")
			}
			if err == nil && pw.written < a.Size {
				err = io.ErrUnexpectedEOF
			}
			if err == nil {
				break
			}
			var fileErr *os.PathError
			if errors.As(err, &fileErr) {
				return "", fmt.Errorf("写入更新包失败: %w", err)
			}
			if ctx.Err() != nil || attempt == 4 {
				return "", fmt.Errorf("下载中断: %w", err)
			}
		}
		prog(PhaseDownload, int(pw.written*100/a.Size), fmt.Sprintf("网络中断，保留已下载的 %s，准备续传（%d/4）", humanSize(pw.written), attempt+1))
		if err := waitRetry(ctx, attempt); err != nil {
			return "", err
		}
	}
	if err := f.Sync(); err != nil {
		return "", fmt.Errorf("落盘失败: %w", err)
	}
	if a.Size > 0 && pw.written != a.Size {
		return "", fmt.Errorf("下载不完整：期望 %d 字节，实际 %d 字节", a.Size, pw.written)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func assetURLs(a Asset) []string {
	if a.ID > 0 {
		return []string{
			"https://api.github.com/repos/" + Repo + "/releases/assets/" + strconv.FormatInt(a.ID, 10),
			a.URL,
		}
	}
	return []string{a.URL}
}

// get 发起一个受白名单约束的 GET，返回响应体。
func getResponse(ctx context.Context, c *http.Client, rawURL string, headers http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if err := checkURL(req.URL); err != nil {
		return nil, err
	}
	req.Header = headers
	req.Header.Set("User-Agent", "yunkun-selfupdate")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return nil, fetchStatusError(resp.StatusCode)
	}
	return resp, nil
}

type fetchStatusError int

func (s fetchStatusError) Error() string { return fmt.Sprintf("HTTP %d", s) }

func retryableFetch(err error) bool {
	var status fetchStatusError
	if errors.As(err, &status) {
		return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500
	}
	var netErr net.Error
	return errors.As(err, &netErr) || errors.Is(err, io.ErrUnexpectedEOF)
}

func waitRetry(ctx context.Context, attempt int) error {
	timer := time.NewTimer(time.Duration(attempt) * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func validContentRange(value string, start, total int64) bool {
	var first, last, size int64
	if n, _ := fmt.Sscanf(value, "bytes %d-%d/%d", &first, &last, &size); n != 3 {
		return false
	}
	return value == fmt.Sprintf("bytes %d-%d/%d", first, last, size) &&
		first == start && last >= first && last < total && size == total
}

// extractBinary 从发布包里取出 artex 可执行文件。
//
// 包内结构是 artex-<版本>-<os>-<arch>/artex，但这里按**基名**匹配而不是拼完整
// 路径：版本号在包名里出现过一次，拼错一个字符就整个升级失败，按基名找更耐改。
func extractBinary(zipPath, dst string) error {
	want := "artex"
	if runtime.GOOS == "windows" {
		want = "artex.exe"
	}
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("打开发布包: %w", err)
	}
	defer zr.Close()

	for _, entry := range zr.File {
		if entry.FileInfo().IsDir() || !strings.EqualFold(path.Base(entry.Name), want) {
			continue
		}
		rc, err := entry.Open()
		if err != nil {
			return fmt.Errorf("读取 %s: %w", entry.Name, err)
		}
		defer rc.Close()

		f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return fmt.Errorf("写出新二进制: %w", err)
		}
		defer f.Close()

		n, err := io.Copy(f, io.LimitReader(rc, maxBinarySize+1))
		if err != nil {
			return fmt.Errorf("解压 %s: %w", entry.Name, err)
		}
		if n > maxBinarySize {
			return fmt.Errorf("发布包内的可执行文件超过 %s，拒绝解压", humanSize(maxBinarySize))
		}
		if n == 0 {
			return fmt.Errorf("发布包内的 %s 是空文件", want)
		}
		return f.Sync()
	}
	return fmt.Errorf("发布包里没有找到 %s", want)
}

// checkWritable 提前确认目录可写。没有这一步，非 root 运行、或二进制被放在系统
// 目录时，会在下载完几十 MB 之后才在换装那一刻失败。
func checkWritable(dir string) error {
	probe, err := os.CreateTemp(dir, ".artex-update-probe-*")
	if err != nil {
		return fmt.Errorf("程序目录 %s 不可写，无法自动更新（请检查权限或改用手动升级）: %w", dir, err)
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)
	return nil
}

// progressWriter 统计已写字节并限频汇报，避免每个 32KiB 分块都推一条 SSE。
type progressWriter struct {
	total   int64
	written int64
	name    string
	prog    Progress
	last    time.Time
	onWrite func()
}

func (w *progressWriter) Write(b []byte) (int, error) {
	w.written += int64(len(b))
	if w.onWrite != nil {
		w.onWrite()
	}
	if time.Since(w.last) < 300*time.Millisecond {
		return len(b), nil
	}
	w.last = time.Now()
	pct := -1
	if w.total > 0 {
		pct = int(w.written * 100 / w.total)
	}
	w.prog(PhaseDownload, pct, fmt.Sprintf("下载中 %s / %s", humanSize(w.written), humanSize(w.total)))
	return len(b), nil
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}

func short(sum string) string {
	if len(sum) > 12 {
		return sum[:12] + "…"
	}
	return sum
}
