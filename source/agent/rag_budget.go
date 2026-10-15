package agent

import (
	"encoding/json"
	"math"
	"unicode/utf8"

	"yunkun/console/db"
)

const (
	ragExcerptBytes  = 1800
	ragResponseBytes = 32 * 1024
)

func utf8Prefix(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	for limit > 0 && !utf8.RuneStart(text[limit]) {
		limit--
	}
	return text[:limit]
}

func ragSearchPayload(baseID, logID int64, mode string, latency int64, results []*db.RAGSearchResult, found int) map[string]any {
	return map[string]any{
		"knowledge_base_id": baseID, "retrieval_log_id": logID,
		"mode": mode, "latency_ms": latency, "results": results, "citations": ragCitations(results),
		"matched_count": found, "returned_count": len(results), "omitted_count": found - len(results),
		"excerpt_max_bytes": ragExcerptBytes, "response_max_bytes": ragResponseBytes,
		"content_scope": "bounded_excerpts", "prompt_inclusion": "not_proven_by_retrieval",
	}
}

// Bound the entire JSON, including duplicated citation metadata. Logs receive
// these same excerpts, so a retrieval log describes what the tool returned.
func boundedRAGResults(baseID int64, mode string, latency int64, results []*db.RAGSearchResult) ([]*db.RAGSearchResult, error) {
	out := make([]*db.RAGSearchResult, 0, len(results))
	seen := make(map[int64]bool)
	for _, result := range results {
		if result == nil || seen[result.ChunkID] {
			continue
		}
		seen[result.ChunkID] = true
		copy := *result
		copy.OriginalContentBytes = len(result.Content)
		copy.Content = utf8Prefix(result.Content, ragExcerptBytes)
		copy.ContentTruncated = len(copy.Content) < len(result.Content)
		candidate := append(out, &copy)
		encoded, err := json.Marshal(ragSearchPayload(baseID, math.MaxInt64, mode, latency, candidate, len(results)))
		if err != nil {
			return nil, err
		}
		if len(encoded) <= ragResponseBytes {
			out = candidate
		}
	}
	return out, nil
}
