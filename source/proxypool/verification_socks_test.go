package proxypool

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestVerificationSOCKS5(t *testing.T) {
	for _, mode := range []string{"no authentication", "authenticated", "authentication rejected", "malformed reply", "target rejected", "handshake timeout"} {
		t.Run(mode, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			done := make(chan error, 1)
			go func() {
				done <- func() error {
					conn, err := listener.Accept()
					if err != nil {
						return err
					}
					defer conn.Close()
					_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
					head := make([]byte, 2)
					if _, err := io.ReadFull(conn, head); err != nil {
						return err
					}
					if head[0] != 5 {
						return fmt.Errorf("expected SOCKS5 greeting, got %v", head)
					}
					methods := make([]byte, int(head[1]))
					if _, err := io.ReadFull(conn, methods); err != nil {
						return err
					}
					if mode == "handshake timeout" {
						_, _ = io.Copy(io.Discard, conn)
						return nil
					}
					method := byte(0)
					if mode == "authenticated" || mode == "authentication rejected" {
						method = 2
					}
					greeting := []byte{5, method}
					if mode == "malformed reply" {
						// Reproduce a live candidate: a valid greeting followed by
						// a CONNECT response with version 0 and an invalid header.
						greeting = []byte{5, 0, 0, 0, 11, 8, 5, 26, 0}
					}
					if _, err := conn.Write(greeting); err != nil {
						return err
					}
					if mode == "malformed reply" {
						_, _ = io.Copy(io.Discard, conn)
						return nil
					}
					if method == 2 {
						if _, err := io.ReadFull(conn, head); err != nil {
							return err
						}
						user := make([]byte, int(head[1]))
						if _, err := io.ReadFull(conn, user); err != nil {
							return err
						}
						size := make([]byte, 1)
						if _, err := io.ReadFull(conn, size); err != nil {
							return err
						}
						password := make([]byte, int(size[0]))
						if _, err := io.ReadFull(conn, password); err != nil {
							return err
						}
						if head[0] != 1 || string(user) != "fixture-user" || string(password) != "fixture-secret" {
							return errors.New("SOCKS5 credentials were not sent correctly")
						}
						status := byte(0)
						if mode == "authentication rejected" {
							status = 1
						}
						if _, err := conn.Write([]byte{1, status}); err != nil {
							return err
						}
						if status != 0 {
							return nil
						}
					}
					request := make([]byte, 5)
					if _, err := io.ReadFull(conn, request); err != nil {
						return err
					}
					if request[0] != 5 || request[1] != 1 || request[3] != 3 {
						return fmt.Errorf("expected SOCKS5 CONNECT with remote DNS: %v", request)
					}
					target := make([]byte, int(request[4])+2)
					if _, err := io.ReadFull(conn, target); err != nil {
						return err
					}
					if string(target[:len(target)-2]) != "echo.invalid" || target[len(target)-2] != 0 || target[len(target)-1] != 80 {
						return errors.New("verification target was not passed to SOCKS5 proxy")
					}
					if mode == "target rejected" {
						_, err := conn.Write([]byte{5, 2, 0, 1, 0, 0, 0, 0, 0, 0})
						return err
					}
					if _, err := conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 80}); err != nil {
						return err
					}
					req, err := http.ReadRequest(bufio.NewReader(conn))
					if err != nil {
						return err
					}
					defer req.Body.Close()
					if req.Host != "echo.invalid" || req.URL.Path != "/ip" || req.Header.Get("Proxy-Authorization") != "" {
						return errors.New("incorrect verification request or leaked proxy credentials")
					}
					body := "198.51.100.42"
					_, err = fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body)
					return err
				}()
			}()
			upstream, _ := url.Parse("socks5://" + listener.Addr().String())
			if mode == "authenticated" || mode == "authentication rejected" {
				upstream.User = url.UserPassword("fixture-user", "fixture-secret")
			}
			timeout := 4 * time.Second
			if mode == "handshake timeout" {
				timeout = 150 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			ip, verifyErr := Verify(ctx, upstream, "http://echo.invalid/ip")
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			expectedCode := map[string]string{
				"authentication rejected": "auth",
				"malformed reply":         "socks_protocol",
				"target rejected":         "target_rejected",
				"handshake timeout":       "timeout",
			}[mode]
			if expectedCode != "" {
				if verifyErr == nil || ip != "" || !strings.Contains(verifyErr.Error(), "[proxy_verify:handshake:"+expectedCode+"]") ||
					strings.Contains(verifyErr.Error(), "fixture-secret") || strings.Contains(verifyErr.Error(), "echo.invalid") {
					t.Fatalf("incorrect or unsafe error: %q %v", ip, verifyErr)
				}
			} else if verifyErr != nil || ip != "198.51.100.42" {
				t.Fatalf("SOCKS5 verification: %q %v", ip, verifyErr)
			}
		})
	}
}

func TestVerificationHTTPSViaProxies(t *testing.T) {
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Proxy-Authorization") != "" {
			t.Error("proxy credentials reached the verification endpoint")
		}
		if r.URL.Path == "/page" {
			_, _ = io.WriteString(w, "<html>"+strings.Repeat("page", 2048)+"</html>")
		} else {
			_, _ = io.WriteString(w, `{"ip":"198.51.100.42"}`)
		}
	}))
	defer target.Close()
	roots := x509.NewCertPool()
	roots.AddCert(target.Certificate())
	targetURL, _ := url.Parse(target.URL)

	for _, protocol := range []string{"http", "socks5"} {
		for _, mode := range []string{VerifyExitIP, VerifyConnectivity, "untrusted-certificate"} {
			t.Run(protocol+"/"+mode, func(t *testing.T) {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer listener.Close()
				done := make(chan error, 1)
				go func() {
					done <- func() error {
						conn, err := listener.Accept()
						if err != nil {
							return err
						}
						defer conn.Close()
						_ = conn.SetDeadline(time.Now().Add(4 * time.Second))
						reader := bufio.NewReader(conn)
						if protocol == "http" {
							req, err := http.ReadRequest(reader)
							if err != nil {
								return err
							}
							defer req.Body.Close()
							if req.Method != http.MethodConnect || req.Host != targetURL.Host {
								return errors.New("incorrect CONNECT destination")
							}
						} else {
							head := make([]byte, 2)
							if _, err := io.ReadFull(reader, head); err != nil {
								return err
							}
							methods := make([]byte, int(head[1]))
							if _, err := io.ReadFull(reader, methods); err != nil {
								return err
							}
							if _, err := conn.Write([]byte{5, 0}); err != nil {
								return err
							}
							request := make([]byte, 10) // IPv4 + port for the local TLS fixture.
							if _, err := io.ReadFull(reader, request); err != nil {
								return err
							}
							if request[0] != 5 || request[1] != 1 || request[3] != 1 ||
								net.IP(request[4:8]).String() != targetURL.Hostname() ||
								fmt.Sprint(int(request[8])<<8|int(request[9])) != targetURL.Port() {
								return errors.New("incorrect SOCKS5 destination")
							}
						}
						up, err := net.DialTimeout("tcp", targetURL.Host, time.Second)
						if err != nil {
							return err
						}
						defer up.Close()
						if protocol == "http" {
							_, err = io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
						} else {
							_, err = conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
						}
						if err != nil {
							return err
						}
						copied := make(chan struct{})
						go func() { _, _ = io.Copy(up, reader); _ = up.Close(); close(copied) }()
						_, _ = io.Copy(conn, up)
						_ = conn.Close()
						<-copied
						return nil
					}()
				}()
				upstream, _ := url.Parse(protocol + "://" + listener.Addr().String())
				endpoint := target.URL
				config := &tls.Config{RootCAs: roots}
				verifyMode := mode
				if mode != VerifyExitIP {
					endpoint += "/page"
				}
				if mode == "untrusted-certificate" {
					config = nil
					verifyMode = VerifyConnectivity
				}
				result, err := verifyEndpointWithTransport(context.Background(), upstream, endpoint, verifyMode, config)
				if fixtureErr := <-done; fixtureErr != nil {
					t.Fatal(fixtureErr)
				}
				if mode == "untrusted-certificate" {
					if err == nil || !strings.Contains(err.Error(), "tls:certificate") {
						t.Fatalf("untrusted TLS accepted: %v", err)
					}
					return
				}
				if err != nil || result.HTTPStatus != 200 || (mode == VerifyExitIP && result.IP != "198.51.100.42") || (mode == VerifyConnectivity && result.IP != "") {
					t.Fatalf("HTTPS verification via %s failed: %+v %v", protocol, result, err)
				}
			})
		}
	}
}

func TestVerificationHTTPFailures(t *testing.T) {
	for _, test := range []struct {
		name, endpoint, code string
		status               int
		body                 string
	}{
		{"plain proxy auth", "http://echo.invalid/ip", "handshake:auth", 407, ""},
		{"CONNECT proxy auth", "https://echo.invalid/ip", "handshake:auth", 407, ""},
		{"CONNECT forbidden", "https://echo.invalid/ip", "handshake:connect_rejected", 403, ""},
		{"endpoint unavailable", "http://echo.invalid/ip", "response:http_status", 503, ""},
		{"redirect", "http://echo.invalid/ip", "response:redirect", 308, "198.51.100.42"},
		{"not an echo endpoint", "http://echo.invalid/ip", "body:invalid_ip", 200, "<html>private-token</html>"},
		{"oversized", "http://echo.invalid/ip", "body:oversized", 200, strings.Repeat("x", 4097)},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "http://user:private-token@echo.invalid/")
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			upstream, _ := url.Parse(server.URL)
			ip, err := Verify(context.Background(), upstream, test.endpoint+"?key=private-token")
			if err == nil || ip != "" || !strings.Contains(err.Error(), "[proxy_verify:"+test.code+"]") ||
				strings.Contains(err.Error(), "private-token") || strings.Contains(err.Error(), "echo.invalid") {
				t.Fatalf("incorrect or unsafe error: %q %v", ip, err)
			}
		})
	}
}

func TestVerificationConnectionErrorsHideSecrets(t *testing.T) {
	for _, test := range []struct {
		err  error
		want string
	}{
		{context.DeadlineExceeded, "timed out"},
		{context.Canceled, "canceled"},
		{syscall.ECONNREFUSED, "connection refused"},
		{syscall.ECONNRESET, "connection reset"},
		{errors.New("socks authentication failed with private-token"), "SOCKS5"},
		{errors.New("https://user:private-token@echo.invalid/"), "connection failed"},
	} {
		err := verificationConnectionError(&url.Error{Op: "Get", URL: "https://echo.invalid/?key=private-token", Err: test.err})
		if !strings.Contains(err.Error(), test.want) || strings.Contains(err.Error(), "private-token") ||
			strings.Contains(err.Error(), "echo.invalid") {
			t.Fatalf("unsafe or unhelpful error: %v", err)
		}
	}
}
