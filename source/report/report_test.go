package report

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"yunkun/console/db"
)

func TestMarkdownCitesPersistedFindingEvidence(t *testing.T) {
	node := &db.Node{ID: 12, Payload: json.RawMessage(`{"name":"Example","severity":"high","summary":"Verified"}`)}
	text := Markdown(Input{
		Title: "Local task", GeneratedAt: time.Unix(0, 0),
		Findings: []*db.Node{node},
		References: map[int64]db.TaskReportReference{12: {
			FindingID: 34, Report: "Detailed reproduction", EvidenceVersion: 2,
			ReportEvidenceVersion: 1,
			Traffic: []db.TaskReportTraffic{{
				BindingID: 56, Role: "proof", SnapshotID: "snapshot-78", SourceTrafficID: "exchange-90",
			}},
		}},
	})
	for _, expected := range []string{"Finding 记录：** #34", "节点 #12", "详细报告引用的证据版本已变化",
		"Detailed reproduction", "绑定 #56", "`snapshot-78`", "`exchange-90`"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("report missing %q: %s", expected, text)
		}
	}
}
