package server

import (
	"encoding/json"
	"errors"
	"net/http"
)

type discoveryBatchRequest struct {
	IDs    []int64 `json:"ids"`
	Action string  `json:"action"`
	PoolID int64   `json:"pool_id"`
	Target string  `json:"target"`
}

func (req discoveryBatchRequest) validate() error {
	limit := 100
	switch req.Action {
	case "verify":
		limit = 5
	case "delete":
	case "approve":
		if req.PoolID <= 0 || (req.Target != "candidate" && req.Target != "production") {
			return errors.New("请指定代理池和加入目标")
		}
	default:
		return errors.New("批量操作无效")
	}
	if len(req.IDs) == 0 || len(req.IDs) > limit {
		return errors.New("批量删除和入池最多 100 条，单次批量验证最多 5 条")
	}
	seen := make(map[int64]bool, len(req.IDs))
	for _, id := range req.IDs {
		if id <= 0 || seen[id] {
			return errors.New("候选 ID 必须为不重复的正整数")
		}
		seen[id] = true
	}
	return nil
}

// Each item reuses the single-item checks, including the current source scope.
func (s *Server) batchProxyDiscoveryResults(w http.ResponseWriter, r *http.Request) {
	if !s.proxyDiscoveryReady(w) {
		return
	}
	var req discoveryBatchRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "批量请求无效")
		return
	}
	if err := req.validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	type outcome struct {
		ID     int64  `json:"id"`
		OK     bool   `json:"ok"`
		Status int    `json:"status"`
		Error  string `json:"error,omitempty"`
	}
	items := make([]outcome, 0, len(req.IDs))
	for _, id := range req.IDs {
		if r.Context().Err() != nil {
			return
		}
		var err error
		var status int
		switch req.Action {
		case "verify":
			_, status, err = s.verifyDiscoveryCandidate(r.Context(), id)
		case "approve":
			_, status, err = s.approveDiscoveryCandidate(r.Context(), id, req.PoolID, req.Target)
		case "delete":
			status, err = s.deleteDiscoveryCandidate(id)
		}
		item := outcome{ID: id, OK: err == nil, Status: status}
		if err != nil {
			item.Error = err.Error()
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) deleteProxyDiscoveryResult(w http.ResponseWriter, r *http.Request) {
	if !s.proxyDiscoveryReady(w) {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok || id <= 0 {
		writeErr(w, http.StatusBadRequest, "候选 ID 无效")
		return
	}
	status, err := s.deleteDiscoveryCandidate(id)
	if err != nil {
		writeErr(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) deleteDiscoveryCandidate(id int64) (int, error) {
	s.proxyDiscovery.mu.Lock()
	defer s.proxyDiscovery.mu.Unlock()
	item, err := s.m.pg.ProxyDiscoveryResult(id)
	if err != nil {
		return http.StatusInternalServerError, errors.New("读取候选失败")
	}
	if item == nil {
		return http.StatusNotFound, errors.New("候选不存在")
	}
	if item.State == "approved" {
		return http.StatusConflict, errors.New("已入池的代理请在代理 IP 中管理，不支持从候选区删除")
	}
	if err := s.m.pg.DeleteProxyDiscoveryResult(id); err != nil {
		return http.StatusInternalServerError, errors.New("删除候选失败")
	}
	return http.StatusOK, nil
}
