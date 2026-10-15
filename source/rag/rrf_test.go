package rag

import "testing"

func TestFuseRRFUsesRankInsteadOfRawScore(t *testing.T) {
	first := []Ranked[string]{
		{ID: 1, Value: "keyword-1", Score: 0.99},
		{ID: 2, Value: "keyword-2", Score: 0.98},
	}
	second := []Ranked[string]{
		{ID: 2, Value: "vector-2", Score: 0.20},
		{ID: 3, Value: "vector-3", Score: 0.19},
	}
	out := FuseRRF(first, second)
	if len(out) != 3 {
		t.Fatalf("fused length = %d, want 3", len(out))
	}
	if out[0].ID != 2 {
		t.Fatalf("top id = %d, want 2", out[0].ID)
	}
	if out[0].Score <= out[1].Score {
		t.Fatalf("top fused score = %f, next = %f", out[0].Score, out[1].Score)
	}
}
