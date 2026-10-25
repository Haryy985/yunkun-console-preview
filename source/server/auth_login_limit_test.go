package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLoginLimiterAccountsAndPeers(t *testing.T) {
	var limiter authLoginLimiter
	now := time.Unix(1_000_000, 0)
	for i := 0; i < loginUserLimit; i++ {
		if retry := limiter.allow("192.0.2.1:1234", "admin", now); retry != 0 {
			t.Fatalf("attempt %d blocked: %s", i, retry)
		}
	}
	if retry := limiter.allow("192.0.2.2:4567", "ADMIN", now); retry != loginWindow {
		t.Fatalf("account limit not shared: %s", retry)
	}
	if retry := limiter.allow("192.0.2.1:1234", "other", now); retry != 0 {
		t.Fatalf("independent account blocked: %s", retry)
	}
	if retry := limiter.allow("192.0.2.2:4567", "admin", now.Add(loginWindow)); retry != 0 {
		t.Fatalf("window not reset: %s", retry)
	}
}

func TestLoginLimiterConcurrentAndBounded(t *testing.T) {
	var limiter authLoginLimiter
	now := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			limiter.allow("192.0.2.1:1234", "admin", now)
		}()
	}
	wg.Wait()
	if got := limiter.entries["user:admin"].count; got != loginUserLimit {
		t.Fatalf("allowed %d attempts, want %d", got, loginUserLimit)
	}
	if retry := limiter.allow("192.0.2.1:1234", "admin", now); retry == 0 {
		t.Fatal("expected rate limit")
	}
}

func TestLoginRateLimitPrecedesDatabase(t *testing.T) {
	s := &Server{m: &Manager{}}
	h := http.HandlerFunc(s.authLogin)
	for i := 0; i < loginUserLimit; i++ {
		r := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"wrong"}`))
		r.RemoteAddr = "192.0.2.1:1234"
		h.ServeHTTP(httptest.NewRecorder(), r)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"wrong"}`))
	r.RemoteAddr = "192.0.2.1:1234"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
		t.Fatalf("rate limit: %d, retry-after %q", w.Code, w.Header().Get("Retry-After"))
	}
}
