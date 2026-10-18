package server

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestTaskProfileRoutesRequireAdmin(t *testing.T) {
	for _, role := range []string{"viewer", "operator"} {
		for _, method := range []string{"GET", "POST"} {
			if roleAllows(role, method, "/api/tasks/1/target-profile") {
				t.Fatalf("%s can %s task profile", role, method)
			}
		}
	}
}

func TestTaskProfileHTTPValidation(t *testing.T) {
	dsn := os.Getenv("YUNKUN_TEST_PG_DSN")
	u, err := url.Parse(dsn)
	if dsn == "" || err != nil || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "yunkun_test_") {
		t.Skip("isolated YUNKUN_TEST_PG_DSN required")
	}
	t.Setenv("YUNKUN_PG_DSN", dsn)
	s, f, request := trafficEvidenceServer(t)
	var taskID int64
	if err := s.m.pg.QueryRow(`SELECT task_id FROM findings WHERE id=$1`, f.FindingID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	_, err = s.m.Assets().AddAgentScope(taskID, "ip", "192.0.2.10", "authorized", "manual")
	if err != nil {
		t.Fatal(err)
	}
	scopes, err := s.m.Assets().ListTaskScope(taskID)
	if err != nil || len(scopes) == 0 {
		t.Fatalf("scope=%+v err=%v", scopes, err)
	}
	path := fmt.Sprintf("/api/tasks/%d/target-profile", taskID)
	body := fmt.Sprintf(`{"scope_id":%d,"kind":"host","label":"candidate.example.test","evidence_kind":"inferred","source_summary":"operator note","confidence":40}`, scopes[0].ID)
	w := request("POST", path, body)
	if w.Code != 201 {
		t.Fatalf("create status=%d body=%s", w.Code, w.Body.String())
	}
	w = request("GET", path+"?limit=1", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "candidate.example.test") {
		t.Fatalf("list status=%d body=%s", w.Code, w.Body.String())
	}
	w = request("GET", path+"?limit=999", "")
	if w.Code != 400 {
		t.Fatalf("invalid limit status=%d", w.Code)
	}
	otherTask, err := s.m.pg.CreateTask("unrelated profile task", "goal", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.m.pg.DeleteTask(otherTask.ID)
	crossTask := fmt.Sprintf(`{"scope_id":%d,"kind":"host","label":"candidate.example.test","evidence_kind":"inferred","source_summary":"note","confidence":40}`, scopes[0].ID)
	w = request("POST", fmt.Sprintf("/api/tasks/%d/target-profile", otherTask.ID), crossTask)
	// A task not loaded in the runtime registry is still rejected.
	if w.Code != 404 {
		t.Fatalf("cross-task status=%d body=%s", w.Code, w.Body.String())
	}
	var after int
	if err := s.m.pg.QueryRowContext(context.Background(), `SELECT count(*) FROM task_scope WHERE task_id=$1`, taskID).Scan(&after); err != nil || after != len(scopes) {
		t.Fatalf("profile changed scope: count=%d err=%v", after, err)
	}
}
