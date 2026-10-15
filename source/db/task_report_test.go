package db

import (
	"os"
	"testing"
)

func TestTaskReportReferencesRequireMatchingTaskAndNode(t *testing.T) {
	if os.Getenv("YUNKUN_RAG_TEST_ISOLATED") != "1" {
		t.Skip("isolated database test disabled")
	}
	d := openIsolatedRAGTestDB(t)
	owner, err := d.CreateTask("report", "verify", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	other, err := d.CreateTask("other", "verify", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	insert := func(expID int64) int64 {
		t.Helper()
		var nodeID int64
		if err := d.QueryRow(`INSERT INTO exploration_nodes(exploration_id,kind,state,payload)
VALUES ($1,'finding','confirmed','{}'::jsonb) RETURNING id`, expID).Scan(&nodeID); err != nil {
			t.Fatal(err)
		}
		return nodeID
	}
	nodeID := insert(owner.ExplorationID)
	otherNodeID := insert(other.ExplorationID)
	findingID, err := d.AddFinding(owner.ID, nodeID, "test", "verified", "high", "summary", "proof", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.AddFinding(other.ID, otherNodeID, "test", "other", "low", "summary", "", "worker", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`UPDATE findings SET report='detailed', evidence_version=2, report_evidence_version=1 WHERE id=$1`, findingID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO traffic_evidence_snapshots
(id,source_traffic_id,captured_at,url,method,status,req_head,resp_head,req_hash,resp_hash,req_len,resp_len)
VALUES ('snapshot-1','exchange-1',1,'http://localhost/','GET',200,'','','req','resp',0,0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO finding_traffic_bindings(finding_id,snapshot_id,role,position)
VALUES ($1,'snapshot-1','proof',0)`, findingID); err != nil {
		t.Fatal(err)
	}
	refs, err := d.TaskReportReferences(owner.ID, []int64{nodeID, otherNodeID})
	if err != nil || len(refs) != 1 {
		t.Fatalf("task-scoped references: %+v, %v", refs, err)
	}
	got := refs[nodeID]
	if got.FindingID != findingID || got.Report != "detailed" || got.EvidenceVersion != 2 ||
		len(got.Traffic) != 1 || got.Traffic[0].SourceTrafficID != "exchange-1" {
		t.Fatalf("finding evidence: %+v", got)
	}
}
