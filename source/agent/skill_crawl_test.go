package agent

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"yunkun/console/db"
)

func TestCrawlWebSurfaceStaysSameOriginAndDoesNotSubmitForms(t *testing.T) {
	var externalRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<html><a href="/page">page</a><a href="http://example.invalid/out">external</a><script src="/assets/app.js"></script></html>`))
		case "/page":
			_, _ = w.Write([]byte(`<html><form method="post" action="/api/login"><input name="user"></form></html>`))
		case "/assets/app.js":
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = w.Write([]byte(`fetch("/api/v1/profile")`))
		default:
			externalRequests.Add(1)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	parsed, err := parseScopedCrawlURL(server.URL+"/", []db.TaskScope{{Kind: "ip", Net: "127.0.0.1/32"}})
	if err != nil {
		t.Fatal(err)
	}
	summary, err := crawlWebSurface(context.Background(), parsed, []db.TaskScope{{Kind: "ip", Net: "127.0.0.1/32"}}, 5, 2)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Pages != 2 || summary.Requests != 3 {
		t.Fatalf("pages=%d requests=%d failures=%v; want 2 and 3", summary.Pages, summary.Requests, summary.Failures)
	}
	if externalRequests.Load() != 0 {
		t.Fatalf("crawler made %d unexpected requests", externalRequests.Load())
	}
	if !containsString(summary.APIPaths, "/api/login") || !containsString(summary.APIPaths, "/api/v1/profile") {
		t.Fatalf("API paths missing form or JavaScript evidence: %v", summary.APIPaths)
	}
	if len(summary.Forms) != 1 || summary.Forms[0].Method != "POST" || summary.Forms[0].Action != server.URL+"/api/login" {
		t.Fatalf("unexpected forms: %+v", summary.Forms)
	}
	for _, link := range summary.Links {
		if strings.Contains(link, "example.invalid") {
			t.Fatalf("out-of-origin link was returned: %s", link)
		}
	}
}

func TestParseScopedCrawlURLRejectsCredentialsAndOutOfScopeHosts(t *testing.T) {
	scopes := []db.TaskScope{{Kind: "root_domain", Domain: "example.test"}}
	for _, raw := range []string{
		"https://user:pass@example.test/",
		"https://example.test/?access_token=secret",
		"https://outside.test/",
		"file:///etc/passwd",
	} {
		if _, err := parseScopedCrawlURL(raw, scopes); err == nil {
			t.Errorf("unsafe crawl URL accepted: %s", raw)
		}
	}
}

func TestScopedCrawlerTransportRequiresExplicitPrivateIPScope(t *testing.T) {
	privateIP := net.ParseIP("127.0.0.1")
	if !privateIP.IsLoopback() {
		t.Fatal("test fixture is not loopback")
	}
	transport := scopedCrawlerTransport([]db.TaskScope{{Kind: "root_domain", Domain: "example.test"}})
	defer transport.CloseIdleConnections()
	_, err := transport.DialContext(context.Background(), "tcp", net.JoinHostPort(privateIP.String(), "80"))
	if err == nil || !strings.Contains(err.Error(), "授权边界") {
		t.Fatalf("loopback was not rejected without explicit IP scope: %v", err)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestCrawlFailureSummaryDoesNotEchoQueryCredentials(t *testing.T) {
	const sentinel = "crawl-secret-sentinel"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "failure", http.StatusInternalServerError)
	}))
	defer server.Close()
	start, err := parseScopedCrawlURL(server.URL+"/?id="+sentinel, []db.TaskScope{{Kind: "ip", Net: "127.0.0.1/32"}})
	if err != nil {
		t.Fatal(err)
	}
	summary, err := crawlWebSurface(context.Background(), start, []db.TaskScope{{Kind: "ip", Net: "127.0.0.1/32"}}, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fmt.Sprint(summary.Failures), sentinel) {
		t.Fatal("failure summary leaked the URL query string")
	}
}
