package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"yunkun/console/db"
	"yunkun/console/proxypool"
)

type proxyPoolService struct {
	mu       sync.Mutex
	vault    *proxypool.Vault
	provider *proxypool.Provider
	gateway  *proxypool.Gateway
	baseURL  string
}

func (s *Server) initProxyPools(keyDir string) error {
	vault, err := proxypool.LoadVault(keyDir)
	if err != nil {
		return err
	}
	if _, err := s.m.pg.RepairQuakeDiscoveryProtocols(); err != nil {
		return fmt.Errorf("repair Quake discovery protocols: %w", err)
	}
	s.proxyPools = &proxyPoolService{vault: vault, provider: proxypool.NewProvider()}
	s.proxyPools.gateway = &proxypool.Gateway{Key: s.jwtKey, Context: s.ctx, Resolve: s.resolveProxyRoute,
		Audit: func(r proxypool.Route, target, result string) error {
			return s.m.pg.AddProxyRouteEvent(r.TaskID, r.PoolID, r.Node.ID, target, result)
		},
		CaptureEnabled: s.m.TrafficEnabled,
		Capture: func(_ proxypool.Route, req *http.Request, resp *http.Response, request, response []byte) (string, error) {
			if s.m.traffic == nil {
				return "", errors.New("traffic storage unavailable")
			}
			return s.m.traffic.RecordHTTP(req, resp, request, response)
		},
		Complete: s.m.pg.AddProxyRouteCompletion,
	}
	return nil
}

// SetProxyGatewayAddress reuses the app listener; no additional port is opened.
// Call before serving requests or launching tasks.
func (s *Server) SetProxyGatewayAddress(addr string) error {
	if s.proxyPools == nil {
		return errors.New("proxy pool service unavailable")
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	s.proxyPools.baseURL = "http://" + net.JoinHostPort(host, port)
	return nil
}

func (s *Server) proxyPoolReady(w http.ResponseWriter) bool {
	if s.proxyPools == nil || s.m == nil || s.m.pg == nil {
		writeErr(w, 503, "代理池服务不可用")
		return false
	}
	return true
}

func (s *Server) proxyPoolRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/proxy-pools", s.listProxyPools)
	mux.HandleFunc("GET /api/proxy-settings", s.getProxySettings)
	mux.HandleFunc("PATCH /api/proxy-settings", s.updateProxySettings)
	mux.HandleFunc("POST /api/proxy-pools", s.saveProxyPool)
	mux.HandleFunc("PUT /api/proxy-pools/{id}", s.saveProxyPool)
	mux.HandleFunc("DELETE /api/proxy-pools/{id}", s.deleteProxyPool)
	mux.HandleFunc("GET /api/proxy-pools/{id}/nodes", s.listProxyNodes)
	mux.HandleFunc("PATCH /api/proxy-pools/{id}/nodes/{node}", s.updateProxyNode)
	mux.HandleFunc("POST /api/proxy-pools/{id}/nodes/batch", s.batchUpdateProxyNodes)
	mux.HandleFunc("DELETE /api/proxy-pools/{id}/nodes/{node}", s.deleteProxyNode)
	mux.HandleFunc("POST /api/proxy-pools/{id}/fetch", s.fetchProxyNodes)
	mux.HandleFunc("POST /api/proxy-pools/{id}/quota", s.refreshProxyQuota)
	mux.HandleFunc("POST /api/proxy-pools/{id}/nodes/{node}/verify", s.verifyProxyNode)
	mux.HandleFunc("GET /api/proxy-pools/{id}/events", s.proxyPoolEvents)
	mux.HandleFunc("GET /api/tasks/{id}/network", s.taskNetworkStatus)
	mux.HandleFunc("POST /api/tasks/{id}/network/lease", s.reassignTaskProxyLease)
}

const (
	proxyGlobalEnabledSetting = "proxy.global_enabled"
	proxyDefaultPoolSetting   = "proxy.default_pool_id"
)

func (s *Server) getProxySettings(w http.ResponseWriter, r *http.Request) {
	if !s.proxyPoolReady(w) {
		return
	}
	enabled := true
	if raw, ok, err := s.m.pg.GetSetting(proxyGlobalEnabledSetting); err == nil && ok {
		enabled = raw != "false"
	}
	var defaultPoolID *int64
	if raw, ok, err := s.m.pg.GetSetting(proxyDefaultPoolSetting); err == nil && ok {
		if value, parseErr := strconv.ParseInt(raw, 10, 64); parseErr == nil && value > 0 {
			defaultPoolID = &value
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": enabled, "default_pool_id": defaultPoolID})
}

func (s *Server) updateProxySettings(w http.ResponseWriter, r *http.Request) {
	if !s.proxyPoolReady(w) {
		return
	}
	var req struct {
		Enabled       *bool  `json:"enabled"`
		DefaultPoolID *int64 `json:"default_pool_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "代理全局设置无效")
		return
	}
	if req.Enabled != nil {
		if err := s.m.pg.SetSetting(proxyGlobalEnabledSetting, strconv.FormatBool(*req.Enabled)); err != nil {
			writeErr(w, http.StatusInternalServerError, "保存代理全局开关失败")
			return
		}
	}
	if req.DefaultPoolID != nil {
		if *req.DefaultPoolID == 0 {
			if err := s.m.pg.SetSetting(proxyDefaultPoolSetting, ""); err != nil {
				writeErr(w, http.StatusInternalServerError, "清除默认代理池失败")
				return
			}
		} else {
			pool, err := s.m.pg.ProxyPool(*req.DefaultPoolID)
			if err != nil || pool == nil || !pool.Config.Enabled {
				writeErr(w, http.StatusBadRequest, "默认代理池不存在或已停用")
				return
			}
			if err := s.m.pg.SetSetting(proxyDefaultPoolSetting, strconv.FormatInt(*req.DefaultPoolID, 10)); err != nil {
				writeErr(w, http.StatusInternalServerError, "保存默认代理池失败")
				return
			}
		}
	}
	s.getProxySettings(w, r)
}

func (s *Server) listProxyPools(w http.ResponseWriter, r *http.Request) {
	if !s.proxyPoolReady(w) {
		return
	}
	pools, err := s.m.pg.ListProxyPools()
	if err != nil {
		writeErr(w, 500, "读取代理来源失败")
		return
	}
	identity, _ := r.Context().Value(authContextKey{}).(*authIdentity)
	admin := identity != nil && identity.Role == "admin"
	for _, p := range pools {
		if admin {
			secrets, err := s.proxyPools.vault.Open(p.Credentials)
			if err == nil {
				p.HasCredentials = secrets != (proxypool.Secrets{})
			}
		} else {
			p.Config = proxypool.Config{Name: p.Config.Name, Kind: p.Config.Kind, Enabled: p.Config.Enabled}
			p.Quota = proxypool.Quota{}
		}
	}
	writeJSON(w, 200, map[string]any{"pools": pools, "enforced_available": false})
}

type saveProxyPoolRequest struct {
	Config  proxypool.Config   `json:"config"`
	Secrets *proxypool.Secrets `json:"secrets,omitempty"`
}

func (s *Server) saveProxyPool(w http.ResponseWriter, r *http.Request) {
	if !s.proxyPoolReady(w) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var req saveProxyPoolRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeErr(w, 400, "代理配置 JSON 无效")
		return
	}
	if err := req.Config.Validate(); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var id int64
	if r.Method == http.MethodPut {
		var ok bool
		id, ok = pathInt(r, "id")
		if !ok || id <= 0 {
			writeErr(w, 400, "来源 ID 无效")
			return
		}
	}
	s.proxyPools.mu.Lock()
	defer s.proxyPools.mu.Unlock()
	var secrets proxypool.Secrets
	requireUnused := false
	if id > 0 {
		old, err := s.m.pg.ProxyPool(id)
		if err != nil {
			writeErr(w, 500, "读取来源失败")
			return
		}
		if old == nil {
			writeErr(w, 404, "来源不存在")
			return
		}
		secrets, err = s.proxyPools.vault.Open(old.Credentials)
		if err != nil {
			writeErr(w, 503, "凭据解密失败，检查应用密钥")
			return
		}
		// Changing endpoint identity under a running lease is ambiguous.
		if old.Config.Kind != req.Config.Kind || old.Config.Protocol != req.Config.Protocol || old.Config.APIURL != req.Config.APIURL ||
			strings.Join(old.Config.Addresses, "\n") != strings.Join(req.Config.Addresses, "\n") || req.Secrets != nil {
			requireUnused = true
		}
	}
	if req.Secrets != nil {
		secrets = *req.Secrets
	}
	if req.Config.Kind == "xiaoxiang" && (secrets.AppKey == "" || secrets.AppSecret == "") {
		writeErr(w, 400, "小象来源需要 AppKey 和 AppSecret")
		return
	}
	if len(secrets.AppKey) > 1024 || len(secrets.AppSecret) > 4096 || len(secrets.APIValue) > 8192 ||
		len(secrets.ProxyUsername) > 1024 || len(secrets.ProxyPassword) > 4096 ||
		strings.ContainsAny(secrets.APIHeader+secrets.APIValue, "\r\n") {
		writeErr(w, 400, "凭据格式无效或过长")
		return
	}
	cipher, err := s.proxyPools.vault.Seal(secrets)
	if err != nil {
		writeErr(w, 500, "保存凭据失败")
		return
	}
	id, err = s.m.pg.SaveProxyPool(id, req.Config, cipher, requireUnused)
	if err != nil {
		if errors.Is(err, db.ErrProxyPoolInUse) {
			writeErr(w, 409, "来源已有任务绑定；请新建来源以更换地址、协议或凭据")
			return
		}
		writeErr(w, 500, "保存来源失败")
		return
	}
	writeJSON(w, 200, map[string]any{"id": id})
}

func (s *Server) deleteProxyPool(w http.ResponseWriter, r *http.Request) {
	if !s.proxyPoolReady(w) {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "来源 ID 无效")
		return
	}
	s.proxyPools.mu.Lock()
	defer s.proxyPools.mu.Unlock()
	if err := s.m.pg.DeleteProxyPool(id); err != nil {
		writeErr(w, 409, "来源不存在或仍有任务绑定，请改为禁用")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) poolForRequest(w http.ResponseWriter, r *http.Request) (*db.ProxyPool, proxypool.Secrets, bool) {
	if !s.proxyPoolReady(w) {
		return nil, proxypool.Secrets{}, false
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "来源 ID 无效")
		return nil, proxypool.Secrets{}, false
	}
	p, err := s.m.pg.ProxyPool(id)
	if err != nil {
		writeErr(w, 500, "读取来源失败")
		return nil, proxypool.Secrets{}, false
	}
	if p == nil {
		writeErr(w, 404, "来源不存在")
		return nil, proxypool.Secrets{}, false
	}
	secrets, err := s.proxyPools.vault.Open(p.Credentials)
	if err != nil {
		writeErr(w, 503, "凭据不可用")
		return nil, proxypool.Secrets{}, false
	}
	return p, secrets, true
}

func (s *Server) listProxyNodes(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.poolForRequest(w, r)
	if !ok {
		return
	}
	nodes, err := s.m.pg.ProxyNodes(p.ID)
	if err != nil {
		writeErr(w, 500, "读取节点失败")
		return
	}
	warningSeconds := p.Config.ExpiryWarningSeconds
	if warningSeconds <= 0 {
		warningSeconds = 3600
	}
	summary, err := s.m.pg.ProxyNodeSummary(p.ID, time.Duration(warningSeconds)*time.Second)
	if err != nil {
		writeErr(w, 500, "读取节点状态失败")
		return
	}
	writeJSON(w, 200, map[string]any{"nodes": nodes, "summary": summary})
}

func (s *Server) updateProxyNode(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.poolForRequest(w, r)
	if !ok {
		return
	}
	nodeID, valid := pathInt(r, "node")
	if !valid {
		writeErr(w, 400, "节点 ID 无效")
		return
	}
	var req struct {
		Enabled *bool  `json:"enabled"`
		Notes   string `json:"notes"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
		writeErr(w, 400, "节点修改请求无效")
		return
	}
	if req.Enabled == nil {
		writeErr(w, 400, "没有可修改的节点字段")
		return
	}
	if err := s.m.pg.SetProxyNodeEnabled(p.ID, nodeID, *req.Enabled); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 404, "节点不存在")
			return
		}
		writeErr(w, 500, "节点状态保存失败")
		return
	}
	if req.Notes != "" {
		if _, err := s.m.pg.Exec(`UPDATE proxy_nodes SET notes=$3 WHERE pool_id=$1 AND id=$2 AND deleted_at IS NULL`, p.ID, nodeID, req.Notes); err != nil {
			writeErr(w, 500, "节点备注保存失败")
			return
		}
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) deleteProxyNode(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.poolForRequest(w, r)
	if !ok {
		return
	}
	nodeID, valid := pathInt(r, "node")
	if !valid {
		writeErr(w, 400, "节点 ID 无效")
		return
	}
	if err := s.m.pg.SoftDeleteProxyNode(p.ID, nodeID); err != nil {
		writeErr(w, 409, "节点不存在、已删除或仍被任务使用，请先停用或换绑")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) batchUpdateProxyNodes(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.poolForRequest(w, r)
	if !ok {
		return
	}
	var req struct {
		NodeIDs []int64 `json:"node_ids"`
		Action  string  `json:"action"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&req); err != nil ||
		len(req.NodeIDs) == 0 || len(req.NodeIDs) > 500 ||
		(req.Action != "enable" && req.Action != "disable") {
		writeErr(w, 400, "批量节点操作无效")
		return
	}
	changed, err := s.m.pg.BatchSetProxyNodes(p.ID, req.NodeIDs, req.Action == "enable")
	if err != nil {
		writeErr(w, 500, "批量节点操作失败")
		return
	}
	writeJSON(w, 200, map[string]int64{"changed": changed})
}

func (s *Server) claimProvider(w http.ResponseWriter, p *db.ProxyPool, secrets proxypool.Secrets, action string, seconds int) bool {
	account := p.Config.Kind + ":" + secrets.AppKey
	if p.Config.Kind != "xiaoxiang" {
		account = p.Config.Kind + ":" + p.Config.APIURL
	}
	hash := sha256.Sum256([]byte(account + ":" + action))
	ok, err := s.m.pg.ClaimProxyProviderCall(hex.EncodeToString(hash[:]), seconds)
	if err != nil {
		writeErr(w, 503, "限流状态不可用")
		return false
	}
	if !ok {
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
		writeErr(w, 429, "操作过于频繁，请稍后重试")
		return false
	}
	return true
}

func (s *Server) fetchProxyNodes(w http.ResponseWriter, r *http.Request) {
	if !s.proxyPoolReady(w) {
		return
	}
	s.proxyPools.mu.Lock()
	defer s.proxyPools.mu.Unlock()
	p, secrets, ok := s.poolForRequest(w, r)
	if !ok {
		return
	}
	if !p.Config.Enabled {
		writeErr(w, 409, "来源已禁用")
		return
	}
	if p.Config.Kind != "fixed" && !s.claimProvider(w, p, secrets, "fetch", 10) {
		return
	}
	if p.Quota.Error == "" && p.Quota.CheckedAt != nil && time.Since(*p.Quota.CheckedAt) < time.Minute &&
		p.Quota.Remaining != nil && *p.Quota.Remaining == 0 {
		writeErr(w, 409, "最近查询余量为零，请刷新套餐余量后再提取")
		return
	}
	nodes, err := s.proxyPools.provider.Fetch(r.Context(), p.Config, secrets, time.Now())
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	if err := s.m.pg.StoreProxyNodes(p.ID, nodes); err != nil {
		writeErr(w, 500, "供应商已返回节点，但保存失败；请勿立即重复提取")
		return
	}
	writeJSON(w, 200, map[string]any{"fetched": len(nodes)})
}

func (s *Server) refreshProxyQuota(w http.ResponseWriter, r *http.Request) {
	if !s.proxyPoolReady(w) {
		return
	}
	s.proxyPools.mu.Lock()
	defer s.proxyPools.mu.Unlock()
	p, secrets, ok := s.poolForRequest(w, r)
	if !ok {
		return
	}
	if p.Config.Kind != "xiaoxiang" || !p.Config.QuotaSupported {
		writeJSON(w, 200, proxypool.Quota{Supported: false})
		return
	}
	if !p.Config.Enabled {
		writeErr(w, 409, "来源已禁用")
		return
	}
	if p.Quota.CheckedAt != nil && p.Quota.Error == "" && time.Since(*p.Quota.CheckedAt) < 30*time.Second {
		writeJSON(w, 200, p.Quota)
		return
	}
	if !s.claimProvider(w, p, secrets, "quota", 1) {
		return
	}
	q, err := s.proxyPools.provider.Remaining(r.Context(), p.Config, secrets)
	if err != nil {
		q = p.Quota
		q.Supported = true
		q.Error = err.Error()
	}
	if e := s.m.pg.StoreProxyQuota(p.ID, q); e != nil {
		writeErr(w, 500, "保存余量状态失败")
		return
	}
	writeJSON(w, 200, q)
}

func (s *Server) verifyProxyNode(w http.ResponseWriter, r *http.Request) {
	p, secrets, ok := s.poolForRequest(w, r)
	if !ok {
		return
	}
	nodeID, valid := pathInt(r, "node")
	if !valid {
		writeErr(w, 400, "节点 ID 无效")
		return
	}
	if !p.Config.Enabled {
		writeErr(w, 409, "来源已禁用")
		return
	}
	nodes, err := s.m.pg.ProxyNodes(p.ID)
	if err != nil {
		writeErr(w, 500, "读取节点失败")
		return
	}
	for _, n := range nodes {
		if n.ID != nodeID {
			continue
		}
		if n.ExpiresAt != nil && !n.ExpiresAt.After(time.Now().Add(time.Duration(p.Config.SafetySeconds)*time.Second)) {
			writeErr(w, 409, "节点已过期或临近到期")
			return
		}
		upstream, err := p.Config.Upstream(n, secrets)
		var ip string
		started := time.Now()
		if err == nil {
			ip, err = proxypool.Verify(r.Context(), upstream, p.Config.VerifyURL)
		}
		if err != nil {
			latency := int(time.Since(started).Milliseconds())
			_ = s.m.pg.VerifyProxyNodeWithLatency(n.ID, "", err.Error(), &latency)
			writeErr(w, 502, err.Error())
			return
		}
		latency := int(time.Since(started).Milliseconds())
		if err := s.m.pg.VerifyProxyNodeWithLatency(n.ID, ip, "", &latency); err != nil {
			writeErr(w, 500, "验证完成但保存失败")
			return
		}
		writeJSON(w, 200, map[string]any{"ip": ip, "checked_at": time.Now().UTC()})
		return
	}
	writeErr(w, 404, "节点不存在")
}

func (s *Server) resolveProxyRoute(ctx context.Context, taskID int64) (proxypool.Route, error) {
	route := proxypool.Route{TaskID: taskID}
	if err := ctx.Err(); err != nil {
		return route, err
	}
	if s.proxyPools == nil {
		return route, errors.New("proxy service unavailable")
	}
	b, err := s.m.pg.TaskProxyBinding(taskID)
	if err != nil || b == nil {
		return route, errors.New("task proxy binding unavailable")
	}
	if b.Mode != "configured" {
		return route, errors.New("enforced executor unavailable")
	}
	var allowed bool
	if err := s.m.pg.QueryRowContext(ctx, `SELECT deleted_at IS NULL AND NOT paused AND NOT queued FROM tasks WHERE id=$1`, taskID).Scan(&allowed); err != nil || !allowed {
		return route, errors.New("task is paused, queued or unavailable")
	}
	p, err := s.m.pg.ProxyPool(b.PoolID)
	if err != nil || p == nil || !p.Config.Enabled {
		return route, errors.New("proxy pool disabled or unavailable")
	}
	route.PoolID = p.ID
	route.Revision = b.Revision
	if b.NodeID == nil {
		return route, errors.New("proxy lease unavailable")
	}
	n, err := s.m.pg.ProxyNode(p.ID, *b.NodeID)
	if err != nil || n == nil {
		return route, errors.New("proxy nodes unavailable")
	}
	if !n.Enabled {
		return route, errors.New("proxy node disabled")
	}
	margin := time.Now().Add(time.Duration(p.Config.SafetySeconds) * time.Second)
	if (b.ExpiresAt != nil && !b.ExpiresAt.After(margin)) || (n.ExpiresAt != nil && !n.ExpiresAt.After(margin)) {
		return route, errors.New("proxy lease expired; pause the task, fetch nodes and reassign its lease")
	}
	secrets, err := s.proxyPools.vault.Open(p.Credentials)
	if err != nil {
		return route, err
	}
	route.Node = *n
	route.Node.ExpiresAt = b.ExpiresAt
	route.Upstream, err = p.Config.Upstream(*n, secrets)
	return route, err
}

func (s *Server) reassignTaskProxyLease(w http.ResponseWriter, r *http.Request) {
	if !s.proxyPoolReady(w) {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "任务 ID 无效")
		return
	}
	if err := s.m.pg.ReassignProxyLease(id); err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) networkStatus(taskID int64) (map[string]any, error) {
	b, err := s.m.pg.TaskProxyBinding(taskID)
	if err != nil {
		return nil, errors.New("读取任务出口失败")
	}
	if b == nil {
		return map[string]any{"mode": "default", "bound": false, "enforced": false}, nil
	}
	out := map[string]any{"bound": true, "binding": b, "enforced": false, "supported_protocols": []string{"HTTP", "HTTPS"}, "failure_policy": "stop", "capture": "connection_metadata_only",
		"managed_tools":   []string{"HTTP", "WebFetch"},
		"unmanaged_tools": []string{"Bash (environment only)", "interactive_shell", "browser/MCP", "custom network tools", "asset enrichment", "web_search", "LLM/provider requests"},
		"coverage":        "managed_http_tools_only"}
	if s.m.TrafficEnabled() {
		if s.m.traffic == nil {
			out["capture"] = "storage_unavailable"
		} else {
			out["capture"] = "http_and_webfetch_complete_bodies"
			out["capture_body_limit"] = proxypool.MaxCaptureBody
		}
	}
	p, err := s.m.pg.ProxyPool(b.PoolID)
	if err != nil || p == nil {
		return nil, errors.New("读取代理来源失败")
	}
	out["pool_name"] = p.Config.Name
	route, err := s.resolveProxyRoute(context.Background(), taskID)
	if err != nil {
		out["status"] = "unavailable"
		out["reason"] = err.Error()
	} else {
		out["status"] = "configured"
		out["node"] = route.Node
	}
	return out, nil
}

func (s *Server) taskNetworkStatus(w http.ResponseWriter, r *http.Request) {
	if !s.proxyPoolReady(w) {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "任务 ID 无效")
		return
	}
	if _, ok := s.m.Task(strconv.FormatInt(id, 10)); !ok {
		writeErr(w, 404, "任务不存在")
		return
	}
	status, err := s.networkStatus(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	events, err := s.m.pg.ProxyRouteEvents(id, 0)
	if err != nil {
		writeErr(w, 500, "读取出口审计失败")
		return
	}
	status["events"] = events
	writeJSON(w, 200, status)
}

func (s *Server) proxyPoolEvents(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.poolForRequest(w, r)
	if !ok {
		return
	}
	events, err := s.m.pg.ProxyRouteEvents(0, p.ID)
	if err != nil {
		writeErr(w, 500, "读取出口审计失败")
		return
	}
	writeJSON(w, 200, map[string]any{"events": events})
}

func (s *Server) taskProxyAddress(taskID int64) (string, string) {
	b, err := s.m.pg.TaskProxyBinding(taskID)
	if err != nil {
		return "http://127.0.0.1:1", ""
	}
	if b == nil {
		return s.m.ProxyAddr(), s.m.ProxyCACert()
	}
	if s.proxyPools == nil {
		return "http://127.0.0.1:1", ""
	}
	if s.proxyPools.baseURL == "" {
		return "http://127.0.0.1:1", ""
	}
	return s.proxyPools.gateway.URL(s.proxyPools.baseURL, taskID), ""
}

func (s *Server) validateTaskProxy(poolID *int64, mode string) error {
	if poolID == nil {
		if mode != "" {
			return errors.New("网络模式需要指定代理池")
		}
		return nil
	}
	if mode != "" && mode != "configured" {
		return errors.New("强制模式尚无已验收的隔离执行器，拒绝降级运行")
	}
	if s.proxyPools == nil || s.proxyPools.baseURL == "" {
		return errors.New("任务出口网关尚未就绪")
	}
	p, err := s.m.pg.ProxyPool(*poolID)
	if err != nil || p == nil || !p.Config.Enabled {
		return errors.New("代理来源不存在或已禁用")
	}
	return nil
}
