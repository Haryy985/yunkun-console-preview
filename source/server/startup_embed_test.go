//go:build embedui

package server

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

func TestStartupEmbeddedLoginIsAvailableDuringOutage(t *testing.T) {
	handler := NewStartupHandler()
	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/login/", nil))
	if page.Code != http.StatusOK {
		t.Fatalf("embedded login status=%d", page.Code)
	}
	script := regexp.MustCompile(`<script[^>]+src="(/_next/[^"]+)"`).FindStringSubmatch(page.Body.String())
	if len(script) != 2 {
		t.Fatal("embedded login is missing its frontend JavaScript")
	}
	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, script[1], nil))
	if asset.Code != http.StatusOK || asset.Body.Len() == 0 {
		t.Fatalf("embedded JavaScript unavailable: status=%d", asset.Code)
	}
}
