//go:build linux || darwin

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfigIsPreservedAndMismatchFailsClosed(t *testing.T) {
	i := installation{root: t.TempDir(), state: installState{
		Schema: 1, DatabasePort: 55435, AdminPassword: secret(), AppPassword: secret(),
	}}
	p := filepath.Join(i.root, "config.json")
	if err := i.ensureConfig(p); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	var saved map[string]any
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	saved["customer_setting"] = map[string]any{"keep": true}
	if err := writeJSON(p, saved); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(p)
	if err := i.ensureConfig(p); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(p)
	if !bytes.Equal(before, after) {
		t.Fatal("existing config was rewritten")
	}
	i.state.AppPassword = secret()
	if err := i.ensureConfig(p); err == nil {
		t.Fatal("mismatched credentials accepted")
	}
	after, _ = os.ReadFile(p)
	if !bytes.Equal(before, after) {
		t.Fatal("mismatched configuration was overwritten")
	}
}

func TestConfigSymlinkIsRejected(t *testing.T) {
	i := installation{root: t.TempDir()}
	target := filepath.Join(i.root, "target")
	if err := os.WriteFile(target, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(i.root, "config.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := i.ensureConfig(link); err == nil {
		t.Fatal("symlink was accepted")
	}
	raw, _ := os.ReadFile(target)
	if string(raw) != "preserve" {
		t.Fatal("symlink target changed")
	}
}

func TestReadyRequiresVersionIdentityAndAuthentication(t *testing.T) {
	for _, tc := range []struct{ name, service, build, auth string }{
		{"valid", "yunkun", version, `{"initialized":false}`},
		{"wrong service", "other", version, `{"initialized":false}`},
		{"wrong version", "yunkun", "older", `{"initialized":false}`},
		{"auth unavailable", "yunkun", version, `{"error":"not ready"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/auth/status" {
					w.Write([]byte(tc.auth))
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"ok": true, "ready": true, "service": tc.service, "version": tc.build})
			}))
			defer s.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 1100*time.Millisecond)
			defer cancel()
			_, err := waitReady(ctx, s.URL, make(chan error))
			if (err == nil) != (tc.name == "valid") {
				t.Fatalf("readiness result: %v", err)
			}
		})
	}
}

func TestEnvDoesNotLeakInheritedDatabaseOrLoaderSettings(t *testing.T) {
	t.Setenv("PGHOST", "unrelated")
	t.Setenv("YUNKUN_PG_DSN", "unrelated")
	t.Setenv("LD_PRELOAD", "unrelated")
	t.Setenv("DYLD_LIBRARY_PATH", "unrelated")
	env := strings.Join(cleanEnv(map[string]string{"YUNKUN_CONFIG": "/managed/config.json"}), "\n")
	for _, key := range []string{"PGHOST=", "YUNKUN_PG_DSN=", "LD_PRELOAD=", "DYLD_LIBRARY_PATH="} {
		if strings.Contains(env, key) {
			t.Fatalf("inherited %s", key)
		}
	}
	if !strings.Contains(env, "YUNKUN_CONFIG=/managed/config.json") {
		t.Fatal("managed setting missing")
	}
}
