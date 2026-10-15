package server

import (
	"net/http"
	"strings"
	"sync"
)

// StartupHandler keeps the UI reachable while stores initialize. No business or
// authentication endpoint is exposed until the complete server is ready.
type StartupHandler struct {
	mu    sync.RWMutex
	ready http.Handler
	ui    http.Handler
}

func NewStartupHandler() *StartupHandler {
	return &StartupHandler{ui: (&Server{}).webuiHandler()}
}

func (h *StartupHandler) SetReady(handler http.Handler) {
	h.mu.Lock()
	h.ready = handler
	h.mu.Unlock()
}

func (h *StartupHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	ready := h.ready
	h.mu.RUnlock()
	if ready != nil {
		ready.ServeHTTP(w, r)
		return
	}
	if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Retry-After", "5")
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"ok": false, "ready": false, "service": "yunkun", "version": BuildVersion,
			"error": "服务正在初始化或等待数据库恢复，请稍后重新检查。",
		})
		return
	}
	h.ui.ServeHTTP(w, r)
}
