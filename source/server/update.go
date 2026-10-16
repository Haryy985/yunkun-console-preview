package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const releaseAPI = "https://api.github.com/repos/Haryy985/yunkun-console-preview/releases/latest"
const releaseBase = "https://github.com/Haryy985/yunkun-console-preview/releases"

var offlineVersion = regexp.MustCompile(`^v?([0-9]+)\.([0-9]+)\.([0-9]+)-offline$`)
var releaseVersion = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+-offline$`)
var sha256Digest = regexp.MustCompile(`^sha256:[a-fA-F0-9]{64}$`)

type updateStatus struct {
	Status         string `json:"status"`
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version,omitempty"`
	ReleaseURL     string `json:"release_url,omitempty"`
	PackageURL     string `json:"package_url,omitempty"`
	PackageSHA256  string `json:"package_sha256,omitempty"`
	CheckedAt      string `json:"checked_at,omitempty"`
}

type releaseFeed struct {
	Tag        string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name   string `json:"name"`
		URL    string `json:"browser_download_url"`
		Digest string `json:"digest"`
	} `json:"assets"`
}

type updateChecker struct {
	mu      sync.Mutex
	expires time.Time
	cached  updateStatus
	client  *http.Client
	apiURL  string
}

var updates = &updateChecker{
	client: &http.Client{Timeout: 6 * time.Second},
	apiURL: releaseAPI,
}

func compareOfflineVersions(current, latest string) (int, bool) {
	a, b := offlineVersion.FindStringSubmatch(current), offlineVersion.FindStringSubmatch(latest)
	if a == nil || b == nil {
		return 0, false
	}
	for i := 1; i <= 3; i++ {
		x, xerr := strconv.ParseUint(a[i], 10, 32)
		y, yerr := strconv.ParseUint(b[i], 10, 32)
		if xerr != nil || yerr != nil {
			return 0, false
		}
		if x < y {
			return -1, true
		}
		if x > y {
			return 1, true
		}
	}
	return 0, true
}

func trustedRelease(feed releaseFeed, current string) (updateStatus, error) {
	result := updateStatus{Status: "unavailable", CurrentVersion: current}
	if feed.Draft || feed.Prerelease || !releaseVersion.MatchString(feed.Tag) {
		return result, fmt.Errorf("not a stable release")
	}
	cmp, valid := compareOfflineVersions(current, feed.Tag)
	if !valid {
		return result, fmt.Errorf("unsupported release tag")
	}
	result.LatestVersion = feed.Tag
	result.ReleaseURL = releaseBase + "/tag/" + feed.Tag
	result.Status = "current"
	if cmp >= 0 {
		return result, nil
	}
	packageName := "yunkun-" + feed.Tag + "-windows-x64.zip"
	for _, asset := range feed.Assets {
		expected := releaseBase + "/download/" + feed.Tag + "/" + packageName
		parsed, err := url.Parse(asset.URL)
		if asset.Name == packageName && err == nil && parsed.String() == expected && sha256Digest.MatchString(asset.Digest) {
			result.Status = "available"
			result.PackageURL = expected
			result.PackageSHA256 = strings.ToLower(strings.TrimPrefix(asset.Digest, "sha256:"))
			return result, nil
		}
	}
	return updateStatus{Status: "unavailable", CurrentVersion: current}, fmt.Errorf("trusted offline asset or digest missing")
}

func (c *updateChecker) check(r *http.Request, current string) updateStatus {
	if !offlineVersion.MatchString(current) {
		return updateStatus{Status: "unsupported", CurrentVersion: current}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !r.URL.Query().Has("refresh") && c.cached.CurrentVersion == current && time.Now().Before(c.expires) {
		return c.cached
	}
	result := updateStatus{Status: "unavailable", CurrentVersion: current}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, c.apiURL, nil)
	if err == nil {
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "Yunkun-Console-Update-Check")
		var resp *http.Response
		resp, err = c.client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				err = fmt.Errorf("release feed HTTP %d", resp.StatusCode)
			} else {
				var feed releaseFeed
				err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&feed)
				if err == nil {
					result, err = trustedRelease(feed, current)
				}
			}
		}
	}
	ttl := 5 * time.Minute
	if err == nil {
		ttl = 30 * time.Minute
	}
	c.cached, c.expires = result, time.Now().Add(ttl)
	result.CheckedAt = time.Now().UTC().Format(time.RFC3339)
	c.cached = result
	return result
}

func updateCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, updates.check(r, BuildVersion))
}

func updateUnavailable(w http.ResponseWriter, _ *http.Request) {
	writeErr(w, http.StatusServiceUnavailable, "自动替换程序已停用；请按离线更新手册备份并迁移安装")
}
