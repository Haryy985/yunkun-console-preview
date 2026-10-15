package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"yunkun/console/db"
	"yunkun/console/proxypool"
)

type proxyDiscoveryService struct {
	client *http.Client
	mu     sync.Mutex
}

const (
	defaultProxyDiscoveryFetchLimit = 100
	minProxyDiscoveryFetchLimit     = 1
	maxProxyDiscoveryFetchLimit     = 100
	fofaDiscoveryURL                = "https://fofa.info/api/v1/search/all"
	quakeDiscoveryURL               = "https://quake.360.net/api/v3/search/quake_service"
)

type proxyDiscoverySaveRequest struct {
	Provider        string            `json:"provider"`
	Name            string            `json:"name"`
	Query           string            `json:"query"`
	AuthorizedScope string            `json:"authorized_scope"`
	VerifyURL       string            `json:"verify_url"`
	FetchLimit      int               `json:"fetch_limit"`
	Credentials     map[string]string `json:"credentials"`
}

func (s *Server) initProxyDiscovery() {
	if s.proxyDiscovery == nil {
		s.proxyDiscovery = &proxyDiscoveryService{client: &http.Client{
			Timeout:       20 * time.Second,
			CheckRedirect: restrictedDiscoveryRedirect,
		}}
	}
}

func (s *Server) proxyDiscoveryReady(w http.ResponseWriter) bool {
	if s.m == nil || s.m.pg == nil || s.proxyPools == nil || s.proxyPools.vault == nil {
		writeErr(w, http.StatusServiceUnavailable, "代理发现服务不可用")
		return false
	}
	s.initProxyDiscovery()
	return true
}

func (s *Server) proxyDiscoveryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/proxy-discovery/sources", s.listProxyDiscoverySources)
	mux.HandleFunc("POST /api/proxy-discovery/sources", s.saveProxyDiscoverySource)
	mux.HandleFunc("PUT /api/proxy-discovery/sources/{id}", s.saveProxyDiscoverySource)
	mux.HandleFunc("DELETE /api/proxy-discovery/sources/{id}", s.deleteProxyDiscoverySource)
	mux.HandleFunc("GET /api/proxy-discovery/results", s.listProxyDiscoveryResults)
	mux.HandleFunc("POST /api/proxy-discovery/sources/{id}/run", s.runProxyDiscovery)
	mux.HandleFunc("POST /api/proxy-discovery/results/{id}/verify", s.verifyProxyDiscoveryResult)
	mux.HandleFunc("POST /api/proxy-discovery/results/{id}/approve", s.approveProxyDiscoveryResult)
	mux.HandleFunc("POST /api/proxy-discovery/results/{id}/ignore", s.ignoreProxyDiscoveryResult)
	mux.HandleFunc("POST /api/proxy-discovery/results/batch", s.batchProxyDiscoveryResults)
	mux.HandleFunc("DELETE /api/proxy-discovery/results/{id}", s.deleteProxyDiscoveryResult)
}

func (s *Server) listProxyDiscoverySources(w http.ResponseWriter, r *http.Request) {
	if !s.proxyDiscoveryReady(w) {
		return
	}
	items, err := s.m.pg.ListProxyDiscoverySources()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "读取发现来源失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": items})
}

func (s *Server) saveProxyDiscoverySource(w http.ResponseWriter, r *http.Request) {
	if !s.proxyDiscoveryReady(w) {
		return
	}
	s.proxyDiscovery.mu.Lock()
	defer s.proxyDiscovery.mu.Unlock()
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var req proxyDiscoverySaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "发现来源请求无效")
		return
	}
	req.Provider = strings.ToLower(strings.TrimSpace(req.Provider))
	req.Name = strings.TrimSpace(req.Name)
	req.Query = strings.TrimSpace(req.Query)
	req.AuthorizedScope = normalizeDiscoveryScope(req.AuthorizedScope)
	req.VerifyURL = strings.TrimSpace(req.VerifyURL)
	if req.FetchLimit == 0 {
		req.FetchLimit = defaultProxyDiscoveryFetchLimit
	}
	if (req.Provider != "fofa" && req.Provider != "quake") || req.Name == "" || len([]rune(req.Name)) > 80 ||
		req.Query == "" {
		writeErr(w, http.StatusBadRequest, "来源类型、名称和查询语句不能为空")
		return
	}
	if req.FetchLimit < minProxyDiscoveryFetchLimit || req.FetchLimit > maxProxyDiscoveryFetchLimit {
		writeErr(w, http.StatusBadRequest, "每次抓取数量必须在 1 到 100 之间")
		return
	}
	if req.AuthorizedScope != "" {
		if err := validateDiscoveryScope(req.AuthorizedScope); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if req.VerifyURL != "" {
		if err := validateDiscoveryVerifyURL(req.VerifyURL); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	id := int64(0)
	if r.Method == http.MethodPut {
		var ok bool
		id, ok = pathInt(r, "id")
		if !ok || id <= 0 {
			writeErr(w, http.StatusBadRequest, "来源 ID 无效")
			return
		}
	}
	oldCredentials := ""
	if id > 0 {
		old, raw, err := s.m.pg.ProxyDiscoverySource(id)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "读取发现来源失败")
			return
		}
		if old == nil {
			writeErr(w, http.StatusNotFound, "发现来源不存在")
			return
		}
		oldCredentials = raw
	}
	credentials := oldCredentials
	if len(req.Credentials) > 0 {
		secret, err := json.Marshal(req.Credentials)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "发现来源凭据无效")
			return
		}
		if len(secret) > 16<<10 {
			writeErr(w, http.StatusBadRequest, "发现来源凭据过长")
			return
		}
		credentials, err = s.proxyPools.vault.Seal(proxypool.Secrets{APIValue: string(secret)})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "保存发现来源凭据失败")
			return
		}
	}
	if id == 0 && credentials == "" {
		writeErr(w, http.StatusBadRequest, "请配置发现来源凭据")
		return
	}
	id, err := s.m.pg.SaveProxyDiscoverySource(id, req.Provider, req.Name, req.Query, req.AuthorizedScope, req.VerifyURL, credentials, req.FetchLimit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "保存发现来源失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

func (s *Server) deleteProxyDiscoverySource(w http.ResponseWriter, r *http.Request) {
	if !s.proxyDiscoveryReady(w) {
		return
	}
	s.proxyDiscovery.mu.Lock()
	defer s.proxyDiscovery.mu.Unlock()
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "来源 ID 无效")
		return
	}
	if err := s.m.pg.DeleteProxyDiscoverySource(id); err != nil {
		writeErr(w, http.StatusNotFound, "发现来源不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) listProxyDiscoveryResults(w http.ResponseWriter, r *http.Request) {
	if !s.proxyDiscoveryReady(w) {
		return
	}
	sourceID := int64(0)
	if raw := r.URL.Query().Get("source_id"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value <= 0 {
			writeErr(w, http.StatusBadRequest, "来源 ID 无效")
			return
		}
		sourceID = value
	}
	page, pageSize := 1, 20
	for key, dest := range map[string]*int{"page": &page, "page_size": &pageSize} {
		if raw := r.URL.Query().Get(key); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 || (key == "page_size" && value > 100) || (key == "page" && value > 1000000) {
				writeErr(w, http.StatusBadRequest, "分页参数无效，每页最多 100 条")
				return
			}
			*dest = value
		}
	}
	summary, err := s.m.pg.ProxyDiscoveryResultSummary(sourceID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "读取发现统计失败")
		return
	}
	state := r.URL.Query().Get("state")
	switch state {
	case "", "pending", "verifying", "verified", "invalid", "approved", "ignored":
	default:
		writeErr(w, http.StatusBadRequest, "候选状态无效")
		return
	}
	total, err := s.m.pg.ProxyDiscoveryResultCount(sourceID, state)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "读取发现数量失败")
		return
	}
	page = min(page, max(1, (total+pageSize-1)/pageSize))
	items, err := s.m.pg.FilteredProxyDiscoveryResultsPage(sourceID, state, pageSize, (page-1)*pageSize)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "读取发现结果失败")
		return
	}
	sources, err := s.m.pg.ListProxyDiscoverySources()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "读取发现来源失败")
		return
	}
	for _, source := range sources {
		for i := range items {
			if items[i].SourceID == source.ID {
				authorizeDiscoveryResults(&source, items[i:i+1])
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": items, "total": total,
		"page": page, "page_size": pageSize, "summary": summary})
}

func (s *Server) runProxyDiscovery(w http.ResponseWriter, r *http.Request) {
	if !s.proxyDiscoveryReady(w) {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "来源 ID 无效")
		return
	}
	source, raw, err := s.m.pg.ProxyDiscoverySource(id)
	if err != nil || source == nil {
		writeErr(w, http.StatusNotFound, "发现来源不存在")
		return
	}
	if !source.Enabled {
		writeErr(w, http.StatusConflict, "发现来源已停用")
		return
	}
	credentials, err := s.proxyPools.vault.Open(raw)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "发现来源凭据不可用")
		return
	}
	var items []db.ProxyDiscoveryResult
	switch source.Provider {
	case "fofa":
		items, err = s.fetchFOFA(r.Context(), source, credentials)
	case "quake":
		items, err = s.fetchQuake(r.Context(), source, credentials)
	default:
		err = errors.New("不支持的发现来源")
	}
	if err != nil {
		_ = s.m.pg.MarkProxyDiscoveryRun(id, err.Error())
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	s.proxyDiscovery.mu.Lock()
	defer s.proxyDiscovery.mu.Unlock()
	// Configuration may have changed while the provider request was running.
	source, _, err = s.m.pg.ProxyDiscoverySource(id)
	if err != nil || source == nil {
		writeErr(w, http.StatusNotFound, "发现来源不存在")
		return
	}
	if err := s.m.pg.UpsertProxyDiscoveryResults(id, authorizeDiscoveryResults(source, items)); err != nil {
		writeErr(w, http.StatusInternalServerError, "保存发现结果失败")
		return
	}
	_ = s.m.pg.MarkProxyDiscoveryRun(id, "")
	writeJSON(w, http.StatusOK, map[string]int{"fetched": len(items)})
}

func (s *Server) fetchFOFA(ctx context.Context, source *db.ProxyDiscoverySource, secrets proxypool.Secrets) ([]db.ProxyDiscoveryResult, error) {
	var raw map[string]string
	if err := json.Unmarshal([]byte(secrets.APIValue), &raw); err != nil || raw["email"] == "" || raw["key"] == "" {
		return nil, errors.New("FOFA 需要 email 和 key")
	}
	q := base64.RawStdEncoding.EncodeToString([]byte(source.Query))
	u := fofaDiscoveryURL + "?email=" + url.QueryEscape(raw["email"]) +
		"&key=" + url.QueryEscape(raw["key"]) + "&qbase64=" + url.QueryEscape(q) +
		"&size=" + strconv.Itoa(discoveryFetchLimit(source)) + "&fields=host,ip,port,protocol,city,region,asn,org"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, errors.New("FOFA 请求无效")
	}
	resp, err := s.proxyDiscovery.client.Do(req)
	if err != nil {
		return nil, errors.New("FOFA 请求失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("FOFA 返回 HTTP %d", resp.StatusCode)
	}
	var body struct {
		Error   bool            `json:"error"`
		Message string          `json:"errmsg"`
		Results [][]interface{} `json:"results"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&body); err != nil {
		return nil, errors.New("FOFA 响应无效")
	}
	if body.Error {
		return nil, errors.New("FOFA 查询失败: " + strings.TrimSpace(body.Message))
	}
	out := make([]db.ProxyDiscoveryResult, 0, len(body.Results))
	for _, row := range body.Results {
		if len(row) < 3 {
			continue
		}
		host := fmt.Sprint(row[0])
		ip := fmt.Sprint(row[1])
		port, _ := strconv.Atoi(fmt.Sprint(row[2]))
		if net.ParseIP(ip) == nil || port < 1 || port > 65535 {
			continue
		}
		protocol := "http"
		if len(row) > 3 && strings.Contains(strings.ToLower(fmt.Sprint(row[3])), "socks") {
			protocol = "socks5"
		}
		out = append(out, makeDiscoveryResult(source, protocol, ip, port, map[string]any{"host": host, "raw": row}))
	}
	return authorizeDiscoveryResults(source, out), nil
}

func (s *Server) fetchQuake(ctx context.Context, source *db.ProxyDiscoverySource, secrets proxypool.Secrets) ([]db.ProxyDiscoveryResult, error) {
	var raw map[string]string
	if err := json.Unmarshal([]byte(secrets.APIValue), &raw); err != nil || raw["key"] == "" {
		return nil, errors.New("Quake 需要 key")
	}
	body, _ := json.Marshal(map[string]any{"query": source.Query, "size": discoveryFetchLimit(source)})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, quakeDiscoveryURL, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("Quake 请求无效")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-QuakeToken", raw["key"])
	resp, err := s.proxyDiscovery.client.Do(req)
	if err != nil {
		return nil, errors.New("Quake 请求失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Quake 返回 HTTP %d", resp.StatusCode)
	}
	var payload map[string]any
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&payload); err != nil {
		return nil, errors.New("Quake 响应无效")
	}
	out := []db.ProxyDiscoveryResult{}
	walkDiscoveryJSON(payload, func(host string, port int, meta map[string]any) {
		if net.ParseIP(host) != nil && port > 0 && port <= 65535 {
			out = append(out, makeDiscoveryResult(source, quakeDiscoveryProtocol(meta), host, port, meta))
		}
	})
	return authorizeDiscoveryResults(source, out), nil
}

func quakeDiscoveryProtocol(metadata map[string]any) string {
	service, _ := metadata["service"].(map[string]any)
	name, _ := service["name"].(string)
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "socks5", "socks5h":
		return "socks5"
	default:
		return "http"
	}
}

func discoveryFetchLimit(source *db.ProxyDiscoverySource) int {
	if source == nil || source.FetchLimit < minProxyDiscoveryFetchLimit || source.FetchLimit > maxProxyDiscoveryFetchLimit {
		return defaultProxyDiscoveryFetchLimit
	}
	return source.FetchLimit
}

func restrictedDiscoveryRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 3 {
		return errors.New("发现来源重定向次数过多")
	}
	if req.URL.Scheme != "https" || !isAllowedDiscoveryHost(req.URL.Hostname()) {
		return errors.New("发现来源重定向目标不受支持")
	}
	return nil
}

func isAllowedDiscoveryHost(host string) bool {
	switch strings.ToLower(strings.TrimSuffix(host, ".")) {
	case "fofa.info", "quake.360.cn", "quake.360.net":
		return true
	default:
		return false
	}
}

func makeDiscoveryResult(source *db.ProxyDiscoverySource, protocol, host string, port int, metadata map[string]any) db.ProxyDiscoveryResult {
	return db.ProxyDiscoveryResult{
		SourceID: source.ID, Address: protocol + "://" + net.JoinHostPort(host, strconv.Itoa(port)),
		Protocol: protocol, Host: host, Port: port, Metadata: metadata,
	}
}

func walkDiscoveryJSON(value any, emit func(string, int, map[string]any)) {
	switch item := value.(type) {
	case []any:
		for _, child := range item {
			walkDiscoveryJSON(child, emit)
		}
	case map[string]any:
		host := ""
		for _, key := range []string{"ip", "host", "address", "ip_address"} {
			if raw, ok := item[key].(string); ok && net.ParseIP(raw) != nil {
				host = raw
				break
			}
		}
		port := 0
		for _, key := range []string{"port", "service_port"} {
			switch raw := item[key].(type) {
			case float64:
				port = int(raw)
			case string:
				port, _ = strconv.Atoi(raw)
			}
		}
		if host != "" && port > 0 {
			emit(host, port, item)
		}
		for _, child := range item {
			walkDiscoveryJSON(child, emit)
		}
	}
}

func authorizeDiscoveryResults(source *db.ProxyDiscoverySource, items []db.ProxyDiscoveryResult) []db.ProxyDiscoveryResult {
	for i := range items {
		items[i].Authorized = hostInScope(items[i].Host, source.AuthorizedScope)
		switch {
		case strings.TrimSpace(source.AuthorizedScope) == "":
			items[i].AuthorizationStatus = "unconfigured"
		case items[i].Authorized:
			items[i].AuthorizationStatus = "allowed"
		default:
			items[i].AuthorizationStatus = "out_of_scope"
		}
		if items[i].State == "" || (items[i].State == "invalid" && items[i].LastError == "不在授权范围内") {
			items[i].State = "pending"
			items[i].LastError = ""
		}
	}
	return items
}

func validateDiscoveryScope(scope string) error {
	for _, token := range splitScope(scope) {
		if net.ParseIP(token) != nil {
			continue
		}
		if _, _, err := net.ParseCIDR(token); err == nil {
			continue
		}
		if strings.ContainsAny(token, " /\\") || !strings.Contains(token, ".") {
			return errors.New("授权范围必须是 IP、CIDR 或域名")
		}
	}
	return nil
}

func validateDiscoveryVerifyURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return errors.New("验证 URL 必须是无凭据的 HTTP(S) 地址")
	}
	return nil
}

func splitScope(scope string) []string {
	fields := strings.FieldsFunc(scope, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' || r == ';' })
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if value := strings.TrimSpace(strings.ToLower(field)); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func normalizeDiscoveryScope(scope string) string {
	tokens := splitScope(scope)
	for i, token := range tokens {
		if token == "0.0.0.0" {
			tokens[i] = "0.0.0.0/0"
		}
	}
	return strings.Join(tokens, ",")
}

func hostInScope(host, scope string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	ip := net.ParseIP(host)
	for _, token := range splitScope(normalizeDiscoveryScope(scope)) {
		if ip != nil {
			if exact := net.ParseIP(token); exact != nil && exact.Equal(ip) {
				return true
			}
			if _, network, err := net.ParseCIDR(token); err == nil && network.Contains(ip) {
				return true
			}
		}
		if ip == nil && (host == token || strings.HasSuffix(host, "."+token)) {
			return true
		}
	}
	return false
}

func (s *Server) verifyProxyDiscoveryResult(w http.ResponseWriter, r *http.Request) {
	if !s.proxyDiscoveryReady(w) {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "结果 ID 无效")
		return
	}
	out, status, err := s.verifyDiscoveryCandidate(r.Context(), id)
	if err != nil {
		writeErr(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) verifyDiscoveryCandidate(ctx context.Context, id int64) (map[string]any, int, error) {
	s.proxyDiscovery.mu.Lock()
	defer s.proxyDiscovery.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, http.StatusRequestTimeout, errors.New("验证已取消")
	}
	result, err := s.m.pg.ProxyDiscoveryResult(id)
	if err != nil {
		return nil, http.StatusInternalServerError, errors.New("读取发现结果失败")
	}
	if result == nil {
		return nil, http.StatusNotFound, errors.New("发现结果不存在")
	}
	source, raw, err := s.m.pg.ProxyDiscoverySource(result.SourceID)
	if err != nil || source == nil {
		return nil, http.StatusNotFound, errors.New("发现来源不存在")
	}
	if !source.Enabled || result.State == "approved" || result.State == "ignored" {
		return nil, http.StatusConflict, errors.New("该结果或来源当前不可验证")
	}
	if strings.TrimSpace(source.AuthorizedScope) == "" {
		return nil, http.StatusBadRequest, errors.New("请先编辑查询，填写有权使用的代理 IP、网段或域名")
	}
	if !hostInScope(result.Host, source.AuthorizedScope) {
		return nil, http.StatusForbidden, errors.New("结果不在授权范围内")
	}
	if source.VerifyURL == "" {
		return nil, http.StatusBadRequest, errors.New("请先编辑查询，配置代理验证地址")
	}
	credentials, err := s.proxyPools.vault.Open(raw)
	if err != nil {
		return nil, http.StatusServiceUnavailable, errors.New("发现来源凭据不可用")
	}
	var sourceSecrets map[string]string
	if err := json.Unmarshal([]byte(credentials.APIValue), &sourceSecrets); err != nil {
		return nil, http.StatusServiceUnavailable, errors.New("发现来源凭据格式无效")
	}
	started := time.Now()
	upstream, err := proxypool.Upstream(proxypool.Node{Address: result.Address}, proxypool.Secrets{
		ProxyUsername: sourceSecrets["proxy_username"], ProxyPassword: sourceSecrets["proxy_password"],
	})
	var ip string
	if err == nil {
		ip, err = proxypool.Verify(ctx, upstream, source.VerifyURL)
	}
	latency := int(time.Since(started).Milliseconds())
	if err != nil {
		if ctx.Err() != nil {
			return nil, http.StatusRequestTimeout, errors.New("验证已取消，请重试")
		}
		if saveErr := s.m.pg.UpdateProxyDiscoveryResult(id, "invalid", true, "", &latency, err.Error()); saveErr != nil {
			return nil, http.StatusInternalServerError, errors.New("保存验证失败状态失败")
		}
		return nil, http.StatusBadGateway, err
	}
	if err := s.m.pg.UpdateProxyDiscoveryResult(id, "verified", true, ip, &latency, ""); err != nil {
		return nil, http.StatusInternalServerError, errors.New("保存验证结果失败")
	}
	return map[string]any{"ok": true, "ip": ip, "latency_ms": latency}, http.StatusOK, nil
}

func (s *Server) approveProxyDiscoveryResult(w http.ResponseWriter, r *http.Request) {
	if !s.proxyDiscoveryReady(w) {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "结果 ID 无效")
		return
	}
	var req struct {
		PoolID int64  `json:"pool_id"`
		Target string `json:"target"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil ||
		req.PoolID <= 0 || (req.Target != "candidate" && req.Target != "production") {
		writeErr(w, http.StatusBadRequest, "请指定代理池和加入目标")
		return
	}
	nodeID, status, err := s.approveDiscoveryCandidate(r.Context(), id, req.PoolID, req.Target)
	if err != nil {
		writeErr(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"node_id": nodeID})
}

func (s *Server) approveDiscoveryCandidate(ctx context.Context, id, poolID int64, target string) (int64, int, error) {
	s.proxyDiscovery.mu.Lock()
	defer s.proxyDiscovery.mu.Unlock()
	s.proxyPools.mu.Lock()
	defer s.proxyPools.mu.Unlock()
	if ctx.Err() != nil {
		return 0, http.StatusRequestTimeout, errors.New("入池已取消")
	}
	result, err := s.m.pg.ProxyDiscoveryResult(id)
	if err != nil {
		return 0, http.StatusInternalServerError, errors.New("读取发现结果失败")
	}
	if result == nil {
		return 0, http.StatusNotFound, errors.New("发现结果不存在")
	}
	source, raw, sourceErr := s.m.pg.ProxyDiscoverySource(result.SourceID)
	if sourceErr != nil || source == nil {
		return 0, http.StatusNotFound, errors.New("发现来源不存在")
	}
	if !source.Enabled || !hostInScope(result.Host, source.AuthorizedScope) {
		return 0, http.StatusForbidden, errors.New("来源已停用或结果不在当前授权范围内")
	}
	if result.State == "approved" {
		return 0, http.StatusConflict, errors.New("该候选已入池，请在代理 IP 中管理")
	}
	if result.State != "verified" || net.ParseIP(result.VerifiedIP) == nil || result.VerifiedAt == nil {
		return 0, http.StatusConflict, errors.New("该候选尚未验证通过，请先执行批量验证，再选择验证通过的候选入池")
	}
	pool, err := s.m.pg.ProxyPool(poolID)
	if err != nil {
		return 0, http.StatusInternalServerError, errors.New("读取目标代理池失败")
	}
	if pool == nil {
		return 0, http.StatusNotFound, errors.New("目标代理池不存在，请刷新后重新选择")
	}
	poolSecrets, err := s.proxyPools.vault.Open(pool.Credentials)
	if err != nil {
		return 0, http.StatusServiceUnavailable, errors.New("目标代理池凭据不可用")
	}
	sourceCredentials, err := s.proxyPools.vault.Open(raw)
	if err != nil {
		return 0, http.StatusServiceUnavailable, errors.New("发现来源凭据不可用")
	}
	var sourceSecrets map[string]string
	if err := json.Unmarshal([]byte(sourceCredentials.APIValue), &sourceSecrets); err != nil {
		return 0, http.StatusServiceUnavailable, errors.New("发现来源凭据格式无效")
	}
	var proof *db.ProxyDiscoveryAdmissionProof
	upstream, err := pool.Config.Upstream(proxypool.Node{Address: result.Address}, poolSecrets)
	if err != nil {
		return 0, http.StatusBadGateway, errors.New("目标代理池连接配置无效")
	}
	var poolUser, poolPassword string
	if upstream.User != nil {
		poolUser = upstream.User.Username()
		poolPassword, _ = upstream.User.Password()
	}
	// The pool's credentials are used at runtime. Evidence collected using
	// different credentials or a different target cannot prove that route works.
	if poolUser != sourceSecrets["proxy_username"] ||
		poolPassword != sourceSecrets["proxy_password"] ||
		(pool.Config.VerifyURL != "" && pool.Config.VerifyURL != source.VerifyURL) {
		endpoint := pool.Config.VerifyURL
		if endpoint == "" {
			endpoint = source.VerifyURL
		}
		started := time.Now()
		ip, verifyErr := proxypool.Verify(ctx, upstream, endpoint)
		if verifyErr != nil {
			return 0, http.StatusBadGateway, fmt.Errorf("目标代理池配置复验失败：%w", verifyErr)
		}
		proof = &db.ProxyDiscoveryAdmissionProof{IP: ip, CheckedAt: time.Now().UTC(),
			LatencyMS: int(time.Since(started).Milliseconds())}
	}
	if ctx.Err() != nil {
		return 0, http.StatusRequestTimeout, errors.New("入池已取消")
	}
	nodeID, err := s.m.pg.PromoteProxyDiscoveryResult(id, poolID, source.Provider, target == "production", proof)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrProxyDiscoveryNotFound):
			return 0, http.StatusNotFound, errors.New("发现结果已删除，请刷新列表")
		case errors.Is(err, db.ErrProxyPoolNotFound):
			return 0, http.StatusNotFound, errors.New("目标代理池不存在，请刷新后重新选择")
		case errors.Is(err, db.ErrProxyDiscoveryNotVerified):
			return 0, http.StatusConflict, errors.New("该候选尚未验证通过，请先执行批量验证，再选择验证通过的候选入池")
		case errors.Is(err, db.ErrProxyDiscoveryAlreadyApproved):
			return 0, http.StatusConflict, errors.New("该候选已入池，请在代理 IP 中管理")
		case errors.Is(err, db.ErrProxyDiscoveryOutOfScope):
			return 0, http.StatusForbidden, errors.New("该候选不在授权范围内，请检查来源配置并重新验证")
		default:
			log.Printf("proxy discovery admission failed: result_id=%d pool_id=%d: %v", id, poolID, err)
			return 0, http.StatusInternalServerError, errors.New("入池失败，请稍后重试或查看服务日志")
		}
	}
	return nodeID, http.StatusOK, nil
}

func (s *Server) ignoreProxyDiscoveryResult(w http.ResponseWriter, r *http.Request) {
	if !s.proxyDiscoveryReady(w) {
		return
	}
	s.proxyDiscovery.mu.Lock()
	defer s.proxyDiscovery.mu.Unlock()
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "结果 ID 无效")
		return
	}
	result, err := s.m.pg.ProxyDiscoveryResult(id)
	if err != nil || result == nil {
		writeErr(w, http.StatusNotFound, "发现结果不存在")
		return
	}
	if result.State == "approved" {
		writeErr(w, http.StatusConflict, "已入池的代理请在代理 IP 中管理")
		return
	}
	if err := s.m.pg.UpdateProxyDiscoveryResult(id, "ignored", false, "", nil, "人工忽略"); err != nil {
		writeErr(w, http.StatusInternalServerError, "更新发现结果失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
