package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"yunkun/console/db"
	"yunkun/console/proxypool"
)

func TestNodeVerificationTargetDefaults(t *testing.T) {
	for _, test := range []struct{ configured, mode, endpoint, wantMode string }{
		{"", "", proxypool.BaiduVerifyURL, proxypool.VerifyConnectivity},
		{"www.baidu.com", "", proxypool.BaiduVerifyURL, proxypool.VerifyConnectivity},
		{"http://www.baidu.com/", "", proxypool.BaiduVerifyURL, proxypool.VerifyConnectivity},
		{"https://echo.invalid/", "", "https://echo.invalid/", proxypool.VerifyExitIP},
		{"https://echo.invalid/", proxypool.VerifyConnectivity, proxypool.BaiduVerifyURL, proxypool.VerifyConnectivity},
		{"", proxypool.VerifyExitIP, proxypool.DefaultIPVerifyURL, proxypool.VerifyExitIP},
		{proxypool.BaiduVerifyURL, proxypool.VerifyExitIP, proxypool.DefaultIPVerifyURL, proxypool.VerifyExitIP},
	} {
		endpoint, mode, err := nodeVerificationTarget(test.configured, test.mode)
		if err != nil || endpoint != test.endpoint || mode != test.wantMode {
			t.Fatalf("verification selection: %q %q %v", endpoint, mode, err)
		}
	}
	if _, _, err := nodeVerificationTarget("", "invalid"); err == nil {
		t.Fatal("invalid mode accepted")
	}
}

func TestProxyNodeVerificationPreservesDisabledStateAndIPEvidence(t *testing.T) {
	s := newProxyFixture(t)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			if r.Host != "www.baidu.com:443" {
				t.Errorf("unexpected connectivity target: %s", r.Host)
			}
			w.WriteHeader(http.StatusProxyAuthRequired)
			return
		}
		_, _ = io.WriteString(w, `{"ip":"198.51.100.42"}`)
	}))
	defer proxy.Close()
	id := createTestPool(t, s, proxy.URL)
	p, _ := s.m.pg.ProxyPool(id)
	p.Config.VerifyURL = "http://echo.invalid/ip"
	if _, err := s.m.pg.SaveProxyPool(id, p.Config, p.Credentials, false); err != nil {
		t.Fatal(err)
	}
	nodes, _ := s.m.pg.ProxyNodes(id)
	nodeID := nodes[0].ID
	if err := s.m.pg.SetProxyNodeEnabled(id, nodeID, false); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/proxy-pools/%d/nodes/%d/verify", id, nodeID)
	response := proxyAPICall(t, s, "POST", path, map[string]string{"mode": proxypool.VerifyExitIP})
	if response.Code != 200 {
		t.Fatalf("IP verification: %d %s", response.Code, response.Body)
	}
	before, _ := s.m.pg.ProxyNode(id, nodeID)
	if before.Enabled || before.VerifiedIP != "198.51.100.42" || before.VerifiedAt == nil || before.LastHTTPStatus != 200 {
		t.Fatalf("incorrect IP state: %+v", before)
	}
	result := proxypool.VerificationResult{Mode: proxypool.VerifyConnectivity, Target: "https://www.baidu.com", HTTPStatus: 200}
	if err := s.m.pg.RecordProxyNodeVerification(id, nodeID, result, "", 123); err != nil {
		t.Fatal(err)
	}
	after, _ := s.m.pg.ProxyNode(id, nodeID)
	if after.Enabled || after.VerifiedIP != before.VerifiedIP || !after.VerifiedAt.Equal(*before.VerifiedAt) ||
		after.LastVerifyMode != proxypool.VerifyConnectivity || after.SuccessCount != 2 || *after.LatencyMS != 123 {
		t.Fatalf("connectivity overwrote IP evidence or enabled the node: %+v", after)
	}
	failed := proxyAPICall(t, s, "POST", path, map[string]string{"mode": proxypool.VerifyConnectivity})
	if failed.Code != 502 {
		t.Fatalf("authentication failure accepted: %d", failed.Code)
	}
	after, _ = s.m.pg.ProxyNode(id, nodeID)
	if after.Enabled || after.VerifiedIP != before.VerifiedIP || after.LastError == "" || after.FailureCount != 1 {
		t.Fatal("failed recheck lost prior evidence or changed enablement")
	}
	if response := proxyAPICall(t, s, "POST", path, map[string]string{"mode": "invalid"}); response.Code != 400 {
		t.Fatal("invalid mode accepted")
	}
}

func TestProxyNodesPaginationAndVerificationBeyond500(t *testing.T) {
	s := newProxyFixture(t)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "198.51.100.42") }))
	defer proxy.Close()
	id := createTestPool(t, s, proxy.URL)
	p, _ := s.m.pg.ProxyPool(id)
	p.Config.VerifyURL = "http://echo.invalid/ip"
	if _, err := s.m.pg.SaveProxyPool(id, p.Config, p.Credentials, false); err != nil {
		t.Fatal(err)
	}
	original, _ := s.m.pg.ProxyNodes(id)
	_, err := s.m.pg.Exec(`INSERT INTO proxy_nodes(pool_id,address) SELECT $1,'socks5://192.0.2.1:'||(20000+n)::text FROM generate_series(1,550) n`, id)
	if err != nil {
		t.Fatal(err)
	}
	response := proxyAPICall(t, s, "GET", fmt.Sprintf("/api/proxy-pools/%d/nodes?page=6&page_size=100", id), nil)
	var page struct {
		Nodes []proxypool.Node `json:"nodes"`
		Total int              `json:"total"`
	}
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &page) != nil || page.Total != 551 || len(page.Nodes) != 51 {
		t.Fatalf("pagination truncated: %d %s", response.Code, response.Body)
	}
	filtered, count, err := s.m.pg.ProxyNodesPage(id, 1, 20, db.ProxyNodeFilter{Query: "192.0.2.1", Protocol: "socks5", Status: "pending"})
	if err != nil || count != 550 || len(filtered) != 20 {
		t.Fatalf("filter mismatch: %d %d %v", count, len(filtered), err)
	}
	response = proxyAPICall(t, s, "POST", fmt.Sprintf("/api/proxy-pools/%d/nodes/%d/verify", id, original[0].ID), nil)
	if response.Code != 200 {
		t.Fatalf("node outside first 500 was not verified: %d %s", response.Code, response.Body)
	}
	for _, query := range []string{"page=0", "page=1&page_size=999", "page=1&status=unknown", "page=1&sort=unknown"} {
		if response := proxyAPICall(t, s, "GET", fmt.Sprintf("/api/proxy-pools/%d/nodes?%s", id, query), nil); response.Code != 400 {
			t.Fatalf("invalid query accepted: %s", query)
		}
	}
	if _, err := s.m.pg.Exec(`UPDATE proxy_nodes SET expires_at=now()+interval '30 minutes' WHERE id=$1`, original[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.m.pg.Exec(`UPDATE proxy_nodes SET last_error='fixture failure' WHERE id=$1`, filtered[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.m.pg.SetProxyNodeEnabled(id, filtered[1].ID, false); err != nil {
		t.Fatal(err)
	}
	summary, err := s.m.pg.ProxyNodeSummary(id, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for status, want := range map[string]int{"available": summary.Available, "pending": summary.Pending,
		"expiring": summary.Expiring, "error": summary.Error, "disabled": summary.Disabled} {
		_, total, err := s.m.pg.ProxyNodesPage(id, 1, 20, db.ProxyNodeFilter{Status: status, WarningSeconds: 3600})
		if err != nil || total != want {
			t.Fatalf("%s filter disagrees with summary: %d != %d (%v)", status, total, want, err)
		}
	}
}

func TestProxyNodeVerificationCancellationAndDuplicate(t *testing.T) {
	s := newProxyFixture(t)
	entered := make(chan struct{})
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
	}))
	defer proxy.Close()
	id := createTestPool(t, s, proxy.URL)
	p, _ := s.m.pg.ProxyPool(id)
	p.Config.VerifyURL = "http://echo.invalid/ip"
	if _, err := s.m.pg.SaveProxyPool(id, p.Config, p.Credentials, false); err != nil {
		t.Fatal(err)
	}
	nodes, _ := s.m.pg.ProxyNodes(id)
	nodeID := nodes[0].ID
	path := fmt.Sprintf("/api/proxy-pools/%d/nodes/%d/verify", id, nodeID)
	token, err := signJWT(s.jwtKey)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest("POST", path, nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+token)
	done := make(chan struct{})
	go func() { s.Handler().ServeHTTP(httptest.NewRecorder(), req); close(done) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("verification never reached fixture")
	}
	if response := proxyAPICall(t, s, "POST", path, nil); response.Code != 409 {
		t.Fatalf("duplicate verification accepted: %d", response.Code)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation did not stop request")
	}
	node, err := s.m.pg.ProxyNode(id, nodeID)
	if err != nil || node.LastCheckedAt != nil || node.FailureCount != 0 || node.LastError != "" {
		t.Fatalf("cancellation changed node health: %+v %v", node, err)
	}
	if len(s.proxyPools.verifySlots) != 0 || len(s.proxyPools.verifying) != 0 {
		t.Fatal("verification concurrency slot leaked")
	}
}
