package selfupdate

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfflineArchiveRejectsUnsafeEntries(t *testing.T) {
	for _, name := range []string{
		"pkg/../bin/yunkun.exe", "pkg/bin/jwt.key", "pkg/.yunkun-local/data/x",
		"pkg/skills/a/config.json", "pkg/skills/a/x:stream", "pkg/skills/a/NUL.txt",
		"pkg\\bin\\yunkun.exe", "/pkg/bin/yunkun.exe", "pkg/skills/a./file", "pkg/.git/config",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := offlineEntry(name); err == nil {
				t.Fatal("accepted unsafe entry")
			}
		})
	}
}

func makeOfflineZip(t *testing.T, extra []string) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for _, name := range append([]string{
		"pkg/bin/yunkun.exe", "pkg/setup-and-start-offline.ps1",
		"pkg/runtime/pgsql-18.4/bin/pg_ctl.exe", "pkg/runtime/pgsql-18.4/bin/postgres.exe",
		"pkg/skills/custom/SKILL.md",
	}, extra...) {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte("fixture"))
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestOfflineArchiveValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		extra []string
		valid bool
	}{
		{"valid", nil, true},
		{"duplicate", []string{"pkg/BIN/yunkun.exe"}, false},
		{"multiple-roots", []string{"other/README.md"}, false},
		{"private", []string{"pkg/.yunkun-local/installation.json"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			archive := filepath.Join(dir, "release.zip")
			if err := os.WriteFile(archive, makeOfflineZip(t, tc.extra), 0o600); err != nil {
				t.Fatal(err)
			}
			err := extractOffline(archive, filepath.Join(dir, "output"))
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}

type offlineTransport func(*http.Request) (*http.Response, error)

func (f offlineTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func offlineTestRoot(t *testing.T) string {
	t.Helper()
	// macOS commonly exposes temporary directories via /var -> /private/var.
	// Use the physical root just as the managed launcher does.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestOfflineChecksumFailureDoesNotTouchInstallation(t *testing.T) {
	root := offlineTestRoot(t)
	private := filepath.Join(root, ".yunkun-local")
	if err := os.Mkdir(private, 0o700); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(private, "record")
	_ = os.WriteFile(secret, []byte("user-data"), 0o600)
	archive := makeOfflineZip(t, nil)
	asset := AssetName("v1.1.0-offline", "windows", "amd64")
	rel := &Release{TagName: "v1.1.0-offline", Assets: []Asset{
		{Name: asset, URL: "https://github.com/fixture/release.zip", Size: int64(len(archive))},
		{Name: "SHA256SUMS.txt", URL: "https://github.com/fixture/sums"},
	}}
	client := &http.Client{Transport: offlineTransport(func(r *http.Request) (*http.Response, error) {
		body := archive
		if strings.HasSuffix(r.URL.Path, "/sums") {
			body = []byte(fmt.Sprintf("%x  %s\n", sha256.Sum256([]byte("wrong")), asset))
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
	})}
	_, err := PrepareOffline(context.Background(), client, rel, "v1.0.6-offline", OfflineInstall{Root: root}, func(Phase, int, string) {})
	if err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("expected checksum rejection: %v", err)
	}
	got, _ := os.ReadFile(secret)
	if string(got) != "user-data" {
		t.Fatal("private data changed")
	}
	if OfflineStatus(root).Phase != "" {
		t.Fatal("failed download requested application shutdown")
	}
}

func TestOfflineAssetCompatibility(t *testing.T) {
	for _, name := range []string{
		"yunkun-1.1.0-offline-windows-amd64.zip",
		"yunkun-v1.1.0-offline-windows-x64.zip",
	} {
		rel := &Release{TagName: "v1.1.0-offline", Assets: []Asset{{Name: name}}}
		if a, ok := FindOfflineAsset(rel, "windows", "amd64"); !ok || a.Name != name {
			t.Fatalf("missing compatibility for %s", name)
		}
	}
}

func TestOfflineArchiveRejectsIncompleteUnixPackage(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "unix.zip")
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for _, name := range []string{"pkg/bin/yunkun", "pkg/bin/yunkun-offline", "pkg/stop.sh"} {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte("not an accepted update package"))
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive, buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if err := extractOffline(archive, filepath.Join(t.TempDir(), "out")); err == nil || !strings.Contains(err.Error(), "缺少") {
		t.Fatalf("expected incomplete package rejection, got %v", err)
	}
}

func TestOfflineReleaseArchive(t *testing.T) {
	archive := os.Getenv("YUNKUN_TEST_OFFLINE_ZIP")
	if archive == "" {
		t.Skip("requires a data-free offline release ZIP fixture")
	}
	if err := extractOffline(archive, filepath.Join(t.TempDir(), "candidate")); err != nil {
		t.Fatal(err)
	}
	// Exercise the complete download/hash/extract/version-check path against
	// controlled HTTP fixtures without publishing a fake GitHub Release.
	body, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	name := AssetName("v1.1.0-offline", "windows", "amd64")
	sums := fmt.Sprintf("%x  %s\n", sha256.Sum256(body), name)
	rel := &Release{TagName: "v1.1.0-offline", Assets: []Asset{
		{Name: name, URL: "https://github.com/fixture/package", Size: int64(len(body))},
		{Name: "SHA256SUMS", URL: "https://github.com/fixture/sums"},
	}}
	client := &http.Client{Transport: offlineTransport(func(r *http.Request) (*http.Response, error) {
		data := body
		if strings.HasSuffix(r.URL.Path, "/sums") {
			data = []byte(sums)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), Header: make(http.Header)}, nil
	})}
	root := offlineTestRoot(t)
	job, err := PrepareOffline(t.Context(), client, rel, "v1.0.6-offline", OfflineInstall{Root: root}, func(Phase, int, string) {})
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{job, filepath.Join(filepath.Dir(job), "file-helper.exe"), filepath.Join(filepath.Dir(job), "worker.ps1")} {
		if _, err := os.Stat(file); err != nil {
			t.Fatal(err)
		}
	}
	if OfflineStatus(root).Phase != "" {
		t.Fatal("preparation must not stop application")
	}
}

func TestOfflineSnapshotRestoresMatchingDataAndProgram(t *testing.T) {
	root := offlineTestRoot(t)
	work := filepath.Join(root, ".yunkun-local", "updates", "fixture")
	write := func(file, value string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	key := filepath.Join(root, "bin", "jwt.key")
	data := filepath.Join(root, ".yunkun-local", "postgres-data", "record")
	skill := filepath.Join(root, "skills", strings.Repeat("deep-", 30), strings.Repeat("nested-", 20), "SKILL.md")
	write(key, "original-secret")
	write(data, "original-data")
	write(skill, "user-customization")
	write(filepath.Join(root, "bin", "yunkun.exe"), "old-program")
	write(filepath.Join(root, ".yunkun-local", "update-state.json"), `{"phase":"backing_up"}`)
	jobPath := filepath.Join(work, "job.json")
	b, _ := json.Marshal(offlineJob{OfflineInstall: OfflineInstall{Root: root}, Work: work})
	write(jobPath, string(b))
	if err := OfflineFiles(jobPath, "backup"); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(work, "backup-verified"), "verified")
	write(data, "new-schema-data")
	write(filepath.Join(root, "bin", "yunkun.exe"), "new-program")
	write(filepath.Join(root, "new-file"), "new-release")
	if err := OfflineFiles(jobPath, "restore"); err != nil {
		t.Fatal(err)
	}
	for file, want := range map[string]string{key: "original-secret", data: "original-data", skill: "user-customization",
		filepath.Join(root, "bin", "yunkun.exe"): "old-program"} {
		got, err := os.ReadFile(file)
		if err != nil || string(got) != want {
			t.Fatalf("restore %s: %v %q", file, err, got)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "new-file")); !os.IsNotExist(err) {
		t.Fatal("new release file left after rollback")
	}
	failed, err := os.ReadFile(filepath.Join(work, "failed-installation", ".yunkun-local", "postgres-data", "record"))
	if err != nil || string(failed) != "new-schema-data" {
		t.Fatal("failed database was not preserved")
	}
	if _, err := os.Stat(filepath.Join(work, "backup", ".yunkun-local", "updates")); !os.IsNotExist(err) {
		t.Fatal("snapshot recursively copied its own workspace")
	}
}
