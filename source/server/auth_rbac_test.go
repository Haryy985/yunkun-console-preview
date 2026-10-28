package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/bcrypt"
	"yunkun/console/db"
)

func TestRBACRevocationAndDefaultDeny(t *testing.T) {
	dsn, _, err := db.DSN()
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := "yunkun_rbac_test_" + uuid.NewString()
	u.Path = "/" + name
	pg, err := db.Open(u.String())
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	t.Cleanup(func() {
		_ = pg.Close()
		u.Path = "/postgres"
		admin, err := sql.Open("pgx", u.String())
		if err != nil {
			t.Error(err)
			return
		}
		defer admin.Close()
		if _, err := admin.Exec(fmt.Sprintf(`DROP DATABASE "%s" WITH (FORCE)`, name)); err != nil {
			t.Error(err)
		}
	})
	m := &Manager{pg: pg, tasks: map[string]*Task{}}
	s := &Server{m: m, engine: NewEngine(m), jwtKey: []byte("test-key-only-for-rbac-fixture-12345")}
	h := s.Handler()
	adminHash, err := bcrypt.GenerateFromPassword([]byte("admin-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := pg.SetSetting(authPassKey, string(adminHash)); err != nil {
		t.Fatal(err)
	}
	admin, err := signIdentityJWT(s.jwtKey, adminUsername, passwordVersion(string(adminHash)))
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path, token string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	username := "reviewer-" + uuid.NewString()
	create := call("POST", "/api/auth/users", admin, map[string]any{
		"username": username, "password": "reviewer-password", "role": "viewer",
	})
	if create.Code != 201 {
		t.Fatalf("create user: %d %s", create.Code, create.Body.String())
	}
	var user struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &user); err != nil {
		t.Fatal(err)
	}
	login := call("POST", "/api/auth/login", "", map[string]any{
		"username": username, "password": "reviewer-password",
	})
	if login.Code != 200 {
		t.Fatalf("login: %d %s", login.Code, login.Body.String())
	}
	var credentials struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &credentials); err != nil {
		t.Fatal(err)
	}
	viewer := credentials.Token
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{"GET", "/api/tasks", 403},
		{"GET", "/api/auth/me", 200},
		{"GET", "/api/tasks/1", 403},
		{"GET", "/api/tasks/1/runtime-metrics", 403},
		{"GET", "/api/exploration/findings", 403},
		{"GET", "/api/exploration/findings/1/traffic", 403},
		{"GET", "/api/proxy-pools", 403},
		{"POST", "/api/tasks", 403},
		{"DELETE", "/api/tasks/1", 403},
		{"GET", "/api/tasks/1/chat/status", 403},
		{"GET", "/api/tasks/1/rag/retrieval-logs", 403},
		{"GET", "/api/tasks/1/scope", 403},
		{"POST", "/api/tasks/1/intents/2/control", 403},
		{"POST", "/api/rag/proposals/1/review", 403},
		{"GET", "/api/workspace/read", 403},
		{"GET", "/api/llm/records", 403},
		{"GET", "/api/auth/users", 403},
		{"POST", "/api/update/apply", 403},
		{"POST", "/api/update/rollback", 403},
		{"GET", "/api/update/status", 403},
		{"GET", "/api/not-yet-classified", 403},
	} {
		rec := call(tc.method, tc.path, viewer, nil)
		if rec.Code != tc.want {
			t.Errorf("%s %s: got %d want %d", tc.method, tc.path, rec.Code, tc.want)
		}
	}
	update := call("PATCH", "/api/auth/users/"+strconv.FormatInt(user.ID, 10), admin,
		map[string]any{"role": "operator", "disabled": false})
	if update.Code != 200 {
		t.Fatalf("update: %d %s", update.Code, update.Body.String())
	}
	if rec := call("GET", "/api/tasks", viewer, nil); rec.Code != 401 {
		t.Fatalf("old token after role change: %d", rec.Code)
	}
	login = call("POST", "/api/auth/login", "", map[string]any{
		"username": username, "password": "reviewer-password",
	})
	_ = json.Unmarshal(login.Body.Bytes(), &credentials)
	operator := credentials.Token
	if roleAllows("operator", "POST", "/api/tasks") ||
		roleAllows("operator", "POST", "/api/tasks/1/control") ||
		roleAllows("operator", "POST", "/api/tasks/1/network/lease") {
		t.Fatal("operator must not run host tools through tasks")
	}
	if roleAllows("operator", "POST", "/api/tasks/1/intents/2/control") ||
		roleAllows("viewer", "GET", "/api/tasks/1/chat/status") {
		t.Fatal("nested task endpoints must not inherit task permissions")
	}
	if rec := call("DELETE", "/api/tasks/1", operator, nil); rec.Code != 403 {
		t.Fatalf("operator delete: %d", rec.Code)
	}
	update = call("PATCH", "/api/auth/users/"+strconv.FormatInt(user.ID, 10), admin,
		map[string]any{"role": "operator", "disabled": true})
	if update.Code != 200 {
		t.Fatalf("disable: %d %s", update.Code, update.Body.String())
	}
	if rec := call("GET", "/api/tasks", operator, nil); rec.Code != 401 {
		t.Fatalf("disabled token: %d", rec.Code)
	}
	if rec := call("POST", "/api/auth/login", "", map[string]any{
		"username": username, "password": "reviewer-password",
	}); rec.Code != 401 {
		t.Fatalf("disabled login: %d", rec.Code)
	}
	if err := pg.SetSetting(authPassKey, "new-password-hash"); err != nil {
		t.Fatal(err)
	}
	if rec := call("GET", "/api/auth/users", admin, nil); rec.Code != 401 {
		t.Fatalf("old admin token after password change: %d", rec.Code)
	}
}

func TestRoleAllowsTaskAndSensitiveAPIsAdminOnly(t *testing.T) {
	paths := []struct{ method, path string }{
		{"GET", "/api/tasks"},
		{"GET", "/api/tasks/1"},
		{"GET", "/api/tasks/1/runtime-metrics"},
		{"GET", "/api/tasks/1/network"},
		{"POST", "/api/tasks"},
		{"POST", "/api/tasks/control/batch"},
		{"POST", "/api/tasks/1/control"},
		{"POST", "/api/tasks/1/network/lease"},
		{"GET", "/api/exploration/findings"},
		{"POST", "/api/exploration/findings/batch-delete"},
		{"GET", "/api/exploration/findings/1/traffic"},
		{"GET", "/api/exploration/activity/history"},
		{"GET", "/api/proxy-pools"},
		{"GET", "/api/proxy-settings"},
		{"POST", "/api/tools/test"},
	}
	for _, tc := range paths {
		if !roleAllows("admin", tc.method, tc.path) {
			t.Errorf("admin lost %s %s", tc.method, tc.path)
		}
		for _, role := range []string{"operator", "viewer", "unknown"} {
			if roleAllows(role, tc.method, tc.path) {
				t.Errorf("%s allowed %s %s", role, tc.method, tc.path)
			}
		}
	}
	for _, role := range []string{"operator", "viewer"} {
		if !roleAllows(role, "GET", "/api/auth/me") || !roleAllows(role, "GET", "/api/health") {
			t.Errorf("%s cannot read basic status", role)
		}
	}
}
