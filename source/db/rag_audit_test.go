package db

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func auditRAGBase(t *testing.T) (*DB, *RAGKnowledgeBase) {
	t.Helper()
	d, err := Open(testDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	kb, err := d.CreateRAGKnowledgeBase("", "", "private",
		fmt.Sprintf("audit-%d", time.Now().UnixNano()), "", "", "", 900, 120)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := d.Exec("DELETE FROM rag_knowledge_bases WHERE id=$1", kb.ID); err != nil {
			t.Error(err)
		}
	})
	return d, kb
}

func TestRAGKnowledgeBaseInvalidUpdateIsAtomic(t *testing.T) {
	d, kb := auditRAGBase(t)
	for _, changeSize := range []bool{true, false} {
		var size, overlap *int
		value := 100
		if changeSize {
			size = &value
		} else {
			value = 900
			overlap = &value
		}
		description := "must not be persisted"
		if _, err := d.UpdateRAGKnowledgeBase(kb.ID, nil, &description, nil, nil, nil, nil, size, overlap); !errors.Is(err, ErrRAGKnowledgeBaseInvalid) {
			t.Fatalf("invalid update: %v", err)
		}
		current, err := d.GetRAGKnowledgeBase(kb.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.ChunkSize != 900 || current.ChunkOverlap != 120 || current.Description != "" {
			t.Fatalf("rejected update modified stored settings: %#v", current)
		}
	}
	size, overlap := 100, 20
	current, err := d.UpdateRAGKnowledgeBase(kb.ID, nil, nil, nil, nil, nil, nil, &size, &overlap)
	if err != nil || current.ChunkSize != size || current.ChunkOverlap != overlap {
		t.Fatalf("valid combined update rejected: %#v %v", current, err)
	}
}

func TestRAGKnowledgeBaseRenameConflict(t *testing.T) {
	d, kb := auditRAGBase(t)
	other, err := d.CreateRAGKnowledgeBase("", "", "private", kb.Name+"-other", "", "", "", 900, 120)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = d.Exec("DELETE FROM rag_knowledge_bases WHERE id=$1", other.ID) })
	if _, err := d.UpdateRAGKnowledgeBase(other.ID, &kb.Name, nil, nil, nil, nil, nil, nil, nil); !errors.Is(err, ErrRAGKnowledgeBaseNameUsed) {
		t.Fatalf("rename conflict must be a domain error: %v", err)
	}
}

func TestRAGEvalCaseRoundTripAndRun(t *testing.T) {
	d, kb := auditRAGBase(t)
	set, err := d.CreateRAGEvalSet(kb.ID, "regression", "")
	if err != nil {
		t.Fatal(err)
	}
	doc, _, err := d.CreateRAGDocument(kb.ID, "test.md", "text", "", "text/markdown", "audit", "audit", "auditmarker")
	if err != nil {
		t.Fatal(err)
	}
	if err := d.ReplaceRAGDocumentChunks(doc.ID, []RAGChunkInput{{Ordinal: 1, Content: "auditmarker", ContentHash: "audit"}}); err != nil {
		t.Fatal(err)
	}
	chunks, err := d.ListRAGDocumentChunks(doc.ID)
	if err != nil || len(chunks) != 1 {
		t.Fatalf("chunks: %v %v", chunks, err)
	}
	wantChunks, wantDocs := []int64{chunks[0].ID}, []int64{doc.ID}
	item, err := d.CreateRAGEvalCase(set.ID, "auditmarker", wantChunks, wantDocs, true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(item.ExpectedChunkIDs, wantChunks) || !reflect.DeepEqual(item.ExpectedDocumentIDs, wantDocs) {
		t.Fatalf("created case lost expected IDs: %#v", item)
	}
	empty, err := d.CreateRAGEvalCase(set.ID, "missingmarker", nil, nil, false)
	if err != nil || empty.ExpectedChunkIDs == nil || empty.ExpectedDocumentIDs == nil {
		t.Fatalf("empty IDs must be arrays: %#v %v", empty, err)
	}
	items, err := d.ListRAGEvalCases(set.ID)
	if err != nil || len(items) != 2 {
		t.Fatalf("list: %#v %v", items, err)
	}
	if !reflect.DeepEqual(items[0].ExpectedChunkIDs, wantChunks) || !reflect.DeepEqual(items[0].ExpectedDocumentIDs, wantDocs) {
		t.Fatalf("listed case lost expected IDs: %#v", items[0])
	}
	run, err := d.RunRAGEval(set.ID, 5)
	if err != nil || run.Status != "succeeded" || run.RecallAtK == nil || *run.RecallAtK != 0.5 {
		t.Fatalf("evaluation run: %#v %v", run, err)
	}
}

func TestRAGIngestionRestartRecovery(t *testing.T) {
	d, kb := auditRAGBase(t)
	for _, status := range []string{"queued", "running", "succeeded", "failed"} {
		doc, jobID, err := d.CreateRAGDocument(kb.ID, status+".txt", "text", "", "text/plain", status, status, status)
		if err != nil {
			t.Fatal(err)
		}
		if err := d.MarkRAGIngestionJob(jobID, status, ""); err != nil {
			t.Fatal(err)
		}
		docStatus := "ready"
		if status == "queued" || status == "running" {
			docStatus = "embedding"
			_, created, err := d.QueueRAGDocumentJob(doc.ID, "document_reparse")
			if err != nil || created {
				t.Fatalf("expected orphaned job to block retry before recovery: %v %v", created, err)
			}
		}
		if err := d.UpdateRAGDocumentStatus(doc.ID, docStatus, ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.RecoverRAGIngestionJobs(); err != nil {
		t.Fatal(err)
	}
	jobs, err := d.ListRAGIngestionJobs(&kb.ID)
	if err != nil || len(jobs) != 4 {
		t.Fatalf("jobs: %v %v", jobs, err)
	}
	for _, job := range jobs {
		doc, err := d.GetRAGDocument(*job.DocumentID)
		if err != nil {
			t.Fatal(err)
		}
		switch doc.Name {
		case "queued.txt", "running.txt":
			if job.Status != "failed" || job.FinishedAt == nil || job.Error == "" || doc.Status != "failed" {
				t.Fatalf("incomplete recovery: %#v %#v", job, doc)
			}
		default:
			if job.Error != "" || doc.Status != "ready" {
				t.Fatalf("terminal job modified: %#v %#v", job, doc)
			}
		}
	}
	if count, err := d.RecoverRAGIngestionJobs(); err != nil || count != 0 {
		t.Fatalf("recovery must be idempotent: %d %v", count, err)
	}
	for _, job := range jobs {
		if job.Error != "" {
			next, created, err := d.QueueRAGDocumentJob(*job.DocumentID, "document_reparse")
			if err != nil || !created || next == job.ID {
				t.Fatalf("retry still blocked after recovery: %d %v %v", next, created, err)
			}
		}
	}
}
