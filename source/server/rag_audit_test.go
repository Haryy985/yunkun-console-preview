package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"yunkun/console/db"
)

func TestRAGAuditErrorResponses(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code int
	}{
		{db.ErrRAGKnowledgeBaseInvalid, 400},
		{db.ErrRAGKnowledgeBaseNameUsed, 409},
		{db.ErrTaskRAGAccessDenied, 403},
	} {
		rec := httptest.NewRecorder()
		writeRAGKnowledgeBaseError(rec, fmt.Errorf("wrapped: %w", tc.err))
		if rec.Code != tc.code {
			t.Fatalf("%v: got %d, want %d", tc.err, rec.Code, tc.code)
		}
	}
}

func TestRAGEvalHTTPRoundTrip(t *testing.T) {
	dsn, _, err := db.DSN()
	if err != nil {
		t.Skip("integration test requires isolated YUNKUN_PG_DSN")
	}
	pg, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Close()
	kb, err := pg.CreateRAGKnowledgeBase("", "", "private", fmt.Sprintf("eval-http-%d", time.Now().UnixNano()), "", "", "", 900, 120)
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Exec("DELETE FROM rag_knowledge_bases WHERE id=$1", kb.ID)
	set, err := pg.CreateRAGEvalSet(kb.ID, "test", "")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{m: &Manager{pg: pg}}
	call := func(method, body string, handler http.HandlerFunc, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetPathValue("id", strconv.FormatInt(set.ID, 10))
		rec := httptest.NewRecorder()
		handler(rec, req)
		if rec.Code != want {
			t.Fatalf("got %d, want %d: %s", rec.Code, want, rec.Body)
		}
		return rec
	}
	call("POST", `{"question":"audit fixture"}`, s.createRAGEvalCase, 201)
	rec := call("GET", "", s.listRAGEvalCases, 200)
	var result struct {
		Cases []db.RAGEvalCase `json:"cases"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || len(result.Cases) != 1 {
		t.Fatalf("cases: %s %v", rec.Body, err)
	}
	if result.Cases[0].ExpectedChunkIDs == nil || result.Cases[0].ExpectedDocumentIDs == nil {
		t.Fatalf("expected arrays in response: %s", rec.Body)
	}
	call("POST", `{"top_k":5}`, s.runRAGEval, 200)
	call("GET", "", s.listRAGEvalRuns, 200)
}
