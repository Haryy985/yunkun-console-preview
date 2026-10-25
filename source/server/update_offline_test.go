package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUpdateRequiresExplicitTarget(t *testing.T) {
	s := &Server{}
	for _, body := range []string{`{}`, `{"url":"https://attacker.invalid/package.zip"}`, `bad`} {
		w := httptest.NewRecorder()
		s.updateApply(w, httptest.NewRequest("POST", "/api/update/apply", strings.NewReader(body)))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d", body, w.Code)
		}
	}
}

func TestUpdateMaintenanceBlocksBusinessWrites(t *testing.T) {
	t.Setenv("YUNKUN_OFFLINE_ROOT", "")
	old := updHub
	updHub = &updateHub{subs: map[chan updateProgress]struct{}{}}
	t.Cleanup(func() { updHub = old })
	updHub.begin("v1.1.0-offline")
	h := (&Server{}).updateMaintenance(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	}))
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/api/tasks", 503}, {"/api/auth/init", 503}, {"/api/update/status", 204}, {"/api/health", 204},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", tc.path, nil))
		if w.Code != tc.want {
			t.Fatalf("%s status=%d want %d", tc.path, w.Code, tc.want)
		}
	}
}
