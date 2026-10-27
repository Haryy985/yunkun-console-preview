//go:build linux || darwin

package selfupdate

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The native acceptance runner supplies a clean, locally built candidate. No
// production state or GitHub request is used by this preparation test.
func TestOfflineNativePrepare(t *testing.T) {
	candidate := os.Getenv("YUNKUN_NATIVE_CANDIDATE")
	if candidate == "" {
		t.Skip("requires native clean candidate from run-unix-update-acceptance.py")
	}
	out, err := exec.Command(filepath.Join(candidate, "bin", "yunkun"), "-version").Output()
	if err != nil {
		t.Fatal(err)
	}
	version := strings.TrimSpace(string(out))
	var archive bytes.Buffer
	z := zip.NewWriter(&archive)
	err = filepath.Walk(candidate, func(path string, info fs.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return walkErr
		}
		rel, err := filepath.Rel(candidate, path)
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name, header.Method = "package/"+filepath.ToSlash(rel), zip.Deflate
		if _, err := offlineEntry(header.Name); err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("not a clean candidate: %s", rel)
		}
		w, err := z.CreateHeader(header)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	name := AssetName(version, runtime.GOOS, runtime.GOARCH)
	release := &Release{TagName: version, Assets: []Asset{
		{Name: name, URL: "https://github.com/fixture/release.zip", Size: int64(archive.Len())},
		{Name: "SHA256SUMS", URL: "https://github.com/fixture/sums"},
	}}
	client := &http.Client{Transport: offlineTransport(func(r *http.Request) (*http.Response, error) {
		body := archive.Bytes()
		if strings.HasSuffix(r.URL.Path, "/sums") {
			body = []byte(fmt.Sprintf("%x  %s\n", sha256.Sum256(body), name))
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
	})}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	job, err := PrepareOffline(t.Context(), client, release, "v98.0.0-offline",
		OfflineInstall{Root: root, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH},
		func(Phase, int, string) {})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(job); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".yunkun-local", "installation.json")); !os.IsNotExist(err) {
		t.Fatal("preflight initialized an installation")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(job), "candidate", ".yunkun-local")); !os.IsNotExist(err) {
		t.Fatal("candidate preflight created private data")
	}
}
