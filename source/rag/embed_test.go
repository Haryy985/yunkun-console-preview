package rag

import "testing"

func TestHashEmbedderIsDeterministicAndNormalized(t *testing.T) {
	embedder := NewHashEmbedder("", "", 64, true)
	left, err := embedder.Embed("Android request signing")
	if err != nil {
		t.Fatal(err)
	}
	right, err := embedder.Embed("Android request signing")
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 64 || len(right) != 64 {
		t.Fatalf("dimensions = %d/%d, want 64", len(left), len(right))
	}
	var norm float64
	for i := range left {
		if left[i] != right[i] {
			t.Fatalf("embedding is not deterministic at index %d", i)
		}
		norm += float64(left[i]) * float64(left[i])
	}
	if norm < 0.99 || norm > 1.01 {
		t.Fatalf("norm = %f, want approximately 1", norm)
	}
}
