//go:build linux

package selfupdate

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestLinuxManagedProcessIdentityAndCaseSensitivePaths(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if !linuxManagedProcess(os.Getpid(), exe) {
		t.Fatal("current process identity rejected")
	}
	for _, pid := range []int{-1, 0, 1, os.Getpid()} {
		if linuxManagedProcess(pid, filepath.Join(t.TempDir(), "different")) {
			t.Fatal("unrelated process accepted")
		}
	}
	if samePath("/tmp/Skills", "/tmp/skills") {
		t.Fatal("Linux paths compared without case sensitivity")
	}
}

func TestLinuxUpdateReadinessAndDeadProcess(t *testing.T) {
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
	instance := linuxUpdateInstance{job: offlineJob{OfflineInstall: OfflineInstall{ConsolePort: number}}, done: make(chan error, 1)}
	if err := instance.healthy("v1.1.1-offline", true); err != nil {
		t.Fatal(err)
	}
	instance.done <- nil
	if err := instance.healthy("v1.1.1-offline", true); err == nil {
		t.Fatal("dead launcher passed readiness")
	}
}
