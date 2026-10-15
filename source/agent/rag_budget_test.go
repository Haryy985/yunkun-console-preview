package agent

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"unicode/utf8"

	"yunkun/console/db"
)

func TestBoundedRAGResultsUTF8AndTotalPayload(t *testing.T) {
	content := strings.Repeat("知识与引用<>&\n", 800)
	var raw []*db.RAGSearchResult
	for i := int64(1); i <= 20; i++ {
		raw = append(raw, &db.RAGSearchResult{
			ChunkID: i, DocumentID: i + 100, Content: content,
			SourceURI: "fixture://original/source", DocumentName: "fixture",
		})
	}
	results, err := boundedRAGResults(1, "hybrid", 42, raw)
	if err != nil || len(results) == 0 || len(results) >= len(raw) {
		t.Fatalf("expected bounded nonempty result: %d, %v", len(results), err)
	}
	body, err := json.Marshal(ragSearchPayload(1, math.MaxInt64, "hybrid", 42, results, len(raw)))
	if err != nil || len(body) > ragResponseBytes {
		t.Fatalf("payload bytes=%d err=%v", len(body), err)
	}
	for i, r := range results {
		if !utf8.ValidString(r.Content) || len(r.Content) > ragExcerptBytes || !strings.HasPrefix(content, r.Content) {
			t.Fatal("invalid UTF8 or excerpt")
		}
		if !r.ContentTruncated || r.OriginalContentBytes != len(content) ||
			r.SourceURI != raw[i].SourceURI || r.ChunkID != raw[i].ChunkID || r.DocumentID != raw[i].DocumentID {
			t.Fatal("lost metadata")
		}
	}
	if raw[0].Content != content || raw[0].ContentTruncated || raw[0].OriginalContentBytes != 0 {
		t.Fatal("mutated search result")
	}
}

func TestBoundedRAGResultsDedupAndOversizedSource(t *testing.T) {
	raw := []*db.RAGSearchResult{
		nil,
		{ChunkID: 1, SourceURI: strings.Repeat("large", ragResponseBytes), Content: "small"},
		{ChunkID: 2, Content: "small", SourceURI: "fixture://source"},
		{ChunkID: 2, Content: "duplicate"},
	}
	out, err := boundedRAGResults(1, "keyword", 0, raw)
	if err != nil || len(out) != 1 || out[0].ChunkID != 2 || out[0].ContentTruncated {
		t.Fatalf("unexpected results: %#v, %v", out, err)
	}
	payload := ragSearchPayload(1, 1, "keyword", 0, out, len(raw))
	if payload["omitted_count"] != 3 || payload["prompt_inclusion"] != "not_proven_by_retrieval" {
		t.Fatal("missing omission or inclusion distinction")
	}
	if out[0].SourceURI != raw[2].SourceURI {
		t.Fatal("source was rewritten")
	}
}
