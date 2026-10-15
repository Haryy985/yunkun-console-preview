package server

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestReportsIncludeCapturedHTTPRequest(t *testing.T) {
	s, finding, request := trafficEvidenceServer(t)
	seedServerEvidenceFlow(t, s, "http-proof", []byte("verified"))
	head := "POST /api/item?id=7 HTTP/1.1\r\nHost: evidence.local\r\nCookie: session=test\r\nContent-Type: application/json"
	body := []byte(`{"item":"proof"}`)
	if _, err := s.m.traffic.DB().Exec(`UPDATE exchange_bodies SET req_head=?,req_body=? WHERE id=?`, head, body, "http-proof"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.m.traffic.DB().Exec(`UPDATE exchanges SET req_len=? WHERE id=?`, len(body), "http-proof"); err != nil {
		t.Fatal(err)
	}
	bind := request("POST", fmt.Sprintf("/api/exploration/findings/%d/traffic", finding.FindingID), `{"traffic_refs":[{"traffic_id":"http-proof","role":"proof"}]}`)
	if bind.Code != 200 {
		t.Fatal(bind.Code, bind.Body)
	}
	row, err := s.m.pg.GetFinding(finding.FindingID)
	if err != nil || row.TaskID == nil {
		t.Fatal(err)
	}
	for _, url := range []string{
		fmt.Sprintf("/api/report?task=%d", *row.TaskID),
		fmt.Sprintf("/api/exploration/findings/export?scope=selected&ids=%d&format=md-single", finding.FindingID),
	} {
		response := request("GET", url, "")
		if response.Code != 200 {
			t.Fatal(response.Code, response.Body)
		}
		for _, want := range []string{"POST /api/item?id=7 HTTP/1.1", "Host: evidence.local", "Cookie: session=test", "Content-Type: application/json\n\n" + string(body)} {
			if !strings.Contains(response.Body.String(), want) {
				t.Fatalf("%s missing %q", url, want)
			}
		}
	}
	response := request("GET", fmt.Sprintf("/api/exploration/findings/export?scope=selected&ids=%d&format=md-zip", finding.FindingID), "")
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body)
	}
	archive, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var markdown, packet []byte
	for _, entry := range archive.File {
		reader, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(entry.Name, ".md") {
			markdown = raw
		} else if strings.HasSuffix(entry.Name, "/request.http") {
			packet = raw
		}
	}
	if !bytes.Contains(markdown, []byte(string(body))) || !bytes.Equal(packet, []byte(head+"\n\n"+string(body))) {
		t.Fatalf("zip lost packet: markdown=%q packet=%q", markdown, packet)
	}
}

func TestTaskReportIncludesPersistedFindingAndTrafficReferences(t *testing.T) {
	s, finding, request := trafficEvidenceServer(t)
	seedServerEvidenceFlow(t, s, "report-proof", []byte("local proof"))
	path := fmt.Sprintf("/api/exploration/findings/%d/traffic", finding.FindingID)
	if response := request("POST", path, `{"traffic_refs":[{"traffic_id":"report-proof","role":"proof"}]}`); response.Code != 200 {
		t.Fatalf("bind traffic: %d %s", response.Code, response.Body.String())
	}
	if _, err := s.m.pg.SetFindingReportByNodeID(finding.NodeID, "Detailed local finding"); err != nil {
		t.Fatal(err)
	}
	row, err := s.m.pg.GetFinding(finding.FindingID)
	if err != nil || row == nil || row.TaskID == nil {
		t.Fatalf("finding: %+v, %v", row, err)
	}
	response := request("GET", fmt.Sprintf("/api/report?task=%d", *row.TaskID), "")
	if response.Code != 200 {
		t.Fatalf("export: %d %s", response.Code, response.Body.String())
	}
	for _, want := range []string{
		fmt.Sprintf("Finding 记录：** #%d", finding.FindingID),
		fmt.Sprintf("节点 #%d", finding.NodeID),
		"Detailed local finding", "report-proof", "流量证据引用",
	} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("report missing %q", want)
		}
	}
}

func TestTaskReportIncludesFindingsBeyondFirstThousand(t *testing.T) {
	s, finding, request := trafficEvidenceServer(t)
	row, err := s.m.pg.GetFinding(finding.FindingID)
	if err != nil || row == nil || row.TaskID == nil {
		t.Fatalf("finding: %+v, %v", row, err)
	}
	task := s.m.ResolveTask(fmt.Sprint(*row.TaskID))
	if task == nil {
		t.Fatal("task missing")
	}
	_, err = s.m.pg.Exec(`INSERT INTO exploration_nodes(exploration_id, kind, state, payload)
SELECT $1, 'finding', 'confirmed',
       jsonb_build_object('name', 'Pagination finding ' || i, 'severity', 'low', 'summary', 'Local fixture')
FROM generate_series(1, 1001) AS i`, task.ExpID)
	if err != nil {
		t.Fatal(err)
	}
	response := request("GET", fmt.Sprintf("/api/report?task=%d", *row.TaskID), "")
	if response.Code != 200 {
		t.Fatalf("export: %d %s", response.Code, response.Body.String())
	}
	for _, want := range []string{
		"确认发现：**1002**",
		"Pagination finding 1",
		"Pagination finding 1001",
		fmt.Sprintf("节点 #%d", finding.NodeID),
	} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("report missing %q", want)
		}
	}
}
