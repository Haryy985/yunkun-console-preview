package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"yunkun/console/db"
	"yunkun/console/rag"
)

type ragKnowledgeBaseRequest struct {
	TenantID          string `json:"tenant_id"`
	OwnerID           string `json:"owner_id"`
	Visibility        string `json:"visibility"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	EmbeddingProvider string `json:"embedding_provider"`
	EmbeddingModel    string `json:"embedding_model"`
	ChunkSize         int    `json:"chunk_size"`
	ChunkOverlap      int    `json:"chunk_overlap"`
	Enabled           *bool  `json:"enabled"`
}

func writeRAGKnowledgeBaseError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, db.ErrRAGKnowledgeBaseInvalid):
		writeErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, db.ErrRAGKnowledgeBaseNameUsed):
		writeErr(w, http.StatusConflict, "知识库名称已存在")
	case errors.Is(err, db.ErrRAGKnowledgeBaseNotFound):
		writeErr(w, http.StatusNotFound, "知识库不存在")
	case errors.Is(err, db.ErrRAGDocumentNotFound):
		writeErr(w, http.StatusNotFound, "文档不存在")
	case errors.Is(err, db.ErrRAGDocumentDuplicate):
		writeErr(w, http.StatusConflict, "相同文档已存在")
	case errors.Is(err, db.ErrRAGProposalInvalid):
		writeErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, db.ErrRAGProposalNotFound):
		writeErr(w, http.StatusNotFound, err.Error())
	case errors.Is(err, db.ErrRAGProposalConflict):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, db.ErrTaskRAGAccessDenied):
		writeErr(w, http.StatusForbidden, "任务未获知识库授权或授权已撤销")
	default:
		writeErr(w, http.StatusInternalServerError, err.Error())
	}
}

func (s *Server) listRAGProposals(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad knowledge base id")
		return
	}
	items, err := pg.ListRAGProposals(id, r.URL.Query().Get("status"))
	if err != nil {
		writeRAGKnowledgeBaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"proposals": items})
}

func (s *Server) reviewRAGProposal(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad proposal id")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var req struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Decision != "approve" && req.Decision != "reject" {
		writeErr(w, http.StatusBadRequest, "decision must be approve or reject")
		return
	}
	// All current JWTs belong to the single administrator; multi-user RBAC is not implemented.
	item, jobID, err := pg.ReviewRAGProposal(id, req.Decision == "approve", "admin", req.Reason)
	if err != nil {
		writeRAGKnowledgeBaseError(w, err)
		return
	}
	if jobID > 0 && item.PublishedDocumentID != nil {
		go s.ingestRAGDocument(*item.PublishedDocumentID, jobID)
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) listRAGKnowledgeBases(w http.ResponseWriter, _ *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	items, err := pg.ListRAGKnowledgeBases()
	if err != nil {
		writeRAGKnowledgeBaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"knowledge_bases": items})
}

func (s *Server) createRAGKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	var req ragKnowledgeBaseRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	item, err := pg.CreateRAGKnowledgeBase(
		req.TenantID, req.OwnerID, req.Visibility, req.Name, req.Description, req.EmbeddingProvider, req.EmbeddingModel,
		req.ChunkSize, req.ChunkOverlap,
	)
	if err != nil {
		writeRAGKnowledgeBaseError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) updateRAGKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad knowledge base id")
		return
	}
	var req struct {
		Name              *string `json:"name"`
		Description       *string `json:"description"`
		EmbeddingProvider *string `json:"embedding_provider"`
		EmbeddingModel    *string `json:"embedding_model"`
		Visibility        *string `json:"visibility"`
		ChunkSize         *int    `json:"chunk_size"`
		ChunkOverlap      *int    `json:"chunk_overlap"`
		Enabled           *bool   `json:"enabled"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	item, err := pg.UpdateRAGKnowledgeBase(
		id, req.Name, req.Description, req.EmbeddingProvider, req.EmbeddingModel,
		req.Visibility, req.Enabled, req.ChunkSize, req.ChunkOverlap,
	)
	if err != nil {
		writeRAGKnowledgeBaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) deleteRAGKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad knowledge base id")
		return
	}
	if err := pg.DeleteRAGKnowledgeBase(id); err != nil {
		writeRAGKnowledgeBaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

func (s *Server) listRAGDocuments(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad knowledge base id")
		return
	}
	items, err := pg.ListRAGDocumentsIncludingDeleted(id)
	if strings.ToLower(r.URL.Query().Get("include_deleted")) != "true" {
		items, err = pg.ListRAGDocuments(id)
	}
	if err != nil {
		writeRAGKnowledgeBaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"documents": items})
}

func (s *Server) ingestRAGDocument(documentID, jobID int64) {
	pg := s.m.pg
	if pg == nil {
		return
	}
	fail := func(err error) {
		message := err.Error()
		_ = pg.UpdateRAGDocumentStatus(documentID, "failed", message)
		_ = pg.MarkRAGIngestionJob(jobID, "failed", message)
	}
	if err := pg.MarkRAGIngestionJob(jobID, "running", ""); err != nil {
		fail(err)
		return
	}
	document, err := pg.GetRAGDocument(documentID)
	if err != nil || document == nil {
		if err == nil {
			err = db.ErrRAGDocumentNotFound
		}
		fail(err)
		return
	}
	base, err := pg.GetRAGKnowledgeBase(document.KnowledgeBaseID)
	if err != nil || base == nil {
		if err == nil {
			err = db.ErrRAGKnowledgeBaseNotFound
		}
		fail(err)
		return
	}
	if err := pg.UpdateRAGDocumentStatus(documentID, "parsing", ""); err != nil {
		fail(err)
		return
	}
	blocks, err := rag.ParseDocument(document.Name, document.MIMEType, []byte(document.Content))
	if err != nil {
		fail(err)
		return
	}
	if err := pg.UpdateRAGDocumentStatus(documentID, "parsed", ""); err != nil {
		fail(err)
		return
	}
	if err := pg.UpdateRAGDocumentStatus(documentID, "chunking", ""); err != nil {
		fail(err)
		return
	}
	chunks := rag.ChunkBlocks(blocks, base.ChunkSize, base.ChunkOverlap)
	if len(chunks) == 0 {
		fail(errors.New("文档没有可提取的文本内容"))
		return
	}
	inputs := make([]db.RAGChunkInput, 0, len(chunks))
	for _, chunk := range chunks {
		inputs = append(inputs, db.RAGChunkInput{
			Ordinal: chunk.Ordinal, Heading: chunk.Heading, HeadingPath: chunk.HeadingPath,
			Content: chunk.Content, ContentHash: chunk.ContentHash,
			LineFrom: chunk.LineFrom, LineTo: chunk.LineTo, SourceURI: document.SourcePath,
		})
	}
	if err := pg.ReplaceRAGDocumentChunks(documentID, inputs); err != nil {
		fail(err)
		return
	}
	// Vectorization is best-effort. Keyword retrieval remains available when an
	// external provider is not configured, while the local provider works out of
	// the box and gives the hybrid retriever real vectors to fuse.
	if embedder, embedErr := rag.ResolveEmbedder(base.EmbeddingProvider, base.EmbeddingModel, 256, true); embedErr == nil {
		chunkRows, listErr := pg.ListRAGChunksForEmbedding(documentID)
		if listErr != nil {
			fail(listErr)
			return
		}
		embeddings := make([]db.RAGEmbeddingInput, 0, len(chunkRows))
		for _, row := range chunkRows {
			values, vectorErr := embedder.Embed(row.Content)
			if vectorErr != nil {
				fail(vectorErr)
				return
			}
			embeddings = append(embeddings, db.RAGEmbeddingInput{
				ChunkID: row.ChunkID, Provider: embedder.Provider(), Model: embedder.Model(),
				Dimensions: embedder.Dimensions(), Normalized: embedder.Normalized(), Values: values,
			})
		}
		if vectorErr := pg.UpsertRAGEmbeddings(embeddings); vectorErr != nil {
			fail(vectorErr)
			return
		}
	}
	if err := pg.CompleteRAGProposalIngestion(documentID, jobID); err != nil {
		fail(err)
		return
	}
}

func (s *Server) listRAGIngestionJobs(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	var knowledgeBaseID *int64
	if value := r.URL.Query().Get("knowledge_base_id"); value != "" {
		id, err := strconv.ParseInt(value, 10, 64)
		ok := err == nil
		if !ok {
			writeErr(w, http.StatusBadRequest, "bad knowledge_base_id")
			return
		}
		knowledgeBaseID = &id
	}
	items, err := pg.ListRAGIngestionJobs(knowledgeBaseID)
	if err != nil {
		writeRAGKnowledgeBaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": items})
}

func (s *Server) getRAGDocument(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad document id")
		return
	}
	item, err := pg.GetRAGDocument(id)
	if err != nil {
		writeRAGKnowledgeBaseError(w, err)
		return
	}
	if item == nil {
		writeRAGKnowledgeBaseError(w, db.ErrRAGDocumentNotFound)
		return
	}
	chunks, err := pg.ListRAGDocumentChunks(id)
	if err != nil {
		writeRAGKnowledgeBaseError(w, err)
		return
	}
	const previewLimit = 256 << 10
	preview := item.Content
	truncated := len(preview) > previewLimit
	if truncated {
		preview = preview[:previewLimit]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"document":          item,
		"content_preview":   preview,
		"content_truncated": truncated,
		"chunks":            chunks,
	})
}

func (s *Server) deleteRAGDocument(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad document id")
		return
	}
	if err := pg.DeleteRAGDocument(id); err != nil {
		writeRAGKnowledgeBaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

func (s *Server) restoreRAGDocument(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad document id")
		return
	}
	if err := pg.RestoreRAGDocument(id); err != nil {
		writeRAGKnowledgeBaseError(w, err)
		return
	}
	item, err := pg.GetRAGDocumentIncludingDeleted(id)
	if err != nil {
		writeRAGKnowledgeBaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) queueRAGDocumentAction(w http.ResponseWriter, r *http.Request, kind string, rebuildVectors bool) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad document id")
		return
	}
	jobID, created, err := pg.QueueRAGDocumentJob(id, kind)
	if err != nil {
		writeRAGKnowledgeBaseError(w, err)
		return
	}
	if created && rebuildVectors {
		go s.rebuildRAGDocumentVectors(id, jobID)
	} else if created {
		go s.ingestRAGDocument(id, jobID)
	}
	status := "queued"
	if !created {
		status = "already_running"
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"document_id": id, "job_id": jobID, "status": status})
}

func (s *Server) reparseRAGDocument(w http.ResponseWriter, r *http.Request) {
	s.queueRAGDocumentAction(w, r, "document_reparse", false)
}

func (s *Server) retryRAGDocument(w http.ResponseWriter, r *http.Request) {
	s.queueRAGDocumentAction(w, r, "document_retry", false)
}

func (s *Server) rebuildRAGDocument(w http.ResponseWriter, r *http.Request) {
	s.queueRAGDocumentAction(w, r, "embedding_rebuild", true)
}

func (s *Server) rebuildRAGDocumentVectors(documentID, jobID int64) {
	pg := s.m.pg
	if pg == nil {
		return
	}
	fail := func(err error) {
		message := err.Error()
		_ = pg.UpdateRAGDocumentStatus(documentID, "failed", message)
		_ = pg.MarkRAGIngestionJob(jobID, "failed", message)
	}
	if err := pg.MarkRAGIngestionJob(jobID, "running", ""); err != nil {
		fail(err)
		return
	}
	document, err := pg.GetRAGDocument(documentID)
	if err != nil || document == nil {
		if err == nil {
			err = db.ErrRAGDocumentNotFound
		}
		fail(err)
		return
	}
	base, err := pg.GetRAGKnowledgeBase(document.KnowledgeBaseID)
	if err != nil || base == nil {
		if err == nil {
			err = db.ErrRAGKnowledgeBaseNotFound
		}
		fail(err)
		return
	}
	embedder, err := rag.ResolveEmbedder(base.EmbeddingProvider, base.EmbeddingModel, 256, true)
	if err != nil {
		fail(err)
		return
	}
	chunkRows, err := pg.ListRAGChunksForEmbedding(documentID)
	if err != nil {
		fail(err)
		return
	}
	if len(chunkRows) == 0 {
		fail(errors.New("文档没有可向量化的切片"))
		return
	}
	if err := pg.UpdateRAGDocumentStatus(documentID, "embedding", ""); err != nil {
		fail(err)
		return
	}
	embeddings := make([]db.RAGEmbeddingInput, 0, len(chunkRows))
	for _, row := range chunkRows {
		values, embedErr := embedder.Embed(row.Content)
		if embedErr != nil {
			fail(embedErr)
			return
		}
		embeddings = append(embeddings, db.RAGEmbeddingInput{
			ChunkID: row.ChunkID, Provider: embedder.Provider(), Model: embedder.Model(),
			Dimensions: embedder.Dimensions(), Normalized: embedder.Normalized(), Values: values,
		})
	}
	if err := pg.UpsertRAGEmbeddings(embeddings); err != nil {
		fail(err)
		return
	}
	if err := pg.UpdateRAGDocumentStatus(documentID, "ready", ""); err != nil {
		fail(err)
		return
	}
	_ = pg.MarkRAGIngestionJob(jobID, "succeeded", "")
}

func (s *Server) searchRAGKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	knowledgeBaseID, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad knowledge base id")
		return
	}
	var req struct {
		Query          string `json:"query"`
		TopK           int    `json:"top_k"`
		TaskID         string `json:"task_id"`
		AgentID        string `json:"agent_id"`
		ConversationID *int64 `json:"conversation_id"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		writeErr(w, http.StatusBadRequest, "检索问题不能为空")
		return
	}
	if req.TopK <= 0 || req.TopK > 50 {
		req.TopK = 5
	}
	base, err := pg.GetRAGKnowledgeBase(knowledgeBaseID)
	if err != nil {
		writeRAGKnowledgeBaseError(w, err)
		return
	}
	if base == nil {
		writeRAGKnowledgeBaseError(w, db.ErrRAGKnowledgeBaseNotFound)
		return
	}
	var queryVector []float32
	model := strings.TrimSpace(base.EmbeddingModel)
	if embedder, embedErr := rag.ResolveEmbedder(base.EmbeddingProvider, base.EmbeddingModel, 256, true); embedErr == nil {
		queryVector, _ = embedder.Embed(req.Query)
		model = embedder.Model()
	}
	started := time.Now()
	results, mode, err := pg.SearchRAGChunksHybrid(knowledgeBaseID, req.Query, model, queryVector, req.TopK)
	if err != nil {
		writeRAGKnowledgeBaseError(w, err)
		return
	}
	latency := time.Since(started).Milliseconds()
	logID, err := pg.LogRAGRetrieval(
		knowledgeBaseID, req.TaskID, req.AgentID, req.ConversationID,
		req.Query, req.TopK, mode, results, latency,
	)
	if err != nil {
		writeRAGKnowledgeBaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"retrieval_log_id": logID,
		"mode":             mode,
		"latency_ms":       latency,
		"results":          results,
	})
}

func (s *Server) ragCapabilities(w http.ResponseWriter, _ *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	available, version, err := pg.ProbeRAGVectorExtension()
	if err != nil {
		writeRAGKnowledgeBaseError(w, err)
		return
	}
	backend := "keyword+local_embedding"
	if available {
		backend = "jsonb_compat+local_embedding"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"backend":          backend,
		"pgvector":         available,
		"pgvector_version": version,
		"vector_storage":   "jsonb_compat",
		"vector_search":    "in_process_cosine",
		"hybrid_retrieval": true,
		"rrf":              true,
	})
}
func (s *Server) listRAGRetrievalLogs(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	knowledgeBaseID, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad knowledge base id")
		return
	}
	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			limit = value
		}
	}
	items, err := pg.ListRAGRetrievalLogs(knowledgeBaseID, limit)
	if err != nil {
		writeRAGKnowledgeBaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": items})
}

func (s *Server) listTaskRAGRetrievalLogs(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	if _, ok := s.m.Task(taskID); !ok {
		writeErr(w, http.StatusNotFound, "task not found")
		return
	}
	pg := s.pg(w)
	if pg == nil {
		return
	}
	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			limit = value
		}
	}
	items, err := pg.ListRAGRetrievalLogsByTask(taskID, limit)
	if err != nil {
		writeRAGKnowledgeBaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": items})
}
