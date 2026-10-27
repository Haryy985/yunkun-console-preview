package selfupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type interruptedReader struct{}

func (interruptedReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func response(status int, body io.Reader) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(body), Header: make(http.Header)}
}

func TestFetchSumsUsesAssetAPIWhenBrowserHostIsUnreachable(t *testing.T) {
	name := "yunkun-1.1.1-offline-windows-amd64.zip"
	raw := []byte(strings.Repeat("a", 64) + "  " + name + "\n")
	sum := fmt.Sprintf("%x", sha256.Sum256(raw))
	rel := &Release{Assets: []Asset{{
		ID: 123, Name: "SHA256SUMS", Size: int64(len(raw)), Digest: "sha256:" + sum,
		URL: "https://github.com/fixture/SHA256SUMS",
	}}}
	var requested []string
	client := &http.Client{Transport: offlineTransport(func(r *http.Request) (*http.Response, error) {
		requested = append(requested, r.URL.String())
		if r.URL.Host != "api.github.com" || r.Header.Get("Accept") != "application/octet-stream" {
			t.Fatalf("checksum should prefer trusted asset API: %s", r.URL)
		}
		return response(http.StatusOK, bytes.NewReader(raw)), nil
	})}
	sums, err := fetchSums(t.Context(), client, rel, nil)
	if err != nil || sums[name] != strings.Repeat("a", 64) || len(requested) != 1 {
		t.Fatalf("sums=%v requests=%v err=%v", sums, requested, err)
	}
}

func TestFetchSumsFallsBackAndRejectsInvalidAPIContent(t *testing.T) {
	raw := []byte(strings.Repeat("b", 64) + "  release.zip\n")
	rel := &Release{Assets: []Asset{{ID: 123, Name: "SHA256SUMS", Size: int64(len(raw)),
		URL: "https://github.com/fixture/SHA256SUMS"}}}
	var requested []string
	client := &http.Client{Transport: offlineTransport(func(r *http.Request) (*http.Response, error) {
		requested = append(requested, r.URL.Host)
		if r.URL.Host == "api.github.com" {
			return response(http.StatusOK, strings.NewReader(`{"message":"not raw data"}`)), nil
		}
		return response(http.StatusOK, bytes.NewReader(raw)), nil
	})}
	sums, err := fetchSums(t.Context(), client, rel, nil)
	if err != nil || sums["release.zip"] != strings.Repeat("b", 64) ||
		strings.Join(requested, ",") != "api.github.com,github.com" {
		t.Fatalf("sums=%v requests=%v err=%v", sums, requested, err)
	}
}

func TestFetchSumsRetriesTLSHandshakeTimeout(t *testing.T) {
	raw := []byte(strings.Repeat("c", 64) + "  release.zip\n")
	rel := &Release{Assets: []Asset{{Name: "SHA256SUMS", URL: "https://github.com/fixture/SHA256SUMS"}}}
	calls := 0
	var stages []string
	client := &http.Client{Transport: offlineTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return nil, &net.DNSError{Err: "TLS handshake timeout", IsTimeout: true}
		}
		return response(http.StatusOK, bytes.NewReader(raw)), nil
	})}
	sums, err := fetchSums(t.Context(), client, rel, func(_ Phase, _ int, msg string) {
		stages = append(stages, msg)
	})
	if err != nil || calls != 2 || sums["release.zip"] != strings.Repeat("c", 64) ||
		len(stages) == 0 || !strings.Contains(stages[0], "重试") {
		t.Fatalf("timeout should retry visibly: calls=%d stages=%v err=%v", calls, stages, err)
	}
}

func TestDownloadResumesAfterInterruptedBody(t *testing.T) {
	content := bytes.Repeat([]byte("abcdefgh"), 1024)
	partial := 1067
	calls := 0
	client := &http.Client{Transport: offlineTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			if r.Header.Get("Range") != "" {
				t.Fatal("initial request must not use Range")
			}
			return response(http.StatusOK, io.MultiReader(bytes.NewReader(content[:partial]), interruptedReader{})), nil
		}
		if r.Header.Get("Range") != fmt.Sprintf("bytes=%d-", partial) {
			t.Fatalf("wrong resume offset: %q", r.Header.Get("Range"))
		}
		res := response(http.StatusPartialContent, bytes.NewReader(content[partial:]))
		res.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", partial, len(content)-1, len(content)))
		return res, nil
	})}
	file := filepath.Join(t.TempDir(), "release.zip")
	var updates []int
	got, err := download(t.Context(), client, Asset{
		Name: "release.zip", URL: "https://github.com/fixture/release.zip", Size: int64(len(content)),
	}, file, func(_ Phase, pct int, _ string) { updates = append(updates, pct) })
	if err != nil {
		t.Fatal(err)
	}
	onDisk, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(onDisk, content) ||
		got != fmt.Sprintf("%x", sha256.Sum256(content)) || calls != 2 || len(updates) == 0 {
		t.Fatalf("download integrity or resume failed: calls=%d progress=%v err=%v", calls, updates, err)
	}
}

func TestDownloadUsesAssetAPIWhenReleasePageIsUnreachable(t *testing.T) {
	body := []byte("trusted release bytes")
	client := &http.Client{Transport: offlineTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.github.com" || r.Header.Get("Accept") != "application/octet-stream" {
			t.Fatalf("unexpected download host or Accept: %s %s", r.URL.Host, r.Header.Get("Accept"))
		}
		return response(http.StatusOK, bytes.NewReader(body)), nil
	})}
	file := filepath.Join(t.TempDir(), "release.zip")
	got, err := download(t.Context(), client, Asset{
		ID: 4321, Name: "release.zip", URL: "https://github.com/fixture/release.zip", Size: int64(len(body)),
	}, file, func(Phase, int, string) {})
	data, readErr := os.ReadFile(file)
	if err != nil || readErr != nil || !bytes.Equal(data, body) ||
		got != fmt.Sprintf("%x", sha256.Sum256(body)) {
		t.Fatalf("asset API download failed: %v %v", err, readErr)
	}
}

func TestDownloadFallsBackWhenAssetAPIIsUnavailable(t *testing.T) {
	body := []byte("trusted release bytes")
	var hosts []string
	client := &http.Client{Transport: offlineTransport(func(r *http.Request) (*http.Response, error) {
		hosts = append(hosts, r.URL.Host)
		if r.URL.Host == "api.github.com" {
			return response(http.StatusNotFound, strings.NewReader("unavailable")), nil
		}
		return response(http.StatusOK, bytes.NewReader(body)), nil
	})}
	got, err := download(t.Context(), client, Asset{
		ID: 4321, Name: "release.zip", URL: "https://github.com/fixture/release.zip", Size: int64(len(body)),
	}, filepath.Join(t.TempDir(), "release.zip"), func(Phase, int, string) {})
	if err != nil || got != fmt.Sprintf("%x", sha256.Sum256(body)) ||
		strings.Join(hosts, ",") != "api.github.com,github.com" {
		t.Fatalf("fallback failed: hosts=%v err=%v", hosts, err)
	}
}

func TestDownloadRejectsInvalidResumeWithoutUsingPartialFile(t *testing.T) {
	content := bytes.Repeat([]byte("x"), 4096)
	for _, tc := range []struct {
		name string
		code int
		cr   string
	}{
		{"range-ignored", http.StatusOK, ""},
		{"wrong-start", http.StatusPartialContent, "bytes 0-4095/4096"},
		{"wrong-total", http.StatusPartialContent, "bytes 512-4095/8192"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: offlineTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return response(http.StatusOK, io.MultiReader(bytes.NewReader(content[:512]), interruptedReader{})), nil
				}
				res := response(tc.code, bytes.NewReader(content))
				res.Header.Set("Content-Range", tc.cr)
				return res, nil
			})}
			_, err := download(context.Background(), client, Asset{
				Name: "release.zip", URL: "https://github.com/fixture/release.zip", Size: int64(len(content)),
			}, filepath.Join(t.TempDir(), "partial"), func(Phase, int, string) {})
			if err == nil || !strings.Contains(err.Error(), "断点续传") || calls != 2 {
				t.Fatalf("invalid resume accepted: calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestChecksumFailureRemovesUpdateWorkDirectory(t *testing.T) {
	root := offlineTestRoot(t)
	archive := makeOfflineZip(t, nil)
	name := AssetName("v1.1.1-offline", "windows", "amd64")
	rel := &Release{TagName: "v1.1.1-offline", Assets: []Asset{
		{Name: name, Size: int64(len(archive)), URL: "https://github.com/fixture/release.zip"},
		{Name: "SHA256SUMS", URL: "https://github.com/fixture/SHA256SUMS"},
	}}
	client := &http.Client{Transport: offlineTransport(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "SHA256SUMS") {
			return response(http.StatusOK, strings.NewReader(strings.Repeat("0", 64)+"  "+name+"\n")), nil
		}
		return response(http.StatusOK, bytes.NewReader(archive)), nil
	})}
	if _, err := PrepareOffline(t.Context(), client, rel, "v1.1.0-offline",
		OfflineInstall{Root: root}, func(Phase, int, string) {}); err == nil {
		t.Fatal("bad digest accepted")
	}
	entries, err := os.ReadDir(filepath.Join(root, ".yunkun-local", "updates"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed update left files behind: %v %v", entries, err)
	}
}
