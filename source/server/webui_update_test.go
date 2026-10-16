//go:build embedui

package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedUpdateInterface(t *testing.T) {
	handler := (&Server{}).webuiHandler()
	for _, path := range []string{"/", "/dashboard/"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "<html") {
			t.Fatalf("%s: embedded page unavailable: %d", path, recorder.Code)
		}
	}
}
