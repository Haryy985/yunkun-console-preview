package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"yunkun/console/rag"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrRAGKnowledgeBaseNotFound = errors.New("rag knowledge base not found")
	ErrRAGKnowledgeBaseNameUsed = errors.New("rag knowledge base name already exists")
	ErrRAGKnowledgeBaseInvalid  = errors.New("invalid rag knowledge base")
	ErrRAGDocumentNotFound      = errors.New("rag document not found")
	ErrRAGDocumentDuplicate     = errors.New("rag document already exists")
)

// RAGKnowledgeBase is the persisted, user-visible knowledge base summary.
// Document and chunk counts are derived from live rows so they remain accurate
// after soft deletion and reprocessing.
type RAGKnowledgeBase struct {
	ID                int64     `json:"id"`
	TenantID          string    `json:"tenant_id"`
	OwnerID           string    `json:"owner_id"`
	Visibility        string    `json:"visibility"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	Enabled           bool      `json:"enabled"`
	EmbeddingProvider string    `json:"embedding_provider"`
	EmbeddingModel    string    `json:"embedding_model"`
	ChunkSize         int       `json:"chunk_size"`
	ChunkOverlap      int       `json:"chunk_overlap"`
	DocumentCount     int       `json:"document_count"`
	ChunkCount        int       `json:"chunk_count"`
	VectorizedChunks  int       `json:"vectorized_chunks"`
	FailedDocuments   int       `json:"failed_documents"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type RAGDocument struct {
	ID               int64      `json:"id"`
	KnowledgeBaseID  int64      `json:"knowledge_base_id"`
	Name             string     `json:"name"`
	SourceType       string     `json:"source_type"`
	SourcePath       string     `json:"source_path"`
	MIMEType         string     `json:"mime_type"`
	ContentHash      string     `json:"content_hash"`
	Version          int        `json:"version"`
	Status           string     `json:"status"`
	Error            string     `json:"error"`
	ChunkCount       int        `json:"chunk_count"`
	VectorizedChunks int        `json:"vectorized_chunks"`
	DeletedAt        *time.Time `json:"deleted_at,omitempty"`
	DeletedBy        string     `json:"deleted_by,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	Content          string     `json:"-"`
}

type RAGChunk struct {
	ID          int64    `json:"id"`
	DocumentID  int64    `json:"document_id"`
	Ordinal     int      `json:"ordinal"`
	Heading     string   `json:"heading"`
	HeadingPath []string `json:"heading_path"`
	Content     string   `json:"content"`
	SourceURI   string   `json:"source_uri"`
	PageFrom    int      `json:"page_from"`
	PageTo      int      `json:"page_to"`
	LineFrom    int      `json:"line_from"`
	LineTo      int      `json:"line_to"`
	ContentHash string   `json:"content_hash"`
}

type RAGChunkInput struct {
	Ordinal     int
	Heading     string
	HeadingPath []string
	Content     string
	ContentHash string
	LineFrom    int
	LineTo      int
	SourceURI   string
}

type RAGSearchResult struct {
	ContentTruncated     bool     `json:"content_truncated,omitempty"`
	OriginalContentBytes int      `json:"original_content_bytes,omitempty"`
	ChunkID              int64    `json:"chunk_id"`
	DocumentID           int64    `json:"document_id"`
	DocumentName         string   `json:"document_name"`
	Score                float64  `json:"score"`
	Content              string   `json:"content"`
	Heading              string   `json:"heading"`
	HeadingPath          []string `json:"heading_path"`
	SourceURI            string   `json:"source_uri"`
	PageFrom             int      `json:"page_from"`
	PageTo               int      `json:"page_to"`
	LineFrom             int      `json:"line_from"`
	LineTo               int      `json:"line_to"`
}

type RAGRetrievalLog struct {
	ID              int64              `json:"id"`
	KnowledgeBaseID int64              `json:"knowledge_base_id"`
	TaskID          string             `json:"task_id"`
	AgentID         string             `json:"agent_id"`
	ConversationID  *int64             `json:"conversation_id,omitempty"`
	Query           string             `json:"query"`
	TopK            int                `json:"top_k"`
	Mode            string             `json:"mode"`
	Results         []*RAGSearchResult `json:"results"`
	ResultCount     int                `json:"result_count"`
	LatencyMS       int64              `json:"latency_ms"`
	CreatedAt       time.Time          `json:"created_at"`
}

// TaskRAGStats counts searches with nonempty results, not answer correctness.
type TaskRAGStats struct {
	Searches       int `json:"searches"`
	Nonempty       int `json:"nonempty"`
	ReturnedChunks int `json:"returned_chunks"`
}

func (d *DB) TaskRAGStatsAll() (map[string]TaskRAGStats, error) {
	rows, err := d.Query(`
SELECT task_id, COUNT(*),
       COUNT(*) FILTER (WHERE CASE WHEN jsonb_typeof(results)='array' THEN jsonb_array_length(results)>0 ELSE false END),
       COALESCE(SUM(CASE WHEN jsonb_typeof(results)='array' THEN jsonb_array_length(results) ELSE 0 END),0)
FROM rag_retrieval_logs WHERE task_id <> '' GROUP BY task_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]TaskRAGStats)
	for rows.Next() {
		var taskID string
		var stats TaskRAGStats
		if err := rows.Scan(&taskID, &stats.Searches, &stats.Nonempty, &stats.ReturnedChunks); err != nil {
			return nil, err
		}
		out[taskID] = stats
	}
	return out, rows.Err()
}

func (d *DB) TaskRAGStatsFor(taskID string) (TaskRAGStats, error) {
	var stats TaskRAGStats
	err := d.QueryRow(`
SELECT COUNT(*),
       COUNT(*) FILTER (WHERE CASE WHEN jsonb_typeof(results)='array' THEN jsonb_array_length(results)>0 ELSE false END),
       COALESCE(SUM(CASE WHEN jsonb_typeof(results)='array' THEN jsonb_array_length(results) ELSE 0 END),0)
FROM rag_retrieval_logs WHERE task_id=$1`, taskID).
		Scan(&stats.Searches, &stats.Nonempty, &stats.ReturnedChunks)
	return stats, err
}

type RAGChunkForEmbedding struct {
	ChunkID    int64
	DocumentID int64
	Content    string
}

type RAGEmbeddingInput struct {
	ChunkID    int64
	Provider   string
	Model      string
	Dimensions int
	Normalized bool
	Values     []float32
}

type RAGIngestionJob struct {
	ID              int64      `json:"id"`
	KnowledgeBaseID int64      `json:"knowledge_base_id"`
	DocumentID      *int64     `json:"document_id,omitempty"`
	Kind            string     `json:"kind"`
	Status          string     `json:"status"`
	Attempts        int        `json:"attempts"`
	Error           string     `json:"error"`
	ScheduledAt     time.Time  `json:"scheduled_at"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func normalizeRAGKnowledgeBaseName(name string) (string, error) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return "", fmt.Errorf("%w: name is required", ErrRAGKnowledgeBaseInvalid)
	}
	if len([]rune(name)) > 120 {
		return "", fmt.Errorf("%w: name is too long", ErrRAGKnowledgeBaseInvalid)
	}
	return name, nil
}

func normalizeRAGVisibility(value string) (string, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "private", nil
	}
	switch value {
	case "private", "team", "shared", "public":
		return value, nil
	default:
		return "", fmt.Errorf("%w: unsupported visibility", ErrRAGKnowledgeBaseInvalid)
	}
}

const ragKnowledgeBaseSelect = `
SELECT kb.id, kb.tenant_id, kb.owner_id, kb.visibility,
       kb.name, kb.description, kb.enabled,
       kb.embedding_provider, kb.embedding_model,
       kb.chunk_size, kb.chunk_overlap,
       (SELECT count(*) FROM rag_documents d
          WHERE d.knowledge_base_id=kb.id AND d.deleted_at IS NULL),
       (SELECT count(*) FROM rag_chunks c
          JOIN rag_documents d ON d.id=c.document_id
         WHERE d.knowledge_base_id=kb.id AND d.deleted_at IS NULL),
       (SELECT count(*) FROM rag_embeddings e
          JOIN rag_chunks c ON c.id=e.chunk_id
          JOIN rag_documents d ON d.id=c.document_id
         WHERE d.knowledge_base_id=kb.id AND d.deleted_at IS NULL
           AND e.status='ready'
           AND e.model=CASE WHEN kb.embedding_model='' THEN 'local-hash-v1' ELSE kb.embedding_model END),
       (SELECT count(*) FROM rag_documents d
          WHERE d.knowledge_base_id=kb.id AND d.deleted_at IS NULL
            AND d.status='failed'),
       kb.created_at, kb.updated_at
FROM rag_knowledge_bases kb
`

func scanRAGKnowledgeBase(row interface{ Scan(...any) error }) (*RAGKnowledgeBase, error) {
	var item RAGKnowledgeBase
	if err := row.Scan(
		&item.ID, &item.TenantID, &item.OwnerID, &item.Visibility,
		&item.Name, &item.Description, &item.Enabled,
		&item.EmbeddingProvider, &item.EmbeddingModel,
		&item.ChunkSize, &item.ChunkOverlap,
		&item.DocumentCount, &item.ChunkCount, &item.VectorizedChunks,
		&item.FailedDocuments, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &item, nil
}

func (d *DB) ListRAGKnowledgeBases() ([]*RAGKnowledgeBase, error) {
	rows, err := d.Query(ragKnowledgeBaseSelect + `
WHERE kb.deleted_at IS NULL
ORDER BY kb.updated_at DESC, kb.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*RAGKnowledgeBase, 0)
	for rows.Next() {
		item, err := scanRAGKnowledgeBase(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (d *DB) GetRAGKnowledgeBase(id int64) (*RAGKnowledgeBase, error) {
	item, err := scanRAGKnowledgeBase(d.QueryRow(ragKnowledgeBaseSelect+`
WHERE kb.id=$1 AND kb.deleted_at IS NULL`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (d *DB) CreateRAGKnowledgeBase(tenantID, ownerID, visibility, name, description, provider, model string, chunkSize, chunkOverlap int) (*RAGKnowledgeBase, error) {
	name, err := normalizeRAGKnowledgeBaseName(name)
	if err != nil {
		return nil, err
	}
	visibility, err = normalizeRAGVisibility(visibility)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(tenantID) == "" {
		tenantID = "default"
	}
	if chunkSize <= 0 {
		chunkSize = 900
	}
	if chunkOverlap < 0 || chunkOverlap >= chunkSize {
		return nil, fmt.Errorf("%w: chunk overlap must be smaller than chunk size", ErrRAGKnowledgeBaseInvalid)
	}
	var id int64
	err = d.QueryRow(`
INSERT INTO rag_knowledge_bases(tenant_id, owner_id, visibility, name, description, embedding_provider, embedding_model, chunk_size, chunk_overlap)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT DO NOTHING
RETURNING id`, tenantID, ownerID, visibility, name, description, provider, model, chunkSize, chunkOverlap).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRAGKnowledgeBaseNameUsed
	}
	if err != nil {
		return nil, err
	}
	return d.GetRAGKnowledgeBase(id)
}

func (d *DB) UpdateRAGKnowledgeBase(id int64, name, description, provider, model, visibility *string, enabled *bool, chunkSize, chunkOverlap *int) (*RAGKnowledgeBase, error) {
	if name != nil {
		normalized, err := normalizeRAGKnowledgeBaseName(*name)
		if err != nil {
			return nil, err
		}
		*name = normalized
	}
	if visibility != nil {
		normalized, err := normalizeRAGVisibility(*visibility)
		if err != nil {
			return nil, err
		}
		*visibility = normalized
	}
	if chunkSize != nil && *chunkSize <= 0 {
		return nil, fmt.Errorf("%w: chunk size must be positive", ErrRAGKnowledgeBaseInvalid)
	}
	if chunkOverlap != nil && *chunkOverlap < 0 {
		return nil, fmt.Errorf("%w: chunk overlap must be non-negative", ErrRAGKnowledgeBaseInvalid)
	}
	result, err := d.Exec(`
UPDATE rag_knowledge_bases
SET name=COALESCE($2,name),
    description=COALESCE($3,description),
    embedding_provider=COALESCE($4,embedding_provider),
    embedding_model=COALESCE($5,embedding_model),
    visibility=COALESCE($6,visibility),
    enabled=COALESCE($7,enabled),
    chunk_size=COALESCE($8,chunk_size),
    chunk_overlap=COALESCE($9,chunk_overlap)
WHERE id=$1 AND deleted_at IS NULL
  AND COALESCE($9,chunk_overlap) < COALESCE($8,chunk_size)`,
		id, name, description, provider, model, visibility, enabled, chunkSize, chunkOverlap)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_rag_kb_live_name" {
			return nil, ErrRAGKnowledgeBaseNameUsed
		}
		return nil, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	item, err := d.GetRAGKnowledgeBase(id)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrRAGKnowledgeBaseNotFound
	}
	if count == 0 {
		return nil, fmt.Errorf("%w: chunk overlap must be smaller than chunk size", ErrRAGKnowledgeBaseInvalid)
	}
	return item, nil
}

func (d *DB) DeleteRAGKnowledgeBase(id int64) error {
	result, err := d.Exec(`
UPDATE rag_knowledge_bases
SET deleted_at=COALESCE(deleted_at, now()), enabled=false
WHERE id=$1 AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrRAGKnowledgeBaseNotFound
	}
	return nil
}

const ragDocumentSelect = `
SELECT d.id, d.knowledge_base_id, d.name, d.source_type, d.source_path,
       d.mime_type, d.content_hash, d.version, d.status, d.error,
       (SELECT count(*) FROM rag_chunks c WHERE c.document_id=d.id),
       (SELECT count(*) FROM rag_embeddings e
          JOIN rag_chunks c ON c.id=e.chunk_id
          JOIN rag_knowledge_bases kb ON kb.id=d.knowledge_base_id
         WHERE c.document_id=d.id AND e.status='ready'
           AND e.model=CASE WHEN kb.embedding_model='' THEN 'local-hash-v1' ELSE kb.embedding_model END),
       d.deleted_at, d.deleted_by, d.created_at, d.updated_at, d.content
FROM rag_documents d
`

func scanRAGDocument(row interface{ Scan(...any) error }) (*RAGDocument, error) {
	var item RAGDocument
	if err := row.Scan(
		&item.ID, &item.KnowledgeBaseID, &item.Name, &item.SourceType, &item.SourcePath,
		&item.MIMEType, &item.ContentHash, &item.Version, &item.Status, &item.Error,
		&item.ChunkCount, &item.VectorizedChunks, &item.DeletedAt, &item.DeletedBy,
		&item.CreatedAt, &item.UpdatedAt, &item.Content,
	); err != nil {
		return nil, err
	}
	return &item, nil
}

func (d *DB) ListRAGDocuments(knowledgeBaseID int64) ([]*RAGDocument, error) {
	return d.listRAGDocuments(knowledgeBaseID, false)
}

func (d *DB) ListRAGDocumentsIncludingDeleted(knowledgeBaseID int64) ([]*RAGDocument, error) {
	return d.listRAGDocuments(knowledgeBaseID, true)
}

func (d *DB) listRAGDocuments(knowledgeBaseID int64, includeDeleted bool) ([]*RAGDocument, error) {
	filter := " AND d.deleted_at IS NULL"
	if includeDeleted {
		filter = ""
	}
	rows, err := d.Query(ragDocumentSelect+`
WHERE d.knowledge_base_id=$1`+filter+`
ORDER BY d.updated_at DESC, d.id DESC`, knowledgeBaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*RAGDocument, 0)
	for rows.Next() {
		item, err := scanRAGDocument(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (d *DB) GetRAGDocument(id int64) (*RAGDocument, error) {
	item, err := scanRAGDocument(d.QueryRow(ragDocumentSelect+`
WHERE d.id=$1 AND d.deleted_at IS NULL`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (d *DB) CreateRAGDocument(knowledgeBaseID int64, name, sourceType, sourcePath, mimeType, contentHash, sha256, content string) (*RAGDocument, int64, error) {
	content = utf8Clean(content)
	tx, err := d.Begin()
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRow(`
INSERT INTO rag_documents(knowledge_base_id, name, source_type, source_path, mime_type, sha256, content_hash, content)
SELECT $1,$2,$3,$4,$5,$6,$7,$8
WHERE EXISTS (
  SELECT 1 FROM rag_knowledge_bases
   WHERE id=$1 AND deleted_at IS NULL
)
ON CONFLICT DO NOTHING
RETURNING id`, knowledgeBaseID, name, sourceType, sourcePath, mimeType, sha256, contentHash, content).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		var exists bool
		if scanErr := d.QueryRow(`SELECT EXISTS(SELECT 1 FROM rag_knowledge_bases WHERE id=$1 AND deleted_at IS NULL)`, knowledgeBaseID).Scan(&exists); scanErr != nil {
			return nil, 0, scanErr
		}
		if !exists {
			return nil, 0, ErrRAGKnowledgeBaseNotFound
		}
		return nil, 0, ErrRAGDocumentDuplicate
	}
	if err != nil {
		return nil, 0, err
	}
	var jobID int64
	if err := tx.QueryRow(`
INSERT INTO rag_ingestion_jobs(knowledge_base_id, document_id, kind)
VALUES ($1,$2,'document_ingest')
RETURNING id`, knowledgeBaseID, id).Scan(&jobID); err != nil {
		return nil, 0, err
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, err
	}
	item, err := d.GetRAGDocument(id)
	return item, jobID, err
}

func (d *DB) UpdateRAGDocumentStatus(id int64, status, message string) error {
	result, err := d.Exec(`
UPDATE rag_documents
SET status=$2, error=$3
WHERE id=$1 AND deleted_at IS NULL`, id, status, message)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrRAGDocumentNotFound
	}
	return nil
}

func (d *DB) ListRAGDocumentChunks(documentID int64) ([]*RAGChunk, error) {
	rows, err := d.Query(`
SELECT id, document_id, ordinal, heading, to_json(heading_path), content, source_uri,
       page_from, page_to, line_from, line_to, content_hash
FROM rag_chunks
WHERE document_id=$1
ORDER BY ordinal, id`, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*RAGChunk, 0)
	for rows.Next() {
		var item RAGChunk
		var headingPath []byte
		if err := rows.Scan(
			&item.ID, &item.DocumentID, &item.Ordinal, &item.Heading, &headingPath,
			&item.Content, &item.SourceURI, &item.PageFrom, &item.PageTo,
			&item.LineFrom, &item.LineTo, &item.ContentHash,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(headingPath, &item.HeadingPath); err != nil {
			return nil, err
		}
		items = append(items, &item)
	}
	return items, rows.Err()
}

func (d *DB) QueueRAGDocumentJob(documentID int64, kind string) (jobID int64, created bool, err error) {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		kind = "document_reparse"
	}
	tx, err := d.Begin()
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	var knowledgeBaseID int64
	if err := tx.QueryRow(`
SELECT knowledge_base_id
FROM rag_documents
WHERE id=$1 AND deleted_at IS NULL
FOR UPDATE`, documentID).Scan(&knowledgeBaseID); errors.Is(err, sql.ErrNoRows) {
		return 0, false, ErrRAGDocumentNotFound
	} else if err != nil {
		return 0, false, err
	}
	var activeID int64
	err = tx.QueryRow(`
SELECT id
FROM rag_ingestion_jobs
WHERE document_id=$1 AND status IN ('queued','running')
ORDER BY id DESC
LIMIT 1`, documentID).Scan(&activeID)
	if err == nil {
		return activeID, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	if _, err := tx.Exec(`
UPDATE rag_documents
SET status=CASE WHEN $2='embedding_rebuild' THEN 'embedding' ELSE 'uploaded' END,
    error=''
WHERE id=$1 AND deleted_at IS NULL`, documentID, kind); err != nil {
		return 0, false, err
	}
	if err := tx.QueryRow(`
INSERT INTO rag_ingestion_jobs(knowledge_base_id, document_id, kind, status, scheduled_at)
VALUES ($1,$2,$3,'queued',now())
RETURNING id`, knowledgeBaseID, documentID, kind).Scan(&jobID); err != nil {
		return 0, false, err
	}
	if err := tx.Commit(); err != nil {
		return 0, false, err
	}
	return jobID, true, nil
}

func (d *DB) ReplaceRAGDocumentChunks(documentID int64, chunks []RAGChunkInput) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM rag_chunks WHERE document_id=$1`, documentID); err != nil {
		return err
	}
	for _, chunk := range chunks {
		headingPath := make([]string, len(chunk.HeadingPath))
		for index, heading := range chunk.HeadingPath {
			headingPath[index] = utf8Clean(heading)
		}
		if _, err := tx.Exec(`
INSERT INTO rag_chunks(document_id, ordinal, heading, heading_path, content, source_uri, line_from, line_to, content_hash)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			documentID, chunk.Ordinal, utf8Clean(chunk.Heading), headingPath, utf8Clean(chunk.Content), utf8Clean(chunk.SourceURI), chunk.LineFrom, chunk.LineTo, chunk.ContentHash); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`
UPDATE rag_documents
SET status='ready', error=''
WHERE id=$1 AND deleted_at IS NULL`, documentID); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) ListRAGChunksForEmbedding(documentID int64) ([]RAGChunkForEmbedding, error) {
	rows, err := d.Query(`
SELECT id, document_id, content
FROM rag_chunks
WHERE document_id=$1
ORDER BY ordinal, id`, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]RAGChunkForEmbedding, 0)
	for rows.Next() {
		var item RAGChunkForEmbedding
		if err := rows.Scan(&item.ChunkID, &item.DocumentID, &item.Content); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (d *DB) UpsertRAGEmbeddings(items []RAGEmbeddingInput) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, item := range items {
		raw, err := json.Marshal(item.Values)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`
INSERT INTO rag_embeddings(chunk_id, provider, model, dimensions, normalized, vector_values, status, error)
VALUES ($1,$2,$3,$4,$5,$6::jsonb,'ready','')
ON CONFLICT (chunk_id, model) DO UPDATE SET
  provider=EXCLUDED.provider,
  dimensions=EXCLUDED.dimensions,
  normalized=EXCLUDED.normalized,
  vector_values=EXCLUDED.vector_values,
  status='ready',
  error='',
  updated_at=now()`,
			item.ChunkID, item.Provider, item.Model, item.Dimensions, item.Normalized, raw); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) MarkRAGIngestionJob(jobID int64, status, message string) error {
	_, err := d.Exec(`
UPDATE rag_ingestion_jobs
SET status=$2,
    attempts=attempts+CASE WHEN $2='running' THEN 1 ELSE 0 END,
    error=$3,
    started_at=CASE WHEN $2='running' THEN COALESCE(started_at, now()) ELSE started_at END,
    finished_at=CASE WHEN $2 IN ('succeeded','failed') THEN now() ELSE finished_at END
WHERE id=$1`, jobID, status, message)
	return err
}

func (d *DB) ListRAGIngestionJobs(knowledgeBaseID *int64) ([]*RAGIngestionJob, error) {
	query := `
SELECT id, knowledge_base_id, document_id, kind, status, attempts, error,
       scheduled_at, started_at, finished_at, created_at, updated_at
FROM rag_ingestion_jobs`
	args := []any{}
	if knowledgeBaseID != nil {
		query += ` WHERE knowledge_base_id=$1`
		args = append(args, *knowledgeBaseID)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT 200`
	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*RAGIngestionJob, 0)
	for rows.Next() {
		var item RAGIngestionJob
		var documentID sql.NullInt64
		var startedTime, finishedTime sql.NullTime
		if err := rows.Scan(
			&item.ID, &item.KnowledgeBaseID, &documentID, &item.Kind, &item.Status, &item.Attempts, &item.Error,
			&item.ScheduledAt, &startedTime, &finishedTime, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if documentID.Valid {
			item.DocumentID = &documentID.Int64
		}
		if startedTime.Valid {
			item.StartedAt = &startedTime.Time
		}
		if finishedTime.Valid {
			item.FinishedAt = &finishedTime.Time
		}
		items = append(items, &item)
	}
	return items, rows.Err()
}

func (d *DB) DeleteRAGDocument(id int64) error {
	result, err := d.Exec(`
UPDATE rag_documents
SET deleted_at=COALESCE(deleted_at, now()), deleted_by=''
WHERE id=$1 AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrRAGDocumentNotFound
	}
	return nil
}

func (d *DB) RestoreRAGDocument(id int64) error {
	result, err := d.Exec(`
UPDATE rag_documents
SET deleted_at=NULL, deleted_by='', error=''
WHERE id=$1 AND deleted_at IS NOT NULL`, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrRAGDocumentNotFound
	}
	return nil
}

func (d *DB) SearchRAGChunks(knowledgeBaseID int64, query string, topK int) ([]*RAGSearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []*RAGSearchResult{}, nil
	}
	if topK <= 0 {
		topK = 5
	}
	if topK > 50 {
		topK = 50
	}
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return []*RAGSearchResult{}, nil
	}
	conditions := make([]string, 0, len(terms))
	scoreParts := make([]string, 0, len(terms))
	args := []any{knowledgeBaseID}
	for _, term := range terms {
		param := fmt.Sprintf("$%d", len(args)+1)
		conditions = append(conditions, fmt.Sprintf("strpos(lower(c.content), lower(%s)) > 0", param))
		scoreParts = append(scoreParts, fmt.Sprintf("CASE WHEN strpos(lower(c.content), lower(%s)) > 0 THEN 1 ELSE 0 END", param))
		args = append(args, term)
	}
	args = append(args, topK)
	querySQL := fmt.Sprintf(`
SELECT c.id, c.document_id, d.name, (%s)::double precision / %d,
       c.content, c.heading, to_json(c.heading_path), c.source_uri, c.page_from, c.page_to, c.line_from, c.line_to
FROM rag_chunks c
JOIN rag_documents d ON d.id=c.document_id
WHERE d.knowledge_base_id=$1 AND d.deleted_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM rag_change_proposals p
    WHERE p.published_document_id=d.id
      AND NOT EXISTS (SELECT 1 FROM rag_ingestion_jobs j
                      WHERE j.document_id=d.id AND j.status='succeeded')
  )
  AND (%s)
ORDER BY (%s) DESC, c.id DESC
LIMIT $%d`,
		strings.Join(scoreParts, " + "), len(scoreParts),
		strings.Join(conditions, " OR "), strings.Join(scoreParts, " + "), len(args))
	rows, err := d.Query(querySQL, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*RAGSearchResult, 0)
	for rows.Next() {
		var item RAGSearchResult
		var headingPath []byte
		if err := rows.Scan(
			&item.ChunkID, &item.DocumentID, &item.DocumentName, &item.Score,
			&item.Content, &item.Heading, &headingPath, &item.SourceURI, &item.PageFrom, &item.PageTo,
			&item.LineFrom, &item.LineTo,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(headingPath, &item.HeadingPath); err != nil {
			return nil, err
		}
		items = append(items, &item)
	}
	return items, rows.Err()
}

func (d *DB) SearchRAGChunksHybrid(knowledgeBaseID int64, query, model string, queryVector []float32, topK int) ([]*RAGSearchResult, string, error) {
	if topK <= 0 {
		topK = 5
	}
	if topK > 50 {
		topK = 50
	}
	candidateK := topK * 2
	if candidateK > 50 {
		candidateK = 50
	}
	keyword, err := d.SearchRAGChunks(knowledgeBaseID, query, candidateK)
	if err != nil {
		return nil, "", err
	}
	vector, err := d.searchRAGVectorChunks(knowledgeBaseID, model, queryVector, candidateK)
	if err != nil {
		return nil, "", err
	}
	if len(vector) == 0 {
		for i, item := range keyword {
			item.Score = float64(i + 1)
		}
		if len(keyword) > topK {
			keyword = keyword[:topK]
		}
		return keyword, "keyword", nil
	}
	keywordRanked := make([]rag.Ranked[*RAGSearchResult], 0, len(keyword))
	for i, item := range keyword {
		keywordRanked = append(keywordRanked, rag.Ranked[*RAGSearchResult]{ID: item.ChunkID, Value: item, Rank: i + 1, Score: item.Score})
	}
	vectorRanked := make([]rag.Ranked[*RAGSearchResult], 0, len(vector))
	for i, item := range vector {
		vectorRanked = append(vectorRanked, rag.Ranked[*RAGSearchResult]{ID: item.ChunkID, Value: item, Rank: i + 1, Score: item.Score})
	}
	fused := rag.FuseRRF(keywordRanked, vectorRanked)
	out := make([]*RAGSearchResult, 0, minInt(topK, len(fused)))
	for i, item := range fused {
		if i >= topK {
			break
		}
		item.Value.Score = item.Score
		out = append(out, item.Value)
	}
	return out, "hybrid_rrf", nil
}

func (d *DB) searchRAGVectorChunks(knowledgeBaseID int64, model string, queryVector []float32, topK int) ([]*RAGSearchResult, error) {
	if len(queryVector) == 0 {
		return nil, nil
	}
	args := []any{knowledgeBaseID}
	query := `
SELECT c.id, c.document_id, d.name, e.vector_values,
       c.content, c.heading, to_json(c.heading_path), c.source_uri, c.page_from, c.page_to, c.line_from, c.line_to
FROM rag_embeddings e
JOIN rag_chunks c ON c.id=e.chunk_id
JOIN rag_documents d ON d.id=c.document_id
WHERE d.knowledge_base_id=$1 AND d.deleted_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM rag_change_proposals p
    WHERE p.published_document_id=d.id
      AND NOT EXISTS (SELECT 1 FROM rag_ingestion_jobs j
                      WHERE j.document_id=d.id AND j.status='succeeded')
  )
  AND e.status='ready'`
	if strings.TrimSpace(model) != "" {
		args = append(args, model)
		query += ` AND e.model=$2`
	}
	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type candidate struct {
		item  *RAGSearchResult
		score float64
	}
	var candidates []candidate
	for rows.Next() {
		var item RAGSearchResult
		var raw []byte
		var headingPath []byte
		if err := rows.Scan(
			&item.ChunkID, &item.DocumentID, &item.DocumentName, &raw,
			&item.Content, &item.Heading, &headingPath, &item.SourceURI, &item.PageFrom, &item.PageTo,
			&item.LineFrom, &item.LineTo,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(headingPath, &item.HeadingPath); err != nil {
			return nil, err
		}
		var values []float32
		if err := json.Unmarshal(raw, &values); err != nil {
			continue
		}
		score := cosineSimilarity(queryVector, values)
		if score > 0 {
			candidates = append(candidates, candidate{item: &item, score: score})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].item.ChunkID < candidates[j].item.ChunkID
	})
	if topK > len(candidates) {
		topK = len(candidates)
	}
	out := make([]*RAGSearchResult, 0, topK)
	for _, item := range candidates[:topK] {
		item.item.Score = item.score
		out = append(out, item.item)
	}
	return out, nil
}

func cosineSimilarity(left, right []float32) float64 {
	if len(left) == 0 || len(left) != len(right) {
		return 0
	}
	var dot, leftNorm, rightNorm float64
	for i := range left {
		l, r := float64(left[i]), float64(right[i])
		dot += l * r
		leftNorm += l * l
		rightNorm += r * r
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func (d *DB) ProbeRAGVectorExtension() (bool, string, error) {
	var version string
	err := d.QueryRow(`SELECT extversion FROM pg_extension WHERE extname='vector'`).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return true, version, nil
}

func (d *DB) LogRAGRetrieval(knowledgeBaseID int64, taskID, agentID string, conversationID *int64, query string, topK int, mode string, results []*RAGSearchResult, latencyMS int64) (int64, error) {
	tx, err := d.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	logID, err := logRAGRetrievalTx(tx, knowledgeBaseID, taskID, agentID, conversationID, query, topK, mode, results, latencyMS)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return logID, nil
}

// PublishTaskRAGRetrieval serializes publication with grant changes. Search and
// embedding may finish after revocation, but their results are discarded if the
// revocation committed before this final authorization check.
func (d *DB) PublishTaskRAGRetrieval(ctx context.Context, taskID, knowledgeBaseID int64, agentID string, conversationID *int64, query string, topK int, mode string, results []*RAGSearchResult, latencyMS int64) (int64, error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var id int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM tasks WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, taskID).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrTaskRAGAccessDenied
		}
		return 0, err
	}
	var allowed bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(
SELECT 1 FROM task_rag_knowledge_bases g
JOIN rag_knowledge_bases kb ON kb.id=g.knowledge_base_id
WHERE g.task_id=$1 AND g.knowledge_base_id=$2
AND kb.enabled AND kb.deleted_at IS NULL)`, taskID, knowledgeBaseID).Scan(&allowed)
	if err != nil {
		return 0, err
	}
	if !allowed {
		return 0, ErrTaskRAGAccessDenied
	}
	logID, err := logRAGRetrievalTx(tx, knowledgeBaseID, fmt.Sprint(taskID), agentID, conversationID, query, topK, mode, results, latencyMS)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return logID, nil
}

func logRAGRetrievalTx(tx *sql.Tx, knowledgeBaseID int64, taskID, agentID string, conversationID *int64, query string, topK int, mode string, results []*RAGSearchResult, latencyMS int64) (int64, error) {
	if results == nil {
		results = []*RAGSearchResult{}
	}
	raw, err := json.Marshal(results)
	if err != nil {
		return 0, err
	}
	var logID int64
	if err := tx.QueryRow(`
INSERT INTO rag_retrieval_logs(knowledge_base_id, task_id, agent_id, conversation_id, query, top_k, mode, results, latency_ms)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
RETURNING id`,
		knowledgeBaseID, taskID, agentID, conversationID, query, topK, mode, raw, latencyMS).Scan(&logID); err != nil {
		return 0, err
	}
	for index, result := range results {
		if _, err := tx.Exec(`
INSERT INTO rag_retrieval_results(retrieval_log_id, chunk_id, document_id, rank, score)
VALUES ($1,$2,$3,$4,$5)`,
			logID, result.ChunkID, result.DocumentID, index+1, result.Score); err != nil {
			return 0, err
		}
	}
	return logID, nil
}
func (d *DB) ListRAGRetrievalLogs(knowledgeBaseID int64, limit int) ([]*RAGRetrievalLog, error) {
	return d.listRAGRetrievalLogs("knowledge_base_id", knowledgeBaseID, limit)
}

func (d *DB) ListRAGRetrievalLogsByTask(taskID string, limit int) ([]*RAGRetrievalLog, error) {
	if strings.TrimSpace(taskID) == "" {
		return []*RAGRetrievalLog{}, nil
	}
	return d.listRAGRetrievalLogs("task_id", taskID, limit)
}

func (d *DB) listRAGRetrievalLogs(column string, value any, limit int) ([]*RAGRetrievalLog, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := d.Query(fmt.Sprintf(`
SELECT id, COALESCE(knowledge_base_id, 0), task_id, agent_id, conversation_id, query, top_k, mode,
       results, CASE WHEN jsonb_typeof(results)='array' THEN jsonb_array_length(results) ELSE 0 END,
       latency_ms, created_at
FROM rag_retrieval_logs
WHERE %s=$1
ORDER BY created_at DESC, id DESC
LIMIT $2`, column), value, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*RAGRetrievalLog, 0)
	for rows.Next() {
		var item RAGRetrievalLog
		var raw []byte
		if err := rows.Scan(&item.ID, &item.KnowledgeBaseID, &item.TaskID, &item.AgentID, &item.ConversationID, &item.Query, &item.TopK, &item.Mode, &raw, &item.ResultCount, &item.LatencyMS, &item.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &item.Results); err != nil {
			return nil, err
		}
		if item.Results == nil {
			item.Results = []*RAGSearchResult{}
		}
		items = append(items, &item)
	}
	return items, rows.Err()
}
