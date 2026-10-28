package server

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestReportExportStatusAndLoginLimitIsolation(t *testing.T) {
	s, finding, request := trafficEvidenceServer(t)
	row, err := s.m.pg.GetFinding(finding.FindingID)
	if err != nil || row.TaskID == nil {
		t.Fatal("missing fixture task", err)
	}
	paths := []string{
		fmt.Sprintf("/api/report?task=%d", *row.TaskID),
		fmt.Sprintf("/api/exploration/findings/export?scope=selected&ids=%d&format=md-single", finding.FindingID),
	}
	for _, path := range paths {
		response := request("GET", path, "")
		if response.Code != http.StatusOK || strings.Contains(response.Body.String(), `"blockers"`) {
			t.Fatalf("original report export must not require review: %s status=%d body=%s", path, response.Code, response.Body)
		}
	}
	for i := 0; i <= loginUserLimit; i++ {
		response := request("POST", "/api/auth/login", `{"username":"rate-limit-fixture","password":"not-a-secret"}`)
		if i == loginUserLimit && response.Code != http.StatusTooManyRequests {
			t.Fatal("fixture did not reach login limit", response.Code)
		}
	}
	approveFormalReportFixture(t, s, finding, "Reviewed local report")
	for i := 0; i < loginUserLimit+2; i++ {
		for _, path := range paths {
			response := request("GET", path, "")
			if response.Code != http.StatusOK {
				t.Fatalf("authenticated report read was limited: iteration=%d status=%d body=%s", i, response.Code, response.Body)
			}
		}
	}
}
