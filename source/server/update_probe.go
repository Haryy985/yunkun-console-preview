package server

import (
	"context"
	"net/http"
	"time"
)

// UpdateProbeHandler exercises migrated stores without constructing the Agent
// runtime, schedulers or notification workers, and never admits user writes.
func UpdateProbeHandler(m *Manager) http.Handler {
	s := &Server{m: m}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/health":
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			ok := m.pg.PingContext(ctx) == nil
			status := http.StatusOK
			if !ok {
				status = http.StatusServiceUnavailable
			}
			writeJSON(w, status, map[string]any{
				"ok": ok, "ready": ok, "version": BuildVersion, "service": "yunkun", "update_probe": true,
			})
		case "/api/auth/status":
			s.authStatus(w, r)
		default:
			writeErr(w, http.StatusServiceUnavailable, "正在检查更新后的数据库，请稍候")
		}
	})
}
