package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"yunkun/console/db"
)

func TestFindingLearningCreatesReviewProposal(t *testing.T) {
	p, err := (findingLearning{
		Reusable: true, KnowledgeBaseID: 4, Title: "Review role boundary",
		Method:        "Compare the same object under two approved roles.",
		Applicability: "Authorized role and object fixtures.",
		Limitations:   "Do not infer impact from status codes alone.",
	}).proposal(2, 17, "worker")
	if err != nil {
		t.Fatal(err)
	}
	if p.SourceRef != "finding:17" || p.TaskID != 2 || p.KnowledgeBaseID != 4 ||
		!strings.Contains(p.Content, "Compare the same object") {
		t.Fatalf("proposal lost provenance or content: %+v", p)
	}
	if _, err := (findingLearning{Reusable: true}).proposal(2, 17, "worker"); err == nil {
		t.Fatal("empty learning candidate accepted")
	}
	if p, err := (findingLearning{Reusable: false}).proposal(2, 17, "worker"); err != nil || p.SourceRef != "" {
		t.Fatal("non-reusable finding created a proposal")
	}
	if _, err := (findingLearning{
		Reusable: true, KnowledgeBaseID: 4, Title: "Secret",
		Method: "Authorization: Bearer real-value", Applicability: "authorized",
		Limitations: "verified",
	}).proposal(2, 17, "worker"); err == nil {
		t.Fatal("credential entered review queue")
	}
}

func TestFindingAutoLearningRequiresToolPermissionAndReview(t *testing.T) {
	if os.Getenv("YUNKUN_RAG_TEST_ISOLATED") != "1" {
		t.Skip("requires isolated PostgreSQL")
	}
	d := testDB(t)
	defer d.Close()
	kb, err := d.CreateRAGKnowledgeBase("", "", "private", "learning-fixture", "", "", "", 900, 120)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteRAGKnowledgeBase(kb.ID)
	task, err := d.CreateTaskWithOptions("learning", "fixture", db.TaskCreateOptions{RAGKnowledgeBaseIDs: []int64{kb.ID}})
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)
	store := d.Exploration(task.ExplorationID)
	intent, err := store.AddIntent(map[string]any{"summary": "validate"}, 1, nil, "planner")
	if err != nil {
		t.Fatal(err)
	}
	step, err := store.AppendActivity(db.Activity{NodeID: &intent, Kind: "tool_result", Tool: "fixture", Detail: "confirmed"})
	if err != nil {
		t.Fatal(err)
	}
	ts := NewToolSet(store, "worker")
	ts.SetTaskID(task.ID)
	ts.SetRAGDB(d)
	args := map[string]any{
		"vulnclass": "fixture", "severity": "medium", "summary": "observed difference",
		"evidence": "fixture trace", "intent_id": intent,
		"validation": map[string]any{
			"authorization": "local fixture", "asset": "fixture", "identity": "test user",
			"preconditions": "test", "baseline": "first response", "mutation": "controlled change",
			"observed_delta": "second response", "security_impact": "verified difference",
			"cleanup": "no change", "evidence_step_ids": []int64{step},
		},
		"learning": map[string]any{
			"reusable": true, "knowledge_base_id": kb.ID, "title": "Compare approved roles",
			"method":        "Compare the same object under two approved roles.",
			"applicability": "Authorized fixture accounts.",
			"limitations":   "Do not infer impact from status alone.",
		},
	}
	call := func() string {
		t.Helper()
		raw, _ := json.Marshal(args)
		out, err := ts.addFinding().Call(context.Background(), raw, nil)
		if err != nil || out.IsError {
			t.Fatalf("finding rejected: %v %s", err, out.Flatten())
		}
		return out.Flatten()
	}
	tool, err := d.GetTool("propose_rag_change")
	if err != nil {
		t.Fatal(err)
	}
	if tool == nil {
		seed := NewToolSet(nil, "").proposeRAGChange()
		schema, _ := json.Marshal(seed.InputSchema())
		if err := d.SeedTool(seed.Name(), seed.Description(), schema, json.RawMessage(`["worker"]`)); err != nil {
			t.Fatal(err)
		}
		tool, err = d.GetTool("propose_rag_change")
		if err != nil || tool == nil {
			t.Fatalf("could not seed proposal tool: %v", err)
		}
	}
	originalAgents, _ := json.Marshal(tool.Agents)
	t.Cleanup(func() {
		d.UpdateTool(tool.Key, tool.Description, tool.Schema, originalAgents, tool.Enabled)
	})
	if err := d.UpdateTool(tool.Key, tool.Description, tool.Schema, json.RawMessage(`["worker"]`), false); err != nil {
		t.Fatal(err)
	}
	if result := call(); !strings.Contains(result, "知识提案工具已禁用") {
		t.Fatal(result)
	}
	pending, err := d.ListRAGProposals(kb.ID, "pending")
	if err != nil || len(pending) != 0 {
		t.Fatalf("disabled tool created proposals: %v %v", len(pending), err)
	}
	if err := d.UpdateTool(tool.Key, tool.Description, tool.Schema, json.RawMessage(`["worker"]`), true); err != nil {
		t.Fatal(err)
	}
	result := call()
	if !strings.Contains(result, "learning_proposal_id") {
		t.Fatal(result)
	}
	pending, err = d.ListRAGProposals(kb.ID, "pending")
	if err != nil || len(pending) != 1 || pending[0].SourceRef == "" || pending[0].Status != "pending" {
		t.Fatalf("expected pending, traceable review: %+v %v", pending, err)
	}
	if got := pending[0].SourceRef; !strings.HasPrefix(got, "finding:") {
		t.Fatal(fmt.Sprintf("missing finding provenance: %s", got))
	}
	chunks, err := d.SearchRAGChunks(kb.ID, "Compare the same object under two approved roles", 5)
	if err != nil || len(chunks) != 0 {
		t.Fatalf("unapproved learning searchable: %v %v", chunks, err)
	}
}
