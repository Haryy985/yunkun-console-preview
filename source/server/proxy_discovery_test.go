package server

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"yunkun/console/db"
	"yunkun/console/proxypool"
)

type discoveryProtocolTransport func(*http.Request) (*http.Response, error)

func (f discoveryProtocolTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestQuakeDiscoveryUsesServiceProtocol(t *testing.T) {
	for _, test := range []struct {
		name, service, want string
	}{
		{"SOCKS5", "socks5", "socks5"},
		{"normalized SOCKS5", " SOCKS5 ", "socks5"},
		{"SOCKS5 remote DNS", "socks5h", "socks5"},
		{"HTTP proxy", "http", "http"},
		{"TCP is not a proxy protocol", "tcp", "http"},
		{"SOCKS4 is not SOCKS5", "socks4", "http"},
		{"missing service", "", "http"},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := `{"data":[{"ip":"203.0.113.7","port":1080,"transport":"tcp","service":{"name":"` + test.service + `"}}]}`
			s := &Server{proxyDiscovery: &proxyDiscoveryService{client: &http.Client{
				Transport: discoveryProtocolTransport(func(r *http.Request) (*http.Response, error) {
					if r.URL.String() != quakeDiscoveryURL || r.Header.Get("X-QuakeToken") != "fixture" {
						t.Fatal("incorrect Quake request")
					}
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(payload)), Header: make(http.Header)}, nil
				}),
			}}}
			results, err := s.fetchQuake(context.Background(), &db.ProxyDiscoverySource{ID: 9, AuthorizedScope: "203.0.113.0/24"},
				proxypool.Secrets{APIValue: `{"key":"fixture"}`})
			if err != nil || len(results) != 1 {
				t.Fatalf("fetch results: %#v %v", results, err)
			}
			got := results[0]
			if got.Protocol != test.want || got.Address != test.want+"://203.0.113.7:1080" {
				t.Fatalf("incorrect candidate protocol: %#v", got)
			}
		})
	}
}

func TestDiscoveryFetchLimitUsesConfiguredValueAndDefault(t *testing.T) {
	if got := discoveryFetchLimit(&db.ProxyDiscoverySource{FetchLimit: 17}); got != 17 {
		t.Fatalf("configured fetch limit: got %d", got)
	}
	for _, source := range []*db.ProxyDiscoverySource{
		nil,
		{FetchLimit: 0},
		{FetchLimit: minProxyDiscoveryFetchLimit - 1},
		{FetchLimit: maxProxyDiscoveryFetchLimit + 1},
	} {
		if got := discoveryFetchLimit(source); got != defaultProxyDiscoveryFetchLimit {
			t.Fatalf("default fetch limit for %#v: got %d", source, got)
		}
	}
}

func TestRestrictedDiscoveryRedirect(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		allowed bool
	}{
		{name: "quake canonical host", target: "https://quake.360.net/api/v3/search/quake_service", allowed: true},
		{name: "quake legacy host", target: "https://quake.360.cn/api/v3/search/quake_service", allowed: true},
		{name: "fofa host", target: "https://fofa.info/api/v1/search/all", allowed: true},
		{name: "plain http", target: "http://quake.360.net/api/v3/search/quake_service", allowed: false},
		{name: "external host", target: "https://example.com/api", allowed: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target, err := url.Parse(test.target)
			if err != nil {
				t.Fatal(err)
			}
			req := &http.Request{URL: target}
			err = restrictedDiscoveryRedirect(req, []*http.Request{{URL: target}})
			if test.allowed && err != nil {
				t.Fatalf("redirect rejected: %v", err)
			}
			if !test.allowed && err == nil {
				t.Fatal("redirect was accepted")
			}
		})
	}
	if err := restrictedDiscoveryRedirect(&http.Request{URL: mustParseDiscoveryURL(t, "https://quake.360.net/second")}, []*http.Request{
		{URL: mustParseDiscoveryURL(t, "https://quake.360.net/first")},
		{URL: mustParseDiscoveryURL(t, "https://quake.360.net/second")},
		{URL: mustParseDiscoveryURL(t, "https://quake.360.net/third")},
	}); err == nil {
		t.Fatal("fourth redirect was accepted")
	}
}

func mustParseDiscoveryURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	value, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
