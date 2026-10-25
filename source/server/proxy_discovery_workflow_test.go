package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"yunkun/console/db"
	"yunkun/console/proxypool"
)

func TestDiscoveryPaginationAndLegacyAuthorization(t *testing.T) {
	s := newProxyFixture(t)
	sourceID, err := s.m.pg.SaveProxyDiscoverySource(0, "quake", "pagination", "port:8080",
		"203.0.113.0/24", "https://check.example/ip", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	items := make([]db.ProxyDiscoveryResult, 1005)
	for i := range items {
		items[i] = db.ProxyDiscoveryResult{Address: fmt.Sprintf("http://203.0.113.1:%d", 10000+i),
			Host: "203.0.113.1", Port: 10000 + i, Protocol: "http"}
	}
	// Simulate old records whose authorization was lost on insertion.
	if err := s.m.pg.UpsertProxyDiscoveryResults(sourceID, items); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Results []db.ProxyDiscoveryResult `json:"results"`
		Total   int                       `json:"total"`
		Page    int                       `json:"page"`
	}
	get := func(page int) {
		t.Helper()
		w := proxyAPICall(t, s, "GET", fmt.Sprintf("/api/proxy-discovery/results?source_id=%d&page=%d&page_size=20", sourceID, page), nil)
		if w.Code != http.StatusOK {
			t.Fatalf("list: %d %s", w.Code, w.Body.String())
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
	}
	get(1)
	if response.Total != 1005 || len(response.Results) != 20 || !response.Results[0].Authorized ||
		response.Results[0].AuthorizationStatus != "allowed" || response.Results[0].State != "pending" {
		t.Fatalf("incorrect first page: %#v", response)
	}
	firstID := response.Results[0].ID
	get(2)
	if len(response.Results) != 20 || response.Results[0].ID == firstID {
		t.Fatal("second page did not advance")
	}
	get(99)
	if response.Page != 51 || len(response.Results) != 5 {
		t.Fatalf("last page was not clamped: %#v", response)
	}
	for _, query := range []string{"page=0", "page_size=101", "page=x", "page_size=-1"} {
		w := proxyAPICall(t, s, "GET", "/api/proxy-discovery/results?"+query, nil)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid pagination accepted: %s", query)
		}
	}
	source, _, _ := s.m.pg.ProxyDiscoverySource(sourceID)
	if err := s.m.pg.UpsertProxyDiscoveryResults(sourceID, authorizeDiscoveryResults(source, items[:1])); err != nil {
		t.Fatal(err)
	}
	stored, _ := s.m.pg.ListProxyDiscoveryResults(sourceID)
	oldest, _ := s.m.pg.ProxyDiscoveryResultsPage(sourceID, 1, 1004)
	persisted, _ := s.m.pg.ProxyDiscoveryResult(oldest[0].ID)
	if len(stored) != 1000 || persisted == nil || !persisted.Authorized {
		t.Fatal("computed authorization was not persisted")
	}
	ignored := proxyAPICall(t, s, "POST", fmt.Sprintf("/api/proxy-discovery/results/%d/ignore", persisted.ID), nil)
	if ignored.Code != http.StatusOK {
		t.Fatalf("old result is not addressable beyond 1000 rows: %s", ignored.Body.String())
	}
	for _, scope := range []string{"", "198.51.100.0/24"} {
		if _, err := s.m.pg.SaveProxyDiscoverySource(sourceID, "quake", "pagination", "port:8080",
			scope, "https://check.example/ip", "", 100); err != nil {
			t.Fatal(err)
		}
		get(1)
		want := "out_of_scope"
		if scope == "" {
			want = "unconfigured"
		}
		if response.Results[0].Authorized || response.Results[0].AuthorizationStatus != want ||
			response.Results[0].State != "pending" {
			t.Fatalf("authorization and verification conflated: %#v", response.Results[0])
		}
	}
}

func TestDiscoveryVerifyPromoteAndConfigurationChange(t *testing.T) {
	s := newProxyFixture(t)
	var requests int
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Host != "check.example" {
			t.Errorf("verification did not use the proxy: %s", r.URL)
		}
		fmt.Fprint(w, "198.51.100.42")
	}))
	defer proxy.Close()
	u, _ := url.Parse(proxy.URL)
	port, _ := strconv.Atoi(u.Port())
	cipher, err := s.proxyPools.vault.Seal(proxypool.Secrets{APIValue: `{"key":"fixture"}`})
	if err != nil {
		t.Fatal(err)
	}
	sourceID, err := s.m.pg.SaveProxyDiscoverySource(0, "quake", "workflow", "fixture", "127.0.0.1",
		"http://check.example/ip", cipher, 100)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.m.pg.UpsertProxyDiscoveryResults(sourceID, []db.ProxyDiscoveryResult{{
		Address: proxy.URL, Host: u.Hostname(), Port: port, Protocol: "http",
	}}); err != nil {
		t.Fatal(err)
	}
	results, _ := s.m.pg.ListProxyDiscoveryResults(sourceID)
	id := results[0].ID
	verify := func(want int) {
		t.Helper()
		w := proxyAPICall(t, s, "POST", fmt.Sprintf("/api/proxy-discovery/results/%d/verify", id), nil)
		if w.Code != want {
			t.Fatalf("verify: %d %s", w.Code, w.Body.String())
		}
	}
	verify(http.StatusOK)
	if requests != 1 {
		t.Fatal("expected one request to the local proxy")
	}
	poolResponse := proxyAPICall(t, s, "POST", "/api/proxy-pools", map[string]any{
		"config": map[string]any{"name": "empty discovery destination", "kind": "fixed", "enabled": true},
	})
	if poolResponse.Code != http.StatusOK {
		t.Fatalf("empty pool: %s", poolResponse.Body.String())
	}
	var pool struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(poolResponse.Body.Bytes(), &pool)
	approve := func(want int) {
		t.Helper()
		w := proxyAPICall(t, s, "POST", fmt.Sprintf("/api/proxy-discovery/results/%d/approve", id),
			map[string]any{"pool_id": pool.ID, "target": "production"})
		if w.Code != want {
			t.Fatalf("approve: %d %s", w.Code, w.Body.String())
		}
	}
	// Revoking the range invalidates old verification; it must not allow promotion.
	if _, err := s.m.pg.SaveProxyDiscoverySource(sourceID, "quake", "workflow", "fixture", "",
		"http://check.example/ip", cipher, 100); err != nil {
		t.Fatal(err)
	}
	verify(http.StatusBadRequest)
	approve(http.StatusForbidden)
	if requests != 1 {
		t.Fatal("missing scope caused a network request")
	}
	if _, err := s.m.pg.SaveProxyDiscoverySource(sourceID, "quake", "workflow", "fixture", "127.0.0.1",
		"file://invalid", cipher, 100); err != nil {
		t.Fatal(err)
	}
	verify(http.StatusBadRequest)
	if requests != 1 {
		t.Fatal("invalid verification URL caused a network request")
	}
	if _, err := s.m.pg.SaveProxyDiscoverySource(sourceID, "quake", "workflow", "fixture", "127.0.0.1",
		"http://check.example/ip", cipher, 100); err != nil {
		t.Fatal(err)
	}
	approve(http.StatusConflict)
	verify(http.StatusOK)
	approve(http.StatusOK)
	nodes, err := s.m.pg.ProxyNodes(pool.ID)
	if err != nil || len(nodes) != 1 || !nodes[0].Enabled || nodes[0].Origin != "quake" ||
		nodes[0].VerifiedIP != "198.51.100.42" || nodes[0].LastSuccessAt == nil {
		t.Fatalf("verification evidence lost during promotion: %v %#v", err, nodes)
	}
	summary, err := s.m.pg.ProxyNodeSummary(pool.ID, 0)
	if err != nil || summary.Available != 1 {
		t.Fatalf("overview does not show available node: %#v %v", summary, err)
	}
	verify(http.StatusConflict)
}
