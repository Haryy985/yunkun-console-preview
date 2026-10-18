package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"yunkun/console/db"
	"yunkun/console/proxypool"
)

func discoveryVerifyFixture(t *testing.T, s *Server, addresses []string) (int64, []int64, string) {
	t.Helper()
	sealed, err := s.proxyPools.vault.Seal(proxypool.Secrets{APIValue: `{"key":"fixture"}`})
	if err != nil {
		t.Fatal(err)
	}
	sourceID, err := s.m.pg.SaveProxyDiscoverySource(0, "quake", "fast verification", "fixture",
		"127.0.0.1", "http://check.invalid/ip", sealed, 100)
	if err != nil {
		t.Fatal(err)
	}
	var candidates []db.ProxyDiscoveryResult
	for _, address := range addresses {
		u, err := url.Parse(address)
		if err != nil {
			t.Fatal(err)
		}
		port, _ := strconv.Atoi(u.Port())
		candidates = append(candidates, db.ProxyDiscoveryResult{
			Address: address, Protocol: "http", Host: u.Hostname(), Port: port,
		})
	}
	if err := s.m.pg.UpsertProxyDiscoveryResults(sourceID, candidates); err != nil {
		t.Fatal(err)
	}
	results, err := s.m.pg.ListProxyDiscoveryResults(sourceID)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]int64, len(results))
	for i, result := range results {
		ids[i] = result.ID
	}
	return sourceID, ids, sealed
}

func TestDiscoveryFastTimeoutRemainsPendingAndCanRetry(t *testing.T) {
	s := newProxyFixture(t)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(300 * time.Millisecond):
			fmt.Fprint(w, "198.51.100.7")
		}
	}))
	defer proxy.Close()
	_, ids, _ := discoveryVerifyFixture(t, s, []string{proxy.URL})
	_, status, err := s.verifyDiscoveryCandidateWithTimeout(context.Background(), ids[0], 30*time.Millisecond)
	if status != http.StatusGatewayTimeout || !proxypool.IsVerificationTimeout(err) {
		t.Fatalf("fast timeout: %d %v", status, err)
	}
	candidate, err := s.m.pg.ProxyDiscoveryResult(ids[0])
	if err != nil || candidate.State != "pending" || candidate.VerifiedAt != nil || candidate.VerifiedIP != "" {
		t.Fatalf("timeout must not establish invalid or verified state: %#v %v", candidate, err)
	}
	_, status, err = s.verifyDiscoveryCandidate(context.Background(), ids[0])
	if status != http.StatusOK || err != nil {
		t.Fatalf("single-item retry failed: %d %v", status, err)
	}
}

func TestDiscoveryVerificationRejectsChangedSource(t *testing.T) {
	s := newProxyFixture(t)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		<-release
		fmt.Fprint(w, "198.51.100.7")
	}))
	defer proxy.Close()
	sourceID, ids, sealed := discoveryVerifyFixture(t, s, []string{proxy.URL})
	done := make(chan int, 1)
	go func() {
		_, status, _ := s.verifyDiscoveryCandidateWithTimeout(context.Background(), ids[0], time.Second)
		done <- status
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("verification never reached proxy")
	}
	s.proxyDiscovery.mu.Lock()
	_, err := s.m.pg.SaveProxyDiscoverySource(sourceID, "quake", "changed", "fixture",
		"", "http://check.invalid/ip", sealed, 100)
	s.proxyDiscovery.mu.Unlock()
	close(release)
	if err != nil {
		t.Fatal(err)
	}
	if status := <-done; status != http.StatusConflict {
		t.Fatalf("outdated verification accepted: %d", status)
	}
	candidate, err := s.m.pg.ProxyDiscoveryResult(ids[0])
	if err != nil || candidate.State != "pending" || candidate.VerifiedIP != "" {
		t.Fatalf("outdated proof persisted: %#v %v", candidate, err)
	}
}

func TestDiscoveryBatchFastVerificationRunsConcurrently(t *testing.T) {
	s := newProxyFixture(t)
	var active, peak atomic.Int32
	var proxies []*httptest.Server
	var addresses []string
	for i := 0; i < 12; i++ {
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			now := active.Add(1)
			for prev := peak.Load(); now > prev && !peak.CompareAndSwap(prev, now); prev = peak.Load() {
			}
			defer active.Add(-1)
			time.Sleep(350 * time.Millisecond)
			fmt.Fprint(w, "198.51.100.7")
		}))
		proxies = append(proxies, proxy)
		addresses = append(addresses, proxy.URL)
	}
	defer func() {
		for _, proxy := range proxies {
			proxy.Close()
		}
	}()
	_, ids, _ := discoveryVerifyFixture(t, s, addresses)
	var wg sync.WaitGroup
	var failures atomic.Int32
	for _, id := range ids {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			_, status, err := s.verifyDiscoveryCandidateWithTimeout(context.Background(), id, discoveryBatchVerifyTimeout)
			if status != http.StatusOK || err != nil {
				failures.Add(1)
			}
		}(id)
	}
	wg.Wait()
	if failures.Load() != 0 || peak.Load() < 2 || peak.Load() > 10 {
		t.Fatalf("batch concurrency: failed=%d peak=%d", failures.Load(), peak.Load())
	}
}

func TestDiscoveryBatchResponseKeepsRequestedOrder(t *testing.T) {
	s := newProxyFixture(t)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "198.51.100.7")
	}))
	defer proxy.Close()
	_, ids, _ := discoveryVerifyFixture(t, s, []string{proxy.URL})
	response := proxyAPICall(t, s, "POST", "/api/proxy-discovery/results/batch",
		map[string]any{"ids": ids, "action": "verify"})
	var body struct {
		Items []struct {
			ID int64 `json:"id"`
			OK bool  `json:"ok"`
		} `json:"items"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil ||
		len(body.Items) != 1 || body.Items[0].ID != ids[0] || !body.Items[0].OK ||
		strings.Contains(response.Body.String(), "fixture") {
		t.Fatalf("batch response: %d %s", response.Code, response.Body.String())
	}
}
