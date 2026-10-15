package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"yunkun/console/rag"
)

var (
	ErrRAGEvalSetNotFound = errors.New("rag eval set not found")
	ErrRAGEvalSetNameUsed = errors.New("rag eval set name already exists")
)

type RAGEvalSet struct {
	ID              int64     `json:"id"`
	KnowledgeBaseID int64     `json:"knowledge_base_id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	CaseCount       int       `json:"case_count"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type RAGEvalCase struct {
	ID                  int64   `json:"id"`
	EvalSetID           int64   `json:"eval_set_id"`
	Question            string  `json:"question"`
	ExpectedChunkIDs    []int64 `json:"expected_chunk_ids"`
	ExpectedDocumentIDs []int64 `json:"expected_document_ids"`
	Answerable          bool    `json:"answerable"`
}

type RAGEvalRun struct {
	ID              int64      `json:"id"`
	EvalSetID       int64      `json:"eval_set_id"`
	Mode            string     `json:"mode"`
	TopK            int        `json:"top_k"`
	Status          string     `json:"status"`
	RecallAtK       *float64   `json:"recall_at_k,omitempty"`
	MRR             *float64   `json:"mrr,omitempty"`
	CitationHitRate *float64   `json:"citation_hit_rate,omitempty"`
	P95LatencyMS    *int       `json:"p95_latency_ms,omitempty"`
	NoAnswerRate    *float64   `json:"no_answer_rate,omitempty"`
	Error           string     `json:"error"`
	CreatedAt       time.Time  `json:"created_at"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
}

func (d *DB) ListRAGEvalSets(knowledgeBaseID int64) ([]*RAGEvalSet, error) {
	rows, err := d.Query(`
SELECT s.id, s.knowledge_base_id, s.name, s.description,
       (SELECT count(*) FROM rag_eval_cases c WHERE c.eval_set_id=s.id),
       s.created_at, s.updated_at
FROM rag_eval_sets s
WHERE s.knowledge_base_id=$1
ORDER BY s.updated_at DESC, s.id DESC`, knowledgeBaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*RAGEvalSet, 0)
	for rows.Next() {
		var item RAGEvalSet
		if err := rows.Scan(&item.ID, &item.KnowledgeBaseID, &item.Name, &item.Description, &item.CaseCount, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, &item)
	}
	return items, rows.Err()
}

func (d *DB) CreateRAGEvalSet(knowledgeBaseID int64, name, description string) (*RAGEvalSet, error) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return nil, fmt.Errorf("评测集名称不能为空")
	}
	var id int64
	err := d.QueryRow(`
INSERT INTO rag_eval_sets(knowledge_base_id, name, description)
VALUES ($1,$2,$3)
ON CONFLICT DO NOTHING
RETURNING id`, knowledgeBaseID, name, description).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRAGEvalSetNameUsed
	}
	if err != nil {
		return nil, err
	}
	items, err := d.ListRAGEvalSets(knowledgeBaseID)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return nil, ErrRAGEvalSetNotFound
}

func (d *DB) ListRAGEvalCases(evalSetID int64) ([]*RAGEvalCase, error) {
	rows, err := d.Query(`
SELECT id, eval_set_id, question, to_json(expected_chunk_ids), to_json(expected_document_ids), answerable
FROM rag_eval_cases
WHERE eval_set_id=$1
ORDER BY id`, evalSetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*RAGEvalCase, 0)
	for rows.Next() {
		item, err := scanRAGEvalCase(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanRAGEvalCase(row interface{ Scan(...any) error }) (*RAGEvalCase, error) {
	var item RAGEvalCase
	var chunks, documents []byte
	if err := row.Scan(&item.ID, &item.EvalSetID, &item.Question, &chunks, &documents, &item.Answerable); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(chunks, &item.ExpectedChunkIDs); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(documents, &item.ExpectedDocumentIDs); err != nil {
		return nil, err
	}
	if item.ExpectedChunkIDs == nil {
		item.ExpectedChunkIDs = []int64{}
	}
	if item.ExpectedDocumentIDs == nil {
		item.ExpectedDocumentIDs = []int64{}
	}
	return &item, nil
}

func (d *DB) CreateRAGEvalCase(evalSetID int64, question string, expectedChunkIDs, expectedDocumentIDs []int64, answerable bool) (*RAGEvalCase, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return nil, fmt.Errorf("评测问题不能为空")
	}
	if expectedChunkIDs == nil {
		expectedChunkIDs = []int64{}
	}
	if expectedDocumentIDs == nil {
		expectedDocumentIDs = []int64{}
	}
	return scanRAGEvalCase(d.QueryRow(`
INSERT INTO rag_eval_cases(eval_set_id, question, expected_chunk_ids, expected_document_ids, answerable)
VALUES ($1,$2,$3,$4,$5)
RETURNING id, eval_set_id, question, to_json(expected_chunk_ids), to_json(expected_document_ids), answerable`,
		evalSetID, question, expectedChunkIDs, expectedDocumentIDs, answerable))
}

func (d *DB) RunRAGEval(evalSetID int64, topK int) (*RAGEvalRun, error) {
	if topK <= 0 || topK > 50 {
		topK = 5
	}
	var knowledgeBaseID int64
	if err := d.QueryRow(`SELECT knowledge_base_id FROM rag_eval_sets WHERE id=$1`, evalSetID).Scan(&knowledgeBaseID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRAGEvalSetNotFound
		}
		return nil, err
	}
	var provider, model string
	if err := d.QueryRow(`
SELECT embedding_provider, embedding_model
FROM rag_knowledge_bases
WHERE id=$1 AND deleted_at IS NULL`, knowledgeBaseID).Scan(&provider, &model); err != nil {
		return nil, err
	}
	cases, err := d.ListRAGEvalCases(evalSetID)
	if err != nil {
		return nil, err
	}
	var runID int64
	if err := d.QueryRow(`
INSERT INTO rag_eval_runs(eval_set_id, mode, top_k, status)
VALUES ($1,'keyword',$2,'running')
RETURNING id`, evalSetID, topK).Scan(&runID); err != nil {
		return nil, err
	}
	latencies := make([]int, 0, len(cases))
	hits := 0
	reciprocal := 0.0
	citationHits := 0
	noAnswers := 0
	runMode := "keyword"
	for _, item := range cases {
		started := time.Now()
		var queryVector []float32
		searchModel := strings.TrimSpace(model)
		if embedder, embedErr := rag.ResolveEmbedder(provider, model, 256, true); embedErr == nil {
			queryVector, _ = embedder.Embed(item.Question)
			searchModel = embedder.Model()
		}
		results, mode, searchErr := d.SearchRAGChunksHybrid(knowledgeBaseID, item.Question, searchModel, queryVector, topK)
		latency := int(time.Since(started).Milliseconds())
		if searchErr != nil {
			_, _ = d.Exec(`UPDATE rag_eval_runs SET status='failed', error=$2, finished_at=now() WHERE id=$1`, runID, searchErr.Error())
			return nil, searchErr
		}
		if mode == "hybrid_rrf" {
			runMode = mode
		}
		latencies = append(latencies, latency)
		if _, logErr := d.LogRAGRetrieval(knowledgeBaseID, "", "eval", nil, item.Question, topK, mode, results, int64(latency)); logErr != nil {
			_, _ = d.Exec(`UPDATE rag_eval_runs SET status='failed', error=$2, finished_at=now() WHERE id=$1`, runID, logErr.Error())
			return nil, logErr
		}
		expectedChunks := make(map[int64]struct{}, len(item.ExpectedChunkIDs))
		expectedDocs := make(map[int64]struct{}, len(item.ExpectedDocumentIDs))
		for _, id := range item.ExpectedChunkIDs {
			expectedChunks[id] = struct{}{}
		}
		for _, id := range item.ExpectedDocumentIDs {
			expectedDocs[id] = struct{}{}
		}
		hit := false
		rank := 0.0
		for index, result := range results {
			_, chunkOK := expectedChunks[result.ChunkID]
			_, documentOK := expectedDocs[result.DocumentID]
			if chunkOK || documentOK {
				if !hit {
					hit = true
					rank = 1.0 / float64(index+1)
				}
			}
		}
		if hit {
			hits++
			reciprocal += rank
			citationHits++
		} else if !item.Answerable && len(results) == 0 {
			noAnswers++
		}
		retrieved := make([]int64, 0, len(results))
		for _, result := range results {
			retrieved = append(retrieved, result.ChunkID)
		}
		if _, err := d.Exec(`
INSERT INTO rag_eval_results(run_id, case_id, retrieved_chunk_ids, recall_hit, reciprocal_rank, latency_ms)
VALUES ($1,$2,$3,$4,$5,$6)`,
			runID, item.ID, retrieved, hit, rank, latency); err != nil {
			return nil, err
		}
	}
	total := float64(len(cases))
	recall, mrr, citation, noAnswer := 0.0, 0.0, 0.0, 0.0
	if total > 0 {
		recall = float64(hits) / total
		mrr = reciprocal / total
		citation = float64(citationHits) / total
		noAnswer = float64(noAnswers) / total
	}
	p95 := 0
	if len(latencies) > 0 {
		sort.Ints(latencies)
		index := int(float64(len(latencies)-1) * 0.95)
		p95 = latencies[index]
	}
	if _, err := d.Exec(`
UPDATE rag_eval_runs
SET status='succeeded', mode=$2, recall_at_k=$3, mrr=$4, citation_hit_rate=$5,
    p95_latency_ms=$6, no_answer_rate=$7, finished_at=now()
WHERE id=$1`, runID, runMode, recall, mrr, citation, p95, noAnswer); err != nil {
		return nil, err
	}
	return d.GetRAGEvalRun(runID)
}

func (d *DB) GetRAGEvalRun(id int64) (*RAGEvalRun, error) {
	var item RAGEvalRun
	var recall, mrr, citation, noAnswer sql.NullFloat64
	var p95 sql.NullInt64
	var finished sql.NullTime
	err := d.QueryRow(`
SELECT id, eval_set_id, mode, top_k, status, recall_at_k, mrr,
       citation_hit_rate, p95_latency_ms, no_answer_rate, error, created_at, finished_at
FROM rag_eval_runs WHERE id=$1`, id).Scan(
		&item.ID, &item.EvalSetID, &item.Mode, &item.TopK, &item.Status,
		&recall, &mrr, &citation, &p95, &noAnswer, &item.Error, &item.CreatedAt, &finished,
	)
	if err != nil {
		return nil, err
	}
	if recall.Valid {
		item.RecallAtK = &recall.Float64
	}
	if mrr.Valid {
		item.MRR = &mrr.Float64
	}
	if citation.Valid {
		item.CitationHitRate = &citation.Float64
	}
	if p95.Valid {
		value := int(p95.Int64)
		item.P95LatencyMS = &value
	}
	if noAnswer.Valid {
		item.NoAnswerRate = &noAnswer.Float64
	}
	if finished.Valid {
		item.FinishedAt = &finished.Time
	}
	return &item, nil
}

func (d *DB) ListRAGEvalRuns(evalSetID int64) ([]*RAGEvalRun, error) {
	rows, err := d.Query(`
SELECT id FROM rag_eval_runs WHERE eval_set_id=$1 ORDER BY created_at DESC, id DESC LIMIT 50`, evalSetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*RAGEvalRun, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		item, err := d.GetRAGEvalRun(id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
