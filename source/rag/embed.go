package rag

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"unicode"
)

// Embedder is the small provider-neutral contract used by ingestion and search.
// Providers can be added later without changing the RAG or agent APIs.
type Embedder interface {
	Provider() string
	Model() string
	Dimensions() int
	Normalized() bool
	Embed(text string) ([]float32, error)
}

// HashEmbedder is a deterministic local fallback. It needs no API key or
// network access, so a new installation remains searchable before an external
// embedding provider is configured.
type HashEmbedder struct {
	provider   string
	model      string
	dimensions int
	normalize  bool
}

func NewHashEmbedder(provider, model string, dimensions int, normalize bool) *HashEmbedder {
	if strings.TrimSpace(provider) == "" {
		provider = "local"
	}
	if strings.TrimSpace(model) == "" {
		model = "local-hash-v1"
	}
	if dimensions < 32 {
		dimensions = 256
	}
	return &HashEmbedder{
		provider:   provider,
		model:      model,
		dimensions: dimensions,
		normalize:  normalize,
	}
}

func (e *HashEmbedder) Provider() string { return e.provider }
func (e *HashEmbedder) Model() string    { return e.model }
func (e *HashEmbedder) Dimensions() int  { return e.dimensions }
func (e *HashEmbedder) Normalized() bool { return e.normalize }
func (e *HashEmbedder) String() string   { return fmt.Sprintf("%s/%s", e.provider, e.model) }

func (e *HashEmbedder) Embed(text string) ([]float32, error) {
	if e == nil || e.dimensions <= 0 {
		return nil, fmt.Errorf("embedding dimensions must be positive")
	}
	tokens := embedTokens(text)
	values := make([]float32, e.dimensions)
	for _, token := range tokens {
		sum := sha256.Sum256([]byte(token))
		index := binary.BigEndian.Uint32(sum[:4]) % uint32(e.dimensions)
		sign := float32(1)
		if sum[4]&1 == 1 {
			sign = -1
		}
		values[index] += sign
	}
	if e.normalize {
		normalizeVector(values)
	}
	return values, nil
}

func embedTokens(text string) []string {
	var tokens []string
	var current []rune
	flush := func() {
		if len(current) == 0 {
			return
		}
		token := strings.ToLower(string(current))
		tokens = append(tokens, token)
		if len([]rune(token)) >= 3 {
			runes := []rune(token)
			for i := 0; i+2 < len(runes); i++ {
				tokens = append(tokens, string(runes[i:i+3]))
			}
		}
		current = current[:0]
	}
	for _, r := range []rune(text) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			current = append(current, r)
			continue
		}
		flush()
	}
	flush()
	return tokens
}

func normalizeVector(values []float32) {
	var norm float64
	for _, value := range values {
		norm += float64(value) * float64(value)
	}
	if norm == 0 {
		return
	}
	scale := float32(1 / math.Sqrt(norm))
	for i := range values {
		values[i] *= scale
	}
}

// ResolveEmbedder returns a built-in provider when the configuration is empty
// or explicitly local. Unknown providers are left to future provider adapters.
func ResolveEmbedder(provider, model string, dimensions int, normalize bool) (Embedder, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" || provider == "local" || provider == "hash" {
		return NewHashEmbedder("local", model, dimensions, normalize), nil
	}
	return nil, fmt.Errorf("embedding provider %q is not configured", provider)
}
