package agent

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"yunkun/console/db"
	actool "github.com/Autumn-27/norma/tool"
)

func TestTaskResourcesAndBoundedRetrievalIntegration(t *testing.T) {
	if os.Getenv("YUNKUN_RAG_TEST_ISOLATED") != "1" {
		t.Skip("isolated database test disabled")
	}
	dsn := os.Getenv("YUNKUN_PG_DSN")
	u, err := url.Parse(dsn)
	if err != nil || u.Hostname() != "127.0.0.1" || u.Port() != "26900" {
		t.Fatal("this integration test requires the dedicated loopback test cluster")
	}
	name := fmt.Sprintf("yunkun_resources_%d", time.Now().UnixNano())
	u.Path = "/" + name
	d, err := db.Open(u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = d.Close()
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
	kb, err := d.CreateRAGKnowledgeBase("", "", "private", "resource fixture", "purpose", "", "", 900, 120)
	if err != nil {
		t.Fatal(err)
	}
	doc, _, err := d.CreateRAGDocument(kb.ID, "fixture.txt", "text", "", "text/plain", "fixture-hash", "fixture-sha", "fixturemarker")
	if err != nil {
		t.Fatal(err)
	}
	content := "fixturemarker " + strings.Repeat("知识与验证条件", 1000)
	if err := d.ReplaceRAGDocumentChunks(doc.ID, []db.RAGChunkInput{{
		Ordinal: 0, Content: content, ContentHash: "chunk-hash", SourceURI: "fixture://original",
	}}); err != nil {
		t.Fatal(err)
	}
	task, err := d.CreateTaskWithOptions("resource fixture", "fixture", db.TaskCreateOptions{RAGKnowledgeBaseIDs: []int64{kb.ID}})
	if err != nil {
		t.Fatal(err)
	}
	ts := NewToolSet(nil, "worker")
	ts.SetRAGDB(d)
	ts.SetTaskID(task.ID)
	call := func(tool actool.CoreTool, input string, wantError bool) string {
		t.Helper()
		result, err := tool.Call(t.Context(), json.RawMessage(input), nil)
		if err != nil || result.IsError != wantError {
			t.Fatalf("tool=%s result=%s err=%v", tool.Name(), result.Flatten(), err)
		}
		return result.Flatten()
	}
	var resources struct {
		DefaultID int64                `json:"default_knowledge_base_id"`
		Bases     []db.TaskRAGResource `json:"knowledge_bases"`
		Review    bool                 `json:"knowledge_changes_require_human_review"`
	}
	if err := json.Unmarshal([]byte(call(ts.availableResources(), `{}`, false)), &resources); err != nil {
		t.Fatal(err)
	}
	if resources.DefaultID != kb.ID || len(resources.Bases) != 1 || !resources.Review {
		t.Fatalf("incorrect resource discovery: %+v", resources)
	}
	body := call(ts.ragSearch(), `{"query":"fixturemarker","top_k":20}`, false)
	var payload struct {
		BaseID  int64                 `json:"knowledge_base_id"`
		LogID   int64                 `json:"retrieval_log_id"`
		Results []*db.RAGSearchResult `json:"results"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatal(err)
	}
	if len(body) > ragResponseBytes || payload.BaseID != resources.DefaultID || len(payload.Results) != 1 ||
		!payload.Results[0].ContentTruncated || payload.Results[0].OriginalContentBytes != len(content) {
		t.Fatalf("incorrect bounded retrieval: bytes=%d payload=%+v", len(body), payload)
	}
	logs, err := d.ListRAGRetrievalLogsByTask(fmt.Sprint(task.ID), 10)
	if err != nil || len(logs) != 1 || logs[0].ID != payload.LogID || !reflect.DeepEqual(logs[0].Results, payload.Results) {
		t.Fatalf("retrieval log differs from actual tool response: logs=%+v err=%v", logs, err)
	}
	if _, err := d.SetTaskRAGKnowledgeBaseIDs(task.ID, nil); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(call(ts.availableResources(), `{}`, false)), &resources); err != nil {
		t.Fatal(err)
	}
	if resources.DefaultID != 0 || len(resources.Bases) != 0 {
		t.Fatal("resource discovery retained revoked access")
	}
	call(ts.ragSearch(), fmt.Sprintf(`{"query":"fixturemarker","knowledge_base_id":%d}`, kb.ID), true)
	logs, err = d.ListRAGRetrievalLogsByTask(fmt.Sprint(task.ID), 10)
	if err != nil || len(logs) != 1 {
		t.Fatalf("denied retrieval produced a log: logs=%+v err=%v", logs, err)
	}
}
