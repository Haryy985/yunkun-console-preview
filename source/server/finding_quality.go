package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"yunkun/console/db"
)

func (s *Server) getFindingQuality(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if id <= 0 {
		writeErr(w, http.StatusBadRequest, "bad finding id")
		return
	}
	q, err := pg.GetFindingQuality(id)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "无法读取漏洞质量状态")
		return
	}
	if q == nil {
		writeErr(w, http.StatusNotFound, "finding quality not found")
		return
	}
	writeJSON(w, http.StatusOK, q)
}

func (s *Server) updateFindingQuality(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if id <= 0 {
		writeErr(w, http.StatusBadRequest, "bad finding id")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	var q db.FindingQuality
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid quality payload")
		return
	}
	q.FindingID = id
	if identity, _ := r.Context().Value(authContextKey{}).(*authIdentity); identity != nil {
		q.Reviewer = identity.Username
	}
	if strings.TrimSpace(q.Reviewer) == "" {
		writeErr(w, http.StatusUnauthorized, "reviewer identity required")
		return
	}
	if err := pg.SetFindingQuality(r.Context(), q); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := pg.GetFindingQuality(id)
	if err != nil || updated == nil {
		writeErr(w, http.StatusServiceUnavailable, "质量状态已更新但读取失败")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
