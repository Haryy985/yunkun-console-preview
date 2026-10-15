package server

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"yunkun/console/db"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestTaskRAGAccessPatch(t *testing.T) {
	if os.Getenv("YUNKUN_RAG_TEST_ISOLATED") != "1" {
		t.Skip("isolated database test disabled")
	}
	baseDSN, _, err := db.DSN()
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(baseDSN)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("yunkun_rag_http_test_%d", time.Now().UnixNano())
	u.Path = "/" + name
	pg, err := db.Open(u.String())
	if err != nil {
		t.Fatal(err)
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
		if _, err := admin.Exec(`DROP DATABASE "` + name + `" WITH (FORCE)`); err != nil {
			t.Error(err)
		}
	})
	kb, err := pg.CreateRAGKnowledgeBase("", "", "private", "http-test", "", "", "", 900, 120)
	if err != nil {
		t.Fatal(err)
	}
	created, err := pg.CreateTask("existing task", "authorize", nil, 0, 600)
	if err != nil {
		t.Fatal(err)
	}
	taskID := strconv.FormatInt(created.ID, 10)
	m := &Manager{pg: pg, tasks: map[string]*Task{
		taskID: taskFromPG(created, pg.Exploration(created.ExplorationID), nil),
	}}
	s := &Server{m: m, engine: NewEngine(m), jwtKey: []byte("test-only-key-with-at-least-32-bytes")}
	token, err := signJWT(s.jwtKey)
	if err != nil {
		t.Fatal(err)
	}
	handler := s.Handler()
	patch := func(body string, authenticated bool) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPatch, "/api/tasks/"+taskID+"/rag/access", strings.NewReader(body))
		if authenticated {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	if rec := patch(`{"rag_knowledge_base_ids":[]}`, false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated patch: %d %s", rec.Code, rec.Body.String())
	}
	if rec := patch(`{}`, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing array: %d %s", rec.Code, rec.Body.String())
	}
	if rec := patch(`{"rag_knowledge_base_ids":[999999999]}`, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid base: %d %s", rec.Code, rec.Body.String())
	}
	if rec := patch(fmt.Sprintf(`{"rag_knowledge_base_ids":[%d]}`, kb.ID), true); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), fmt.Sprintf(`"rag_knowledge_base_ids":[%d]`, kb.ID)) {
		t.Fatalf("grant: %d %s", rec.Code, rec.Body.String())
	}
	if rec := patch(`{"rag_knowledge_base_ids":[]}`, true); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `"rag_knowledge_base_ids":[]`) {
		t.Fatalf("revoke: %d %s", rec.Code, rec.Body.String())
	}
}
