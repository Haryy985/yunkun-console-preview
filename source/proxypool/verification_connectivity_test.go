package proxypool

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestConnectivityVerificationHTMLAndFailures(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
		code   string
	}{
		{"web page", 200, "<html>" + strings.Repeat("page", 2048) + "</html>", ""},
		{"redirect", 302, "redirect", "response:redirect"},
		{"authentication", 407, "private-token", "handshake:auth"},
		{"forbidden", 403, "private-token", "response:http_status"},
		{"unavailable", 503, "private-token", "response:http_status"},
	} {
		t.Run(test.name, func(t *testing.T) {
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Host != "web.invalid" || r.Method != http.MethodGet {
					t.Errorf("verification did not travel through the configured proxy: %s", r.Method)
				}
				w.Header().Set("Location", "https://redirect.invalid/private-token")
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			defer proxy.Close()
			upstream, _ := url.Parse(proxy.URL)
			result, err := VerifyEndpoint(context.Background(), upstream, "http://web.invalid/?key=private-token", VerifyConnectivity)
			if result.IP != "" || result.Target != "http://web.invalid" || result.HTTPStatus != test.status || result.Mode != VerifyConnectivity {
				t.Fatalf("incorrect connectivity result: %+v", result)
			}
			if test.code == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.code) || strings.Contains(err.Error(), "private-token") {
				t.Fatalf("incorrect or unsafe error: %v", err)
			}
		})
	}
}

func TestConnectivityTimeoutAndNoDirectFallback(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer proxy.Close()
	upstream, _ := url.Parse(proxy.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := VerifyEndpoint(ctx, upstream, "http://web.invalid/", VerifyConnectivity)
	if !IsVerificationTimeout(err) {
		t.Fatalf("expected timeout, got %v", err)
	}

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("verification fell back to a direct connection")
	}))
	defer target.Close()
	proxy.Close()
	if _, err := VerifyEndpoint(context.Background(), upstream, target.URL, VerifyConnectivity); err == nil {
		t.Fatal("unreachable proxy was accepted")
	}
	if _, err := VerifyEndpoint(context.Background(), nil, target.URL, VerifyConnectivity); err == nil {
		t.Fatal("missing proxy was accepted")
	}
	if _, err := VerifyEndpoint(context.Background(), upstream, target.URL, "invalid"); err == nil {
		t.Fatal("invalid verification mode was accepted")
	}
}

func TestLiveBaiduConnectivity(t *testing.T) {
	if os.Getenv("YUNKUN_PROXY_LIVE_BAIDU") != "1" {
		t.Skip("set YUNKUN_PROXY_LIVE_BAIDU=1 for the optional public endpoint smoke test")
	}
	// Only this known public destination is allowed through the local fixture.
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != "www.baidu.com:443" {
			http.Error(w, "unexpected target", http.StatusForbidden)
			return
		}
		upstream, err := net.DialTimeout("tcp", r.Host, 10*time.Second)
		if err != nil {
			http.Error(w, "upstream unavailable", 502)
			return
		}
		defer upstream.Close()
		client, buffered, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer client.Close()
		_ = client.SetDeadline(time.Now().Add(25 * time.Second))
		_ = upstream.SetDeadline(time.Now().Add(25 * time.Second))
		_, _ = io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n")
		done := make(chan struct{})
		go func() { _, _ = io.Copy(upstream, buffered); _ = upstream.Close(); close(done) }()
		_, _ = io.Copy(client, upstream)
		_ = client.Close()
		<-done
	}))
	defer proxy.Close()
	upstream, _ := url.Parse(proxy.URL)
	result, err := VerifyEndpoint(context.Background(), upstream, BaiduVerifyURL, VerifyConnectivity)
	if err != nil || result.HTTPStatus != 200 || result.IP != "" {
		t.Fatalf("live Baidu verification failed: %+v %v", result, err)
	}
	t.Logf("proxy-routed Baidu HTTPS verification: HTTP %d, TLS certificate validated", result.HTTPStatus)
}
