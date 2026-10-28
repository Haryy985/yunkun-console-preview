package server

import (
	"fmt"
	"testing"

	"yunkun/console/db"
)

func TestFindingBatchDeleteIsAtomic(t *testing.T) {
	s, finding, request := trafficEvidenceServer(t)
	other, err := s.m.pg.Exploration(s.m.ResolveTask(fmt.Sprint(*findingTaskID(t, s, finding))).ExpID).RecordFinding(
		t.Context(), db.RecordFindingInput{TaskID: *findingTaskID(t, s, finding), Worker: "fixture",
			VulnClass: "logic", Severity: db.SeverityHigh, Summary: "other", Evidence: "proof"})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/exploration/findings/batch-delete"
	for _, test := range []struct {
		body string
		code int
	}{
		{fmt.Sprintf(`{"ids":[%d,%d]}`, finding.FindingID, finding.FindingID), 400},
		{fmt.Sprintf(`{"ids":[%d,999999999]}`, finding.FindingID), 409},
	} {
		if r := request("POST", path, test.body); r.Code != test.code {
			t.Fatalf("selection status=%d want=%d body=%s response=%s", r.Code, test.code, test.body, r.Body.String())
		}
		if row, err := s.m.pg.GetFinding(finding.FindingID); err != nil || row == nil {
			t.Fatalf("partial deletion: %+v, %v", row, err)
		}
	}
	if r := request("POST", path, fmt.Sprintf(`{"ids":[%d,%d]}`, other.FindingID, finding.FindingID)); r.Code != 200 {
		t.Fatalf("batch delete: %d %s", r.Code, r.Body.String())
	}
	for _, id := range []int64{other.FindingID, finding.FindingID} {
		if row, err := s.m.pg.GetFinding(id); err != nil || row != nil {
			t.Fatalf("finding %d remains: %+v, %v", id, row, err)
		}
	}
}

func findingTaskID(t *testing.T, s *Server, finding *db.RecordedFinding) *int64 {
	t.Helper()
	row, err := s.m.pg.GetFinding(finding.FindingID)
	if err != nil || row == nil || row.TaskID == nil {
		t.Fatalf("missing task: %+v %v", row, err)
	}
	return row.TaskID
}
