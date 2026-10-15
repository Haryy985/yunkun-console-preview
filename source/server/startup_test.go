package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStartupBlocksAuthUntilReady(t *testing.T) {
	h := NewStartupHandler()
	for _, path := range []string{"/api/auth/status", "/api/auth/init", "/api/auth/login", "/api/tasks", "/api/health"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") != "5" {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "initialized") {
			t.Fatal("database outage must not be reported as an uninitialized password")
		}
	}
	h.SetReady(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/auth/status", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("recovery did not switch handlers: %d", rec.Code)
	}
}

func TestHealthDoesNotClaimReadyWithoutDatabase(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	s.health(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"ready":false`) {
		t.Fatalf("false healthy response: %d %s", rec.Code, rec.Body.String())
	}
}
