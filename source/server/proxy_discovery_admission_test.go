package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"yunkun/console/db"
	"yunkun/console/proxypool"
)

func TestProxyDiscoveryAdmissionStateErrors(t *testing.T) {
	s := newProxyFixture(t)
	cipher, err := s.proxyPools.vault.Seal(proxypool.Secrets{
		APIValue: `{"proxy_username":"test-user","proxy_password":"never-echo-this-password"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	sourceID, err := s.m.pg.SaveProxyDiscoverySource(
		0, "quake", "admission fixture", "port:8080", "203.0.113.0/24",
		"http://127.0.0.1:9/ip", cipher, 25,
	)
	if err != nil {
		t.Fatal(err)
	}
	poolID := createTestPool(t, s, "http://127.0.0.1:9997")
	var candidates []db.ProxyDiscoveryResult
	for i := 10; i <= 15; i++ {
		host := fmt.Sprintf("203.0.113.%d", i)
		candidates = append(candidates, db.ProxyDiscoveryResult{
			Address: "http://" + host + ":8080", Protocol: "http", Host: host, Port: 8080,
			Metadata: map[string]any{},
		})
	}
	if err := s.m.pg.UpsertProxyDiscoveryResults(sourceID, candidates); err != nil {
		t.Fatal(err)
	}
	results, err := s.m.pg.ListProxyDiscoveryResults(sourceID)
	if err != nil || len(results) != len(candidates) {
		t.Fatalf("results: %v %#v", err, results)
	}
	ids := map[string]int64{}
	for _, result := range results {
		ids[result.Host] = result.ID
	}
	for _, change := range []struct {
		host, state, ip string
		authorized      bool
	}{
		{"203.0.113.10", "pending", "", true},
		{"203.0.113.11", "invalid", "", true},
		{"203.0.113.12", "verified", "", true},
		{"203.0.113.13", "verified", "198.51.100.10", false},
		{"203.0.113.14", "verified", "198.51.100.10", true},
		{"203.0.113.15", "verified", "198.51.100.10", true},
	} {
		if err := s.m.pg.UpdateProxyDiscoveryResult(ids[change.host], change.state, change.authorized, change.ip, nil, ""); err != nil {
			t.Fatal(err)
		}
	}
	pendingID := ids["203.0.113.10"]
	pending := proxyAPICall(t, s, "POST", fmt.Sprintf("/api/proxy-discovery/results/%d/approve", pendingID),
		map[string]any{"pool_id": poolID, "target": "production"})
	if pending.Code != http.StatusConflict || !strings.Contains(pending.Body.String(), "尚未验证通过") {
		t.Fatalf("pending admission: %d %s", pending.Code, pending.Body.String())
	}
	validID := ids["203.0.113.14"]
	batch := proxyAPICall(t, s, "POST", "/api/proxy-discovery/results/batch", map[string]any{
		"ids":     []int64{pendingID, ids["203.0.113.11"], ids["203.0.113.12"], ids["203.0.113.13"], validID},
		"pool_id": poolID, "action": "approve", "target": "production",
	})
	var response struct {
		Items []struct {
			ID     int64  `json:"id"`
			OK     bool   `json:"ok"`
			Status int    `json:"status"`
			Error  string `json:"error"`
		} `json:"items"`
	}
	if batch.Code != http.StatusOK || json.Unmarshal(batch.Body.Bytes(), &response) != nil || len(response.Items) != 5 {
		t.Fatalf("batch admission: %d %s", batch.Code, batch.Body.String())
	}
	for i, item := range response.Items {
		switch {
		case i < 3:
			if item.OK || item.Status != http.StatusConflict || !strings.Contains(item.Error, "尚未验证通过") {
				t.Errorf("unverified item: %+v", item)
			}
		case i == 3:
			if item.OK || item.Status != http.StatusForbidden || !strings.Contains(item.Error, "授权范围") {
				t.Errorf("unauthorized item: %+v", item)
			}
		default:
			if !item.OK || item.Status != http.StatusOK {
				t.Errorf("verified item: %+v", item)
			}
		}
		if strings.Contains(item.Error, "sql:") {
			t.Errorf("database error leaked: %+v", item)
		}
	}
	nodes, err := s.m.pg.ProxyNodes(poolID)
	if err != nil || len(nodes) != 2 {
		t.Fatalf("only the verified candidate should be added: %v %#v", err, nodes)
	}
	for _, node := range nodes {
		if node.Origin == "quake" && (!node.Enabled || node.VerifiedIP != "198.51.100.10") {
			t.Errorf("enabled admission lost verification: %+v", node)
		}
	}
	for _, test := range []struct {
		name     string
		id, pool int64
		status   int
		message  string
	}{
		{"already admitted", validID, poolID, http.StatusConflict, "已入池"},
		{"deleted pool", ids["203.0.113.15"], poolID + 1000000, http.StatusNotFound, "目标代理池不存在"},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := proxyAPICall(t, s, "POST", fmt.Sprintf("/api/proxy-discovery/results/%d/approve", test.id),
				map[string]any{"pool_id": test.pool, "target": "production"})
			if w.Code != test.status || !strings.Contains(w.Body.String(), test.message) {
				t.Fatalf("admission: %d %s", w.Code, w.Body.String())
			}
		})
	}
	unchanged, err := s.m.pg.ProxyDiscoveryResult(pendingID)
	if err != nil || unchanged == nil || unchanged.State != "pending" {
		t.Fatalf("rejected pending candidate changed: %v %#v", err, unchanged)
	}
}
