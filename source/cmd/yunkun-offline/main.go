//go:build linux || darwin

// Command yunkun-offline bootstraps a private, bundled PostgreSQL installation.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"yunkun/console/selfupdate"
)

type installState struct {
	Schema        int    `json:"schema"`
	DatabasePort  int    `json:"database_port"`
	AdminPassword string `json:"admin_password"`
	AppPassword   string `json:"app_password"`
}

type installation struct {
	root, home, pg, data string
	state                installState
}

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Yunkun:", err)
		os.Exit(1)
	}
}

func run() error {
	port := flag.Int("port", 5173, "console port (loopback only)")
	dbPort := flag.Int("database-port", 5433, "private database port on first installation")
	noBrowser := flag.Bool("no-browser", false, "do not open a browser")
	stop := flag.Bool("stop", false, "stop this installation, retaining all data")
	check := flag.Bool("check", false, "check bundled dependencies without creating an installation")
	showVersion := flag.Bool("version", false, "print build version")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return nil
	}
	if flag.NArg() != 0 || !validPort(*port) || !validPort(*dbPort) || *port == *dbPort {
		return errors.New("use distinct ports between 1024 and 65535")
	}
	syscall.Umask(0077)
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}
	i := installation{root: filepath.Dir(filepath.Dir(exe))}
	i.home = filepath.Join(i.root, ".yunkun-local")
	i.pg = filepath.Join(i.root, "runtime")
	i.data = filepath.Join(i.home, "postgres-data")
	if *stop {
		return i.stop(exe)
	}
	if err := i.preflight(); err != nil {
		return err
	}
	if *check {
		fmt.Printf("Dependencies ready: %s %s/%s (no installation data created)\n", version, runtime.GOOS, runtime.GOARCH)
		return nil
	}
	if os.Geteuid() == 0 {
		return errors.New("run as a regular user, without sudo; PostgreSQL cannot run as root")
	}
	if err := privateDir(i.home); err != nil {
		return err
	}
	update := selfupdate.OfflineStatus(i.root)
	updateJob := os.Getenv("YUNKUN_UPDATE_JOB")
	if selfupdate.OfflineActive(update.Phase) || update.Phase == "recovery_required" {
		if updateJob == "" || updateJob != update.Job ||
			(update.Phase != "starting" && update.Phase != "rolling_back") {
			return errors.New("offline update or recovery is in progress; inspect .yunkun-local/update-state.json")
		}
	}
	lock, err := os.OpenFile(filepath.Join(i.home, "launcher.lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return errors.New("this installation is already running")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	if err := writeJSON(filepath.Join(i.home, "launcher.json"), map[string]any{"pid": os.Getpid()}); err != nil {
		return err
	}
	defer os.Remove(filepath.Join(i.home, "launcher.json"))
	for _, p := range []string{i.data, filepath.Join(i.home, "data"), filepath.Join(i.home, "logs")} {
		if err := privateDir(p); err != nil {
			return err
		}
	}
	statePath := filepath.Join(i.home, "installation.json")
	raw, err := readPrivate(statePath)
	if errors.Is(err, os.ErrNotExist) {
		entries, e := os.ReadDir(i.data)
		if e != nil {
			return e
		}
		if len(entries) != 0 {
			return errors.New("database files exist without installation state; restore the original state, do not reinitialize")
		}
		i.state = installState{Schema: 1, DatabasePort: *dbPort, AdminPassword: secret(), AppPassword: secret()}
		if err := writeJSON(statePath, i.state); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if err := json.Unmarshal(raw, &i.state); err != nil {
		return fmt.Errorf("invalid installation state: %w", err)
	}
	if !validState(i.state) {
		return errors.New("invalid installation state; existing credentials were not changed")
	}
	var explicitDBPort bool
	flag.Visit(func(f *flag.Flag) { explicitDBPort = explicitDBPort || f.Name == "database-port" })
	if explicitDBPort && *dbPort != i.state.DatabasePort {
		return errors.New("database-port differs from saved installation; existing database was not changed")
	}
	if *port == i.state.DatabasePort {
		return errors.New("console port conflicts with the saved database port")
	}
	if err := portFree(*port); err != nil {
		return err
	}
	configPath := filepath.Join(i.home, "config.json")
	if err := i.ensureConfig(configPath); err != nil {
		return err
	}
	if err := i.ensureDatabase(); err != nil {
		return err
	}
	defer i.stopDatabase()
	if err := i.provision(); err != nil {
		return err
	}
	appLog, err := os.OpenFile(filepath.Join(i.home, "logs", "console.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer appLog.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	app := exec.Command(filepath.Join(i.root, "bin", "yunkun"),
		"-addr", "127.0.0.1:"+strconv.Itoa(*port), "-proxy=", "-data", filepath.Join(i.home, "data"))
	app.Dir = i.home
	app.Env = cleanEnv(map[string]string{
		"YUNKUN_CONFIG": configPath, "YUNKUN_SKILL_DIR": filepath.Join(i.root, "skills"),
		"YUNKUN_OFFLINE_ROOT": i.root, "YUNKUN_OFFLINE_CONSOLE_PORT": strconv.Itoa(*port),
		"YUNKUN_OFFLINE_PUBLIC": "0",
		"YUNKUN_UPDATE_JOB":     updateJob,
		"YUNKUN_UPDATE_PROBE": func() string {
			if updateJob != "" && updateJob == update.Job && os.Getenv("YUNKUN_UPDATE_PROBE") == "1" {
				return "1"
			}
			return ""
		}(),
		"NORMA_RIPGREP_NO_INSTALL": "1",
	})
	app.Stdout, app.Stderr = appLog, appLog
	// A separate process group lets shutdown reach child tools as well as the server.
	app.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := app.Start(); err != nil {
		return err
	}
	exited := make(chan error, 1)
	go func() { exited <- app.Wait() }()
	defer func() {
		_ = syscall.Kill(-app.Process.Pid, syscall.SIGTERM)
		select {
		case <-exited:
		case <-time.After(15 * time.Second):
			_ = syscall.Kill(-app.Process.Pid, syscall.SIGKILL)
			<-exited
		}
	}()
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", *port)
	ready, err := waitReady(ctx, baseURL, exited)
	if err != nil {
		// The deferred cleanup waits for this channel too.
		if !ready {
			exited <- err
		}
		return err
	}
	fmt.Printf("Yunkun is ready: %s/login/\nKeep this terminal open. Ctrl+C or ./stop.sh stops this installation and preserves its data.\n", baseURL)
	if !*noBrowser {
		browser := "xdg-open"
		if runtime.GOOS == "darwin" {
			browser = "open"
		}
		cmd := exec.Command(browser, baseURL+"/login/")
		if err := cmd.Start(); err == nil {
			go cmd.Wait()
		}
	}
	select {
	case <-ctx.Done():
		return nil
	case err := <-exited:
		exited <- err
		if selfupdate.OfflineActive(selfupdate.OfflineStatus(i.root).Phase) {
			fmt.Println("The update supervisor is restarting this installation; follow progress in the console.")
			return nil
		}
		if err == nil {
			return errors.New("console exited unexpectedly; inspect .yunkun-local/logs/console.log")
		}
		return fmt.Errorf("console exited; inspect .yunkun-local/logs/console.log: %w", err)
	}
}

func validPort(port int) bool { return port >= 1024 && port <= 65535 }

func validState(s installState) bool {
	if s.Schema != 1 || !validPort(s.DatabasePort) {
		return false
	}
	for _, password := range []string{s.AdminPassword, s.AppPassword} {
		b, err := hex.DecodeString(password)
		if err != nil || len(b) != 32 {
			return false
		}
	}
	return true
}

func secret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func privateDir(path string) error {
	if err := os.Mkdir(path, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 || st.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("unsafe private directory: %s", path)
	}
	return os.Chmod(path, 0700)
}

func readPrivate(path string) ([]byte, error) {
	return readPrivateLimit(path, 8192)
}

func readPrivateLimit(path string, limit int64) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || st.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) {
		return nil, fmt.Errorf("unsafe private file: %s", path)
	}
	if st.Size() > limit {
		return nil, errors.New("private file is too large")
	}
	b := make([]byte, st.Size())
	_, err = f.ReadAt(b, 0)
	return b, err
}

func (i installation) ensureConfig(path string) error {
	raw, err := readPrivateLimit(path, 4<<20)
	if errors.Is(err, os.ErrNotExist) {
		return writeJSON(path, map[string]any{
			"database": map[string]any{
				"host": "127.0.0.1", "port": i.state.DatabasePort, "user": "yunkun",
				"password": i.state.AppPassword, "dbname": "yunkun", "sslmode": "disable",
			},
			"skill_dir": filepath.Join(i.root, "skills"),
		})
	}
	if err != nil {
		return err
	}
	var config struct {
		Database struct {
			Host, User, Password, DBName, SSLMode string
			Port                                  int
		}
	}
	if json.Unmarshal(raw, &config) != nil {
		return errors.New("invalid config.json; existing configuration was preserved")
	}
	d := config.Database
	if d.Host != "127.0.0.1" || d.Port != i.state.DatabasePort || d.User != "yunkun" ||
		d.Password != i.state.AppPassword || d.DBName != "yunkun" || d.SSLMode != "disable" {
		return errors.New("config.json differs from this managed database; existing configuration was preserved")
	}
	return nil
}

func (i installation) preflight() error {
	for _, tool := range []string{"bash", "ps"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("required system command %s is missing; use a full supported OS installation", tool)
		}
	}
	for _, name := range []string{"postgres", "initdb", "pg_ctl"} {
		if err := i.pgCommand(name, "--version"); err != nil {
			return fmt.Errorf("bundled PostgreSQL dependency check: %w; use the package matching your OS and CPU; see README.txt", err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	app := exec.CommandContext(ctx, filepath.Join(i.root, "bin", "yunkun"), "-version")
	app.Env = cleanEnv(nil)
	out, err := app.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bundled console could not execute: %w; use the package matching your OS and CPU", err)
	}
	if strings.TrimSpace(string(out)) != version {
		return errors.New("console and launcher versions differ; extract a complete matching package")
	}
	for _, relative := range []string{"runtime/share/postgresql/postgres.bki", "skills"} {
		if _, err := os.Stat(filepath.Join(i.root, relative)); err != nil {
			return fmt.Errorf("incomplete offline package: %s", relative)
		}
	}
	return nil
}

func writeJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".state-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func cleanEnv(extra map[string]string) []string {
	var out []string
	for _, e := range os.Environ() {
		key, _, _ := strings.Cut(e, "=")
		if strings.HasPrefix(key, "PG") || strings.HasPrefix(key, "YUNKUN_") || strings.HasPrefix(key, "ARTEX_") ||
			key == "LD_PRELOAD" || key == "LD_LIBRARY_PATH" || strings.HasPrefix(key, "DYLD_") {
			continue
		}
		out = append(out, e)
	}
	for k, v := range extra {
		out = append(out, k+"="+v)
	}
	return out
}

func portFree(port int) error {
	l, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("port %d is unavailable; no existing process was stopped", port)
	}
	return l.Close()
}

func (i installation) pgCommand(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 65*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(i.pg, "bin", args[0]), args[1:]...)
	cmd.Env = cleanEnv(map[string]string{"LD_LIBRARY_PATH": filepath.Join(i.pg, "lib")})
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s failed: %w\n%s", args[0], err, output)
	}
	return nil
}

func (i installation) ensureDatabase() error {
	if _, err := os.Stat(filepath.Join(i.data, "PG_VERSION")); errors.Is(err, os.ErrNotExist) {
		entries, e := os.ReadDir(i.data)
		if e != nil || len(entries) != 0 {
			return errors.New("database initialization is incomplete; preserve data and inspect logs before retrying")
		}
		pw, e := os.CreateTemp(i.home, ".init-password-")
		if e != nil {
			return e
		}
		defer os.Remove(pw.Name())
		if _, e = pw.WriteString(i.state.AdminPassword + "\n"); e != nil {
			pw.Close()
			return e
		}
		if e = pw.Close(); e != nil {
			return e
		}
		if e = i.pgCommand("initdb", "-D", i.data, "-U", "yunkun_admin", "--pwfile="+pw.Name(),
			"--encoding=UTF8", "--locale=C", "--auth-host=scram-sha-256", "--auth-local=scram-sha-256"); e != nil {
			return e
		}
	} else if err != nil {
		return err
	}
	major, err := os.ReadFile(filepath.Join(i.data, "PG_VERSION"))
	if err != nil || strings.TrimSpace(string(major)) != "18" {
		return errors.New("this launcher requires PostgreSQL 18 data; existing database was not changed")
	}
	if i.pgCommand("pg_ctl", "-D", i.data, "status") == nil {
		return i.verifyDatabase()
	}
	if err := portFree(i.state.DatabasePort); err != nil {
		return err
	}
	options := fmt.Sprintf("-h 127.0.0.1 -p %d -c unix_socket_directories='' -c max_connections=20", i.state.DatabasePort)
	if err := i.pgCommand("pg_ctl", "-D", i.data, "-l", filepath.Join(i.home, "logs", "postgresql.log"),
		"-o", options, "-w", "-t", "45", "start"); err != nil {
		return err
	}
	return i.verifyDatabase()
}

func (i installation) connect() (*pgx.Conn, error) {
	c, err := pgx.ParseConfig("")
	if err != nil {
		return nil, err
	}
	c.Host, c.Port = "127.0.0.1", uint16(i.state.DatabasePort)
	c.User, c.Password, c.Database = "yunkun_admin", i.state.AdminPassword, "postgres"
	c.TLSConfig, c.Fallbacks = nil, nil
	c.ConnectTimeout = 5 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return pgx.ConnectConfig(ctx, c)
}

func (i installation) verifyDatabase() error {
	c, err := i.connect()
	if err != nil {
		return errors.New("private database connection failed; existing credentials were not reset")
	}
	defer c.Close(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var dataDir string
	if err := c.QueryRow(ctx, "SHOW data_directory").Scan(&dataDir); err != nil {
		return err
	}
	if filepath.Clean(dataDir) != filepath.Clean(i.data) {
		return errors.New("database belongs to another installation; no changes were made")
	}
	return nil
}

func (i installation) provision() error {
	c, err := i.connect()
	if err != nil {
		return err
	}
	defer c.Close(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var exists bool
	if err := c.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='yunkun')").Scan(&exists); err != nil {
		return err
	}
	if !exists {
		// State validation permits only a fixed-length hex secret here.
		if _, err := c.Exec(ctx, "CREATE ROLE yunkun LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD '"+i.state.AppPassword+"'"); err != nil {
			return err
		}
	}
	if err := c.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname='yunkun')").Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if _, err := c.Exec(ctx, "CREATE DATABASE yunkun OWNER yunkun ENCODING 'UTF8'"); err != nil {
			return err
		}
	}
	var elevated bool
	if err := c.QueryRow(ctx, "SELECT rolsuper OR rolcreatedb OR rolcreaterole FROM pg_roles WHERE rolname='yunkun'").Scan(&elevated); err != nil {
		return err
	}
	if elevated {
		return errors.New("application database role has unexpected elevated privileges")
	}
	return nil
}

func (i installation) stopDatabase() error {
	if err := i.verifyDatabase(); err != nil {
		return err
	}
	return i.pgCommand("pg_ctl", "-D", i.data, "-m", "fast", "-w", "-t", "45", "stop")
}

func (i installation) stop(executable string) error {
	st, err := os.Lstat(i.home)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Println("This installation is not running.")
		return nil
	}
	if err != nil || !st.IsDir() || st.Mode().Perm()&0077 != 0 || st.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) {
		return errors.New("unsafe installation directory")
	}
	raw, err := readPrivate(filepath.Join(i.home, "launcher.json"))
	if errors.Is(err, os.ErrNotExist) {
		fmt.Println("No running console launcher; data was preserved.")
		return nil
	}
	if err != nil {
		return err
	}
	var record struct {
		PID int `json:"pid"`
	}
	if err := json.Unmarshal(raw, &record); err != nil || record.PID <= 1 || record.PID == os.Getpid() {
		return errors.New("invalid launcher process record")
	}
	if err := ownedProcess(record.PID, executable); err != nil {
		return err
	}
	if err := syscall.Kill(record.PID, syscall.SIGTERM); err != nil {
		return err
	}
	for end := time.Now().Add(70 * time.Second); time.Now().Before(end); {
		if _, err := os.Stat(filepath.Join(i.home, "launcher.json")); errors.Is(err, os.ErrNotExist) {
			fmt.Println("Yunkun stopped; all local data was preserved.")
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return errors.New("shutdown has not completed; inspect the private logs")
}

func ownedProcess(pid int, executable string) error {
	uid, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "uid=").Output()
	if err != nil || strings.TrimSpace(string(uid)) != strconv.Itoa(os.Geteuid()) {
		return errors.New("process ownership could not be verified; nothing was stopped")
	}
	var path string
	if runtime.GOOS == "linux" {
		path, err = os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	} else {
		var out []byte
		out, err = exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
		path = strings.TrimSpace(string(out))
		if err == nil && path != executable && !strings.HasPrefix(path, executable+" ") {
			err = errors.New("process command does not match executable")
		}
		if err == nil {
			return nil
		}
	}
	if err != nil || path != executable {
		return errors.New("process executable does not match this installation; nothing was stopped")
	}
	return nil
}

// The boolean is false only when the child's exit notification was consumed.
func waitReady(ctx context.Context, baseURL string, exited <-chan error) (bool, error) {
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(100 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case <-ctx.Done():
			return true, ctx.Err()
		case err := <-exited:
			return false, fmt.Errorf("console exited before readiness; inspect .yunkun-local/logs/console.log: %v", err)
		case <-timeout.C:
			return true, errors.New("console readiness timed out; inspect .yunkun-local/logs/console.log")
		case <-ticker.C:
			response, err := client.Get(baseURL + "/api/health")
			if err != nil {
				continue
			}
			var health struct {
				Ready   bool   `json:"ready"`
				OK      bool   `json:"ok"`
				Service string `json:"service"`
				Version string `json:"version"`
			}
			err = json.NewDecoder(response.Body).Decode(&health)
			response.Body.Close()
			if err == nil && response.StatusCode == 200 && health.Ready && health.OK &&
				health.Service == "yunkun" && health.Version == version {
				auth, err := client.Get(baseURL + "/api/auth/status")
				if err != nil {
					continue
				}
				var status struct {
					Initialized *bool `json:"initialized"`
				}
				err = json.NewDecoder(auth.Body).Decode(&status)
				auth.Body.Close()
				if err == nil && auth.StatusCode == 200 && status.Initialized != nil {
					return true, nil
				}
			}
		}
	}
}
