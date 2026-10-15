package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestAdminLoginIssuesCurrentIdentity(t *testing.T) {
	pg := newIdentityTestStore(t)
	hash, err := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pg.Exec(`UPDATE settings SET value=? WHERE key=?`, string(hash), authPassKey); err != nil {
		t.Fatal(err)
	}
	s := &Server{m: &Manager{pg: pg}, jwtKey: []byte("brand-login-test-key")}
	for _, username := range []string{adminUsername} {
		t.Run(username, func(t *testing.T) {
			body, err := json.Marshal(map[string]string{"username": username, "password": "test-password"})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(string(body)))
			rec := httptest.NewRecorder()
			s.authLogin(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("login status=%d body=%s", rec.Code, rec.Body.String())
			}
			var reply struct {
				Token string `json:"token"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil {
				t.Fatal(err)
			}
			claims, err := parseIdentityJWT(reply.Token, s.jwtKey)
			if err != nil {
				t.Fatal(err)
			}
			if claims.Subject != adminUsername || claims.Version != passwordVersion(string(hash)) {
				t.Fatalf("unexpected new identity: %#v", claims)
			}
			identity, err := s.identity(reply.Token)
			if err != nil || identity.Username != adminUsername || identity.Role != "admin" {
				t.Fatalf("identity=%#v err=%v", identity, err)
			}
		})
	}
}

func TestAuthTokenSourcePrecedence(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/tasks?token=sse-token", nil)
	if got := extractToken(req); got != "sse-token" {
		t.Fatalf("SSE token=%q", got)
	}
	req.AddCookie(&http.Cookie{Name: "unrelated_token", Value: "ignored-token"})
	if got := extractToken(req); got != "sse-token" {
		t.Fatalf("unrelated cookie changed token=%q", got)
	}
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: "current-token"})
	if got := extractToken(req); got != "current-token" {
		t.Fatalf("current cookie must take precedence: %q", got)
	}
	req.Header.Set("Authorization", "Bearer header-token")
	if got := extractToken(req); got != "header-token" {
		t.Fatalf("header must take precedence: %q", got)
	}
}

func TestAdminIdentityRejectsUnrecognisedSubject(t *testing.T) {
	pg := newIdentityTestStore(t)
	s := &Server{m: &Manager{pg: pg}, jwtKey: []byte("brand-reject-test-key")}
	token, err := signIdentityJWT(s.jwtKey, "obsolete-admin", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.identity(token); err == nil {
		t.Fatal("unrecognised admin identity accepted")
	}
}
