package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// TestTaskTraceBaseline verifies the identifiers already available to a task
// trace. It deliberately does not infer a Finding-to-tool or RAG-to-LLM edge.
func TestTaskTraceBaseline(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v)", err)
	}
	defer d.Close()
	if err := d.EnsureLLMUsageTable(); err != nil {
		t.Fatal(err)
	}
	if err := d.EnsureLLMRecordsTable(); err != nil {
		t.Fatal(err)
	}

	task, err := d.CreateTask("trace baseline", "fixture", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	taskID := fmt.Sprint(task.ID)
	sourceID := "trace-fixture-" + taskID
	var snapshotID string
	t.Cleanup(func() {
		_, _ = d.Exec(`DELETE FROM rag_retrieval_logs WHERE task_id=$1`, taskID)
		_, _ = d.Exec(`DELETE FROM llm_usage WHERE task_id=$1`, taskID)
		_, _ = d.Exec(`DELETE FROM llm_records WHERE task_id=$1`, taskID)
		_, _ = d.Exec(`DELETE FROM findings WHERE task_id=$1`, task.ID)
		if snapshotID != "" {
			_, _ = d.Exec(`DELETE FROM traffic_evidence_snapshots WHERE id=$1`, snapshotID)
		}
		_ = d.DeleteTask(task.ID)
	})
	store := d.Exploration(task.ExplorationID)
	intentID, err := store.AddNode(KindIntent, map[string]any{"summary": "verify fixture"}, 5, "open", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	useID, err := store.AppendActivity(Activity{
		NodeID: &intentID, Worker: "worker", Kind: "tool_use",
		Tool: "Bash", ToolUseID: "trace-fixture-call", Summary: "controlled probe",
	})
	if err != nil {
		t.Fatal(err)
	}
	resultID, err := store.AppendActivity(Activity{
		NodeID: &intentID, Worker: "worker", Kind: "tool_result",
		Tool: "Bash", ToolUseID: "trace-fixture-call", Summary: "fixture result",
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := TrafficEvidenceSnapshot{
		SourceTrafficID: sourceID, CapturedAt: time.Now().Unix(),
		URL: "https://fixture.invalid/probe", Method: "GET", Status: 200,
		ReqHead: "GET /probe HTTP/1.1", RespHead: "HTTP/1.1 200 OK",
		ReqHash: "fixture-request", RespHash: "fixture-response",
	}
	snapshot.ID = TrafficSnapshotID(snapshot)
	snapshotID = snapshot.ID
	var finding *RecordedFinding
	err = d.WithEvidenceTx(context.Background(), func(tx *sql.Tx) error {
		var recordErr error
		finding, recordErr = RecordFindingTx(context.Background(), tx, RecordFindingInput{
			TaskID: task.ID, IntentID: intentID, VulnClass: "fixture",
			ExplorationID: task.ExplorationID, Severity: SeverityLow,
			Summary: "controlled finding", Worker: "worker",
		}, []PreparedTrafficEvidence{{
			Ref: TrafficRef{TrafficID: sourceID, Role: "proof"}, Snapshot: snapshot,
		}})
		return recordErr
	})
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := d.GetFindingTraffic(context.Background(), finding.FindingID)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings.Bindings) != 1 || bindings.Bindings[0].SnapshotID != snapshotID ||
		bindings.Bindings[0].Snapshot.SourceTrafficID != sourceID {
		t.Fatalf("finding traffic attribution: %+v", bindings)
	}
	if err := d.InsertLLMUsage(&LLMUsage{
		TaskID: taskID, ExplorationID: task.ExplorationID, Worker: "worker",
		Model: "fixture-model", SessionID: "exp-fixture-worker", RequestID: "trace-fixture-" + taskID,
		InputTokens: 10, OutputTokens: 2, Status: "ok",
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.InsertLLMRecord(&LLMRecord{
		TaskID: taskID, SessionID: "exp-fixture-worker", RequestID: "trace-fixture-" + taskID,
		Worker: "worker", Model: "fixture-model", InputTokens: 10, Status: "ok",
	}); err != nil {
		t.Fatal(err)
	}
	var correlated int
	if err := d.QueryRow(`
SELECT count(*) FROM llm_usage u JOIN llm_records r ON r.request_id=u.request_id
WHERE u.task_id=$1 AND r.task_id=$1 AND u.session_id=r.session_id`, taskID).Scan(&correlated); err != nil || correlated != 1 {
		t.Fatalf("request correlation=%d, %v", correlated, err)
	}
	latest, err := d.LatestTaskLLMUsage(taskID)
	if err != nil || latest == nil || latest.RequestID != "trace-fixture-"+taskID ||
		latest.SessionID != "exp-fixture-worker" || latest.InputTokens != 10 {
		t.Fatalf("latest request: %+v, %v", latest, err)
	}
	allLatest, err := d.LatestTaskLLMUsageAll()
	if err != nil || allLatest[taskID].RequestID != latest.RequestID {
		t.Fatalf("bulk latest: %+v, %v", allLatest[taskID], err)
	}

	var linkedUse, linkedResult, linkedFinding, metered int
	err = d.QueryRow(`
SELECT (SELECT count(*) FROM activity
        WHERE exploration_id=$1 AND node_id=$2 AND id=$3 AND tool_use_id=$4),
       (SELECT count(*) FROM activity
        WHERE exploration_id=$1 AND node_id=$2 AND id=$5 AND tool_use_id=$4),
       (SELECT count(*) FROM findings f
        JOIN exploration_edges e ON e.dst_id=f.node_id AND e.exploration_id=$1
        WHERE f.id=$6 AND f.task_id=$7 AND e.src_id=$2 AND e.rel=$8),
       (SELECT count(*) FROM llm_usage
        WHERE task_id=$9 AND exploration_id=$1 AND worker='worker')`,
		task.ExplorationID, intentID, useID, "trace-fixture-call", resultID,
		finding.FindingID, task.ID, RelYields, taskID).
		Scan(&linkedUse, &linkedResult, &linkedFinding, &metered)
	if err != nil {
		t.Fatal(err)
	}
	if linkedUse != 1 || linkedResult != 1 || linkedFinding != 1 || metered != 1 {
		t.Fatalf("trace links: tool use=%d result=%d finding=%d usage=%d",
			linkedUse, linkedResult, linkedFinding, metered)
	}

	kb, err := d.CreateRAGKnowledgeBase("trace-fixture", "trace-fixture", "private",
		fmt.Sprintf("trace-fixture-%d", time.Now().UnixNano()), "", "local", "fixture", 512, 64)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = d.Exec(`DELETE FROM rag_knowledge_bases WHERE id=$1`, kb.ID)
	})
	logID, err := d.LogRAGRetrieval(kb.ID, taskID, "worker", nil,
		"controlled search", 5, "keyword", nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	ref := json.RawMessage(fmt.Sprintf(`{"record_ref":{"kind":"rag_retrieval","id":"%d"}}`, logID))
	refActivityID, err := store.AppendActivity(Activity{
		NodeID: &intentID, Worker: "worker", Kind: "tool_result",
		Tool: "rag_search", ToolUseID: "trace-rag-call", Metadata: ref,
	})
	if err != nil {
		t.Fatal(err)
	}
	activities, _, err := store.ActivityList(&intentID, resultID, 10)
	if err != nil {
		t.Fatal(err)
	}
	var persisted bool
	for _, a := range activities {
		var metadata struct {
			RecordRef struct {
				Kind string `json:"kind"`
				ID   string `json:"id"`
			} `json:"record_ref"`
		}
		if err := json.Unmarshal(a.Metadata, &metadata); err != nil {
			t.Fatal(err)
		}
		persisted = persisted || a.ID == refActivityID &&
			a.ToolUseID == "trace-rag-call" && metadata.RecordRef.Kind == "rag_retrieval" &&
			metadata.RecordRef.ID == fmt.Sprint(logID)
	}
	if !persisted {
		t.Fatalf("retrieval call reference not persisted: %+v", activities)
	}
	logs, err := d.ListRAGRetrievalLogsByTask(taskID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].ID != logID || logs[0].AgentID != "worker" ||
		logs[0].KnowledgeBaseID != kb.ID || logs[0].ResultCount != 0 {
		t.Fatalf("retrieval attribution: %+v", logs)
	}
	ragStats, err := d.TaskRAGStatsAll()
	if err != nil || ragStats[taskID].Searches != 1 || ragStats[taskID].Nonempty != 0 ||
		ragStats[taskID].ReturnedChunks != 0 {
		t.Fatalf("rag stats: %+v, %v", ragStats[taskID], err)
	}
	if _, err := d.Exec(`UPDATE rag_retrieval_logs SET results='null'::jsonb WHERE id=$1`, logID); err != nil {
		t.Fatal(err)
	}
	legacy, err := d.ListRAGRetrievalLogsByTask(taskID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(legacy) != 1 || legacy[0].ResultCount != 0 || len(legacy[0].Results) != 0 {
		t.Fatalf("legacy empty retrieval: %+v", legacy)
	}
}
