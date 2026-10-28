package server

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"yunkun/console/db"
)

func TestConfidentialAccessFailsClosed(t *testing.T) {
	s := &Server{m: &Manager{pg: &db.DB{}}}
	for _, tc := range []struct {
		name     string
		identity *authIdentity
		want     int
	}{
		{"missing identity", nil, 403},
		{"operator", &authIdentity{Username: "operator", Role: "operator"}, 403},
		{"admin without audit database", &authIdentity{Username: "yunkun", Role: "admin"}, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/api/exploration/findings/export", nil)
			if tc.identity != nil {
				r = r.WithContext(context.WithValue(r.Context(), authContextKey{}, tc.identity))
			}
			w := httptest.NewRecorder()
			if s.auditConfidentialAccess(w, r, "export", "finding_report", 1, "md-single:selected:1") {
				t.Fatal("unavailable audit or insufficient role allowed confidential data")
			}
			if w.Code != tc.want || w.Header().Get("Cache-Control") != "no-store" ||
				w.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("response %d headers=%v body=%s", w.Code, w.Header(), w.Body.String())
			}
		})
	}
	for _, role := range []string{"operator", "viewer"} {
		for _, path := range []string{
			"/api/exploration/findings/export",
			"/api/exploration/findings/1",
			"/api/exploration/findings/1/traffic/2",
			"/api/exploration/findings/1/traffic/2/body",
			"/api/report",
			"/api/confidential-access-events",
		} {
			if roleAllows(role, "GET", path) {
				t.Fatalf("%s can access %s", role, path)
			}
		}
	}
	r := httptest.NewRequest("GET", "/api/confidential-access-events", nil)
	w := httptest.NewRecorder()
	s.listConfidentialAccess(w, r)
	if w.Code != 403 {
		t.Fatalf("audit log accessible without identity: %d", w.Code)
	}
}

func TestConfidentialFindingExportPreservesCapturedCookieAndAudits(t *testing.T) {
	dsn := os.Getenv("YUNKUN_TEST_PG_DSN")
	u, err := url.Parse(dsn)
	if dsn == "" || err != nil || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "yunkun_test_") {
		t.Skip("YUNKUN_TEST_PG_DSN must point to an isolated yunkun_test_* database")
	}
	t.Setenv("YUNKUN_PG_DSN", dsn)
	s, f, request := trafficEvidenceServer(t)
	seedServerEvidenceFlow(t, s, "confidential-cookie", []byte("response"))
	const secret = "sid=confidential-test-value"
	_, err = s.m.traffic.DB().Exec(`UPDATE exchange_bodies SET req_head=?
		WHERE id=?`, "GET /private HTTP/1.1\r\nHost: evidence.local\r\nCookie: "+secret, "confidential-cookie")
	if err != nil {
		t.Fatal(err)
	}
	binding, err := s.evidenceStore().Bind(context.Background(), f.FindingID,
		[]db.TrafficRef{{TrafficID: "confidential-cookie", Role: "proof"}})
	if err != nil {
		t.Fatal(err)
	}
	approveFormalReportFixture(t, s, f, "## 复现步骤\n\n使用已捕获的请求包验证。")
	path := fmt.Sprintf("/api/exploration/findings/export?scope=selected&ids=%d&format=md-single", f.FindingID)
	w := request("GET", path, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), secret) {
		t.Fatalf("export must preserve captured cookie: status=%d body=%s", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("confidential export may be cached")
	}
	w = request("GET", fmt.Sprintf("/api/exploration/findings/%d/traffic/%d", f.FindingID, binding.Bindings[0].ID), "")
	if w.Code != 200 {
		t.Fatalf("evidence detail: %d %s", w.Code, w.Body.String())
	}
	var count int
	err = s.m.pg.QueryRow(`SELECT count(*) FROM confidential_access_events
		WHERE finding_id=$1 AND action='view' AND resource='finding_evidence'
		AND detail=$2`, f.FindingID, fmt.Sprintf("binding:%d:detail", binding.Bindings[0].ID)).Scan(&count)
	if err != nil || count != 1 {
		t.Fatalf("evidence audit: count=%d err=%v", count, err)
	}
	err = s.m.pg.QueryRow(`SELECT count(*) FROM confidential_access_events
		WHERE actor_name='yunkun' AND action='export' AND resource='finding_report'
		AND detail='md-single:selected:1' AND $1=ANY(finding_ids)`, f.FindingID).Scan(&count)
	if err != nil || count < 1 {
		t.Fatalf("export audit: count=%d err=%v", count, err)
	}
	var leaked int
	err = s.m.pg.QueryRow(`SELECT count(*) FROM confidential_access_events WHERE detail LIKE $1`, "%"+secret+"%").Scan(&leaked)
	if err != nil || leaked != 0 {
		t.Fatalf("secret leaked to audit: count=%d err=%v", leaked, err)
	}
}
