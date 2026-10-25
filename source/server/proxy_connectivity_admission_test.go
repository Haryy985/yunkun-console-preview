package server

import (
	"context"
	"testing"
	"time"

	"yunkun/console/db"
	"yunkun/console/proxypool"
)

// Network verification is exercised by proxypool's controlled proxy fixtures.
// This test covers persistence, promotion, SQL lease eligibility and routing.
func TestConnectivityCandidateAdmissionAndLeaseWithoutIPEvidence(t *testing.T) {
	s := newProxyFixture(t)
	poolID := createTestPool(t, s, "http://127.0.0.1:9997")
	existing, err := s.m.pg.ProxyNodes(poolID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.m.pg.SetProxyNodeEnabled(poolID, existing[0].ID, false); err != nil {
		t.Fatal(err)
	}
	_, ids, _ := discoveryVerifyFixture(t, s, []string{"http://127.0.0.1:9998"})
	id := ids[0]
	if err := s.m.pg.UpdateProxyDiscoveryResult(id, "verifying", true, "", nil, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.m.pg.CompleteProxyDiscoveryVerification(id, "verified", true, "", 15, "",
		proxypool.VerifyConnectivity, "https://www.baidu.com", "success", "", "", 200); err != nil {
		t.Fatal(err)
	}
	candidate, err := s.m.pg.ProxyDiscoveryResult(id)
	if err != nil || !candidate.HasSuccessfulVerification() || candidate.VerifiedIP != "" || candidate.VerifiedAt != nil {
		t.Fatalf("connectivity fabricated exit IP evidence: %+v %v", candidate, err)
	}
	nodeID, err := s.m.pg.PromoteProxyDiscoveryResult(id, poolID, "quake", true, &db.ProxyDiscoveryAdmissionProof{
		Mode: proxypool.VerifyConnectivity, Target: "https://www.baidu.com", HTTPStatus: 200,
		CheckedAt: time.Now().UTC(), LatencyMS: 16,
	})
	if err != nil {
		t.Fatal(err)
	}
	node, err := s.m.pg.ProxyNode(poolID, nodeID)
	if err != nil || node.VerifiedIP != "" || node.VerifiedAt != nil || !node.HasFreshVerification(time.Now()) {
		t.Fatalf("node evidence: %+v %v", node, err)
	}
	for _, explicit := range []bool{false, true} {
		options := db.TaskCreateOptions{ProxyPoolID: &poolID, NetworkMode: "configured"}
		if explicit {
			options.ProxyNodeID = &nodeID
		}
		task, err := s.m.pg.CreateTaskWithOptions("connectivity route fixture", "no external requests", options)
		if err != nil {
			t.Fatal(err)
		}
		route, err := s.resolveProxyRoute(context.Background(), task.ID)
		if err != nil || route.Node.ID != nodeID {
			t.Fatalf("connectivity route rejected: %+v %v", route, err)
		}
	}
	if _, err := s.m.pg.Exec(`UPDATE proxy_nodes SET last_success_at=now()-interval '16 minutes' WHERE id=$1`, nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.m.pg.CreateTaskWithOptions("expired route", "no requests",
		db.TaskCreateOptions{ProxyPoolID: &poolID, NetworkMode: "configured"}); err == nil {
		t.Fatal("expired connectivity evidence received a task lease")
	}
}

func TestDiscoverySourceBaiduAndProviderDefaults(t *testing.T) {
	for _, provider := range []string{"fofa", "quake"} {
		source := &db.ProxyDiscoverySource{Provider: provider}
		normalizeProxyDiscoverySource(source)
		if source.Query != discoveryDefaultQuery(provider) || source.FetchLimit != 500 ||
			source.VerifyURL != defaultProxyDiscoveryVerifyURL {
			t.Fatalf("legacy defaults: %+v", source)
		}
		source.Query = "custom query unchanged"
		normalizeProxyDiscoverySource(source)
		if source.Query != "custom query unchanged" {
			t.Fatal("custom query was overwritten")
		}
	}
	s := newProxyFixture(t)
	response := proxyAPICall(t, s, "POST", "/api/proxy-discovery/sources", map[string]any{
		"provider": "fofa", "name": "baidu fixture", "verify_url": "www.baidu.com",
		"credentials": map[string]string{"email": "fixture@example.invalid", "key": "fixture"},
	})
	if response.Code != 200 {
		t.Fatalf("bare Baidu hostname rejected: %d %s", response.Code, response.Body)
	}
	sources, err := s.m.pg.ListProxyDiscoverySources()
	if err != nil || len(sources) != 1 || sources[0].VerifyURL != proxypool.BaiduVerifyURL ||
		sources[0].Query != defaultFOFADiscoveryQuery {
		t.Fatalf("saved defaults: %+v %v", sources, err)
	}
}
