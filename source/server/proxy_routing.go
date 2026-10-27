package server

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"yunkun/console/proxypool"
)

// Validate the selected route without consuming another turn or comparing it
// with the next worker's node. Policy and credential revisions still revoke it.
func (s *Server) validateProxyRoute(ctx context.Context, route proxypool.Route) error {
	b, err := s.m.pg.TaskProxyBinding(route.TaskID)
	if err != nil || b == nil || b.PoolID != route.PoolID || b.Revision != route.Revision || b.Mode != "configured" ||
		b.Strategy != route.Strategy || (b.Strategy == "fixed" && (b.NodeID == nil || *b.NodeID != route.Node.ID)) {
		return errors.New("proxy route revoked")
	}
	var allowed bool
	if err := s.m.pg.QueryRowContext(ctx, `SELECT deleted_at IS NULL AND NOT paused AND NOT queued FROM tasks WHERE id=$1`, route.TaskID).Scan(&allowed); err != nil || !allowed {
		return errors.New("task is paused, queued or unavailable")
	}
	p, err := s.m.pg.ProxyPool(route.PoolID)
	if err != nil || p == nil || !p.Config.Enabled || sha256.Sum256([]byte(p.Credentials)) != route.CredentialTag {
		return errors.New("proxy pool disabled or credentials changed")
	}
	n, err := s.m.pg.ProxyNode(route.PoolID, route.Node.ID)
	if err != nil || n == nil || !n.Enabled || n.Address != route.Node.Address || n.LastError != "" {
		return errors.New("proxy node revoked")
	}
	margin := time.Now().Add(time.Duration(p.Config.SafetySeconds) * time.Second)
	if (n.ExpiresAt != nil && !n.ExpiresAt.After(margin)) ||
		(route.Node.ExpiresAt != nil && !route.Node.ExpiresAt.After(margin)) ||
		((n.Origin == "fofa" || n.Origin == "quake") && !n.HasFreshVerification(time.Now())) {
		return errors.New("proxy route expired or verification unavailable")
	}
	return ctx.Err()
}

func (s *Server) proxyRuntimeFailure(route proxypool.Route, reason string) error {
	if reason != "proxy_connect_failed" && reason != "proxy_authentication_failed" {
		return errors.New("invalid runtime failure reason")
	}
	if err := s.m.pg.RecordProxyRuntimeFailure(route, reason); err != nil {
		return err
	}
	return s.m.pg.AddProxyRouteCompletion(route, "", reason+":cooldown_60s", "")
}

func (s *Server) completeProxyRoute(route proxypool.Route, target, result, trafficID string) error {
	if err := s.m.pg.AddProxyRouteCompletion(route, target, result, trafficID); err != nil {
		return err
	}
	if (strings.HasPrefix(result, "http_") && !strings.HasPrefix(result, "http_407")) || result == "tunnel_closed" {
		return s.m.pg.RecordProxyRuntimeSuccess(route)
	}
	return nil
}

func (s *Server) setTaskProxyRouting(w http.ResponseWriter, r *http.Request) {
	if !s.proxyPoolReady(w) {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "任务 ID 无效")
		return
	}
	var req struct {
		Strategy string `json:"strategy"`
		NodeID   *int64 `json:"node_id"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(&req) != nil {
		writeErr(w, 400, "代理调度配置无效")
		return
	}
	if err := s.m.pg.SetProxyRouting(id, req.Strategy, req.NodeID); err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) recoverProxyCredentials(w http.ResponseWriter, r *http.Request) {
	p, ok := s.poolMetadataForRequest(w, r)
	if !ok {
		return
	}
	var req struct {
		Confirm bool               `json:"confirm"`
		NoAuth  bool               `json:"no_auth"`
		Secrets *proxypool.Secrets `json:"secrets"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(&req) != nil || !req.Confirm || req.Secrets == nil {
		writeErr(w, 400, "必须明确确认并提交整组新凭据")
		return
	}
	if _, err := s.proxyPools.vault.Open(p.Credentials); err == nil {
		writeErr(w, 409, "此池凭据可解密，请使用正常编辑流程")
		return
	}
	secrets := *req.Secrets
	if req.NoAuth {
		if p.Config.Kind != "fixed" || secrets != (proxypool.Secrets{}) {
			writeErr(w, 400, "无认证恢复仅适用于固定地址池，且不能同时提交凭据")
			return
		}
	} else if secrets == (proxypool.Secrets{}) {
		writeErr(w, 400, "空凭据需要明确选择无需认证")
		return
	}
	if p.Config.Kind == "xiaoxiang" && (secrets.AppKey == "" || secrets.AppSecret == "") {
		writeErr(w, 400, "小象来源需要 AppKey 和 AppSecret")
		return
	}
	if strings.ContainsAny(secrets.APIHeader+secrets.APIValue, "\r\n") ||
		len(secrets.ProxyUsername) > 1024 || len(secrets.ProxyPassword) > 4096 ||
		len(secrets.APIValue) > 8192 || len(secrets.AppKey) > 1024 || len(secrets.AppSecret) > 4096 {
		writeErr(w, 400, "凭据格式无效或过长")
		return
	}
	raw, err := s.proxyPools.vault.Seal(secrets)
	if err != nil {
		writeErr(w, 500, "凭据加密失败")
		return
	}
	if err := s.m.pg.RecoverProxyCredentials(p.ID, p.Credentials, raw); err != nil {
		writeErr(w, 409, "恢复未执行：请暂停此池绑定的全部任务后重试；"+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "requires_reverification": true})
}
