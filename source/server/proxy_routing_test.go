package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"yunkun/console/db"
	"yunkun/console/proxypool"
)

func TestProxyRoundRobinDefaultAndConcurrentSelection(t *testing.T) {
	s := newProxyFixture(t)
	poolID := createTestPool(t, s, "http://127.0.0.1:19991")
	if err := s.m.pg.StoreProxyNodes(poolID, []proxypool.Node{
		{Address: "http://127.0.0.1:19992"}, {Address: "http://127.0.0.1:19993"},
	}); err != nil {
		t.Fatal(err)
	}
	task, err := s.m.pg.CreateTaskWithOptions("rotation", "local fixture", db.TaskCreateOptions{ProxyPoolID: &poolID})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := s.m.pg.TaskProxyBinding(task.ID)
	if b.Strategy != "round_robin" {
		t.Fatal("new task did not default to round robin")
	}
	ids := []int64{}
	for range 6 {
		route, err := s.selectProxyRoute(context.Background(), task.ID)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, route.Node.ID)
		if err := s.validateProxyRoute(context.Background(), route); err != nil {
			t.Fatal(err)
		}
	}
	if ids[0] == ids[1] || ids[1] == ids[2] || ids[0] == ids[2] ||
		ids[0] != ids[3] || ids[1] != ids[4] || ids[2] != ids[5] {
		t.Fatalf("not round robin: %v", ids)
	}
	// Reading status/validation must not consume scheduler turns.
	for range 3 {
		if _, err := s.networkStatus(task.ID); err != nil {
			t.Fatal(err)
		}
	}
	next, err := s.selectProxyRoute(context.Background(), task.ID)
	if err != nil || next.Node.ID != ids[0] {
		t.Fatal("status consumed a scheduler turn")
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	counts := map[int64]int{}
	for range 18 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			route, err := s.selectProxyRoute(context.Background(), task.ID)
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			counts[route.Node.ID]++
			mu.Unlock()
			if err := s.validateProxyRoute(context.Background(), next); err != nil {
				t.Error("concurrent selection revoked earlier request:", err)
			}
		}()
	}
	wg.Wait()
	if len(counts) != 3 {
		t.Fatal(counts)
	}
	for _, count := range counts {
		if count != 6 {
			t.Fatal(counts)
		}
	}
	after, _ := s.m.pg.TaskProxyBinding(task.ID)
	if after.Revision != b.Revision {
		t.Fatal("round robin mutated task policy revision")
	}
}

func TestProxyRoundRobinFixedCooldownAndPolicyChanges(t *testing.T) {
	s := newProxyFixture(t)
	poolID := createTestPool(t, s, "http://127.0.0.1:19991")
	if err := s.m.pg.StoreProxyNodes(poolID, []proxypool.Node{{Address: "http://127.0.0.1:19992"}}); err != nil {
		t.Fatal(err)
	}
	nodes, _ := s.m.pg.ProxyNodes(poolID)
	pinned := nodes[0].ID
	task, err := s.m.pg.CreateTaskWithOptions("fixed", "local fixture", db.TaskCreateOptions{ProxyPoolID: &poolID, ProxyNodeID: &pinned})
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		route, err := s.selectProxyRoute(context.Background(), task.ID)
		if err != nil || route.Node.ID != pinned || route.Strategy != "fixed" {
			t.Fatal("explicit pin lost", err)
		}
	}
	child, err := s.m.pg.CreateTaskWithOptions("child", "local fixture", db.TaskCreateOptions{ParentTaskID: &task.ID})
	if err != nil {
		t.Fatal(err)
	}
	cb, _ := s.m.pg.TaskProxyBinding(child.ID)
	if cb.Strategy != "fixed" || cb.NodeID == nil || *cb.NodeID != pinned {
		t.Fatal("child lost explicit pin")
	}
	if err := s.m.pg.SetProxyRouting(task.ID, "round_robin", nil); err == nil {
		t.Fatal("changed running task")
	}
	s.m.pg.Exec(`UPDATE tasks SET paused=true WHERE id=$1`, task.ID)
	if err := s.m.pg.SetProxyRouting(task.ID, "round_robin", nil); err != nil {
		t.Fatal(err)
	}
	s.m.pg.Exec(`UPDATE tasks SET paused=false WHERE id=$1`, task.ID)
	first, err := s.selectProxyRoute(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.proxyRuntimeFailure(first, "proxy_connect_failed"); err != nil {
		t.Fatal(err)
	}
	next, err := s.selectProxyRoute(context.Background(), task.ID)
	if err != nil || first.Node.ID == next.Node.ID {
		t.Fatal("cooldown did not exclude failed node", err)
	}
	if err := s.proxyRuntimeFailure(next, "proxy_connect_failed"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.selectProxyRoute(context.Background(), task.ID); err == nil {
		t.Fatal("exhausted pool routed")
	}
	// Cooldown state is persisted; constructing a new service cannot clear it.
	vault := s.proxyPools.vault
	if err := s.initProxyPools(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	s.proxyPools.vault = vault
	if _, err := s.selectProxyRoute(context.Background(), task.ID); err == nil {
		t.Fatal("restart bypassed cooldown")
	}
	if _, err := s.m.pg.Exec(`UPDATE proxy_nodes SET runtime_cooldown_until=now()-interval '1 second' WHERE pool_id=$1`, poolID); err != nil {
		t.Fatal(err)
	}
	trial, err := s.selectProxyRoute(context.Background(), task.ID)
	if err != nil {
		t.Fatal("cooldown never recovered", err)
	}
	if err := s.completeProxyRoute(trial, "", "http_200", ""); err != nil {
		t.Fatal(err)
	}
	node, _ := s.m.pg.ProxyNode(poolID, trial.Node.ID)
	if node.RuntimeError != "" || node.RuntimeCooldownUntil != nil {
		t.Fatal("successful trial did not clear runtime fault")
	}
}

func TestProxyCredentialBoundRecoveryExplicitAndPreservesInventory(t *testing.T) {
	s := newProxyFixture(t)
	poolID := createTestPool(t, s, "http://127.0.0.1:19991")
	task, err := s.m.pg.CreateTaskWithOptions("recovery", "local fixture", db.TaskCreateOptions{ProxyPoolID: &poolID})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := s.m.pg.ProxyPool(poolID)
	nodesBefore, _ := s.m.pg.ProxyNodes(poolID)
	oldRoute, err := s.selectProxyRoute(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	changeProxyFixtureVault(t, s)
	sourceRaw, err := s.proxyPools.vault.Seal(proxypool.Secrets{APIValue: "fixture-discovery-key"})
	if err != nil {
		t.Fatal(err)
	}
	sourceID, err := s.m.pg.SaveProxyDiscoverySource(0, "quake", "fixture source", "ip:192.0.2.1", "192.0.2.0/24", "https://example.test", sourceRaw, 100)
	if err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/proxy-pools/%d/credentials/recover", poolID)
	for _, req := range []any{
		map[string]any{"confirm": false, "secrets": map[string]string{}, "no_auth": true},
		map[string]any{"confirm": true, "secrets": map[string]string{}},
	} {
		if response := proxyAPICall(t, s, "POST", path, req); response.Code != 400 {
			t.Fatal(response.Code, response.Body)
		}
	}
	req := map[string]any{"confirm": true, "secrets": map[string]string{}, "no_auth": true}
	if response := proxyAPICall(t, s, "POST", path, req); response.Code != 409 {
		t.Fatal("running task recovery allowed", response.Code)
	}
	s.m.pg.Exec(`UPDATE tasks SET paused=true WHERE id=$1`, task.ID)
	response := proxyAPICall(t, s, "POST", path, req)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body)
	}
	after, _ := s.m.pg.ProxyPool(poolID)
	secrets, err := s.proxyPools.vault.Open(after.Credentials)
	if err != nil || secrets != (proxypool.Secrets{}) {
		t.Fatal("explicit no-auth recovery failed", err)
	}
	nodesAfter, _ := s.m.pg.ProxyNodes(poolID)
	if len(nodesAfter) != len(nodesBefore) || nodesAfter[0].ID != nodesBefore[0].ID || nodesAfter[0].Enabled ||
		nodesAfter[0].LastSuccessAt != nil {
		t.Fatal("inventory lost or stale evidence retained")
	}
	b, _ := s.m.pg.TaskProxyBinding(task.ID)
	if b == nil || b.PoolID != poolID || b.Revision != 2 {
		t.Fatal("binding lost or not revoked")
	}
	var backup string
	if err := s.m.pg.QueryRow(`SELECT previous_credentials FROM proxy_credential_recoveries WHERE pool_id=$1`, poolID).Scan(&backup); err != nil || backup != before.Credentials {
		t.Fatal("missing recovery backup", err)
	}
	if err := s.m.pg.RecordProxyNodeVerification(poolID, nodesBefore[0].ID, proxypool.VerificationResult{}, "", 1, before.Credentials); err != db.ErrProxyPoolCredentialsChanged {
		t.Fatal("old verification accepted", err)
	}
	if err := s.m.pg.RecordProxyRuntimeFailure(oldRoute, "proxy_connect_failed"); err != db.ErrProxyPoolCredentialsChanged {
		t.Fatal("old in-flight failure poisoned recovered pool", err)
	}
	_, sourceAfter, err := s.m.pg.ProxyDiscoverySource(sourceID)
	if err != nil || sourceAfter != sourceRaw {
		t.Fatal("pool recovery mutated discovery source")
	}
	s.m.pg.Exec(`UPDATE tasks SET paused=false WHERE id=$1`, task.ID)
	if _, err := s.selectProxyRoute(context.Background(), task.ID); err == nil {
		t.Fatal("recovery silently enabled nodes")
	}
	anonymous := httptest.NewRecorder()
	s.Handler().ServeHTTP(anonymous, httptest.NewRequest("POST", path, strings.NewReader(`{}`)))
	if anonymous.Code != 401 {
		t.Fatal("recovery not authenticated")
	}
	for _, role := range []string{"operator", "viewer"} {
		if roleAllows(role, "POST", path) || roleAllows(role, "PUT", fmt.Sprintf("/api/tasks/%d/network/routing", task.ID)) {
			t.Fatal("recovery/routing RBAC bypass")
		}
	}
}

func TestProxyRoundRobinGatewayFailureUsesNextWithoutDirect(t *testing.T) {
	s := newProxyFixture(t)
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()
	var used atomic.Int32
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		used.Add(1)
		w.Write([]byte("healthy proxy"))
	}))
	defer live.Close()
	poolID := createTestPool(t, s, deadURL)
	if err := s.m.pg.StoreProxyNodes(poolID, []proxypool.Node{{Address: live.URL}}); err != nil {
		t.Fatal(err)
	}
	task, err := s.m.pg.CreateTaskWithOptions("failover", "local fixture", db.TaskCreateOptions{ProxyPoolID: &poolID})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "http://target.invalid/", nil)
	ex, err := s.proxyPools.gateway.RoundTrip(req, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(ex.Response.Body)
	ex.Response.Body.Close()
	if string(body) != "healthy proxy" || used.Load() != 1 {
		t.Fatal("wrong route")
	}
	var cooled int
	s.m.pg.QueryRow(`SELECT count(*) FROM proxy_nodes WHERE pool_id=$1 AND runtime_cooldown_until>now()`, poolID).Scan(&cooled)
	if cooled != 1 {
		t.Fatal("failed node not persisted")
	}
}
