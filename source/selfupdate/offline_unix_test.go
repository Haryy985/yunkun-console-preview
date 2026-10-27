//go:build linux || darwin

package selfupdate

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"testing"
)

func TestUnixManagedProcessIdentityAndCaseSensitivePaths(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		t.Fatal(err)
	}
	if !ManagedOfflineProcess(os.Getpid(), exe) {
		t.Fatal("current process identity rejected")
	}
	for _, pid := range []int{-1, 0, 1, os.Getpid()} {
		if ManagedOfflineProcess(pid, filepath.Join(t.TempDir(), "different")) {
			t.Fatal("unrelated process accepted")
		}
	}
	if samePath("/tmp/Skills", "/tmp/skills") {
		t.Fatal("Unix paths compared without case sensitivity")
	}
}

func TestUnixWorkerRejectsForeignPlatformBeforeTouchingInstallation(t *testing.T) {
	for _, job := range []offlineJob{
		{OfflineInstall: OfflineInstall{GOOS: "other", GOARCH: runtime.GOARCH}},
		{OfflineInstall: OfflineInstall{GOOS: runtime.GOOS, GOARCH: "other"}},
		{OfflineInstall: OfflineInstall{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Public: true}},
	} {
		file := filepath.Join(t.TempDir(), "job.json")
		if err := writeOfflineJSON(file, job); err != nil {
			t.Fatal(err)
		}
		if err := RunOfflineWorker(file); err == nil {
			t.Fatal("invalid worker job accepted")
		}
	}
}

func TestUnixStopRefusesUnrelatedProcess(t *testing.T) {
	cmd := exec.Command("/bin/sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	instance := unixUpdateInstance{cmd: cmd, done: make(chan error, 1),
		job: offlineJob{OfflineInstall: OfflineInstall{Root: t.TempDir()}}}
	if err := instance.stop(); err == nil {
		t.Fatal("unrelated process was signaled")
	}
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatal("unrelated process did not survive")
	}
}

// Re-exec the test binary under the real installation names so the identity
// checks are exercised, not bypassed by replacing production functions.
func TestUnixManagedInstallationHelper(t *testing.T) {
	mode := os.Getenv("YUNKUN_TEST_INSTALL_MODE")
	if mode == "" {
		return
	}
	if mode == "console" {
		in, err := CurrentOfflineInstall()
		wantFailure := os.Getenv("YUNKUN_TEST_INSTALL_BAD") == "1"
		if (err != nil) != wantFailure {
			t.Fatalf("installation check: %v", err)
		}
		if err == nil && (in.GOOS != runtime.GOOS || in.GOARCH != runtime.GOARCH || in.DatabasePort != 55435) {
			t.Fatal("wrong platform or managed database")
		}
		return
	}
	root := os.Getenv("YUNKUN_OFFLINE_ROOT")
	if err := writeOfflineJSON(filepath.Join(root, ".yunkun-local", "launcher.json"), map[string]int{"pid": os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(filepath.Join(root, "bin", "yunkun"), "-test.run=^TestUnixManagedInstallationHelper$")
	cmd.Env = append(os.Environ(), "YUNKUN_TEST_INSTALL_MODE=console")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("console check: %v\n%s", err, output)
	}
}

func TestUnixCurrentOfflineInstallation(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		t.Run(strconv.FormatBool(invalid), func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			root = filepath.Join(root, "installation with spaces")
			for _, dir := range []string{"bin", "skills", "runtime/bin", ".yunkun-local/postgres-data"} {
				if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
					t.Fatal(err)
				}
			}
			self, _ := os.Executable()
			for _, name := range []string{"yunkun", "yunkun-offline"} {
				if err := copyVerifiedFile(self, filepath.Join(root, "bin", name), 0700); err != nil {
					t.Fatal(err)
				}
			}
			files := map[string]string{
				"runtime/bin/pg_ctl": "fixture", "runtime/bin/postgres": "fixture",
				".yunkun-local/postgres-data/PG_VERSION": "18\n",
				".yunkun-local/installation.json":        `{"schema":1,"database_port":55435}`,
				".yunkun-local/config.json":              `{"database":{"host":"127.0.0.1","port":55435,"user":"yunkun","dbname":"yunkun"}}`,
			}
			if invalid {
				files[".yunkun-local/config.json"] = `{"database":{"host":"remote","port":55435,"user":"yunkun","dbname":"yunkun"}}`
			}
			for path, body := range files {
				if err := os.WriteFile(filepath.Join(root, path), []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command(filepath.Join(root, "bin", "yunkun-offline"), "-test.run=^TestUnixManagedInstallationHelper$")
			bad := "0"
			if invalid {
				bad = "1"
			}
			cmd.Env = append(os.Environ(), "YUNKUN_TEST_INSTALL_MODE=launcher", "YUNKUN_TEST_INSTALL_BAD="+bad,
				"YUNKUN_OFFLINE_ROOT="+root, "YUNKUN_CONFIG="+filepath.Join(root, ".yunkun-local", "config.json"),
				"YUNKUN_SKILL_DIR="+filepath.Join(root, "skills"), "YUNKUN_OFFLINE_CONSOLE_PORT=53513",
				"YUNKUN_OFFLINE_PUBLIC=0", "YUNKUN_PG_DSN=")
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("managed installation: %v\n%s", err, output)
			}
		})
	}
}

func TestUnixUpdateReadinessAndDeadProcess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/status" {
			_, _ = w.Write([]byte(`{"initialized":true}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ready": true, "version": "v1.1.1-offline", "service": "yunkun", "update_probe": true})
	}))
	defer server.Close()
	_, port, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	number, _ := strconv.Atoi(port)
	instance := unixUpdateInstance{job: offlineJob{OfflineInstall: OfflineInstall{ConsolePort: number}}, done: make(chan error, 1)}
	if err := instance.healthy("v1.1.1-offline", true); err != nil {
		t.Fatal(err)
	}
	instance.done <- nil
	if err := instance.healthy("v1.1.1-offline", true); err == nil {
		t.Fatal("dead launcher passed readiness")
	}
}
