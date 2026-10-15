package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"
)

func TestTaskRAGResourceDiscovery(t *testing.T) {
	if os.Getenv("YUNKUN_RAG_TEST_ISOLATED") != "1" {
		t.Skip("isolated database test disabled")
	}
	d := openIsolatedRAGTestDB(t)
	var ids []int64
	for _, name := range []string{"first", "second", "ungranted"} {
		kb, err := d.CreateRAGKnowledgeBase("", "", "private", name, "purpose", "", "credential-fixture", 900, 120)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, kb.ID)
	}
	task, err := d.CreateTaskWithOptions("resources", "fixture", TaskCreateOptions{
		RAGKnowledgeBaseIDs: []int64{ids[1], ids[0]},
	})
	if err != nil {
		t.Fatal(err)
	}
	check := func(want ...int64) {
		t.Helper()
		resources, err := d.TaskRAGResources(t.Context(), task.ID)
		if err != nil || len(resources) != len(want) {
			t.Fatalf("resources=%+v want=%v err=%v", resources, want, err)
		}
		ordered, err := d.TaskRAGKnowledgeBaseIDs(task.ID)
		if err != nil || len(ordered) != len(resources) {
			t.Fatalf("default selection differs: %v err=%v", ordered, err)
		}
		for i, resource := range resources {
			if resource.ID != want[i] || resource.ID != ordered[i] || resource.Description != "purpose" {
				t.Fatalf("wrong resource: %+v", resource)
			}
			encoded, err := json.Marshal(resource)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err := json.Unmarshal(encoded, &fields); err != nil || len(fields) != 3 {
				t.Fatalf("unexpected resource fields: %s err=%v", encoded, err)
			}
		}
	}
	// A single transaction gives both grants the same timestamp; KB ID breaks ties.
	check(ids[0], ids[1])
	if _, err := d.SetTaskRAGKnowledgeBaseIDs(task.ID, []int64{ids[1]}); err != nil {
		t.Fatal(err)
	}
	check(ids[1])
	if _, err := d.Exec(`UPDATE rag_knowledge_bases SET enabled=false WHERE id=$1`, ids[1]); err != nil {
		t.Fatal(err)
	}
	check()
	if _, err := d.Exec(`UPDATE rag_knowledge_bases SET enabled=true,deleted_at=CURRENT_TIMESTAMP WHERE id=$1`, ids[1]); err != nil {
		t.Fatal(err)
	}
	check()
	if _, err := d.SetTaskRAGKnowledgeBaseIDs(task.ID, []int64{ids[0]}); err != nil {
		t.Fatal(err)
	}
	check(ids[0])
	if _, err := d.Exec(`UPDATE tasks SET deleted_at=CURRENT_TIMESTAMP WHERE id=$1`, task.ID); err != nil {
		t.Fatal(err)
	}
	check()
}

func openIsolatedRAGTestDB(t *testing.T) *DB {
	t.Helper()
	baseDSN, _, err := DSN()
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(baseDSN)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("yunkun_rag_test_%d", time.Now().UnixNano())
	u.Path = "/" + name
	d, err := Open(u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = d.Close()
		u.Path = "/postgres"
		admin, err := sql.Open("pgx", u.String())
		if err != nil {
			t.Errorf("cleanup connection: %v", err)
			return
		}
		defer admin.Close()
		if _, err := admin.Exec(`DROP DATABASE "` + name + `" WITH (FORCE)`); err != nil {
			t.Errorf("cleanup isolated database: %v", err)
		}
	})
	return d
}

func TestNormalizeTaskRAGKnowledgeBaseIDs(t *testing.T) {
	for _, ids := range [][]int64{{0}, {-1}, {1, 1}, make([]int64, MaxTaskRAGKnowledgeBases+1)} {
		if _, err := NormalizeTaskRAGKnowledgeBaseIDs(ids); !errors.Is(err, ErrTaskRAGKnowledgeBaseIDsInvalid) {
			t.Fatalf("accepted invalid ids %v: %v", ids, err)
		}
	}
}

func TestTaskRAGAccess(t *testing.T) {
	if os.Getenv("YUNKUN_RAG_TEST_ISOLATED") != "1" {
		t.Skip("isolated database test disabled")
	}
	d := openIsolatedRAGTestDB(t)
	kb, err := d.CreateRAGKnowledgeBase("", "", "private", "access-test", "", "", "", 900, 120)
	if err != nil {
		t.Fatal(err)
	}
	other, err := d.CreateRAGKnowledgeBase("", "", "private", "access-other", "", "", "", 900, 120)
	if err != nil {
		t.Fatal(err)
	}
	ungranted, err := d.CreateTask("ungranted", "none", nil, 0, 600)
	if err != nil {
		t.Fatal(err)
	}
	if allowed, err := d.TaskCanAccessRAGKnowledgeBase(ungranted.ID, kb.ID); err != nil || allowed {
		t.Fatalf("ungranted task access: %v, %v", allowed, err)
	}
	proposal := RAGChangeProposal{KnowledgeBaseID: kb.ID, TaskID: ungranted.ID, AgentID: "agent",
		Kind: "create", Title: "new", Content: "content", Rationale: "reason", SourceRef: "fixture"}
	if _, err := d.CreateRAGProposal(proposal); !errors.Is(err, ErrTaskRAGAccessDenied) {
		t.Fatalf("ungranted proposal: %v", err)
	}
	if _, err := d.CreateTaskWithOptions("invalid", "none", TaskCreateOptions{RAGKnowledgeBaseIDs: []int64{kb.ID, 1 << 62}}); !errors.Is(err, ErrTaskRAGKnowledgeBaseNotFound) {
		t.Fatalf("invalid grant accepted: %v", err)
	}
	task, err := d.CreateTaskWithOptions("granted", "one", TaskCreateOptions{RAGKnowledgeBaseIDs: []int64{kb.ID}})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := d.GetTask(task.ID)
	if err != nil || len(loaded.RAGKnowledgeBaseIDs) != 1 || loaded.RAGKnowledgeBaseIDs[0] != kb.ID {
		t.Fatalf("grant hydration: %#v, %v", loaded, err)
	}
	for _, id := range []int64{kb.ID, other.ID} {
		allowed, err := d.TaskCanAccessRAGKnowledgeBase(task.ID, id)
		if err != nil || allowed != (id == kb.ID) {
			t.Fatalf("kb %d access: %v, %v", id, allowed, err)
		}
	}
	proposal.TaskID = task.ID
	if _, err := d.CreateRAGProposal(proposal); err != nil {
		t.Fatal(err)
	}
	proposal.KnowledgeBaseID = other.ID
	if _, err := d.CreateRAGProposal(proposal); !errors.Is(err, ErrTaskRAGAccessDenied) {
		t.Fatalf("cross-base proposal: %v", err)
	}
	if _, err := d.SetTaskRAGKnowledgeBaseIDs(ungranted.ID, []int64{kb.ID}); err != nil {
		t.Fatalf("grant existing task: %v", err)
	}
	if allowed, err := d.TaskCanAccessRAGKnowledgeBase(ungranted.ID, kb.ID); err != nil || !allowed {
		t.Fatalf("existing task grant: %v, %v", allowed, err)
	}
	proposal.KnowledgeBaseID = kb.ID
	proposal.TaskID = ungranted.ID
	pending, err := d.CreateRAGProposal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.SetTaskRAGKnowledgeBaseIDs(ungranted.ID, []int64{other.ID, 1 << 62}); !errors.Is(err, ErrTaskRAGKnowledgeBaseNotFound) {
		t.Fatalf("invalid replacement accepted: %v", err)
	}
	if allowed, err := d.TaskCanAccessRAGKnowledgeBase(ungranted.ID, kb.ID); err != nil || !allowed {
		t.Fatalf("invalid replacement changed grant: %v, %v", allowed, err)
	}
	if ids, err := d.SetTaskRAGKnowledgeBaseIDs(ungranted.ID, []int64{}); err != nil || len(ids) != 0 {
		t.Fatalf("revoke: %v, %v", ids, err)
	}
	if _, _, err := d.ReviewRAGProposal(pending.ID, true, "admin", "verified"); !errors.Is(err, ErrTaskRAGAccessDenied) {
		t.Fatalf("revoked task proposal approved: %v", err)
	}
	if allowed, err := d.TaskCanAccessRAGKnowledgeBase(ungranted.ID, kb.ID); err != nil || allowed {
		t.Fatalf("revoked task access: %v, %v", allowed, err)
	}
	if _, err := d.Exec(`UPDATE rag_knowledge_bases SET enabled=false WHERE id=$1`, kb.ID); err != nil {
		t.Fatal(err)
	}
	allowed, err := d.TaskCanAccessRAGKnowledgeBase(task.ID, kb.ID)
	if err != nil || allowed {
		t.Fatalf("disabled base access: %v, %v", allowed, err)
	}
}

func TestRAGReviewAndGrantRevocationSerialize(t *testing.T) {
	if os.Getenv("YUNKUN_RAG_TEST_ISOLATED") != "1" {
		t.Skip("isolated database test disabled")
	}
	d := openIsolatedRAGTestDB(t)
	kb, err := d.CreateRAGKnowledgeBase("", "", "private", "race-test", "", "", "", 900, 120)
	if err != nil {
		t.Fatal(err)
	}
	task, err := d.CreateTaskWithOptions("race", "review versus revoke", TaskCreateOptions{RAGKnowledgeBaseIDs: []int64{kb.ID}})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := d.CreateRAGProposal(RAGChangeProposal{
		KnowledgeBaseID: kb.ID, TaskID: task.ID, AgentID: "worker", Kind: "create",
		Title: "race.md", Content: "race_content", Rationale: "fixture", SourceRef: "fixture",
	})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	var reviewErr, revokeErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, _, reviewErr = d.ReviewRAGProposal(proposal.ID, true, "admin", "checked")
	}()
	go func() {
		defer wg.Done()
		<-start
		_, revokeErr = d.SetTaskRAGKnowledgeBaseIDs(task.ID, []int64{})
	}()
	close(start)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("review and revocation deadlocked")
	}
	if revokeErr != nil {
		t.Fatalf("revoke: %v", revokeErr)
	}
	if reviewErr != nil && !errors.Is(reviewErr, ErrTaskRAGAccessDenied) {
		t.Fatalf("review: %v", reviewErr)
	}
	if allowed, err := d.TaskCanAccessRAGKnowledgeBase(task.ID, kb.ID); err != nil || allowed {
		t.Fatalf("grant survived revocation: %v, %v", allowed, err)
	}
}

func TestRAGPublicationAfterRevocation(t *testing.T) {
	if os.Getenv("YUNKUN_RAG_TEST_ISOLATED") != "1" {
		t.Skip("isolated database test disabled")
	}
	d := openIsolatedRAGTestDB(t)
	kb, err := d.CreateRAGKnowledgeBase("", "", "private", "publication-test", "", "", "", 900, 120)
	if err != nil {
		t.Fatal(err)
	}
	task, err := d.CreateTaskWithOptions("publication", "revoke during search", TaskCreateOptions{RAGKnowledgeBaseIDs: []int64{kb.ID}})
	if err != nil {
		t.Fatal(err)
	}
	allowed, err := d.TaskCanAccessRAGKnowledgeBase(task.ID, kb.ID)
	if err != nil || !allowed {
		t.Fatalf("initial authorization: %v, %v", allowed, err)
	}
	if _, err := d.SetTaskRAGKnowledgeBaseIDs(task.ID, []int64{}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.PublishTaskRAGRetrieval(context.Background(), task.ID, kb.ID, "worker", nil, "search in flight", 5, "keyword", nil, 1); !errors.Is(err, ErrTaskRAGAccessDenied) {
		t.Fatalf("revoked result published: %v", err)
	}
	logs, err := d.ListRAGRetrievalLogsByTask(fmt.Sprint(task.ID), 10)
	if err != nil || len(logs) != 0 {
		t.Fatalf("revoked result logged: %+v, %v", logs, err)
	}
}

func TestRAGPublicationSerializesWithRevocation(t *testing.T) {
	if os.Getenv("YUNKUN_RAG_TEST_ISOLATED") != "1" {
		t.Skip("isolated database test disabled")
	}
	d := openIsolatedRAGTestDB(t)
	kb, err := d.CreateRAGKnowledgeBase("", "", "private", "publication-race", "", "", "", 900, 120)
	if err != nil {
		t.Fatal(err)
	}
	task, err := d.CreateTaskWithOptions("publication race", "revoke versus publish", TaskCreateOptions{RAGKnowledgeBaseIDs: []int64{kb.ID}})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	var publishID int64
	var publishErr, revokeErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		publishID, publishErr = d.PublishTaskRAGRetrieval(context.Background(), task.ID, kb.ID, "worker", nil, "race", 5, "keyword", nil, 1)
	}()
	go func() {
		defer wg.Done()
		<-start
		_, revokeErr = d.SetTaskRAGKnowledgeBaseIDs(task.ID, []int64{})
	}()
	close(start)
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("publication and revocation deadlocked")
	}
	if revokeErr != nil || (publishErr != nil && !errors.Is(publishErr, ErrTaskRAGAccessDenied)) {
		t.Fatalf("publish=%v, revoke=%v", publishErr, revokeErr)
	}
	logs, err := d.ListRAGRetrievalLogsByTask(fmt.Sprint(task.ID), 10)
	if err != nil {
		t.Fatal(err)
	}
	if (publishErr == nil) != (len(logs) == 1 && logs[0].ID == publishID) {
		t.Fatalf("publication disagrees with log: id=%d err=%v logs=%+v", publishID, publishErr, logs)
	}
}

func TestRAGPromptInclusionVerifiesPersistedResult(t *testing.T) {
	if os.Getenv("YUNKUN_RAG_TEST_ISOLATED") != "1" {
		t.Skip("isolated database test disabled")
	}
	d := openIsolatedRAGTestDB(t)
	if err := d.EnsureLLMUsageTable(); err != nil {
		t.Fatal(err)
	}
	kb, err := d.CreateRAGKnowledgeBase("", "", "private", "inclusion-test", "", "", "", 900, 120)
	if err != nil {
		t.Fatal(err)
	}
	task, err := d.CreateTaskWithOptions("inclusion", "test", TaskCreateOptions{RAGKnowledgeBaseIDs: []int64{kb.ID}})
	if err != nil {
		t.Fatal(err)
	}
	other, err := d.CreateTask("other", "test", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var logID int64
	err = d.QueryRow(`INSERT INTO rag_retrieval_logs(knowledge_base_id,task_id,agent_id,query,results)
VALUES ($1,$2,'worker','needle',$3::jsonb) RETURNING id`,
		kb.ID, fmt.Sprint(task.ID), `[{"chunk_id":7,"content":"exact chunk"}]`).Scan(&logID)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		task, content string
		chunk         int64
	}{
		{fmt.Sprint(other.ID), "exact chunk", 7},
		{fmt.Sprint(task.ID), "altered", 7},
		{fmt.Sprint(task.ID), "exact chunk", 8},
	} {
		if err := d.RecordRAGPromptInclusions(tc.task, "not-real", []RAGPromptCandidate{{
			RetrievalLogID: logID, ChunkID: tc.chunk, ToolUseID: "call-1", Content: tc.content,
		}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.RecordRAGPromptInclusions(fmt.Sprint(task.ID), "real", []RAGPromptCandidate{{
		RetrievalLogID: logID, ChunkID: 7, ToolUseID: "call-1", Content: "exact chunk",
	}}); err != nil {
		t.Fatal(err)
	}
	items, err := d.ListTaskRAGPromptInclusions(fmt.Sprint(task.ID), 10)
	if err != nil || len(items) != 1 || items[0].RequestID != "real" || items[0].ChunkID != 7 {
		t.Fatalf("inclusion verification: %+v, %v", items, err)
	}
}
