package server

import (
	"fmt"
	"net"
	"strconv"
	"testing"

	"yunkun/console/db"
)

func TestQuakeLegacyProtocolRepair(t *testing.T) {
	s := newProxyFixture(t)
	quakeID, err := s.m.pg.SaveProxyDiscoverySource(0, "quake", "legacy SOCKS5", "fixture", "0.0.0.0/0", "", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	fofaID, err := s.m.pg.SaveProxyDiscoverySource(0, "fofa", "unrelated provider", "fixture", "0.0.0.0/0", "", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	type candidate struct {
		id                       int64
		state, protocol, address string
		repair                   bool
	}
	var candidates []candidate
	seed := func(sourceID int64, host string, port int, service, state, protocol string, repair bool) {
		t.Helper()
		address := protocol + "://" + net.JoinHostPort(host, strconv.Itoa(port))
		if err := s.m.pg.UpsertProxyDiscoveryResults(sourceID, []db.ProxyDiscoveryResult{{
			Address: address, Protocol: protocol, Host: host, Port: port, Authorized: true,
			Metadata: map[string]any{"service": map[string]any{"name": service}},
		}}); err != nil {
			t.Fatal(err)
		}
		var id int64
		if err := s.m.pg.QueryRow(`UPDATE proxy_discovery_results SET state=$3,verified_ip='198.51.100.1',
verified_at=now(),latency_ms=12,last_error='old error'
WHERE source_id=$1 AND address=$2 RETURNING id`, sourceID, address, state).Scan(&id); err != nil {
			t.Fatal(err)
		}
		candidates = append(candidates, candidate{id, state, protocol, address, repair})
	}
	for i, state := range []string{"pending", "invalid", "verified", "verifying", "approved", "ignored"} {
		seed(quakeID, "203.0.113.1", 10000+i, " SOCKS5 ", state, "http", i < 4)
	}
	seed(quakeID, "2001:db8::1", 1080, "socks5h", "invalid", "http", true)
	seed(quakeID, "203.0.113.2", 1080, "http", "invalid", "http", false)
	seed(fofaID, "203.0.113.2", 1080, "socks5", "invalid", "http", false)
	// A collision must preserve both independent review records.
	seed(quakeID, "203.0.113.3", 1080, "socks5", "invalid", "http", false)
	seed(quakeID, "203.0.113.3", 1080, "socks5", "verified", "socks5", false)
	repaired, err := s.m.pg.RepairQuakeDiscoveryProtocols()
	if err != nil || repaired != 5 {
		t.Fatalf("repair: count=%d err=%v", repaired, err)
	}
	for _, want := range candidates {
		got, err := s.m.pg.ProxyDiscoveryResult(want.id)
		if err != nil || got == nil {
			t.Fatalf("candidate %d lost: %v", want.id, err)
		}
		if !got.Authorized {
			t.Fatalf("candidate %d lost authorization", want.id)
		}
		if want.repair {
			expected := "socks5://" + net.JoinHostPort(got.Host, strconv.Itoa(got.Port))
			if got.Protocol != "socks5" || got.Address != expected || got.State != "pending" ||
				got.VerifiedAt != nil || got.VerifiedIP != "" || got.LatencyMS != nil || got.LastError != "" {
				t.Fatalf("candidate %d not reset for verification: %#v", want.id, got)
			}
		} else if got.Protocol != want.protocol || got.Address != want.address || got.State != want.state ||
			got.VerifiedIP != "198.51.100.1" || got.LastError != "old error" {
			t.Fatalf("unrelated candidate changed: %s", fmt.Sprint(got))
		}
	}
	if repaired, err := s.m.pg.RepairQuakeDiscoveryProtocols(); err != nil || repaired != 0 {
		t.Fatalf("repair is not idempotent: count=%d err=%v", repaired, err)
	}
}
