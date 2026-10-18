package proxypool

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

type verificationStage int32

const (
	verifyTCP verificationStage = iota
	verifyHandshake
	verifyTLS
	verifyResponse
	verifyBody
)

func (s verificationStage) String() string {
	return [...]string{"tcp", "handshake", "tls", "response", "body"}[s]
}

type verificationFailure struct {
	stage   verificationStage
	code    string
	message string
}

func IsVerificationTimeout(err error) bool {
	var failure *verificationFailure
	return errors.As(err, &failure) && failure.code == "timeout"
}

func (e *verificationFailure) Error() string {
	return fmt.Sprintf("[proxy_verify:%s:%s] proxy verification %s", e.stage, e.code, e.message)
}

func verificationProblem(stage verificationStage, code, message string) error {
	return &verificationFailure{stage: stage, code: code, message: message}
}

func Verify(ctx context.Context, upstream *url.URL, endpoint string) (string, error) {
	return verifyWithTransport(ctx, upstream, endpoint, nil)
}

// A transport override is used only by local TLS fixtures to trust their CA.
func verifyWithTransport(ctx context.Context, upstream *url.URL, endpoint string, tlsConfig *tls.Config) (string, error) {
	if upstream == nil || upstream.Host == "" ||
		(upstream.Scheme != "http" && upstream.Scheme != "https" && upstream.Scheme != "socks5") {
		return "", errors.New("verification requires an HTTP, HTTPS or SOCKS5 proxy")
	}
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Host == "" {
		return "", errors.New("configure a verification URL first")
	}
	var stage atomic.Int32
	tr := proxyTransport(upstream)
	tr.TLSClientConfig = tlsConfig
	dial := tr.DialContext
	tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := dial(ctx, network, address)
		if err == nil {
			next := verifyHandshake
			if upstream.Scheme == "http" && u.Scheme == "http" {
				next = verifyResponse
			}
			stage.Store(int32(next))
		}
		return conn, err
	}
	tr.OnProxyConnectResponse = func(_ context.Context, _ *url.URL, _ *http.Request, resp *http.Response) error {
		if resp.StatusCode == http.StatusProxyAuthRequired {
			return verificationProblem(verifyHandshake, "auth", "HTTP proxy authentication required or rejected (HTTP 407)")
		}
		if resp.StatusCode != http.StatusOK {
			return verificationProblem(verifyHandshake, "connect_rejected", fmt.Sprintf("HTTP proxy rejected CONNECT (HTTP %d)", resp.StatusCode))
		}
		return nil
	}
	defer tr.CloseIdleConnections()
	trace := &httptrace.ClientTrace{
		TLSHandshakeStart: func() { stage.Store(int32(verifyTLS)) },
		TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
			if err == nil {
				stage.Store(int32(verifyHandshake))
			}
		},
		GotConn: func(httptrace.GotConnInfo) { stage.Store(int32(verifyResponse)) },
	}
	client := &http.Client{Transport: tr, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), "GET", u.String(), nil)
	if err != nil {
		return "", errors.New("invalid verification request")
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", verificationErrorAt(err, verificationStage(stage.Load()))
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusProxyAuthRequired {
		return "", verificationProblem(verifyHandshake, "auth", "HTTP proxy authentication required or rejected (HTTP 407)")
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return "", verificationProblem(verifyResponse, "redirect", fmt.Sprintf("endpoint redirected (HTTP %d); configure the final URL", resp.StatusCode))
	}
	if resp.StatusCode != http.StatusOK {
		return "", verificationProblem(verifyResponse, "http_status", fmt.Sprintf("endpoint returned HTTP %d", resp.StatusCode))
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4097))
	if err != nil {
		return "", verificationErrorAt(err, verifyBody)
	}
	if len(b) > 4096 {
		return "", verificationProblem(verifyBody, "oversized", "endpoint response exceeds 4096 bytes")
	}
	ip := strings.TrimSpace(string(b))
	if net.ParseIP(ip) != nil {
		return ip, nil
	}
	var payload struct {
		IP     string `json:"ip"`
		Origin string `json:"origin"`
	}
	if json.Unmarshal(b, &payload) == nil {
		for _, value := range []string{payload.IP, payload.Origin} {
			value = strings.TrimSpace(value)
			if net.ParseIP(value) != nil {
				return value, nil
			}
		}
	}
	return "", verificationProblem(verifyBody, "invalid_ip", "endpoint must return one IP address as plain text or JSON ip/origin")
}

// Raw network errors may contain credentials, endpoint secrets or untrusted
// proxy response text. Persist only fixed descriptions and numeric HTTP codes.
func verificationErrorAt(err error, stage verificationStage) error {
	var failure *verificationFailure
	if errors.As(err, &failure) {
		return failure
	}
	var networkError net.Error
	var certificateError *tls.CertificateVerificationError
	switch {
	case errors.Is(err, context.Canceled):
		return verificationProblem(stage, "canceled", "canceled")
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &networkError) && networkError.Timeout():
		return verificationProblem(stage, "timeout", "timed out")
	case errors.As(err, &certificateError):
		return verificationProblem(verifyTLS, "certificate", "TLS certificate validation failed")
	}
	// SOCKS destination failures must not be mistaken for refusing the initial
	// TCP connection to the proxy.
	raw := strings.ToLower(err.Error())
	if strings.Contains(raw, "socks") {
		switch {
		case strings.Contains(raw, "authentication"), strings.Contains(raw, "username/password"):
			return verificationProblem(stage, "auth", "SOCKS5 authentication required or rejected")
		case strings.Contains(raw, "unexpected protocol version"), strings.Contains(raw, "non-zero reserved field"),
			strings.Contains(raw, "unknown address type"), strings.Contains(raw, "unsupported authentication method"):
			return verificationProblem(stage, "socks_protocol", "SOCKS5 returned a malformed or unsupported protocol response")
		case strings.Contains(raw, "unknown error"):
			return verificationProblem(stage, "target_rejected", "SOCKS5 proxy could not connect to the verification target")
		}
	}
	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		return verificationProblem(stage, "refused", "connection refused")
	case errors.Is(err, syscall.ECONNRESET):
		return verificationProblem(stage, "reset", "connection reset")
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return verificationProblem(stage, "closed", "connection closed before verification completed")
	case strings.Contains(raw, "socks"):
		return verificationProblem(stage, "socks_protocol", "SOCKS5 negotiation or connection failed")
	default:
		return verificationProblem(stage, "connection", "connection failed")
	}
}

func verificationConnectionError(err error) error {
	return verificationErrorAt(err, verifyTCP)
}
