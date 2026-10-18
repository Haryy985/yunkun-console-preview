package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUpdateUnavailable(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		recorder := httptest.NewRecorder()
		updateUnavailable(recorder, httptest.NewRequest(method, "/api/update/check", nil))
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s: status = %d", method, recorder.Code)
		}
		if !strings.Contains(recorder.Body.String(), "自动替换未启用") {
			t.Fatalf("%s: unexpected response: %s", method, recorder.Body.String())
		}
	}
}
