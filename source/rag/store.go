// Package rag contains storage-neutral contracts for knowledge retrieval.
// Database and HTTP code should depend on these shapes instead of assuming a
// particular vector vendor.
package rag

type Backend string

const (
	BackendKeyword  Backend = "keyword"
	BackendPGVector Backend = "pgvector"
	BackendExternal Backend = "external"
)

type IndexSpec struct {
	KnowledgeBaseID int64
	Name            string
	Model           string
	Dimensions      int
	Metric          string
	Parameters      map[string]any
}

type Embedding struct {
	ChunkID    int64
	Provider   string
	Model      string
	Dimensions int
	Normalized bool
	Values     []float32
}

type SearchRequest struct {
	KnowledgeBaseID int64
	Query           string
	Embedding       []float32
	TopK            int
}

type SearchResult struct {
	ChunkID     int64
	DocumentID  int64
	Score       float64
	Content     string
	HeadingPath []string
	SourceURI   string
	PageFrom    int
	PageTo      int
	LineFrom    int
	LineTo      int
}

// VectorStore is deliberately small: indexing, upsert, search, and deletion
// are enough for the ingestion worker and retrieval service. Implementations
// can be backed by pgvector, PostgreSQL full-text search, or an external store.
type VectorStore interface {
	Backend() Backend
	EnsureIndex(IndexSpec) error
	Upsert([]Embedding) error
	Search(SearchRequest) ([]SearchResult, error)
	DeleteChunks([]int64) error
}

func DefaultBackend(pgvectorAvailable bool) Backend {
	if pgvectorAvailable {
		return BackendPGVector
	}
	return BackendKeyword
}
