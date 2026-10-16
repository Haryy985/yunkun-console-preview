package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOfflineVersionComparison(t *testing.T) {
	for _, tc := range []struct {
		current, latest string
		want            int
		valid           bool
	}{
		{"v1.0.1-offline", "v1.0.2-offline", -1, true},
		{"1.0.1-offline", "v1.0.2-offline", -1, true},
		{"v1.9.9-offline", "v1.10.0-offline", -1, true},
		{"v2.0.0-offline", "v1.10.0-offline", 1, true},
		{"v1.0.1-offline", "v1.0.1-offline", 0, true},
		{"dev", "v1.0.1-offline", 0, false},
		{"v1.0.0", "v1.0.1-offline", 0, false},
		{"v1.0.1-offline", "v1.0.2-offline-beta", 0, false},
	} {
		got, ok := compareOfflineVersions(tc.current, tc.latest)
		if got != tc.want || ok != tc.valid {
			t.Errorf("%q vs %q = %d, %v", tc.current, tc.latest, got, ok)
		}
	}
}

func TestUpdateCheckTrustAndCache(t *testing.T) {
	calls := 0
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Write([]byte(`{"tag_name":"v1.0.2-offline","assets":[{"name":"yunkun-v1.0.2-offline-windows-x64.zip","browser_download_url":"https://github.com/Haryy985/yunkun-console-preview/releases/download/v1.0.2-offline/yunkun-v1.0.2-offline-windows-x64.zip","digest":"sha256:` + strings.Repeat("a", 64) + `"}]}`))
	}))
	defer feed.Close()
	checker := &updateChecker{client: feed.Client(), apiURL: feed.URL}
	req := httptest.NewRequest(http.MethodGet, "/api/update/check", nil)
	got := checker.check(req, "v1.0.1-offline")
	if got.Status != "available" || got.PackageSHA256 != strings.Repeat("a", 64) {
		t.Fatalf("valid release = %+v", got)
	}
	checker.check(req, "v1.0.1-offline")
	if calls != 1 {
		t.Fatalf("cache missed: %d requests", calls)
	}
	checker.check(httptest.NewRequest(http.MethodGet, "/api/update/check?refresh=1", nil), "v1.0.1-offline")
	if calls != 2 {
		t.Fatalf("manual refresh did not fetch release: %d requests", calls)
	}
	if checker.check(req, "dev").Status != "unsupported" {
		t.Fatal("dev builds must not offer an update")
	}
	checker.expires = time.Time{}
	got = checker.check(req, "v1.0.2-offline")
	if got.Status != "current" || calls != 3 {
		t.Fatalf("up-to-date result = %+v, calls = %d", got, calls)
	}
}

func TestUpdateCheckUntrustedOrOffline(t *testing.T) {
	for _, payload := range []string{
		`{"tag_name":"v1.0.2-offline","prerelease":true}`,
		`{"tag_name":"v1.0.2-offline","assets":[{"name":"yunkun-v1.0.2-offline-windows-x64.zip","browser_download_url":"https://evil.example/file.zip","digest":"sha256:` + strings.Repeat("a", 64) + `"}]}`,
		`{"tag_name":"v1.0.2-offline","assets":[{"name":"yunkun-v1.0.2-offline-windows-x64.zip","browser_download_url":"https://github.com/Haryy985/yunkun-console-preview/releases/download/v1.0.2-offline/yunkun-v1.0.2-offline-windows-x64.zip","digest":""}]}`,
	} {
		feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(payload)) }))
		checker := &updateChecker{client: feed.Client(), apiURL: feed.URL}
		if got := checker.check(httptest.NewRequest("GET", "/", nil), "v1.0.1-offline"); got.Status != "unavailable" || got.PackageURL != "" {
			t.Fatalf("untrusted release = %+v", got)
		}
		feed.Close()
	}
	checker := &updateChecker{client: &http.Client{Timeout: 100 * time.Millisecond}, apiURL: "http://127.0.0.1:1"}
	if got := checker.check(httptest.NewRequest("GET", "/", nil), "v1.0.1-offline"); got.Status != "unavailable" {
		t.Fatalf("offline check = %+v", got)
	}
	recorder := httptest.NewRecorder()
	updateUnavailable(recorder, httptest.NewRequest("POST", "/api/update/apply", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("automatic update must stay disabled: %d", recorder.Code)
	}
}
