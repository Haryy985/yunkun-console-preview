package selfupdate

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// The helper comes from the current Windows build, never from an unverified ZIP.
var offlineWorker, offlineWorkerErr = windowsOfflineWorker()

type OfflineInstall struct {
	Root         string `json:"root"`
	ConsolePort  int    `json:"console_port"`
	DatabasePort int    `json:"database_port"`
	Public       bool   `json:"public"`
	GOOS         string `json:"goos"`
	GOARCH       string `json:"goarch"`
}

type OfflineState struct {
	Phase     string `json:"phase"`
	Percent   int    `json:"percent"`
	Message   string `json:"message"`
	Error     string `json:"error,omitempty"`
	Version   string `json:"version,omitempty"`
	Job       string `json:"job,omitempty"`
	Backup    string `json:"backup,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type offlineJob struct {
	OfflineInstall
	ParentPID int    `json:"parent_pid"`
	From      string `json:"from"`
	To        string `json:"to"`
	Work      string `json:"work"`
}

// CurrentOfflineInstall only accepts the managed local database layout. External
// databases cannot be safely restored by a filesystem backup.
func CurrentOfflineInstall() (OfflineInstall, error) {
	var in OfflineInstall
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		return in, errors.New("当前平台暂不支持一键离线更新；请按离线升级手册操作")
	}
	if runtime.GOARCH != "amd64" && !(runtime.GOOS == "linux" && runtime.GOARCH == "arm64") {
		return in, errors.New("当前平台暂不支持一键离线更新；Linux/macOS 请按离线升级手册操作")
	}
	root := os.Getenv("YUNKUN_OFFLINE_ROOT")
	if root == "" {
		return in, errors.New("请通过新版 setup-and-start-offline 启动器运行后使用一键更新")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return in, err
	}
	in.Root = root
	in.GOOS, in.GOARCH = runtime.GOOS, runtime.GOARCH
	if resolved, err := filepath.EvalSymlinks(root); err != nil || !samePath(resolved, root) {
		return in, errors.New("离线安装根目录不能是链接")
	}
	in.ConsolePort, _ = strconv.Atoi(os.Getenv("YUNKUN_OFFLINE_CONSOLE_PORT"))
	in.Public = os.Getenv("YUNKUN_OFFLINE_PUBLIC") == "1"
	exe, err := os.Executable()
	appName, pgctlName, postgresName, pgMajor := "yunkun", "pg_ctl", "postgres", "18"
	if runtime.GOOS == "windows" {
		appName, pgctlName, postgresName, pgMajor = "yunkun.exe", "pg_ctl.exe", "postgres.exe", "pgsql-18.4"
	}
	if err != nil || !samePath(exe, filepath.Join(root, "bin", appName)) {
		return in, errors.New("程序路径与离线安装目录不一致，拒绝自动更新")
	}
	private := filepath.Join(root, ".yunkun-local")
	if !samePath(os.Getenv("YUNKUN_CONFIG"), filepath.Join(private, "config.json")) ||
		os.Getenv("YUNKUN_PG_DSN") != "" ||
		!samePath(os.Getenv("YUNKUN_SKILL_DIR"), filepath.Join(root, "skills")) {
		return in, errors.New("检测到自定义数据库或配置路径，请按手动升级流程处理")
	}
	var state struct {
		Format           int `json:"format"`
		Schema           int `json:"schema"`
		DatabasePort     int `json:"databasePort"`
		UnixDatabasePort int `json:"database_port"`
	}
	if err := readJSON(filepath.Join(private, "installation.json"), &state); err != nil {
		return in, fmt.Errorf("读取离线安装信息: %w", err)
	}
	in.DatabasePort = state.DatabasePort
	validFormat := state.Format == 1
	if runtime.GOOS == "linux" {
		in.DatabasePort = state.UnixDatabasePort
		validFormat = state.Schema == 1
	}
	if !validFormat ||
		in.ConsolePort < 1024 || in.ConsolePort > 65535 ||
		in.DatabasePort < 1024 || in.DatabasePort > 65535 || in.DatabasePort == in.ConsolePort {
		return in, errors.New("离线端口或安装格式无效")
	}
	var cfg struct {
		Database struct {
			DSN    string `json:"dsn"`
			Host   string `json:"host"`
			Port   int    `json:"port"`
			User   string `json:"user"`
			DBName string `json:"dbname"`
		} `json:"database"`
	}
	if err := readJSON(filepath.Join(private, "config.json"), &cfg); err != nil {
		return in, fmt.Errorf("读取离线配置: %w", err)
	}
	if cfg.Database.DSN != "" || cfg.Database.Host != "127.0.0.1" ||
		cfg.Database.Port != in.DatabasePort || cfg.Database.User != "yunkun" || cfg.Database.DBName != "yunkun" {
		return in, errors.New("数据库不属于标准离线安装，不能自动备份和回滚")
	}
	pgctlPath := filepath.Join("runtime", pgMajor, "bin", pgctlName)
	postgresPath := filepath.Join("runtime", pgMajor, "bin", postgresName)
	if runtime.GOOS == "linux" {
		pgctlPath = filepath.Join("runtime", "bin", pgctlName)
		postgresPath = filepath.Join("runtime", "bin", postgresName)
		var launcher struct {
			PID int `json:"pid"`
		}
		if readJSON(filepath.Join(private, "launcher.json"), &launcher) != nil ||
			!linuxManagedProcess(launcher.PID, filepath.Join(root, "bin", "yunkun-offline")) {
			return in, errors.New("请使用新版受管离线启动器启动后更新")
		}
	}
	for _, rel := range []string{filepath.Join("bin", appName), pgctlPath, postgresPath, filepath.Join(".yunkun-local", "postgres-data", "PG_VERSION")} {
		if st, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel))); err != nil || !st.Mode().IsRegular() {
			return in, fmt.Errorf("离线安装缺少正常文件: %s", rel)
		}
	}
	pgVersion, err := os.ReadFile(filepath.Join(private, "postgres-data", "PG_VERSION"))
	if err != nil || strings.TrimSpace(string(pgVersion)) != "18" {
		return in, errors.New("不支持自动升级此 PostgreSQL 主版本")
	}
	return in, nil
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func readJSON(file string, v any) error {
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func OfflineStatus(root string) OfflineState {
	var state OfflineState
	_ = readJSON(filepath.Join(root, ".yunkun-local", "update-state.json"), &state)
	return state
}

func OfflineActive(phase string) bool {
	switch phase {
	case "waiting", "backing_up", "installing", "starting", "rolling_back":
		return true
	}
	return false
}

// FindOfflineAsset accepts the published 1.0.x spelling and the unpublished 1.1.0
// candidate spelling. New packages use AssetName consistently.
func FindOfflineAsset(rel *Release, goos, goarch string) (Asset, bool) {
	if a, ok := rel.FindAsset(AssetName(rel.TagName, goos, goarch)); ok {
		return a, true
	}
	if goos == "windows" && goarch == "amd64" {
		return rel.FindAsset("yunkun-" + rel.TagName + "-windows-x64.zip")
	}
	return Asset{}, false
}

const maxOfflineZip = 2 << 30
const maxOfflineFiles = 25000
const maxOfflineExpanded = 4 << 30

// PrepareOffline completes all network and archive validation before the helper
// is allowed to stop the application. The authenticated caller supplies a version,
// not a URL or filesystem path.
func PrepareOffline(ctx context.Context, c *http.Client, rel *Release, current string, in OfflineInstall, prog Progress) (string, error) {
	if cmp, ok := CompareVersions(current, rel.TagName); !ok || cmp >= 0 || rel.Draft || rel.Prerelease {
		return "", errors.New("更新目标必须是更高的正式版本")
	}
	goos, goarch := in.GOOS, in.GOARCH
	if goos == "" {
		goos, goarch = "windows", "amd64"
	}
	asset, ok := FindOfflineAsset(rel, goos, goarch)
	if !ok || asset.Size <= 0 || asset.Size > maxOfflineZip {
		return "", errors.New("缺少当前平台有效的完整离线包")
	}
	updates := filepath.Join(in.Root, ".yunkun-local", "updates")
	if err := os.MkdirAll(updates, 0o700); err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(updates); err != nil || !samePath(resolved, updates) {
		return "", errors.New("更新工作目录不能是链接")
	}
	work, err := os.MkdirTemp(updates, "update-")
	if err != nil {
		return "", err
	}
	prog(PhaseDownload, 0, "正在获取更新包与校验清单")
	sums, err := fetchSums(ctx, c, rel)
	if err != nil {
		return "", err
	}
	want, ok := sums[asset.Name]
	if !ok {
		return "", errors.New("校验清单未收录此更新包")
	}
	archive := filepath.Join(work, "release.zip")
	got, err := download(ctx, c, asset, archive, prog)
	if err != nil {
		return "", err
	}
	prog(PhaseVerify, -1, "正在校验更新包完整性")
	if !strings.EqualFold(got, want) {
		return "", errors.New("更新包 SHA-256 不匹配，旧版未改变")
	}
	prog(PhaseExtract, -1, "正在检查并解压完整离线包")
	candidate := filepath.Join(work, "candidate")
	if err := extractOffline(archive, candidate); err != nil {
		return "", err
	}
	appName := "yunkun"
	if goos == "windows" {
		appName = "yunkun.exe"
	}
	bin := filepath.Join(candidate, "bin", appName)
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, bin, "-version")
	hideProcess(cmd)
	out, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(out)) != rel.TagName {
		return "", errors.New("更新程序无法运行或程序版本与发布标签不一致")
	}
	if goos == "linux" {
		launcher := filepath.Join(candidate, "bin", "yunkun-offline")
		out, err := exec.CommandContext(probeCtx, launcher, "--version").Output()
		if err != nil || strings.TrimSpace(string(out)) != rel.TagName {
			return "", errors.New("更新包启动器版本不匹配")
		}
		if err := exec.CommandContext(probeCtx, launcher, "--check").Run(); err != nil {
			return "", errors.New("更新包依赖预检查失败，当前服务未改变")
		}
	}
	// These release files are restored together with the database on startup failure.
	job := offlineJob{OfflineInstall: in, ParentPID: os.Getpid(), From: current, To: rel.TagName, Work: work}
	data, err := json.Marshal(job)
	if err != nil {
		return "", err
	}
	jobPath := filepath.Join(work, "job.json")
	if err := os.WriteFile(jobPath, data, 0o600); err != nil {
		return "", err
	}
	if goos == "windows" {
		if offlineWorkerErr != nil {
			return "", offlineWorkerErr
		}
		if err := os.WriteFile(filepath.Join(work, "worker.ps1"), offlineWorker, 0o600); err != nil {
			return "", err
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if goos == "windows" {
		if err := copyVerifiedFile(exe, filepath.Join(work, "file-helper.exe"), 0o700); err != nil {
			return "", err
		}
	} else {
		if err := copyVerifiedFile(exe, filepath.Join(work, "file-helper"), 0o700); err != nil {
			return "", err
		}
	}
	return jobPath, nil
}

func StartOfflineWorker(jobPath string) error {
	if runtime.GOOS == "linux" {
		return startLinuxOfflineWorker(jobPath)
	}
	powershell := filepath.Join(os.Getenv("WINDIR"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	work := filepath.Dir(jobPath)
	cmd := exec.Command(powershell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-File", filepath.Join(work, "worker.ps1"), "-JobFile", jobPath)
	hideProcess(cmd)
	logFile, err := os.OpenFile(filepath.Join(work, "worker.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return err
	}
	_ = logFile.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timeout := time.NewTimer(20 * time.Second)
	defer timeout.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-done:
			return fmt.Errorf("更新助手启动失败，请检查更新日志: %v", err)
		case <-timeout.C:
			_ = cmd.Process.Kill()
			return errors.New("更新助手未就绪，当前服务继续运行")
		case <-tick.C:
			if _, err := os.Stat(filepath.Join(work, "ready")); err == nil {
				return nil
			}
		}
	}
}

func offlineEntry(name string) (string, error) {
	if strings.ContainsAny(name, "\\:\x00") || strings.HasPrefix(name, "/") {
		return "", errors.New("发布包路径无效")
	}
	parts := strings.Split(strings.TrimSuffix(name, "/"), "/")
	if len(parts) < 2 {
		return "", errors.New("发布包必须包含单一顶层目录")
	}
	for _, p := range parts {
		if p == "" || p == "." || p == ".." || strings.TrimRight(p, ". ") != p {
			return "", errors.New("发布包含非法路径")
		}
		base := strings.ToUpper(strings.SplitN(p, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" ||
			(len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9') {
			return "", errors.New("发布包含 Windows 保留名称")
		}
	}
	rel := path.Join(parts[1:]...)
	first := strings.ToLower(parts[1])
	switch first {
	case "bin":
		if !strings.EqualFold(rel, "bin/yunkun.exe") && !strings.EqualFold(rel, "bin/yunkun") && !strings.EqualFold(rel, "bin/yunkun-offline") {
			return "", errors.New("发布包 bin 目录含有非程序文件")
		}
	case "runtime", "skills", "docs", "screenshots":
	case "setup-and-start-offline.ps1", "setup-and-start-offline.bat", "stop-offline.bat", "upgrade-offline.ps1",
		"readme.md", "changelog.md", "notice.md", "license", "config.example.json", "version",
		"start.sh", "stop.sh", "check.sh", "readme.txt", "package.json", "files.sha256",
		"postgresql_license.txt", "third_party_notices.txt":
		if len(parts) != 2 {
			return "", errors.New("发布包文件路径无效")
		}
	default:
		return "", fmt.Errorf("发布包含不允许的文件: %s", first)
	}
	if strings.HasSuffix(strings.ToLower(rel), ".key") || strings.EqualFold(path.Base(rel), "config.json") {
		return "", errors.New("发布包不得包含私有密钥或配置")
	}
	return rel, nil
}

func extractOffline(archive, dst string) error {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer z.Close()
	if len(z.File) > maxOfflineFiles {
		return errors.New("更新包文件过多")
	}
	var total uint64
	seen := map[string]bool{}
	root := ""
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue // directories are created only as parents of validated files
		}
		if !f.Mode().IsRegular() {
			return errors.New("更新包不允许链接或特殊文件")
		}
		rel, err := offlineEntry(f.Name)
		if err != nil {
			return err
		}
		prefix := strings.SplitN(f.Name, "/", 2)[0]
		if root != "" && prefix != root {
			return errors.New("更新包包含多个顶层目录")
		}
		root = prefix
		key := strings.ToLower(rel)
		if seen[key] {
			return errors.New("更新包包含重复文件")
		}
		seen[key] = true
		if f.UncompressedSize64 > maxBinarySize || total+f.UncompressedSize64 > maxOfflineExpanded {
			return errors.New("更新包解压体积超限")
		}
		total += f.UncompressedSize64
		file := filepath.Join(dst, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			return err
		}
		if err := extractOfflineFile(f, file); err != nil {
			return err
		}
	}
	if seen["bin/yunkun.exe"] {
		for _, required := range []string{"bin/yunkun.exe", "setup-and-start-offline.ps1", "runtime/pgsql-18.4/bin/pg_ctl.exe", "runtime/pgsql-18.4/bin/postgres.exe"} {
			if !seen[required] {
				return fmt.Errorf("不是完整离线包，缺少 %s", required)
			}
		}
	} else {
		for _, required := range []string{"bin/yunkun", "bin/yunkun-offline", "start.sh", "stop.sh",
			"runtime/bin/pg_ctl", "runtime/bin/postgres", "runtime/bin/initdb", "runtime/share/postgresql/postgres.bki"} {
			if !seen[required] {
				return fmt.Errorf("不是完整离线包，缺少 %s", required)
			}
		}
	}
	return nil
}

func extractOfflineFile(entry *zip.File, dst string) error {
	r, err := entry.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	w, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(w, io.LimitReader(r, maxBinarySize+1))
	closeErr := w.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return err
	}
	mode := entry.Mode().Perm() & 0o755
	if mode == 0 {
		mode = 0o600
	}
	return os.Chmod(dst, mode)
}
