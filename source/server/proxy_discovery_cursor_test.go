package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"yunkun/console/db"
	"yunkun/console/proxypool"
)

func TestDiscoveryCursorCommitRollbackAndCrossPageSelection(t *testing.T) {
	s := newProxyFixture(t)
	s.initProxyDiscovery()
	sealed, err := s.proxyPools.vault.Seal(proxypool.Secrets{APIValue: `{"key":"fixture"}`})
	if err != nil {
		t.Fatal(err)
	}
	sourceID, err := s.m.pg.SaveProxyDiscoverySource(0, "quake", "cursor fixture", "fixture",
		"203.0.113.0/24", "", sealed, 100)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.m.pg.DeleteProxyDiscoverySource(sourceID) })
	fail := true
	var starts []int
	s.proxyDiscovery.client = &http.Client{Transport: discoveryProtocolTransport(func(r *http.Request) (*http.Response, error) {
		var request struct{ Start, Size int }
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		starts = append(starts, request.Start)
		if fail {
			return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader("{}"))}, nil
		}
		items := make([]map[string]any, 100)
		for i := range items {
			items[i] = map[string]any{"ip": "203.0.113.7", "port": 10000 + request.Start + i}
		}
		payload, _ := json.Marshal(map[string]any{"code": 0, "data": items})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(payload)))}, nil
	})}
	runURL := fmt.Sprintf("/api/proxy-discovery/sources/%d/run", sourceID)
	if w := proxyAPICall(t, s, "POST", runURL, nil); w.Code != 502 {
		t.Fatalf("provider error: %d %s", w.Code, w.Body.String())
	}
	source, _, _ := s.m.pg.ProxyDiscoverySource(sourceID)
	if source.NextOffset != 0 {
		t.Fatal("provider error skipped a page")
	}
	fail = false
	for range 2 {
		if w := proxyAPICall(t, s, "POST", runURL, nil); w.Code != 200 {
			t.Fatalf("run: %d %s", w.Code, w.Body.String())
		}
	}
	if fmt.Sprint(starts) != "[0 0 100]" {
		t.Fatalf("provider offsets: %v", starts)
	}
	w := proxyAPICall(t, s, "GET", fmt.Sprintf("/api/proxy-discovery/results/ids?source_id=%d", sourceID), nil)
	var selected struct{ IDs []int64 }
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &selected) != nil || len(selected.IDs) != 200 {
		t.Fatalf("cross-page selection omitted candidates: %d %s", w.Code, w.Body.String())
	}
	items := []db.ProxyDiscoveryResult{
		{Address: "http://203.0.113.7:20001", Protocol: "http", Host: "203.0.113.7", Port: 20001},
		{Address: "http://203.0.113.7:20002", Protocol: "invalid", Host: "203.0.113.7", Port: 20002},
	}
	if err := s.m.pg.StoreProxyDiscoveryRun(sourceID, items, 300, false); err == nil {
		t.Fatal("invalid candidate did not roll back the batch")
	}
	source, _, _ = s.m.pg.ProxyDiscoverySource(sourceID)
	count, _ := s.m.pg.ProxyDiscoveryResultCount(sourceID, "")
	if source.NextOffset != 200 || count != 200 {
		t.Fatalf("partial commit: offset=%d count=%d", source.NextOffset, count)
	}
}

func TestQuakeApplicationErrorDoesNotBecomeEmptySuccess(t *testing.T) {
	s := &Server{proxyDiscovery: &proxyDiscoveryService{client: &http.Client{
		Transport: discoveryProtocolTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":240,"data":[]}`))}, nil
		}),
	}}}
	if _, _, err := s.fetchQuake(context.Background(), &db.ProxyDiscoverySource{FetchLimit: 100},
		proxypool.Secrets{APIValue: `{"key":"fixture"}`}); err == nil {
		t.Fatal("provider application error was marked as exhausted")
	}
}
