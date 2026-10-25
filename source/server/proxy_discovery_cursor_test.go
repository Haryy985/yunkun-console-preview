package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	latest, err := s.m.pg.LatestProxyDiscoveryRun(sourceID)
	if err != nil || latest == nil || latest.Run.OffsetStart != 100 || latest.Run.Returned != 100 || latest.Run.Inserted != 100 || latest.Attempted != 0 {
		t.Fatalf("latest discovery run: %+v %v", latest, err)
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

func TestDiscoveryRepeatedPageKeepsRequestedCountAndNewCandidateDenominator(t *testing.T) {
	s := newProxyFixture(t)
	sourceID, err := s.m.pg.SaveProxyDiscoverySource(0, "quake", "repeated", "fixture", "203.0.113.0/24", "", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	item := db.ProxyDiscoveryResult{Address: "http://203.0.113.8:8080", Protocol: "http", Host: "203.0.113.8", Port: 8080, Authorized: true}
	ids, firstRun, err := s.m.pg.StoreProxyDiscoveryRunWithMetrics(sourceID, []db.ProxyDiscoveryResult{item}, 0, 100, 100, 100, false)
	if err != nil || len(ids) != 1 {
		t.Fatalf("first batch: %v %v", ids, err)
	}
	_, secondRun, err := s.m.pg.StoreProxyDiscoveryRunWithMetrics(sourceID, []db.ProxyDiscoveryResult{item}, 100, 100, 100, 200, false)
	if err != nil || secondRun == firstRun {
		t.Fatalf("repeated page: %v", err)
	}
	var original int64
	if err := s.m.pg.QueryRow(`SELECT first_run_id FROM proxy_discovery_results WHERE id=$1`, ids[0]).Scan(&original); err != nil || original != firstRun {
		t.Fatalf("original provenance changed: %d %v", original, err)
	}
	if err := s.m.pg.UpdateProxyDiscoveryResult(ids[0], "verifying", true, "", nil, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.m.pg.CompleteProxyDiscoveryVerification(ids[0], "verified", true, "203.0.113.8", 1, "", "exit_ip", "https://example.invalid", "success", "response", "ok", 200); err != nil {
		t.Fatal(err)
	}
	latest, err := s.m.pg.LatestProxyDiscoveryRun(sourceID)
	if err != nil || latest == nil || latest.Run.Requested != 100 || latest.Run.Returned != 100 || latest.Run.Parsed != 1 || latest.Run.Inserted != 0 || latest.Attempted != 0 {
		t.Fatalf("repeated page falsely counted old verification: %+v %v", latest, err)
	}
	var attributed int64
	if err := s.m.pg.QueryRow(`SELECT run_id FROM proxy_discovery_verifications WHERE result_id=$1`, ids[0]).Scan(&attributed); err != nil || attributed != firstRun {
		t.Fatalf("verification attributed to repeated batch: %d %v", attributed, err)
	}
	// The legacy API must take its cursor from the source, not the filtered result count.
	if _, err := s.m.pg.StoreProxyDiscoveryRunWithIDs(sourceID, []db.ProxyDiscoveryResult{item}, 300, false); err != nil {
		t.Fatalf("legacy call with filtered page: %v", err)
	}
	latest, err = s.m.pg.LatestProxyDiscoveryRun(sourceID)
	if err != nil || latest.Run.OffsetStart != 200 || latest.Run.Requested != 100 || latest.Run.Inserted != 0 {
		t.Fatalf("legacy cursor or metrics incorrect: %+v %v", latest, err)
	}
	if _, _, err := s.m.pg.StoreProxyDiscoveryRunWithMetrics(sourceID, []db.ProxyDiscoveryResult{item}, 100, 100, 100, 200, false); err == nil {
		t.Fatal("stale cursor accepted")
	}
}

func TestDiscoveryRecoveryOnlyResetsStaleInFlight(t *testing.T) {
	s := newProxyFixture(t)
	sourceID, err := s.m.pg.SaveProxyDiscoverySource(0, "quake", "recovery", "fixture", "127.0.0.1", "", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	items := []db.ProxyDiscoveryResult{
		{Address: "http://127.0.0.1:18081", Protocol: "http", Host: "127.0.0.1", Port: 18081},
		{Address: "http://127.0.0.1:18082", Protocol: "http", Host: "127.0.0.1", Port: 18082},
	}
	if err := s.m.pg.UpsertProxyDiscoveryResults(sourceID, items); err != nil {
		t.Fatal(err)
	}
	if _, err := s.m.pg.Exec(`UPDATE proxy_discovery_results SET state='verifying',
updated_at=CASE WHEN port=18081 THEN now()-interval '2 minutes' ELSE now() END WHERE source_id=$1`, sourceID); err != nil {
		t.Fatal(err)
	}
	changed, err := s.m.pg.RecoverStaleProxyDiscoveryVerifications()
	if err != nil || changed != 1 {
		t.Fatalf("recovery count: %d %v", changed, err)
	}
	rows, err := s.m.pg.ListProxyDiscoveryResults(sourceID)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Port == 18081 && row.State != "pending" || row.Port == 18082 && row.State != "verifying" {
			t.Fatalf("unexpected recovery state: %+v", row)
		}
	}
}

func TestDiscoveryProtocolConstraintMigrationIsIdempotent(t *testing.T) {
	s := newProxyFixture(t)
	sourceID, err := s.m.pg.SaveProxyDiscoverySource(0, "quake", "old constraint", "fixture", "", "", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	item := db.ProxyDiscoveryResult{Address: "http://127.0.0.1:18083", Protocol: "http", Host: "127.0.0.1", Port: 18083}
	if err := s.m.pg.UpsertProxyDiscoveryResults(sourceID, []db.ProxyDiscoveryResult{item}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.m.pg.Exec(`ALTER TABLE proxy_discovery_results DROP CONSTRAINT proxy_discovery_results_protocol_check`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.m.pg.Exec(`ALTER TABLE proxy_discovery_results ADD CONSTRAINT proxy_discovery_results_protocol_check
CHECK (protocol IN ('http','https','socks5'))`); err != nil {
		t.Fatal(err)
	}
	dsn, _, err := db.DSN()
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	var database string
	if err := s.m.pg.QueryRow(`SELECT current_database()`).Scan(&database); err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + database
	var migratedOID int64
	for iteration := 0; iteration < 2; iteration++ {
		opened, err := db.Open(u.String())
		if err != nil {
			t.Fatalf("schema restart %d: %v", iteration, err)
		}
		var oid int64
		if err := opened.QueryRow(`SELECT oid FROM pg_constraint WHERE conrelid='proxy_discovery_results'::regclass
AND conname='proxy_discovery_results_protocol_check'`).Scan(&oid); err != nil {
			opened.Close()
			t.Fatal(err)
		}
		if iteration == 0 {
			migratedOID = oid
			if err := opened.UpsertProxyDiscoveryResults(sourceID, []db.ProxyDiscoveryResult{{
				Address: "unknown://127.0.0.1:18084", Protocol: "unknown", Host: "127.0.0.1", Port: 18084,
			}}); err != nil {
				opened.Close()
				t.Fatalf("unknown protocol rejected after upgrade: %v", err)
			}
		} else {
			var count int
			if err := opened.QueryRow(`SELECT count(*) FROM proxy_discovery_results WHERE source_id=$1`, sourceID).Scan(&count); err != nil || count != 2 {
				opened.Close()
				t.Fatalf("upgrade lost data: %d %v", count, err)
			}
			if oid != migratedOID {
				opened.Close()
				t.Fatal("schema startup rebuilt an already compatible constraint")
			}
		}
		opened.Close()
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
