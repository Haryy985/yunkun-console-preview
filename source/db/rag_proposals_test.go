package db

import (
	"errors"
	"os"
	"sync"
	"testing"
)

func TestRAGProposalReviewLifecycle(t *testing.T) {
	if os.Getenv("YUNKUN_RAG_TEST_ISOLATED") != "1" {
		t.Skip("isolated database test disabled")
	}
	d := openIsolatedRAGTestDB(t)
	var err error
	kb, err := d.CreateRAGKnowledgeBase("", "", "private", "review-test", "", "", "", 900, 120)
	if err != nil {
		t.Fatal(err)
	}
	task, err := d.CreateTaskWithOptions("rag review test", "review lifecycle", TaskCreateOptions{RAGKnowledgeBaseIDs: []int64{kb.ID}})
	if err != nil {
		t.Fatal(err)
	}
	proposal := RAGChangeProposal{
		KnowledgeBaseID: kb.ID, TaskID: task.ID, AgentID: "worker", Kind: "create",
		Title: "review.md", Content: "review_lifecycle_unique", Rationale: "observed in test",
		SourceRef: "test:fixture-1",
	}
	if _, err = d.CreateRAGProposal(RAGChangeProposal{KnowledgeBaseID: kb.ID, TaskID: task.ID, AgentID: "worker", Kind: "create"}); !errors.Is(err, ErrRAGProposalInvalid) {
		t.Fatalf("empty content accepted: %v", err)
	}
	if _, err = d.CreateRAGProposal(RAGChangeProposal{
		KnowledgeBaseID: kb.ID + 1000, TaskID: task.ID, AgentID: "worker", Kind: "create",
		Title: "wrong.md", Content: "text", Rationale: "test", SourceRef: "test:fixture",
	}); !errors.Is(err, ErrTaskRAGAccessDenied) {
		t.Fatalf("unknown KB accepted: %v", err)
	}
	otherKB, err := d.CreateRAGKnowledgeBase("", "", "private", "review-test-other", "", "", "", 900, 120)
	if err != nil {
		t.Fatal(err)
	}
	otherDoc, _, err := d.CreateRAGDocument(otherKB.ID, "other.md", "manual", "", "text/markdown", "other", "other", "other")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d.CreateRAGProposal(RAGChangeProposal{
		KnowledgeBaseID: kb.ID, TaskID: task.ID, AgentID: "worker", Kind: "update",
		TargetDocumentID: &otherDoc.ID, Title: "wrong.md", Content: "text",
		Rationale: "test", SourceRef: "test:fixture",
	}); !errors.Is(err, ErrRAGDocumentNotFound) {
		t.Fatalf("cross-KB update accepted: %v", err)
	}
	p, err := d.CreateRAGProposal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	results, err := d.SearchRAGChunks(kb.ID, "review_lifecycle_unique", 5)
	if err != nil || len(results) != 0 {
		t.Fatalf("pending proposal searchable: %v, %v", results, err)
	}
	rejected, err := d.CreateRAGProposal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	if _, job, err := d.ReviewRAGProposal(rejected.ID, false, "admin", "not verified"); err != nil || job != 0 {
		t.Fatalf("reject: job=%d err=%v", job, err)
	}
	if _, _, err := d.ReviewRAGProposal(rejected.ID, true, "admin", ""); !errors.Is(err, ErrRAGProposalConflict) {
		t.Fatalf("rejected proposal approved: %v", err)
	}
	approved, job, err := d.ReviewRAGProposal(p.ID, true, "admin", "verified")
	if err != nil || approved.PublishedDocumentID == nil || job == 0 {
		t.Fatalf("approve: %#v, job=%d, err=%v", approved, job, err)
	}
	if _, _, err := d.ReviewRAGProposal(p.ID, true, "admin", ""); !errors.Is(err, ErrRAGProposalConflict) {
		t.Fatalf("duplicate approval: %v", err)
	}
	concurrent := proposal
	concurrent.Content = "concurrent_review_unique"
	race, err := d.CreateRAGProposal(concurrent)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	outcomes := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := d.ReviewRAGProposal(race.ID, true, "admin", "checked")
			outcomes <- err
		}()
	}
	wg.Wait()
	close(outcomes)
	successes, conflicts := 0, 0
	for err := range outcomes {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrRAGProposalConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent review error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent reviews: success=%d conflict=%d", successes, conflicts)
	}
	docID := *approved.PublishedDocumentID
	if err := d.ReplaceRAGDocumentChunks(docID, []RAGChunkInput{{Ordinal: 1, HeadingPath: []string{}, Content: proposal.Content, ContentHash: "fixture"}}); err != nil {
		t.Fatal(err)
	}
	results, err = d.SearchRAGChunks(kb.ID, "review_lifecycle_unique", 5)
	if err != nil || len(results) != 0 {
		t.Fatalf("unfinished ingestion searchable: %v, %v", results, err)
	}
	if err := d.MarkRAGIngestionJob(job, "running", ""); err != nil {
		t.Fatal(err)
	}
	if err := d.CompleteRAGProposalIngestion(docID, job); err != nil {
		t.Fatal(err)
	}
	results, err = d.SearchRAGChunks(kb.ID, "review_lifecycle_unique", 5)
	if err != nil || len(results) != 1 {
		t.Fatalf("published content absent: %v, %v", results, err)
	}

	update := RAGChangeProposal{
		KnowledgeBaseID: kb.ID, TaskID: task.ID, AgentID: "worker", Kind: "update",
		TargetDocumentID: &docID, Title: "review-v2.md", Content: "review_lifecycle_revised",
		Rationale: "verified correction", SourceRef: "test:fixture-2",
	}
	stale, err := d.CreateRAGProposal(update)
	if err != nil {
		t.Fatal(err)
	}
	next, err := d.CreateRAGProposal(update)
	if err != nil {
		t.Fatal(err)
	}
	accepted, nextJob, err := d.ReviewRAGProposal(next.ID, true, "admin", "verified")
	if err != nil {
		t.Fatal(err)
	}
	if err := d.ReplaceRAGDocumentChunks(*accepted.PublishedDocumentID, []RAGChunkInput{{Ordinal: 1, HeadingPath: []string{}, Content: update.Content, ContentHash: "fixture2"}}); err != nil {
		t.Fatal(err)
	}
	if err := d.MarkRAGIngestionJob(nextJob, "running", ""); err != nil {
		t.Fatal(err)
	}
	if err := d.CompleteRAGProposalIngestion(*accepted.PublishedDocumentID, nextJob); err != nil {
		t.Fatal(err)
	}
	if _, _, err := d.ReviewRAGProposal(stale.ID, true, "admin", ""); !errors.Is(err, ErrRAGProposalConflict) {
		t.Fatalf("stale revision accepted: %v", err)
	}
	results, err = d.SearchRAGChunks(kb.ID, "review_lifecycle_unique", 5)
	if err != nil || len(results) != 0 {
		t.Fatalf("old version still searchable: %v, %v", results, err)
	}
	results, err = d.SearchRAGChunks(kb.ID, "review_lifecycle_revised", 5)
	if err != nil || len(results) != 1 {
		t.Fatalf("new version missing: %v, %v", results, err)
	}
}
