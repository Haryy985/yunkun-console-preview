package proxypool

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

type Route struct {
	Strategy      string
	CredentialTag [32]byte
	Revision      int64
	TaskID        int64
	PoolID        int64
	Node          Node
	Upstream      *url.URL
}

type Gateway struct {
	Key            []byte
	Context        context.Context
	Resolve        func(context.Context, int64) (Route, error)
	Select         func(context.Context, int64) (Route, error)
	Validate       func(context.Context, Route) error
	Failure        func(Route, string) error
	Audit          func(Route, string, string) error
	CaptureEnabled func() bool
	Capture        func(Route, *http.Request, *http.Response, []byte, []byte) (string, error)
	Complete       func(Route, string, string, string) error
	// TLSRoots optionally supplies trusted private CAs; verification stays on.
	TLSRoots     *x509.CertPool
	captureOnce  sync.Once
	captureSlots chan struct{}
}

func (g *Gateway) Token(taskID int64) string {
	mac := hmac.New(sha256.New, g.Key)
	fmt.Fprintf(mac, "proxy-task:%d", taskID)
	return hex.EncodeToString(mac.Sum(nil))
}

func (g *Gateway) URL(base string, taskID int64) string {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return ""
	}
	u.User = url.UserPassword(strconv.FormatInt(taskID, 10), g.Token(taskID))
	return u.String()
}

func (g *Gateway) authenticated(r *http.Request) (int64, bool) {
	raw := r.Header.Get("Proxy-Authorization")
	if !strings.HasPrefix(raw, "Basic ") {
		return 0, false
	}
	b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(raw, "Basic "))
	if err != nil {
		return 0, false
	}
	user, token, ok := strings.Cut(string(b), ":")
	id, err := strconv.ParseInt(user, 10, 64)
	return id, ok && err == nil && id > 0 && len(g.Key) >= 32 && hmac.Equal([]byte(token), []byte(g.Token(id)))
}

// Wrap uses the existing application listener. A request cannot reach the
// forwarding path without task-scoped proxy credentials.
func (g *Gateway) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect || r.URL.IsAbs() {
			g.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func proxyTransport(upstream *url.URL) *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyURL(upstream),
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			conn, err := (&net.Dialer{Timeout: 15 * time.Second}).DialContext(ctx, network, address)
			if err != nil && ctx.Err() == nil {
				return nil, &proxyDialError{err}
			}
			return conn, err
		},
		TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 30 * time.Second,
		DisableKeepAlives: true,
	}
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := g.authenticated(r)
	if !ok {
		w.Header().Set("Proxy-Authenticate", `Basic realm="Yunkun task egress"`)
		http.Error(w, "task proxy authentication required", http.StatusProxyAuthRequired)
		return
	}
	if r.Method != http.MethodConnect {
		exchange, err := g.RoundTrip(r, id)
		if err != nil {
			status := http.StatusBadGateway
			if errors.Is(err, errRouteUnavailable) || errors.Is(err, errRouteAudit) {
				status = http.StatusServiceUnavailable
			}
			http.Error(w, "task proxy request failed; direct fallback forbidden", status)
			return
		}
		resp := exchange.Response
		defer resp.Body.Close()
		removeHopHeaders(resp.Header)
		for k, values := range resp.Header {
			for _, v := range values {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
		return
	}
	route, err := g.selectRoute(r.Context(), id)
	if err != nil || route.Upstream == nil {
		http.Error(w, "task proxy unavailable; direct fallback forbidden", http.StatusServiceUnavailable)
		return
	}
	target := r.URL.Host
	if r.Method == http.MethodConnect {
		target = r.Host
	}
	if len(target) > 255 || strings.ContainsAny(target, "\r\n") {
		http.Error(w, "invalid target", 400)
		return
	}
	if g.Audit != nil && g.Audit(route, target, "accepted") != nil {
		http.Error(w, "route audit unavailable", 503)
		return
	}
	result := "failed"
	defer func() {
		if g.Audit != nil {
			_ = g.Audit(route, target, result)
		}
	}()
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	if _, _, err := net.SplitHostPort(target); err != nil {
		http.Error(w, "CONNECT needs host:port", 400)
		return
	}
	var upstream net.Conn
	var connectionCtx context.Context
	for attempt := 0; attempt < 3; attempt++ {
		var connectionCancel context.CancelFunc
		connectionCtx, connectionCancel = context.WithCancel(ctx)
		g.watchRoute(connectionCtx, connectionCancel, route)
		upstream, err = dialTunnel(connectionCtx, route.Upstream, target)
		if err == nil {
			defer connectionCancel()
			break
		}
		quarantined := g.reportProxyFailure(connectionCtx, route, err)
		connectionCancel()
		var dial *proxyDialError
		if !quarantined || route.Strategy != "round_robin" || !errors.As(err, &dial) || attempt == 2 {
			http.Error(w, "upstream proxy connection failed", 502)
			return
		}
		if g.Audit != nil && g.Audit(route, target, "connect_failed") != nil {
			http.Error(w, "route audit unavailable", 503)
			return
		}
		route, err = g.selectRoute(ctx, id)
		if err != nil || route.Upstream == nil {
			http.Error(w, "task proxy unavailable; direct fallback forbidden", 503)
			return
		}
		if g.Audit != nil && g.Audit(route, target, "accepted") != nil {
			http.Error(w, "route audit unavailable", 503)
			return
		}
	}
	defer upstream.Close()
	if connectionCtx.Err() != nil || g.validateRoute(connectionCtx, route) != nil {
		http.Error(w, "task proxy route revoked", 503)
		return
	}
	if r.Method == http.MethodConnect {
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "tunneling unavailable", 500)
			return
		}
		client, buffered, err := hj.Hijack()
		if err != nil {
			return
		}
		defer client.Close()
		closeOnCancel := context.AfterFunc(connectionCtx, func() { _ = client.Close(); _ = upstream.Close() })
		defer closeOnCancel()
		if _, err := buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			return
		}
		if err := buffered.Flush(); err != nil {
			return
		}
		done := make(chan struct{})
		go func() {
			_, _ = io.Copy(upstream, buffered)
			_ = upstream.Close()
			close(done)
		}()
		_, _ = io.Copy(client, upstream)
		_ = client.Close()
		_ = upstream.Close()
		<-done
		result = "tunnel_closed"
		if connectionCtx.Err() != nil {
			result = "interrupted"
		}
		return
	}
}

func removeHopHeaders(h http.Header) {
	for _, v := range h.Values("Connection") {
		for _, key := range strings.Split(v, ",") {
			h.Del(strings.TrimSpace(key))
		}
	}
	for _, k := range []string{"Proxy-Authorization", "Proxy-Authenticate", "Proxy-Connection", "Connection", "Keep-Alive", "TE", "Trailer", "Transfer-Encoding", "Upgrade"} {
		h.Del(k)
	}
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(b []byte) (int, error) { return c.reader.Read(b) }

func dialTunnel(ctx context.Context, u *url.URL, target string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	if u.Scheme == "socks5" {
		var auth *proxy.Auth
		if u.User != nil {
			pw, _ := u.User.Password()
			auth = &proxy.Auth{User: u.User.Username(), Password: pw}
		}
		d, err := proxy.SOCKS5("tcp", u.Host, auth, &markedProxyDialer{dialer})
		if err != nil {
			return nil, errors.New("invalid SOCKS proxy")
		}
		cd, ok := d.(proxy.ContextDialer)
		if !ok {
			return nil, errors.New("SOCKS context support unavailable")
		}
		return cd.DialContext(ctx, "tcp", target)
	}
	var conn net.Conn
	var err error
	if u.Scheme == "https" {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: &tls.Config{ServerName: u.Hostname(), MinVersion: tls.VersionTLS12}}).DialContext(ctx, "tcp", u.Host)
	} else if u.Scheme == "http" {
		conn, err = dialer.DialContext(ctx, "tcp", u.Host)
	} else {
		return nil, errors.New("unsupported proxy")
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &proxyDialError{err}
	}
	ok := false
	defer func() {
		if !ok {
			_ = conn.Close()
		}
	}()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	req := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: target}, Host: target, Header: make(http.Header)}
	if u.User != nil {
		pw, _ := u.User.Password()
		req.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(u.User.Username()+":"+pw)))
	}
	if err := req.Write(conn); err != nil {
		return nil, errors.New("proxy CONNECT failed")
	}
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, req)
	if err != nil {
		return nil, errors.New("proxy CONNECT response invalid")
	}
	if resp.StatusCode != 200 {
		if resp.StatusCode == http.StatusProxyAuthRequired {
			return nil, errProxyAuthentication
		}
		return nil, errors.New("proxy rejected CONNECT")
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	_ = conn.SetDeadline(time.Time{})
	ok = true
	return &bufferedConn{Conn: conn, reader: reader}, nil
}
