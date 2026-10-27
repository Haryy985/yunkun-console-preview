package proxypool

import (
	"context"
	"errors"
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
)

func TestGatewayTruncatedResponseIsNotRuntimeSuccess(t *testing.T) {
	p := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.Write([]byte("truncated"))
	}))
	defer p.Close()
	u, _ := url.Parse(p.URL)
	var result string
	g := &Gateway{
		Resolve: func(context.Context, int64) (Route, error) {
			return Route{Strategy: "round_robin", Upstream: u}, nil
		},
		Validate: func(context.Context, Route) error { return nil },
		Failure: func(Route, string) error {
			t.Error("ambiguous response failure must not quarantine proxy")
			return nil
		},
		Complete: func(_ Route, _, outcome, _ string) error { result = outcome; return nil },
	}
	ex, err := g.RoundTrip(httptest.NewRequest("GET", "http://target.invalid/", nil), 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(ex.Response.Body); err == nil {
		t.Fatal("fixture should return truncated body")
	}
	ex.Response.Body.Close()
	if strings.HasPrefix(result, "http_") {
		t.Fatal("truncated response recorded as successful completion:", result)
	}
}

func TestGatewayRotationDoesNotRevokeConcurrentRequests(t *testing.T) {
	proxies := []*httptest.Server{}
	for _, label := range []string{"A", "B"} {
		p := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(1200 * time.Millisecond)
			w.Write([]byte(label))
		}))
		proxies = append(proxies, p)
		defer p.Close()
	}
	var count, validated atomic.Int32
	g := &Gateway{
		Resolve: func(context.Context, int64) (Route, error) {
			t.Error("validation used selecting resolver")
			return Route{}, errors.New("unexpected")
		},
		Select: func(_ context.Context, id int64) (Route, error) {
			n := int(count.Add(1)-1) % 2
			u, _ := url.Parse(proxies[n].URL)
			return Route{TaskID: id, Strategy: "round_robin", Node: Node{ID: int64(n + 1)}, Upstream: u}, nil
		},
		Validate: func(context.Context, Route) error { validated.Add(1); return nil },
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ex, err := g.RoundTrip(httptest.NewRequest("GET", "http://target.invalid/", nil), 1)
			if err != nil {
				t.Error(err)
				return
			}
			data, err := io.ReadAll(ex.Response.Body)
			ex.Response.Body.Close()
			if err != nil || (string(data) != "A" && string(data) != "B") {
				t.Error("in-flight request interrupted", err)
			}
		}()
	}
	wg.Wait()
	if count.Load() != 4 || validated.Load() < 8 {
		t.Fatal("selection/validation not separated", count.Load(), validated.Load())
	}
}

func TestGatewayFailureClassificationAndNoWriteReplay(t *testing.T) {
	for _, test := range []struct {
		name, method                    string
		status                          int
		dead                            bool
		expectedCalls, expectedFailures int
	}{
		{"connect-read", "GET", 0, true, 3, 3},
		{"connect-write", "POST", 0, true, 1, 1},
		{"target-forbidden", "GET", 403, false, 1, 0},
		{"target-error", "GET", 500, false, 1, 0},
		{"proxy-auth", "GET", 407, false, 1, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(test.status) }))
			defer p.Close()
			if test.dead {
				p.Close()
			}
			u, _ := url.Parse(p.URL)
			calls, failures := 0, 0
			g := &Gateway{
				Resolve: func(context.Context, int64) (Route, error) {
					calls++
					return Route{Strategy: "round_robin", Upstream: u}, nil
				},
				Validate: func(context.Context, Route) error { return nil },
				Failure:  func(Route, string) error { failures++; return nil },
			}
			var body io.Reader
			if test.method == "POST" {
				body = strings.NewReader("do-not-replay")
			}
			ex, err := g.RoundTrip(httptest.NewRequest(test.method, "http://target.invalid/", body), 1)
			if test.dead && err == nil {
				t.Fatal("dead proxy succeeded")
			}
			if !test.dead && err != nil {
				t.Fatal(err)
			}
			if ex != nil {
				io.Copy(io.Discard, ex.Response.Body)
				ex.Response.Body.Close()
			}
			if calls != test.expectedCalls || failures != test.expectedFailures {
				t.Fatal(calls, failures)
			}
		})
	}
}

func TestGatewayFixedFailureNeverSelectsAnotherNode(t *testing.T) {
	p := httptest.NewServer(http.NotFoundHandler())
	p.Close()
	u, _ := url.Parse(p.URL)
	calls := 0
	g := &Gateway{
		Resolve: func(context.Context, int64) (Route, error) {
			calls++
			return Route{Strategy: "fixed", Upstream: u}, nil
		},
		Validate: func(context.Context, Route) error { return nil },
		Failure:  func(Route, string) error { return nil },
	}
	if _, err := g.RoundTrip(httptest.NewRequest("GET", "http://target.invalid/", nil), 1); err == nil || calls != 1 {
		t.Fatal("fixed policy retried", calls, err)
	}
}

func TestGatewayCONNECTRotatesBeforeTunnelAndStillRevokes(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close()
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" {
			http.Error(w, "CONNECT only", 400)
			return
		}
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		rw.Flush()
		io.Copy(conn, rw)
	}))
	defer live.Close()
	var selected, failed atomic.Int32
	var revoked atomic.Bool
	g := &Gateway{Key: []byte(strings.Repeat("c", 32)),
		Resolve: func(context.Context, int64) (Route, error) {
			return Route{}, errors.New("unexpected selecting resolver")
		},
		Select: func(_ context.Context, id int64) (Route, error) {
			n := selected.Add(1)
			address := live.URL
			if n == 1 {
				address = deadURL
			}
			u, _ := url.Parse(address)
			return Route{TaskID: id, Strategy: "round_robin", Node: Node{ID: int64(n)}, Upstream: u}, nil
		},
		Validate: func(context.Context, Route) error {
			if revoked.Load() {
				return errors.New("revoked")
			}
			return nil
		},
		Failure: func(Route, string) error { failed.Add(1); return nil },
	}
	gateway := httptest.NewServer(g)
	defer gateway.Close()
	u, _ := url.Parse(g.URL(gateway.URL, 1))
	conn, err := dialTunnel(context.Background(), u, "target.invalid:443")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(4 * time.Second))
	if _, err := io.WriteString(conn, "hello"); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 5)
	if _, err := io.ReadFull(conn, b); err != nil || string(b) != "hello" {
		t.Fatal("tunnel failed", err)
	}
	if selected.Load() != 2 || failed.Load() != 1 {
		t.Fatal("CONNECT did not rotate")
	}
	revoked.Store(true)
	_, err = conn.Read(make([]byte, 1))
	if err == nil {
		t.Fatal("revoked tunnel remained open")
	}
	if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("revocation did not close tunnel")
	}
}

func TestGatewayCancellationDoesNotPoisonPool(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	g := &Gateway{Failure: func(Route, string) error { t.Fatal("cancellation classified as proxy fault"); return nil }}
	if g.reportProxyFailure(ctx, Route{}, &proxyDialError{context.Canceled}) {
		t.Fatal("cancellation allowed retry")
	}
}
