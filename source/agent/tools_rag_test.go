package agent

import (
	"testing"

	"yunkun/console/db"
)

func TestRetrievalConversationID(t *testing.T) {
	tests := []struct {
		session string
		want    int64
	}{
		{"conv-42", 42},
		{"conv-0", 0},
		{"conv--1", 0},
		{"conv-nope", 0},
		{"task-42", 0},
		{"conv-42-extra", 0},
	}
	for _, tc := range tests {
		got := retrievalConversationID(tc.session)
		if tc.want == 0 && got != nil {
			t.Errorf("%q: got %d, want nil", tc.session, *got)
		}
		if tc.want != 0 && (got == nil || *got != tc.want) {
			t.Errorf("%q: got %v, want %d", tc.session, got, tc.want)
		}
	}
}

func TestRAGCitationsExcludeContent(t *testing.T) {
	citations := ragCitations([]*db.RAGSearchResult{{
		ChunkID: 7, DocumentID: 3, DocumentName: "note.md",
		Content: "private text", SourceURI: "upload/note.md",
		HeadingPath: []string{"Checks"}, LineFrom: 2, LineTo: 4,
	}})
	if len(citations) != 1 || citations[0]["chunk_id"] != int64(7) {
		t.Fatalf("unexpected citations: %#v", citations)
	}
	if _, ok := citations[0]["content"]; ok {
		t.Fatal("citation metadata must not duplicate chunk content")
	}
}
