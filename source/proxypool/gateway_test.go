package proxypool

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGatewayTaskIsolationAndNoFallback(t *testing.T) {
	var direct atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { direct.Add(1); _, _ = w.Write([]byte("direct")) }))
	defer target.Close()
	proxies := []*httptest.Server{}
	for _, label := range []string{"one", "two"} {
		proxies = append(proxies, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Proxy-Authorization") != "" {
				t.Error("task credentials forwarded")
			}
			_, _ = w.Write([]byte(label))
		})))
	}
	defer proxies[0].Close()
	defer proxies[1].Close()
	var disabled atomic.Bool
	var audited atomic.Int32
	g := &Gateway{Key: []byte(strings.Repeat("k", 32)), Resolve: func(_ context.Context, id int64) (Route, error) {
		if disabled.Load() {
			return Route{}, errors.New("disabled")
		}
		if id < 1 || id > 2 {
			return Route{}, errors.New("unknown")
		}
		u, _ := url.Parse(proxies[id-1].URL)
		return Route{TaskID: id, Node: Node{ID: id}, Upstream: u}, nil
	}, Audit: func(Route, string, string) error { audited.Add(1); return nil }}
	gateway := httptest.NewServer(g.Wrap(http.NotFoundHandler()))
	defer gateway.Close()
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := int64(i%2 + 1)
			u, _ := url.Parse(g.URL(gateway.URL, id))
			tr := proxyTransport(u)
			defer tr.CloseIdleConnections()
			resp, err := (&http.Client{Transport: tr, Timeout: 3 * time.Second}).Get(target.URL)
			if err != nil {
				t.Error(err)
				return
			}
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			if string(b) != []string{"one", "two"}[id-1] {
				t.Errorf("wrong route: %s", b)
			}
		}()
	}
	wg.Wait()
	if direct.Load() != 0 || audited.Load() != 24 {
		t.Fatalf("direct=%d audit=%d", direct.Load(), audited.Load())
	}
	u, _ := url.Parse(g.URL(gateway.URL, 1))
	tr := proxyTransport(u)
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 3 * time.Second}
	proxies[0].Close()
	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 502 || direct.Load() != 0 {
		t.Fatal("proxy failure reached target")
	}
	disabled.Store(true)
	resp, err = client.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 503 || direct.Load() != 0 {
		t.Fatal("disabled route reached target")
	}
	unauth, _ := url.Parse(gateway.URL)
	unauthTr := proxyTransport(unauth)
	defer unauthTr.CloseIdleConnections()
	resp, err = (&http.Client{Transport: unauthTr}).Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 407 {
		t.Fatal("open proxy")
	}
}

func TestGatewayCONNECTAndRevocation(t *testing.T) {
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Proxy-Authorization") != "" {
			t.Error("proxy credentials leaked to target")
		}
		_, _ = w.Write([]byte("tunneled"))
	}))
	defer target.Close()
	targetURL, _ := url.Parse(target.URL)
	var connects atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Proxy-Authorization") != "Basic "+base64.StdEncoding.EncodeToString([]byte("app-user:app-password")) {
			t.Error("missing upstream application authentication")
			http.Error(w, "authentication required", 407)
			return
		}
		if r.Method != "CONNECT" || r.Host != targetURL.Host {
			http.Error(w, "bad target", 400)
			return
		}
		connects.Add(1)
		conn, err := net.DialTimeout("tcp", targetURL.Host, time.Second)
		if err != nil {
			http.Error(w, "failed", 502)
			return
		}
		client, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			_ = conn.Close()
			return
		}
		defer conn.Close()
		defer client.Close()
		_, _ = rw.WriteString("HTTP/1.1 200 OK\r\n\r\n")
		_ = rw.Flush()
		done := make(chan struct{})
		go func() { _, _ = io.Copy(conn, rw); _ = conn.Close(); close(done) }()
		_, _ = io.Copy(client, conn)
		_ = client.Close()
		<-done
	}))
	defer upstream.Close()
	u, err := (Config{Kind: "xiaoxiang"}).Upstream(Node{Address: upstream.URL}, Secrets{AppKey: "app-user", AppSecret: "app-password"})
	if err != nil {
		t.Fatal(err)
	}
	var disabled atomic.Bool
	var revision atomic.Int64
	g := &Gateway{Key: []byte(strings.Repeat("x", 32)), Resolve: func(context.Context, int64) (Route, error) {
		if disabled.Load() {
			return Route{}, errors.New("revoked")
		}
		return Route{Node: Node{ID: 1}, Upstream: u, Revision: revision.Load()}, nil
	}}
	gateway := httptest.NewServer(g)
	defer gateway.Close()
	proxyURL, _ := url.Parse(g.URL(gateway.URL, 1))
	tr := proxyTransport(proxyURL)
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // local fixture certificate only
	defer tr.CloseIdleConnections()
	resp, err := (&http.Client{Transport: tr, Timeout: 3 * time.Second}).Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(b) != "tunneled" || connects.Load() != 1 {
		t.Fatal("CONNECT did not use upstream")
	}

	conn, err := dialTunnel(context.Background(), proxyURL, targetURL.Host)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	disabled.Store(true)
	_ = conn.SetReadDeadline(time.Now().Add(4 * time.Second))
	_, err = conn.Read(make([]byte, 1))
	if err == nil {
		t.Fatal("revoked tunnel stayed open")
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatal("revocation did not interrupt tunnel")
	}
	disabled.Store(false)
	renewed, err := dialTunnel(context.Background(), proxyURL, targetURL.Host)
	if err != nil {
		t.Fatal(err)
	}
	defer renewed.Close()
	revision.Add(1)
	_ = renewed.SetReadDeadline(time.Now().Add(4 * time.Second))
	_, err = renewed.Read(make([]byte, 1))
	if err == nil {
		t.Fatal("reassigned lease kept old tunnel")
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatal("same-node reassignment did not interrupt old tunnel")
	}
}

func TestVerificationNeverInventsIP(t *testing.T) {
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ip":
			_, _ = w.Write([]byte("203.0.113.25\n"))
		default:
			_, _ = w.Write([]byte("not an IP"))
		}
	}))
	defer proxyServer.Close()
	u, _ := url.Parse(proxyServer.URL)
	ip, err := Verify(context.Background(), u, "http://does-not-resolve.invalid/ip")
	if err != nil || ip != "203.0.113.25" {
		t.Fatalf("verify: %s %v", ip, err)
	}
	if _, err := Verify(context.Background(), u, "http://does-not-resolve.invalid/invalid"); err == nil {
		t.Fatal("invented IP")
	}
	if _, err := Verify(context.Background(), nil, proxyServer.URL); err == nil {
		t.Fatal("direct verification permitted")
	}
}

func TestVerificationEchoFormats(t *testing.T) {
	for _, test := range []struct {
		name, body, want string
		status           int
	}{
		{name: "plain IPv4", body: " 203.0.113.25\n", want: "203.0.113.25"},
		{name: "plain IPv6", body: "2001:db8::1", want: "2001:db8::1"},
		{name: "JSON ip", body: `{"ip":"203.0.113.26"}`, want: "203.0.113.26"},
		{name: "JSON origin", body: `{"origin":" 203.0.113.27 "}`, want: "203.0.113.27"},
		{name: "JSON IPv6", body: `{"ip":"2001:db8::2"}`, want: "2001:db8::2"},
		{name: "multiple origins", body: `{"origin":"203.0.113.25, 203.0.113.26"}`},
		{name: "bad IP", body: `{"ip":"999.0.0.1"}`},
		{name: "not JSON", body: `{"ip":`},
		{name: "wrong field type", body: `{"ip":123}`},
		{name: "HTML", body: "<html>203.0.113.25</html>"},
		{name: "empty", body: ""},
		{name: "oversized", body: strings.Repeat(" ", 4097) + "203.0.113.25"},
		{name: "error response", body: "203.0.113.25", status: http.StatusBadGateway},
		{name: "redirect", body: "203.0.113.25", status: http.StatusTemporaryRedirect},
	} {
		t.Run(test.name, func(t *testing.T) {
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Host != "echo.invalid" {
					t.Errorf("unexpected proxy request: %s", r.URL)
				}
				if test.status != 0 {
					w.Header().Set("Location", "http://elsewhere.invalid/ip")
					w.WriteHeader(test.status)
				}
				_, _ = io.WriteString(w, test.body)
			}))
			defer proxy.Close()
			upstream, _ := url.Parse(proxy.URL)
			ip, err := Verify(context.Background(), upstream, "http://echo.invalid/ip")
			if test.want == "" {
				if err == nil || ip != "" {
					t.Fatalf("invalid response accepted: %q %v", ip, err)
				}
			} else if err != nil || ip != test.want {
				t.Fatalf("got %q %v, want %q", ip, err, test.want)
			}
		})
	}
}

func TestGatewayAuditFailureBlocksForward(t *testing.T) {
	var called atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called.Add(1) }))
	defer upstream.Close()
	u, _ := url.Parse(upstream.URL)
	g := &Gateway{Key: []byte(strings.Repeat("a", 32)), Resolve: func(context.Context, int64) (Route, error) { return Route{Upstream: u}, nil },
		Audit: func(Route, string, string) error { return errors.New("disk unavailable") }}
	server := httptest.NewServer(g)
	defer server.Close()
	p, _ := url.Parse(g.URL(server.URL, 1))
	tr := proxyTransport(p)
	defer tr.CloseIdleConnections()
	resp, err := (&http.Client{Transport: tr}).Get("http://example.invalid/" + strconv.Itoa(1))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 503 || called.Load() != 0 {
		t.Fatal("unaudited traffic permitted")
	}
}
