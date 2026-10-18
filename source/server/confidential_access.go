package server

import (
	"fmt"
	"net/http"
	"strconv"
)

func (s *Server) auditConfidentialAccess(w http.ResponseWriter, r *http.Request, action, resource string, findingID int64, detail string, findingIDs ...[]int64) bool {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	identity, _ := r.Context().Value(authContextKey{}).(*authIdentity)
	if identity == nil || identity.Role != "admin" {
		writeErr(w, http.StatusForbidden, "保密证据仅管理员可访问")
		return false
	}
	if s.m == nil || s.m.pg == nil ||
		s.m.pg.RecordConfidentialAccess(r.Context(), identity.ID, identity.Username, action, resource, findingID, detail, findingIDs...) != nil {
		writeErr(w, http.StatusServiceUnavailable, "访问审计不可用，保密证据未输出")
		return false
	}
	return true
}

func confidentialExportDetail(format, scope string, count int) string {
	return fmt.Sprintf("%s:%s:%d", format, scope, count)
}

func (s *Server) listConfidentialAccess(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	identity, _ := r.Context().Value(authContextKey{}).(*authIdentity)
	if identity == nil || identity.Role != "admin" {
		writeErr(w, http.StatusForbidden, "权限不足")
		return
	}
	before, err := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	if r.URL.Query().Get("before") == "" {
		before = 0
		err = nil
	}
	if err != nil || before < 0 {
		writeErr(w, http.StatusBadRequest, "invalid before")
		return
	}
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if r.URL.Query().Get("limit") == "" {
		limit = 50
		err = nil
	}
	if err != nil || limit < 1 || limit > 100 {
		writeErr(w, http.StatusBadRequest, "invalid limit")
		return
	}
	if s.m == nil || s.m.pg == nil {
		writeErr(w, http.StatusServiceUnavailable, "审计不可用")
		return
	}
	events, err := s.m.pg.ListConfidentialAccess(r.Context(), before, limit)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "审计不可用")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}
