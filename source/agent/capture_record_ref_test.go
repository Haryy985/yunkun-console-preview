package agent

import (
	"encoding/json"
	"testing"
)

func TestToolResultRecordRef(t *testing.T) {
	tests := []struct {
		name, tool, input, output, wantKind, wantID, wantNode string
		isError                                               bool
	}{
		{"rag", "rag_search", "", `{"retrieval_log_id":42,"results":[{"content":"secret"}]}`, "rag_retrieval", "42", "", false},
		{"proposal", "propose_rag_change", "", `{"proposal_id":24,"knowledge_base_id":7,"status":"pending","content":"secret"}`, "rag_proposal", "24", "", false},
		{"finding", "report_finding", "", "finding recorded: 9\n" +
			`{"finding_id":"17","finding_node_id":"9","evidence":"secret"}`, "finding", "17", "9", false},
		{"traffic read", "traffic_get", `{"id":"123-0001","secret":"hidden"}`, "=== REQUEST ===\nGET / HTTP/1.1\n\n=== RESPONSE ===\nHTTP/1.1 200 OK", "traffic_read", "123-0001", "", false},
		{"traffic error", "traffic_get", `{"id":"123-0001"}`, "=== REQUEST ===\n\n=== RESPONSE ===\n", "", "", "", true},
		{"traffic malformed input", "traffic_get", `not JSON`, "=== REQUEST ===\n\n=== RESPONSE ===\n", "", "", "", false},
		{"traffic missing result", "traffic_get", `{"id":"123-0001"}`, "exchange not found", "", "", "", false},
		{"rag error", "rag_search", "", `{"retrieval_log_id":42}`, "", "", "", true},
		{"malformed", "rag_search", "", `not JSON`, "", "", "", false},
		{"zero ID", "rag_search", "", `{"retrieval_log_id":0}`, "", "", "", false},
		{"mismatch", "report_finding", "", "finding recorded: 8\n" +
			`{"finding_id":"17","finding_node_id":"9"}`, "", "", "", false},
		{"unrelated", "Bash", "", `{"retrieval_log_id":42}`, "", "", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := toolResultRecordRef(tc.tool, tc.input, tc.output, tc.isError)
			if tc.wantKind == "" {
				if len(raw) != 0 {
					t.Fatalf("unexpected reference: %s", raw)
				}
				return
			}
			var got struct {
				RecordRef struct {
					Kind   string `json:"kind"`
					ID     string `json:"id"`
					NodeID string `json:"node_id"`
				} `json:"record_ref"`
			}
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			if got.RecordRef.Kind != tc.wantKind || got.RecordRef.ID != tc.wantID ||
				got.RecordRef.NodeID != tc.wantNode {
				t.Fatalf("reference: %+v", got.RecordRef)
			}
			if tc.name == "rag" || tc.name == "finding" || tc.name == "proposal" || tc.name == "traffic read" {
				if string(raw) == tc.output || len(raw) > 120 {
					t.Fatalf("metadata must contain IDs only: %s", raw)
				}
			}
		})
	}
}

func TestToolRequestMetadata(t *testing.T) {
	ref := toolResultRecordRef("rag_search", "", `{"retrieval_log_id":42}`, false)
	raw := toolRequestMetadata("request-1", ref)
	var got struct {
		RequestID string `json:"llm_request_id"`
		RecordRef struct {
			Kind string `json:"kind"`
			ID   string `json:"id"`
		} `json:"record_ref"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.RequestID != "request-1" || got.RecordRef.Kind != "rag_retrieval" || got.RecordRef.ID != "42" {
		t.Fatalf("metadata lost causal IDs: %s", raw)
	}
	if string(toolRequestMetadata("", ref)) != string(ref) {
		t.Fatal("unknown request must not create a causal edge")
	}
}
