package server

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	_ "modernc.org/sqlite"
	"yunkun/console/db"
)

// These identity reads use portable SQL. Keep the outage regression independent
// of a running PostgreSQL service, including when that service is the failure.
func newIdentityTestStore(t *testing.T) *db.DB {
	t.Helper()
	pool, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = pool.Close() })
	for _, query := range []string{
		`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT)`,
		`INSERT INTO settings VALUES ('auth.password_hash', 'test-hash')`,
		`CREATE TABLE auth_users (id INTEGER PRIMARY KEY, username TEXT, password_hash TEXT, role TEXT, disabled BOOLEAN, token_version INTEGER)`,
		`INSERT INTO auth_users VALUES (1, 'viewer', 'test-hash', 'viewer', false, 1)`,
	} {
		if _, err := pool.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	return &db.DB{DB: pool}
}

func TestIdentityOutagePreservesSessionAndRevocation(t *testing.T) {
	for _, subject := range []string{adminUsername, "1"} {
		t.Run(subject, func(t *testing.T) {
			pg := newIdentityTestStore(t)
			s := &Server{m: &Manager{pg: pg}, jwtKey: []byte("test-key-only-for-identity-outage")}
			version := "1"
			if subject == adminUsername {
				version = passwordVersion("test-hash")
			}
			token, err := signIdentityJWT(s.jwtKey, subject, version)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			handler := s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(http.StatusNoContent)
			}))
			request := func(want int) *httptest.ResponseRecorder {
				t.Helper()
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				handler.ServeHTTP(rec, req)
				if rec.Code != want {
					t.Fatalf("status=%d want=%d body=%s", rec.Code, want, rec.Body.String())
				}
				return rec
			}
			request(http.StatusNoContent)
			_ = pg.Close()
			rec := request(http.StatusServiceUnavailable)
			if calls != 1 || rec.Header().Get("Retry-After") != "5" ||
				!strings.Contains(rec.Body.String(), errDataSourceUnavailable) {
				t.Fatalf("outage must fail closed without invalidating identity: calls=%d body=%s", calls, rec.Body.String())
			}
			s.m.pg = newIdentityTestStore(t)
			request(http.StatusNoContent)
			query := `UPDATE auth_users SET disabled=true WHERE id=1`
			if subject == adminUsername {
				query = `UPDATE settings SET value='new-hash' WHERE key='auth.password_hash'`
			}
			if _, err := s.m.pg.Exec(query); err != nil {
				t.Fatal(err)
			}
			request(http.StatusUnauthorized)
			if calls != 2 {
				t.Fatalf("revoked identity reached handler: calls=%d", calls)
			}
		})
	}
}

func TestIdentityOutageDoesNotHideInvalidTokens(t *testing.T) {
	s := &Server{jwtKey: []byte("test-key-only-for-identity-outage")}
	expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, authClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: adminUsername, ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
		},
	}).SignedString(s.jwtKey)
	if err != nil {
		t.Fatal(err)
	}
	handler := s.requireAuth(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("invalid token reached protected handler")
	}))
	for _, token := range []string{"", "malformed", expired} {
		req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("invalid token status=%d want 401", rec.Code)
		}
	}
}

func TestPasswordChangeIdentityOutageReturns503(t *testing.T) {
	pg := newIdentityTestStore(t)
	s := &Server{m: &Manager{pg: pg}, jwtKey: []byte("test-key-only-for-identity-outage")}
	token, err := signIdentityJWT(s.jwtKey, adminUsername, passwordVersion("test-hash"))
	if err != nil {
		t.Fatal(err)
	}
	_ = pg.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.authChangePassword(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want 503; body=%s", rec.Code, rec.Body.String())
	}
}
