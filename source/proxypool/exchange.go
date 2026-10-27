package proxypool

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const MaxCaptureBody = 2 << 20

var errRouteUnavailable = errors.New("task proxy unavailable")
var errRouteAudit = errors.New("route audit unavailable")

// Exchange capture fields are final only after Response.Body.Close.
type Exchange struct {
	Response      *http.Response
	TrafficID     string
	CaptureStatus string
}

type captureBody struct {
	io.ReadCloser
	mu                    sync.Mutex
	data                  []byte
	eof, failed, overflow bool
}

func (b *captureBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.mu.Lock()
	defer b.mu.Unlock()
	room := MaxCaptureBody - len(b.data)
	if n > room {
		b.overflow = true
	}
	b.data = append(b.data, p[:min(n, room)]...)
	b.eof = b.eof || err == io.EOF
	b.failed = b.failed || (err != nil && err != io.EOF)
	return n, err
}

func (b *captureBody) snapshot() ([]byte, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.data...), b.eof && !b.failed && !b.overflow
}

type completedBody struct {
	io.ReadCloser
	once   sync.Once
	mu     sync.Mutex
	eof    bool
	failed bool
	finish func()
	err    error
}

func (b *completedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.mu.Lock()
	b.eof = b.eof || err == io.EOF
	b.failed = b.failed || (err != nil && err != io.EOF)
	b.mu.Unlock()
	return n, err
}

func (b *completedBody) complete() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.eof && !b.failed && b.err == nil
}

func (b *completedBody) Close() error {
	b.once.Do(func() {
		err := b.ReadCloser.Close()
		b.mu.Lock()
		b.err = err
		b.mu.Unlock()
		b.finish()
	})
	return b.err
}

// RoundTrip is for trusted in-process callers with a server-owned task ID.
// External callers must pass through ServeHTTP authentication.
func (g *Gateway) RoundTrip(r *http.Request, taskID int64) (*Exchange, error) {
	for attempt := 0; attempt < 3; attempt++ {
		ex, retry, err := g.roundTripAttempt(r, taskID)
		if !retry || attempt == 2 {
			return ex, err
		}
	}
	return nil, errRouteUnavailable
}

func (g *Gateway) roundTripAttempt(r *http.Request, taskID int64) (*Exchange, bool, error) {
	if g == nil || g.Resolve == nil || r.URL == nil || r.URL.Hostname() == "" ||
		(r.URL.Scheme != "http" && r.URL.Scheme != "https") || r.URL.User != nil ||
		len(r.URL.Host) > 255 || strings.ContainsAny(r.URL.Host, "\r\n") {
		return nil, false, errors.New("task proxy unavailable or invalid HTTP target")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	route, err := g.selectRoute(ctx, taskID)
	if err != nil || route.Upstream == nil {
		cancel()
		return nil, false, errRouteUnavailable
	}
	if g.Audit != nil && g.Audit(route, r.URL.Host, "accepted") != nil {
		cancel()
		return nil, false, errRouteAudit
	}
	g.watchRoute(ctx, cancel, route)
	ex := &Exchange{CaptureStatus: "disabled"}
	finish := func(result string) {
		cancel()
		if g.Complete != nil {
			if g.Complete(route, r.URL.Host, result, ex.TrafficID) != nil {
				ex.CaptureStatus = "audit_failed"
			}
		} else if g.Audit != nil {
			if g.Audit(route, r.URL.Host, result) != nil {
				ex.CaptureStatus = "audit_failed"
			}
		}
	}
	req := r.Clone(ctx)
	req.RequestURI = ""
	removeHopHeaders(req.Header)
	tr := proxyTransport(route.Upstream)
	if g.TLSRoots != nil {
		tr.TLSClientConfig = &tls.Config{RootCAs: g.TLSRoots, MinVersion: tls.VersionTLS12}
	}
	// Preserve compressed bytes and headers together in evidence.
	tr.DisableCompression = true
	capture := g.Capture != nil && g.CaptureEnabled != nil && g.CaptureEnabled()
	g.captureOnce.Do(func() { g.captureSlots = make(chan struct{}, 8) })
	release := func() {}
	if capture {
		select {
		case g.captureSlots <- struct{}{}:
			release = func() { <-g.captureSlots }
		default:
			capture = false
			ex.CaptureStatus = "busy"
		}
	}
	var requestBody *captureBody
	if capture {
		ex.CaptureStatus = "incomplete_or_oversized"
		requestBody = &captureBody{ReadCloser: req.Body}
		if req.Body == nil || req.Body == http.NoBody {
			requestBody.eof = true
		} else {
			req.Body = requestBody
		}
	}
	resp, err := tr.RoundTrip(req)
	if err != nil {
		release()
		tr.CloseIdleConnections()
		quarantined := g.reportProxyFailure(ctx, route, err)
		var dial *proxyDialError
		// Restrict replay to bodyless read requests that never connected.
		retry := quarantined && route.Strategy == "round_robin" && errors.As(err, &dial) &&
			(r.Method == http.MethodGet || r.Method == http.MethodHead) &&
			(r.Body == nil || r.Body == http.NoBody)
		finish("failed")
		return nil, retry, errors.New("upstream proxy request failed")
	}
	if resp.StatusCode == http.StatusProxyAuthRequired && r.URL.Scheme == "http" {
		g.reportProxyFailure(ctx, route, errProxyAuthentication)
	}
	ex.Response = resp
	var responseBody *captureBody
	if capture {
		responseBody = &captureBody{ReadCloser: resp.Body}
		resp.Body = responseBody
	}
	body := &completedBody{ReadCloser: resp.Body}
	body.finish = func() {
		defer release()
		defer tr.CloseIdleConnections()
		result := "http_" + strconv.Itoa(resp.StatusCode)
		if ctx.Err() != nil {
			result = "interrupted"
		} else if !body.complete() {
			result = "response_incomplete"
		}
		if capture {
			reqBytes, reqComplete := requestBody.snapshot()
			respBytes, respComplete := responseBody.snapshot()
			if reqComplete && respComplete && ctx.Err() == nil && len(req.Trailer) == 0 && len(resp.Trailer) == 0 {
				ex.TrafficID, err = g.Capture(route, req, resp, reqBytes, respBytes)
				ex.CaptureStatus = "captured"
				if err != nil || ex.TrafficID == "" {
					ex.TrafficID = ""
					ex.CaptureStatus = "storage_failed"
				}
			}
			result += ":" + ex.CaptureStatus
		} else if ex.CaptureStatus == "busy" {
			result += ":capture_busy"
		}
		finish(result)
	}
	resp.Body = body
	return ex, false, nil
}
