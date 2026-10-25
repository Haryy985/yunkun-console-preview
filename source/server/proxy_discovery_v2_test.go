package server

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"yunkun/console/db"
	"yunkun/console/proxypool"
)

func TestFOFAUnknownProtocolRequiresConfirmation(t *testing.T) {
	s := &Server{proxyDiscovery: &proxyDiscoveryService{client: &http.Client{
		Transport: discoveryProtocolTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"results":[["x","203.0.113.4",1080,"socks4"],["x","203.0.113.5",8080,"https"]]}`))}, nil
		}),
	}}}
	got, fetched, err := s.fetchFOFA(context.Background(), &db.ProxyDiscoverySource{AuthorizedScope: "203.0.113.0/24", FetchLimit: 100}, proxypool.Secrets{APIValue: `{"email":"x","key":"x"}`})
	if err != nil || fetched != 2 || len(got) != 2 || got[0].Protocol != "unknown" || got[1].Protocol != "https" {
		t.Fatalf("FOFA protocol classification: %+v fetched=%d err=%v", got, fetched, err)
	}
}

func TestDiscoveryManualProtocolConfirmationKeepsEvidenceSafe(t *testing.T) {
	s := newProxyFixture(t)
	sealed, err := s.proxyPools.vault.Seal(proxypool.Secrets{APIValue: `{"key":"fixture"}`})
	if err != nil {
		t.Fatal(err)
	}
	sourceID, err := s.m.pg.SaveProxyDiscoverySource(0, "quake", "unknown-protocol", "fixture", "127.0.0.1", "", sealed, 100)
	if err != nil {
		t.Fatal(err)
	}
	candidate := db.ProxyDiscoveryResult{Address: "unknown://127.0.0.1:18080", Protocol: "unknown", Host: "127.0.0.1", Port: 18080, Authorized: true}
	if err := s.m.pg.UpsertProxyDiscoveryResults(sourceID, []db.ProxyDiscoveryResult{candidate}); err != nil {
		t.Fatal(err)
	}
	items, err := s.m.pg.ListProxyDiscoveryResults(sourceID)
	if err != nil || len(items) != 1 {
		t.Fatalf("fixture: %v %v", items, err)
	}
	id := items[0].ID
	if w := proxyAPICall(t, s, "POST", "/api/proxy-discovery/results/"+strconv.FormatInt(id, 10)+"/verify", nil); w.Code != 409 {
		t.Fatalf("unknown protocol was probed: %d", w.Code)
	}
	if w := proxyAPICall(t, s, "PUT", "/api/proxy-discovery/results/"+strconv.FormatInt(id, 10)+"/protocol", map[string]string{"protocol": "socks5"}); w.Code != 200 {
		t.Fatalf("confirmation: %d %s", w.Code, w.Body.String())
	}
	updated, err := s.m.pg.ProxyDiscoveryResult(id)
	if err != nil || updated.Protocol != "socks5" || updated.Address != "socks5://127.0.0.1:18080" || updated.State != "pending" || updated.VerifiedIP != "" {
		t.Fatalf("confirmation did not reset evidence: %+v %v", updated, err)
	}
	if w := proxyAPICall(t, s, "PUT", "/api/proxy-discovery/results/"+strconv.FormatInt(id, 10)+"/protocol", map[string]string{"protocol": "http"}); w.Code != 409 {
		t.Fatalf("protocol confirmation overwritten a known candidate: %d", w.Code)
	}
}
