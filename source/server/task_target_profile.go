package server

import (
	"errors"
	"net/http"
	"strconv"

	"yunkun/console/db"
)

const maxTaskProfileRequestBytes = 8 << 10

func (s *Server) taskProfileList(w http.ResponseWriter, r *http.Request) {
	task, ok := s.m.Task(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "task not found")
		return
	}
	if s.m.pg == nil {
		writeErr(w, http.StatusServiceUnavailable, "任务画像不可用")
		return
	}
	before := int64(0)
	if raw := r.URL.Query().Get("before"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value < 0 {
			writeErr(w, http.StatusBadRequest, "invalid before")
			return
		}
		before = value
	}
	limit := 25
	if raw := r.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > db.MaxTaskProfilePage {
			writeErr(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = value
	}
	taskID, _ := parseTaskID(task.ID)
	entries, err := s.m.pg.ListTaskProfileEntries(taskID, before, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "任务画像读取失败")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

func (s *Server) taskProfileAdd(w http.ResponseWriter, r *http.Request) {
	task, ok := s.m.Task(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "task not found")
		return
	}
	if s.m.pg == nil {
		writeErr(w, http.StatusServiceUnavailable, "任务画像不可用")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxTaskProfileRequestBytes)
	var input db.TaskProfileInput
	if err := decode(r, &input); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeErr(w, http.StatusRequestEntityTooLarge, "请求正文过大")
		} else {
			writeErr(w, http.StatusBadRequest, "invalid JSON")
		}
		return
	}
	taskID, _ := parseTaskID(task.ID)
	entry, err := s.m.pg.AddTaskProfileEntry(taskID, input)
	switch {
	case errors.Is(err, db.ErrTaskProfileInvalid):
		writeErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, db.ErrTaskProfileTaskNotFound):
		writeErr(w, http.StatusBadRequest, "范围或证据节点不属于当前任务")
	case err != nil:
		writeErr(w, http.StatusInternalServerError, "任务画像保存失败")
	default:
		writeJSON(w, http.StatusCreated, entry)
	}
}
