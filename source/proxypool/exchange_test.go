package proxypool

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPRevocationInterruptsWithoutPublishingCapture(t *testing.T) {
	started := make(chan struct{})
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("start"))
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer proxy.Close()
	u, _ := url.Parse(proxy.URL)
	var revoked atomic.Bool
	g := &Gateway{
		Resolve: func(context.Context, int64) (Route, error) {
			if revoked.Load() {
				return Route{}, errors.New("revoked")
			}
			return Route{Upstream: u}, nil
		},
		CaptureEnabled: func() bool { return true },
		Capture: func(Route, *http.Request, *http.Response, []byte, []byte) (string, error) {
			t.Error("published interrupted exchange")
			return "bad", nil
		},
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://fixture.invalid/", nil)
	ex, err := g.RoundTrip(req, 1)
	if err != nil {
		t.Fatal(err)
	}
	<-started
	revoked.Store(true)
	_, err = io.ReadAll(ex.Response.Body)
	_ = ex.Response.Body.Close()
	if err == nil || ctx.Err() != nil || ex.TrafficID != "" || ex.CaptureStatus != "incomplete_or_oversized" {
		t.Fatalf("revocation did not interrupt correctly: %v %+v", err, ex)
	}
}

func TestCaptureConcurrencyLimitAndRelease(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("response"))
	}))
	defer proxy.Close()
	u, _ := url.Parse(proxy.URL)
	g := &Gateway{Resolve: func(context.Context, int64) (Route, error) { return Route{Upstream: u}, nil },
		CaptureEnabled: func() bool { return true },
		Capture:        func(Route, *http.Request, *http.Response, []byte, []byte) (string, error) { return "captured", nil }}
	var exchanges []*Exchange
	for i := 0; i < 9; i++ {
		req, _ := http.NewRequestWithContext(t.Context(), "GET", "http://fixture.invalid/", nil)
		ex, err := g.RoundTrip(req, 1)
		if err != nil {
			t.Fatal(err)
		}
		exchanges = append(exchanges, ex)
		t.Cleanup(func() { _ = ex.Response.Body.Close() })
	}
	if exchanges[8].CaptureStatus != "busy" {
		t.Fatal("capture memory not bounded")
	}
	for _, ex := range exchanges {
		_, _ = io.Copy(io.Discard, ex.Response.Body)
		_ = ex.Response.Body.Close()
		_ = ex.Response.Body.Close()
	}
	if len(g.captureSlots) != 0 {
		t.Fatal("capture slots leaked")
	}
	req, _ := http.NewRequestWithContext(t.Context(), "GET", "http://fixture.invalid/", nil)
	ex, err := g.RoundTrip(req, 1)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, ex.Response.Body)
	_ = ex.Response.Body.Close()
	if ex.CaptureStatus != "captured" {
		t.Fatal("capture did not recover")
	}
}

func TestAuthenticatedHTTPGatewayCapturesWithoutProxyCredentials(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Proxy-Authorization") != "" {
			t.Error("gateway credentials leaked")
		}
		_, _ = io.Copy(w, r.Body)
	}))
	defer proxy.Close()
	u, _ := url.Parse(proxy.URL)
	done := make(chan struct{})
	g := &Gateway{Key: []byte(strings.Repeat("k", 32)),
		Resolve:        func(_ context.Context, id int64) (Route, error) { return Route{TaskID: id, Upstream: u}, nil },
		CaptureEnabled: func() bool { return true },
		Capture: func(route Route, req *http.Request, _ *http.Response, request, response []byte) (string, error) {
			if route.TaskID != 7 || string(request) != "payload" || string(response) != "payload" || req.Header.Get("Proxy-Authorization") != "" {
				t.Error("incorrect authenticated capture")
			}
			close(done)
			return "capture-id", nil
		}}
	server := httptest.NewServer(g)
	defer server.Close()
	address, _ := url.Parse(g.URL(server.URL, 7))
	tr := proxyTransport(address)
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 3 * time.Second}
	resp, err := client.Post("http://fixture.invalid/proof", "text/plain", strings.NewReader("payload"))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("gateway capture missing")
	}
}
