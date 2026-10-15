package server

import (
	"errors"
	"net/http"

	"yunkun/console/db"
)

func writeRAGEvalError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, db.ErrRAGEvalSetNotFound):
		writeErr(w, http.StatusNotFound, "评测集不存在")
	case errors.Is(err, db.ErrRAGEvalSetNameUsed):
		writeErr(w, http.StatusConflict, "评测集名称已存在")
	case errors.Is(err, db.ErrRAGKnowledgeBaseNotFound):
		writeErr(w, http.StatusNotFound, "知识库不存在")
	default:
		writeErr(w, http.StatusBadRequest, err.Error())
	}
}

func (s *Server) listRAGEvalSets(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad knowledge base id")
		return
	}
	items, err := pg.ListRAGEvalSets(id)
	if err != nil {
		writeRAGEvalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sets": items})
}

func (s *Server) createRAGEvalSet(w http.ResponseWriter, r *http.Request) {
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
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	item, err := pg.CreateRAGEvalSet(id, req.Name, req.Description)
	if err != nil {
		writeRAGEvalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) listRAGEvalCases(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad eval set id")
		return
	}
	items, err := pg.ListRAGEvalCases(id)
	if err != nil {
		writeRAGEvalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cases": items})
}

func (s *Server) createRAGEvalCase(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad eval set id")
		return
	}
	var req struct {
		Question            string  `json:"question"`
		ExpectedChunkIDs    []int64 `json:"expected_chunk_ids"`
		ExpectedDocumentIDs []int64 `json:"expected_document_ids"`
		Answerable          *bool   `json:"answerable"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	answerable := true
	if req.Answerable != nil {
		answerable = *req.Answerable
	}
	item, err := pg.CreateRAGEvalCase(id, req.Question, req.ExpectedChunkIDs, req.ExpectedDocumentIDs, answerable)
	if err != nil {
		writeRAGEvalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) listRAGEvalRuns(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad eval set id")
		return
	}
	items, err := pg.ListRAGEvalRuns(id)
	if err != nil {
		writeRAGEvalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": items})
}

func (s *Server) runRAGEval(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad eval set id")
		return
	}
	var req struct {
		TopK int `json:"top_k"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	item, err := pg.RunRAGEval(id, req.TopK)
	if err != nil {
		writeRAGEvalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}
