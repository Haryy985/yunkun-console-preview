package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"yunkun/console/db"
	"yunkun/console/proxypool"
)

func TestDiscoveryAdmissionRechecksPoolConfiguration(t *testing.T) {
	for _, test := range []struct {
		name, poolUser, poolEndpoint string
		reject                       bool
		wantRequests                 int32
		appAuth                      bool
	}{
		{"matching proof", "source-user", "", false, 2, false},
		{"different valid credentials", "pool-user", "", false, 2, false},
		{"different rejected credentials", "rejected-user", "", true, 2, false},
		{"different endpoint", "source-user", "http://pool-check.invalid/ip", false, 2, false},
		{"xiaoxiang application credentials", "pool-user", "", false, 2, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := newProxyFixture(t)
			var requests atomic.Int32
			echo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				auth := r.Header.Get("Proxy-Authorization")
				sourceAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("source-user:fixture-secret"))
				poolAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("pool-user:fixture-secret"))
				if auth != sourceAuth && auth != poolAuth {
					w.WriteHeader(http.StatusProxyAuthRequired)
					return
				}
				if r.URL.Host != "source-check.invalid" && r.URL.Host != "pool-check.invalid" {
					t.Errorf("verification bypassed the selected target: %s", r.URL)
				}
				ip := "198.51.100.10"
				if auth == poolAuth || r.URL.Host == "pool-check.invalid" {
					ip = "198.51.100.20"
				}
				fmt.Fprint(w, ip)
			}))
			defer echo.Close()
			u, _ := url.Parse(echo.URL)
			port, _ := strconv.Atoi(u.Port())
			sealed, err := s.proxyPools.vault.Seal(proxypool.Secrets{
				APIValue: `{"proxy_username":"source-user","proxy_password":"fixture-secret"}`,
			})
			if err != nil {
				t.Fatal(err)
			}
			sourceID, err := s.m.pg.SaveProxyDiscoverySource(0, "quake", "recheck fixture", "fixture",
				"127.0.0.1", "http://source-check.invalid/ip", sealed, 100)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.m.pg.UpsertProxyDiscoveryResults(sourceID, []db.ProxyDiscoveryResult{{
				Address: echo.URL, Protocol: "http", Host: u.Hostname(), Port: port,
			}}); err != nil {
				t.Fatal(err)
			}
			results, err := s.m.pg.ListProxyDiscoveryResults(sourceID)
			if err != nil || len(results) != 1 {
				t.Fatalf("candidate: %v %#v", err, results)
			}
			id := results[0].ID
			verified := proxyAPICall(t, s, "POST", fmt.Sprintf("/api/proxy-discovery/results/%d/verify", id), nil)
			if verified.Code != http.StatusOK {
				t.Fatalf("verify: %d %s", verified.Code, verified.Body.String())
			}
			poolRequest := saveProxyPoolRequest{
				Config: proxypool.Config{Name: "destination", Kind: "fixed", Enabled: true,
					VerifyURL: test.poolEndpoint},
				Secrets: &proxypool.Secrets{ProxyUsername: test.poolUser, ProxyPassword: "fixture-secret"},
			}
			if test.appAuth {
				poolRequest.Config.Kind = "xiaoxiang"
				poolRequest.Secrets = &proxypool.Secrets{AppKey: test.poolUser, AppSecret: "fixture-secret"}
			}
			created := proxyAPICall(t, s, "POST", "/api/proxy-pools", poolRequest)
			var pool struct {
				ID int64 `json:"id"`
			}
			if created.Code != http.StatusOK || json.Unmarshal(created.Body.Bytes(), &pool) != nil {
				t.Fatalf("pool: %d %s", created.Code, created.Body.String())
			}
			admitted := proxyAPICall(t, s, "POST", fmt.Sprintf("/api/proxy-discovery/results/%d/approve", id),
				map[string]any{"pool_id": pool.ID, "target": "production"})
			candidate, err := s.m.pg.ProxyDiscoveryResult(id)
			if err != nil || candidate == nil || candidate.VerifiedIP != "198.51.100.10" {
				t.Fatalf("source proof was lost: %v %#v", err, candidate)
			}
			nodes, err := s.m.pg.ProxyNodes(pool.ID)
			if err != nil {
				t.Fatal(err)
			}
			if requests.Load() != test.wantRequests {
				t.Errorf("requests: got %d want %d", requests.Load(), test.wantRequests)
			}
			if test.reject {
				if admitted.Code != http.StatusBadGateway || len(nodes) != 0 || candidate.State != "verified" ||
					!strings.Contains(admitted.Body.String(), "[proxy_verify:handshake:auth]") ||
					strings.Contains(admitted.Body.String(), "fixture-secret") {
					t.Fatalf("failed recheck admitted a node or lost proof: %d %s %#v", admitted.Code, admitted.Body.String(), nodes)
				}
				return
			}
			wantIP := "198.51.100.10"
			if test.poolUser == "pool-user" || test.poolEndpoint != "" {
				wantIP = "198.51.100.20"
			}
			if admitted.Code != http.StatusOK || candidate.State != "approved" || len(nodes) != 1 ||
				!nodes[0].Enabled || nodes[0].VerifiedIP != wantIP || nodes[0].LastSuccessAt == nil {
				t.Fatalf("admission proof: %d %s %#v", admitted.Code, admitted.Body.String(), nodes)
			}
			task, err := s.m.pg.CreateTaskWithOptions("proxy fixture", "local test",
				db.TaskCreateOptions{ProxyPoolID: &pool.ID, NetworkMode: "configured"})
			if err != nil {
				t.Fatal(err)
			}
			route, err := s.resolveProxyRoute(context.Background(), task.ID)
			if err != nil || route.Node.ID != nodes[0].ID || route.Upstream.User.Username() != test.poolUser {
				t.Fatalf("task cannot use admitted route: %v", err)
			}
			ip, err := proxypool.Verify(context.Background(), route.Upstream, "http://source-check.invalid/ip")
			if err != nil || ip == "" {
				t.Fatalf("task route does not work: %q %v", ip, err)
			}
		})
	}
}

func TestDiscoveryBatchVerificationPersistsSafeFailures(t *testing.T) {
	s := newProxyFixture(t)
	echo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusProxyAuthRequired)
		fmt.Fprint(w, "private-response-do-not-store")
	}))
	defer echo.Close()
	u, _ := url.Parse(echo.URL)
	port, _ := strconv.Atoi(u.Port())
	sealed, err := s.proxyPools.vault.Seal(proxypool.Secrets{APIValue: `{"key":"fixture"}`})
	if err != nil {
		t.Fatal(err)
	}
	sourceID, err := s.m.pg.SaveProxyDiscoverySource(0, "quake", "batch failure fixture", "fixture",
		"127.0.0.1", "http://echo.invalid/ip?key=private-target-key", sealed, 100)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.m.pg.UpsertProxyDiscoveryResults(sourceID, []db.ProxyDiscoveryResult{{
		Address: echo.URL, Protocol: "http", Host: u.Hostname(), Port: port,
	}}); err != nil {
		t.Fatal(err)
	}
	results, err := s.m.pg.ListProxyDiscoveryResults(sourceID)
	if err != nil || len(results) != 1 {
		t.Fatalf("candidate: %v %#v", err, results)
	}
	id := results[0].ID
	response := proxyAPICall(t, s, "POST", "/api/proxy-discovery/results/batch",
		map[string]any{"ids": []int64{id}, "action": "verify"})
	var batch struct {
		Items []struct {
			OK     bool   `json:"ok"`
			Status int    `json:"status"`
			Error  string `json:"error"`
		} `json:"items"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &batch) != nil ||
		len(batch.Items) != 1 || batch.Items[0].OK || batch.Items[0].Status != http.StatusBadGateway ||
		!strings.Contains(batch.Items[0].Error, "[proxy_verify:handshake:auth]") {
		t.Fatalf("batch failure: %d %s", response.Code, response.Body.String())
	}
	candidate, err := s.m.pg.ProxyDiscoveryResult(id)
	if err != nil || candidate == nil || candidate.State != "invalid" || candidate.VerifiedIP != "" ||
		candidate.LastError != batch.Items[0].Error || strings.Contains(candidate.LastError, "private-") {
		t.Fatalf("unsafe persisted failure: %v %#v", err, candidate)
	}
}
