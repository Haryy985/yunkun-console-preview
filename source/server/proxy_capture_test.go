package server

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"yunkun/console/agent"
	"yunkun/console/db"
	"yunkun/console/evidence"
	"yunkun/console/proxypool"
	"yunkun/console/report"
	"yunkun/console/traffic"
	actool "github.com/Autumn-27/norma/tool"
)

func attachCaptureStore(t *testing.T, s *Server) {
	t.Helper()
	tr, err := traffic.Open(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tr.Close() })
	s.m.traffic, s.m.trafficOn = tr, true
	if s.m.ProxyAddr() != "" || s.m.ProxyCACert() != "" {
		t.Fatal("storage-only recorder advertised a listener")
	}
}

func TestTaskProxyCaptureEvidenceAndReport(t *testing.T) {
	s := newProxyFixture(t)
	attachCaptureStore(t, s)
	var connects atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Proxy-Authorization") != "" {
			t.Error("proxy secret leaked to target")
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("captured HTTPS target"))
	}))
	defer target.Close()
	targetURL, _ := url.Parse(target.URL)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" || r.Host != targetURL.Host {
			http.Error(w, "unexpected", 400)
			return
		}
		connects.Add(1)
		conn, err := net.DialTimeout("tcp", targetURL.Host, time.Second)
		if err != nil {
			http.Error(w, "unavailable", 502)
			return
		}
		client, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			conn.Close()
			return
		}
		defer client.Close()
		defer conn.Close()
		_, _ = rw.WriteString("HTTP/1.1 200 OK\r\n\r\n")
		_ = rw.Flush()
		done := make(chan struct{})
		go func() { _, _ = io.Copy(conn, rw); _ = conn.Close(); close(done) }()
		_, _ = io.Copy(client, conn)
		_ = client.Close()
		<-done
	}))
	defer proxy.Close()
	s.proxyPools.gateway.TLSRoots = x509.NewCertPool()
	s.proxyPools.gateway.TLSRoots.AddCert(target.Certificate())
	pool := createTestPool(t, s, proxy.URL)
	task, err := s.m.pg.CreateTaskWithOptions("HTTPS evidence", "fixture", db.TaskCreateOptions{ProxyPoolID: &pool})
	if err != nil {
		t.Fatal(err)
	}
	tools := s.proxyNetworkTools(agent.WithRunInfo(t.Context(), agent.RunInfo{TaskID: task.ID}), []actool.CoreTool{actool.NewWebFetch(actool.WebFetchConfig{})})
	input, _ := json.Marshal(map[string]string{"url": target.URL + "/proof?value=1"})
	result, err := tools[0].Call(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result)
	if !strings.Contains(string(raw), "traffic_id:") || !strings.Contains(string(raw), "captured HTTPS target") || connects.Load() != 1 {
		t.Fatalf("HTTPS capture failed: %s connects=%d", raw, connects.Load())
	}
	events, err := s.m.pg.ProxyRouteEvents(task.ID, 0)
	if err != nil || len(events) < 2 || events[0].TrafficID == "" || events[0].Revision <= 0 {
		t.Fatalf("missing route/traffic association: %+v %v", events, err)
	}
	fid, err := s.m.pg.AddFinding(task.ID, 0, "fixture", "captured request", "low", "summary", "evidence", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	store := evidence.New(s.m.pg, s.m.traffic, t.TempDir())
	binding, err := store.Bind(t.Context(), fid, []db.TrafficRef{{TrafficID: events[0].TrafficID}})
	if err != nil || len(binding.Bindings) != 1 {
		t.Fatalf("bind: %v", err)
	}
	packet, err := report.ReadRequestPacket(store, binding.Bindings[0].Snapshot)
	if err != nil || packet.Reason != "" || !strings.Contains(packet.Text, "GET /proof?value=1 HTTP/1.1") || strings.Contains(packet.Text, "Proxy-Authorization") {
		t.Fatalf("report packet: %+v %v", packet, err)
	}
	if _, _, err := s.m.traffic.DeleteAll(); err != nil {
		t.Fatal(err)
	}
	packet2, err := report.ReadRequestPacket(store, binding.Bindings[0].Snapshot)
	if err != nil || packet2 != packet {
		t.Fatal("evidence did not survive traffic deletion", err)
	}
}

func TestProxyHTTPCompleteCaptureAndFailureStates(t *testing.T) {
	s := newProxyFixture(t)
	attachCaptureStore(t, s)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		switch r.URL.Path {
		case "/oversized":
			_, _ = w.Write([]byte(strings.Repeat("x", proxypool.MaxCaptureBody+1)))
		case "/partial":
			w.Header().Set("Content-Length", "200")
			_, _ = w.Write([]byte("short"))
		default:
			w.WriteHeader(503) // A target HTTP error is still valid captured evidence.
			_, _ = w.Write([]byte("target unavailable"))
		}
	}))
	defer proxy.Close()
	pool := createTestPool(t, s, proxy.URL)
	task, err := s.m.pg.CreateTaskWithOptions("HTTP evidence", "fixture", db.TaskCreateOptions{ProxyPoolID: &pool})
	if err != nil {
		t.Fatal(err)
	}
	g := s.proxyPools.gateway
	for _, tc := range []struct{ path, want string }{
		{"/ok", "captured"}, {"/oversized", "incomplete_or_oversized"}, {"/partial", "incomplete_or_oversized"},
	} {
		req, _ := http.NewRequestWithContext(t.Context(), "POST", "http://fixture.invalid"+tc.path, strings.NewReader("a=1"))
		req.Host = "virtual.fixture"
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		ex, err := g.RoundTrip(req, task.ID)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, ex.Response.Body)
		_ = ex.Response.Body.Close()
		if ex.CaptureStatus != tc.want {
			t.Fatalf("%s: %s", tc.path, ex.CaptureStatus)
		}
		if tc.want == "captured" {
			packet, _, err := s.m.traffic.Get(ex.TrafficID)
			if err != nil || !strings.Contains(packet, "Host: virtual.fixture") || !strings.Contains(packet, "Content-Length: 3") {
				t.Fatalf("incorrect POST packet: %q %v", packet, err)
			}
		} else if ex.TrafficID != "" {
			t.Fatal("partial capture got evidence ID")
		}
	}
	s.m.trafficOn = false
	req, _ := http.NewRequestWithContext(t.Context(), "GET", "http://fixture.invalid/ok", nil)
	ex, err := g.RoundTrip(req, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, ex.Response.Body)
	_ = ex.Response.Body.Close()
	if ex.CaptureStatus != "disabled" || ex.TrafficID != "" {
		t.Fatal("disabled capture wrote evidence")
	}
	s.m.trafficOn = true
	_ = s.m.traffic.Close()
	ex, err = g.RoundTrip(req, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, ex.Response.Body)
	_ = ex.Response.Body.Close()
	if ex.CaptureStatus != "storage_failed" || ex.TrafficID != "" {
		t.Fatal("failed storage advertised capture")
	}
}

func TestProxyLifecycleRetestArchiveAndRestart(t *testing.T) {
	s := newProxyFixture(t)
	if err := s.m.pg.EnsureLLMRecordsTable(); err != nil {
		t.Fatal(err)
	}
	if err := s.m.pg.EnsureLLMUsageTable(); err != nil {
		t.Fatal(err)
	}
	pool := createTestPool(t, s, "http://127.0.0.1:9")
	task, err := s.m.pg.CreateTaskWithOptions("lifecycle", "fixture", db.TaskCreateOptions{ProxyPoolID: &pool})
	if err != nil {
		t.Fatal(err)
	}
	fid, err := s.m.pg.AddFinding(task.ID, 0, "fixture", "retest", "low", "summary", "evidence", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, conv, _, err := s.m.pg.CreateFindingRetest(t.Context(), fid, "")
	if err != nil {
		t.Fatal(err)
	}
	retest, err := s.m.pg.FindingRetestForConversation(t.Context(), conv.ID)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := s.findingRetestRunContext(t.Context(), retest)
	if err != nil || agent.RunInfoFrom(ctx).TaskID != task.ID {
		t.Fatal("retest lost task", err)
	}
	if err := s.m.pg.SetPaused(task.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.findingRetestRunContext(t.Context(), retest); err == nil {
		t.Fatal("paused retest allowed")
	}
	// A fresh service reads the same persisted lease without process-local routing state.
	restarted := &Server{m: s.m, ctx: context.Background(), jwtKey: s.jwtKey, proxyPools: s.proxyPools}
	if _, err := restarted.resolveProxyRoute(t.Context(), task.ID); err == nil {
		t.Fatal("restart ignored pause")
	}
	if err := s.m.pg.SetPaused(task.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.resolveProxyRoute(t.Context(), task.ID); err != nil {
		t.Fatal(err)
	}
	// Seal the retest before archiving its source task.
	if err := s.m.pg.FailPendingRetestForConversation(conv.ID, "fixture ended"); err != nil {
		t.Fatal(err)
	}
	if err := s.m.pg.SetPaused(task.ID, true); err != nil {
		t.Fatal(err)
	}
	before, err := s.m.pg.TaskProxyBinding(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := s.m.pg.QueueTaskArchive(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.m.pg.ClaimTaskArchiveJob(t.Context()); err != nil {
		t.Fatal(err)
	}
	var records bytes.Buffer
	snapshot, err := s.m.pg.SnapshotTaskArchiveWithLLMRecords(task.ID, &records)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.m.pg.CompleteTaskArchive(archive.ID, snapshot, "fixture.tar.zst", "fixture", 1, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.resolveProxyRoute(t.Context(), task.ID); err == nil {
		t.Fatal("archived task routed")
	}
	if _, err := s.findingRetestRunContext(t.Context(), retest); err == nil {
		t.Fatal("archived retest ran")
	}
	if _, err := s.m.pg.QueueTaskArchiveRestore(archive.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.m.pg.ClaimTaskArchiveJob(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.m.pg.RestoreTaskArchiveWithLLMRecords(archive.ID, snapshot, 600, bytes.NewReader(records.Bytes())); err != nil {
		t.Fatal(err)
	}
	if err := s.m.pg.CompleteTaskArchiveRestore(archive.ID); err != nil {
		t.Fatal(err)
	}
	after, err := s.m.pg.TaskProxyBinding(task.ID)
	if err != nil || before.PoolID != after.PoolID || before.Revision != after.Revision || *before.NodeID != *after.NodeID {
		t.Fatal("restore changed proxy binding", err)
	}
}

func TestProxyConcurrentCaptureAttribution(t *testing.T) {
	s := newProxyFixture(t)
	attachCaptureStore(t, s)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(r.URL.Path)) }))
	defer proxy.Close()
	pool := createTestPool(t, s, proxy.URL)
	var ids []int64
	for _, name := range []string{"one", "two"} {
		task, err := s.m.pg.CreateTaskWithOptions(name, "fixture", db.TaskCreateOptions{ProxyPoolID: &pool})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, task.ID)
	}
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			path := []string{"/one", "/two"}[i]
			req, _ := http.NewRequestWithContext(t.Context(), "GET", "http://fixture.invalid"+path, nil)
			ex, err := s.proxyPools.gateway.RoundTrip(req, id)
			if err != nil {
				t.Error(err)
				return
			}
			_, _ = io.Copy(io.Discard, ex.Response.Body)
			_ = ex.Response.Body.Close()
			events, err := s.m.pg.ProxyRouteEvents(id, 0)
			if err != nil || len(events) < 2 || events[0].TrafficID != ex.TrafficID || ex.TrafficID == "" {
				t.Error("cross-task capture attribution", err)
				return
			}
			reqText, _, err := s.m.traffic.Get(ex.TrafficID)
			if err != nil || !strings.Contains(reqText, "GET "+path+" ") {
				t.Error("cross-task body", err)
			}
		}()
	}
	wg.Wait()
}
