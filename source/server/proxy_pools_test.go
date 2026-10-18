package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"yunkun/console/agent"
	"yunkun/console/db"
	"yunkun/console/proxypool"
	actool "github.com/Autumn-27/norma/tool"
)

func newProxyFixture(t *testing.T) *Server {
	t.Helper()
	dsn, _, err := db.DSN()
	if err != nil {
		t.Skip("isolated postgres fixture needs a configured connection")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("yunkun_proxy_test_%d", time.Now().UnixNano())
	u.Path = "/" + name
	pg, err := db.Open(u.String())
	if err != nil {
		t.Fatal("open isolated proxy test database:", err)
	}
	t.Cleanup(func() {
		_ = pg.Close()
		u.Path = "/postgres"
		admin, err := sql.Open("pgx", u.String())
		if err != nil {
			t.Error(err)
			return
		}
		defer admin.Close()
		if _, err := admin.Exec(`DROP DATABASE "` + name + `" WITH (FORCE)`); err != nil {
			t.Error(err)
		}
	})
	m := &Manager{pg: pg, tasks: map[string]*Task{}}
	s := &Server{m: m, ctx: context.Background(), engine: NewEngine(m), jwtKey: []byte("proxy-test-only-signing-key-at-least-32")}
	if err := s.initProxyPools(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	s.initProxyDiscovery()
	return s
}

func proxyAPICall(t *testing.T, s *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	token, err := signJWT(s.jwtKey)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(b))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func createTestPool(t *testing.T, s *Server, address string) int64 {
	t.Helper()
	w := proxyAPICall(t, s, "POST", "/api/proxy-pools", saveProxyPoolRequest{
		Config:  proxypool.Config{Name: "fixture", Kind: "fixed", Enabled: true, Addresses: []string{address}, SafetySeconds: 1},
		Secrets: &proxypool.Secrets{ProxyUsername: "test-user", ProxyPassword: "never-echo-this-password"},
	})
	if w.Code != 200 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var result struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.ID
}

func TestXiaoxiangUnavailableQuotaClearsCountersAndAllowsFetch(t *testing.T) {
	s := newProxyFixture(t)
	w := proxyAPICall(t, s, "POST", "/api/proxy-pools", saveProxyPoolRequest{
		Config:  proxypool.Config{Name: "xiaoxiang fixture", Kind: "xiaoxiang", Enabled: true, QuotaSupported: true},
		Secrets: &proxypool.Secrets{AppKey: "fixture-app-key", AppSecret: "fixture-app-secret"},
	})
	var created struct {
		ID int64 `json:"id"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &created) != nil {
		t.Fatal("failed to create quota fixture")
	}
	oldCheck, total, remaining := time.Now().Add(-40*time.Second), int64(100), int64(0)
	if err := s.m.pg.StoreProxyQuota(created.ID, proxypool.Quota{
		Supported: true, Total: &total, Remaining: &remaining, CheckedAt: &oldCheck,
	}); err != nil {
		t.Fatal(err)
	}
	var quotaRequests, fetchRequests int
	s.proxyPools.provider.Client.Transport = discoveryProtocolTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.xiaoxiangdaili.com" || r.URL.Query().Get("appKey") != "fixture-app-key" ||
			r.URL.Query().Get("appSecret") != "fixture-app-secret" {
			t.Fatal("incorrect provider authentication")
		}
		var payload string
		switch r.URL.Path {
		case "/ip/remaining":
			quotaRequests++
			payload = `{"code":200,"success":true,"data":null,"msg":"暂无承载数据"}`
		case "/ip/get":
			fetchRequests++
			start := time.Now().In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02 15:04:05")
			payload = fmt.Sprintf(`{"code":200,"data":[{"ip":"198.51.100.7","port":8080,"startTime":%q,"during":10}]}`, start)
		default:
			t.Fatal("unexpected provider path")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(payload)),
			Header: make(http.Header)}, nil
	})
	path := fmt.Sprintf("/api/proxy-pools/%d/quota", created.ID)
	for range 2 {
		response := proxyAPICall(t, s, "POST", path, nil)
		var quota proxypool.Quota
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &quota) != nil ||
			!quota.Supported || !quota.Unavailable || quota.Total != nil || quota.Remaining != nil ||
			quota.CheckedAt == nil || quota.Error != "" {
			t.Fatal("unavailable quota was not returned as a successful unknown state")
		}
	}
	stored, err := s.m.pg.ProxyPool(created.ID)
	if err != nil || stored == nil || !stored.Quota.Unavailable || stored.Quota.Remaining != nil ||
		stored.Quota.Total != nil || stored.Quota.Error != "" || quotaRequests != 1 {
		t.Fatal("unavailable quota was not persisted or cached")
	}
	fetched := proxyAPICall(t, s, "POST", fmt.Sprintf("/api/proxy-pools/%d/fetch", created.ID), nil)
	if fetched.Code != http.StatusOK || fetchRequests != 1 || !strings.Contains(fetched.Body.String(), `"fetched":1`) {
		t.Fatal("unavailable quota incorrectly blocked extraction")
	}
}

func TestProxyPoolAPIAndTaskLifecycle(t *testing.T) {
	s := newProxyFixture(t)
	id := createTestPool(t, s, "http://127.0.0.1:9999")
	list := proxyAPICall(t, s, "GET", "/api/proxy-pools", nil)
	if list.Code != 200 || strings.Contains(list.Body.String(), "never-echo") || strings.Contains(list.Body.String(), "test-user") || !strings.Contains(list.Body.String(), `"has_credentials":true`) {
		t.Fatalf("unsafe list: %s", list.Body.String())
	}
	stored, err := s.m.pg.ProxyPool(id)
	if err != nil || strings.Contains(stored.Credentials, "never-echo") {
		t.Fatal("credentials not encrypted")
	}
	for _, role := range []string{"viewer", "operator"} {
		if !roleAllows(role, "GET", "/api/proxy-pools") || roleAllows(role, "POST", "/api/proxy-pools") ||
			roleAllows(role, "GET", fmt.Sprintf("/api/proxy-pools/%d/nodes", id)) {
			t.Fatalf("bad RBAC for %s", role)
		}
		r := httptest.NewRequest("GET", "/api/proxy-pools", nil)
		r = r.WithContext(context.WithValue(r.Context(), authContextKey{}, &authIdentity{Role: role}))
		w := httptest.NewRecorder()
		s.listProxyPools(w, r)
		if strings.Contains(w.Body.String(), "127.0.0.1") || strings.Contains(w.Body.String(), "never-echo") {
			t.Fatal("non-admin got source details")
		}
	}
	task, err := s.m.pg.CreateTaskWithOptions("proxy fixture", "test", db.TaskCreateOptions{ProxyPoolID: &id, NetworkMode: "configured"})
	if err != nil {
		t.Fatal(err)
	}
	s.m.tasks[strconv.FormatInt(task.ID, 10)] = taskFromPG(task, s.m.pg.Exploration(task.ExplorationID), nil)
	child, err := s.m.pg.CreateTaskWithOptions("child", "test", db.TaskCreateOptions{ParentTaskID: &task.ID})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.m.pg.TaskProxyBinding(child.ID)
	if err != nil || b == nil || b.PoolID != id {
		t.Fatal("spawn lost proxy policy")
	}
	if _, err := s.m.pg.CreateTaskWithOptions("invalid", "test", db.TaskCreateOptions{ProxyPoolID: &id, NetworkMode: "enforced"}); err == nil {
		t.Fatal("enforced downgraded")
	}
	if err := s.m.pg.ReassignProxyLease(task.ID); err == nil {
		t.Fatal("running task reassigned")
	}
	if _, err := s.m.pg.Exec(`UPDATE tasks SET paused=true WHERE id=$1`, task.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.m.pg.ReassignProxyLease(task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.resolveProxyRoute(context.Background(), task.ID); err == nil {
		t.Fatal("paused task routed")
	}
	if _, err := s.m.pg.Exec(`UPDATE tasks SET paused=false WHERE id=$1`, task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.resolveProxyRoute(context.Background(), task.ID); err != nil {
		t.Fatal(err)
	}
	if w := proxyAPICall(t, s, "DELETE", fmt.Sprintf("/api/proxy-pools/%d", id), nil); w.Code != 409 {
		t.Fatal("deleted bound source")
	}
	update := saveProxyPoolRequest{Config: stored.Config, Secrets: &proxypool.Secrets{ProxyPassword: "replacement"}}
	if w := proxyAPICall(t, s, "PUT", fmt.Sprintf("/api/proxy-pools/%d", id), update); w.Code != 409 {
		t.Fatal("changed credentials under lease")
	}
	update.Secrets = nil
	update.Config.Enabled = false
	if w := proxyAPICall(t, s, "PUT", fmt.Sprintf("/api/proxy-pools/%d", id), update); w.Code != 200 {
		t.Fatalf("disable: %s", w.Body.String())
	}
	if _, err := s.resolveProxyRoute(context.Background(), task.ID); err == nil {
		t.Fatal("disabled source routed")
	}
}

func TestProxyProviderRateLimitAndExpirySnapshot(t *testing.T) {
	s := newProxyFixture(t)
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := s.m.pg.ClaimProxyProviderCall("same-account:fetch", 10)
			if err != nil {
				t.Error(err)
			}
			if ok {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 1 {
		t.Fatalf("rate accepted %d", accepted.Load())
	}
	id := createTestPool(t, s, "http://127.0.0.1:9999")
	nodes, err := s.m.pg.ProxyNodes(id)
	if err != nil {
		t.Fatal(err)
	}
	expiry := time.Now().Add(time.Minute)
	nodes[0].ExpiresAt = &expiry
	if err := s.m.pg.StoreProxyNodes(id, nodes); err != nil {
		t.Fatal(err)
	}
	task, err := s.m.pg.CreateTaskWithOptions("expiry", "test", db.TaskCreateOptions{ProxyPoolID: &id})
	if err != nil {
		t.Fatal(err)
	}
	newExpiry := time.Now().Add(time.Hour)
	nodes[0].ExpiresAt = &newExpiry
	if err := s.m.pg.StoreProxyNodes(id, nodes); err != nil {
		t.Fatal(err)
	}
	b, err := s.m.pg.TaskProxyBinding(task.ID)
	if err != nil || b.ExpiresAt == nil || b.ExpiresAt.After(expiry.Add(time.Second)) {
		t.Fatal("refresh silently extended lease")
	}
	if _, err := s.m.pg.Exec(`UPDATE task_proxy_bindings SET expires_at=now()-interval '1 second' WHERE task_id=$1`, task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.resolveProxyRoute(context.Background(), task.ID); err == nil {
		t.Fatal("expired binding routed via refreshed node")
	}
	if _, err := s.m.pg.Exec(`UPDATE tasks SET paused=true WHERE id=$1`, task.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.m.pg.ReassignProxyLease(task.ID); err != nil {
		t.Fatal(err)
	}
	b, err = s.m.pg.TaskProxyBinding(task.ID)
	if err != nil || b.Revision != 2 || b.ExpiresAt == nil || b.ExpiresAt.Before(newExpiry.Add(-time.Second)) {
		t.Fatal("explicit reassignment did not renew")
	}
}

func TestProxyPoolSafetyBoundaries(t *testing.T) {
	s := newProxyFixture(t)
	poolID := createTestPool(t, s, "http://127.0.0.1:9999")
	nodes, err := s.m.pg.ProxyNodes(poolID)
	if err != nil || len(nodes) != 1 {
		t.Fatalf("nodes: %v %#v", err, nodes)
	}
	nodeID := nodes[0].ID

	if err := s.m.pg.SetProxyNodeEnabled(poolID, nodeID, false); err != nil {
		t.Fatal("disable node:", err)
	}
	if _, err := s.m.pg.CreateTaskWithOptions("disabled node", "must reject", db.TaskCreateOptions{
		ProxyPoolID: &poolID, ProxyNodeID: &nodeID,
	}); err == nil {
		t.Fatal("disabled node was accepted for a new task")
	}
	if err := s.m.pg.SetProxyNodeEnabled(poolID, nodeID, true); err != nil {
		t.Fatal("re-enable node:", err)
	}
	if err := s.m.pg.SoftDeleteProxyNode(poolID, nodeID); err != nil {
		t.Fatal("soft delete unused node:", err)
	}
	if _, err := s.m.pg.CreateTaskWithOptions("deleted node", "must reject", db.TaskCreateOptions{
		ProxyPoolID: &poolID, ProxyNodeID: &nodeID,
	}); err == nil {
		t.Fatal("soft-deleted node was accepted for a new task")
	}
}

func TestProxyDiscoveryApprovalUsesResultSourceAndTargetPool(t *testing.T) {
	s := newProxyFixture(t)
	cipher, err := s.proxyPools.vault.Seal(proxypool.Secrets{
		APIValue: `{"proxy_username":"test-user","proxy_password":"never-echo-this-password"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	sourceID, err := s.m.pg.SaveProxyDiscoverySource(
		0, "fofa", "fixture source", `port="8080"`, "203.0.113.0/24",
		"http://127.0.0.1:9/ip", cipher, 100,
	)
	if err != nil {
		t.Fatal("source:", err)
	}
	firstPool := createTestPool(t, s, "http://127.0.0.1:9998")
	targetPool := createTestPool(t, s, "http://127.0.0.1:9997")
	if err := s.m.pg.UpsertProxyDiscoveryResults(sourceID, []db.ProxyDiscoveryResult{{
		SourceID: sourceID, Address: "http://203.0.113.10:8080", Protocol: "http",
		Host: "203.0.113.10", Port: 8080, Metadata: map[string]any{"fixture": true},
	}}); err != nil {
		t.Fatal("result:", err)
	}
	results, err := s.m.pg.ListProxyDiscoveryResults(sourceID)
	if err != nil || len(results) != 1 {
		t.Fatalf("list results: %v %#v", err, results)
	}
	if err := s.m.pg.UpdateProxyDiscoveryResult(results[0].ID, "verified", true, "198.51.100.10", nil, ""); err != nil {
		t.Fatal("verify result:", err)
	}

	approved := proxyAPICall(t, s, "POST", fmt.Sprintf("/api/proxy-discovery/results/%d/approve", results[0].ID), map[string]any{
		"pool_id": targetPool, "target": "candidate",
	})
	if approved.Code != http.StatusOK {
		t.Fatalf("approve candidate: %d %s", approved.Code, approved.Body.String())
	}
	targetNodes, err := s.m.pg.ProxyNodes(targetPool)
	if err != nil || len(targetNodes) != 2 {
		t.Fatalf("target pool nodes: %v %#v", err, targetNodes)
	}
	var discovered *proxypool.Node
	for i := range targetNodes {
		if targetNodes[i].Origin == "fofa" {
			discovered = &targetNodes[i]
			break
		}
	}
	if discovered == nil || discovered.Enabled {
		t.Fatalf("candidate promotion did not create a disabled FOFA node: %#v", discovered)
	}
	firstNodes, err := s.m.pg.ProxyNodes(firstPool)
	if err != nil || len(firstNodes) != 1 {
		t.Fatalf("first pool changed by approval: %v %#v", err, firstNodes)
	}
}

func TestProxyDiscoverySourceAllowsOptionalScopeAndVerifyURL(t *testing.T) {
	s := newProxyFixture(t)
	w := proxyAPICall(t, s, "POST", "/api/proxy-discovery/sources", map[string]any{
		"provider": "fofa", "name": "optional safety fields", "query": `port="8080"`,
		"authorized_scope": "", "verify_url": "",
		"credentials": map[string]string{"email": "fixture@example.invalid", "key": "fixture-key"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("optional fields rejected: %d %s", w.Code, w.Body.String())
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil || created.ID <= 0 {
		t.Fatalf("created source response: %s", w.Body.String())
	}
	source, _, err := s.m.pg.ProxyDiscoverySource(created.ID)
	if err != nil || source == nil {
		t.Fatalf("read optional source: %v %#v", err, source)
	}
	if source.AuthorizedScope != "" || source.VerifyURL != "" {
		t.Fatalf("optional fields were not preserved as empty: %#v", source)
	}
	if source.FetchLimit != 100 {
		t.Fatalf("default fetch limit: got %d", source.FetchLimit)
	}
}

func TestProxyDiscoverySourcePersistsFetchLimit(t *testing.T) {
	s := newProxyFixture(t)
	w := proxyAPICall(t, s, "POST", "/api/proxy-discovery/sources", map[string]any{
		"provider": "quake", "name": "limited source", "query": `port:8080`,
		"fetch_limit": 200,
		"credentials": map[string]string{"key": "fixture-key"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("custom fetch limit rejected: %d %s", w.Code, w.Body.String())
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil || created.ID <= 0 {
		t.Fatalf("created source response: %s", w.Body.String())
	}
	source, _, err := s.m.pg.ProxyDiscoverySource(created.ID)
	if err != nil || source == nil {
		t.Fatalf("read custom source: %v %#v", err, source)
	}
	if source.FetchLimit != 200 {
		t.Fatalf("custom fetch limit was not persisted: got %d", source.FetchLimit)
	}
}

func TestProxyDiscoverySourceRejectsInvalidFetchLimit(t *testing.T) {
	s := newProxyFixture(t)
	for _, value := range []int{-1, 99, 1001} {
		w := proxyAPICall(t, s, "POST", "/api/proxy-discovery/sources", map[string]any{
			"provider": "quake", "name": "invalid limit", "query": `port:8080`,
			"fetch_limit": value,
			"credentials": map[string]string{"key": "fixture-key"},
		})
		if w.Code != http.StatusBadRequest {
			t.Fatalf("fetch limit %d accepted: %d %s", value, w.Code, w.Body.String())
		}
	}
}

func TestProxyNodeSummaryCountsExpiredNodesAsErrors(t *testing.T) {
	s := newProxyFixture(t)
	poolID := createTestPool(t, s, "http://127.0.0.1:9996")
	nodes, err := s.m.pg.ProxyNodes(poolID)
	if err != nil || len(nodes) != 1 {
		t.Fatalf("nodes: %v %#v", err, nodes)
	}
	if _, err := s.m.pg.Exec(`UPDATE proxy_nodes SET expires_at=now()-interval '1 minute' WHERE id=$1`, nodes[0].ID); err != nil {
		t.Fatal("expire node:", err)
	}
	summary, err := s.m.pg.ProxyNodeSummary(poolID, time.Hour)
	if err != nil {
		t.Fatal("summary:", err)
	}
	if summary.Error != 1 || summary.Available != 0 || summary.Pending != 0 || summary.Expiring != 0 {
		t.Fatalf("expired node status mismatch: %+v", summary)
	}
}

func TestTaskProxyFetchNeverRetriesDirectAndRecordsRoute(t *testing.T) {
	s := newProxyFixture(t)
	var reached atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached.Add(1); _, _ = w.Write([]byte("direct")) }))
	defer target.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Proxy-Authorization") == "" {
			t.Error("missing provider authentication")
		}
		_, _ = w.Write([]byte("proxied response"))
	}))
	defer proxy.Close()
	id := createTestPool(t, s, proxy.URL)
	task, err := s.m.pg.CreateTaskWithOptions("fetch", "test", db.TaskCreateOptions{ProxyPoolID: &id})
	if err != nil {
		t.Fatal(err)
	}
	gateway := httptest.NewServer(s.Handler())
	defer gateway.Close()
	s.proxyPools.baseURL = gateway.URL
	tools := s.proxyNetworkTools(agent.WithRunInfo(context.Background(), agent.RunInfo{TaskID: task.ID}), []actool.CoreTool{actool.NewWebFetch(actool.WebFetchConfig{})})
	input, _ := json.Marshal(map[string]string{"url": target.URL})
	result, err := tools[0].Call(context.Background(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result)
	if !strings.Contains(string(raw), "proxied response") || reached.Load() != 0 {
		t.Fatalf("not proxied: %s", raw)
	}
	events, err := s.m.pg.ProxyRouteEvents(task.ID, 0)
	if err != nil || len(events) < 2 {
		t.Fatalf("audit missing: %v", err)
	}
	proxy.Close()
	result, err = tools[0].Call(context.Background(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	if reached.Load() != 0 {
		t.Fatal("WebFetch directly retried target")
	}
	s.proxyPools.gateway = nil
	_, _ = tools[0].Call(context.Background(), input, nil)
	if reached.Load() != 0 {
		t.Fatal("gateway failure caused direct fallback")
	}
	// An ordinary API request remains authenticated on the same listener.
	req := httptest.NewRequest("GET", "/api/proxy-pools", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatal("API auth bypass")
	}
}
