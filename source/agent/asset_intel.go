package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	actool "github.com/Autumn-27/norma/tool"
	"golang.org/x/net/idna"
	"yunkun/console/db"
)

const (
	maxIntelResponseBytes = 2 << 20
	maxIntelFieldRunes    = 300
	maxIntelOutputBytes   = 24 << 10
)

type intelCredentials struct {
	label string
	key   string
	email string
}

type intelCandidate struct {
	Provider string `json:"provider"`
	Domain   string `json:"domain,omitempty"`
	IP       string `json:"ip,omitempty"`
	Port     string `json:"port,omitempty"`
	URL      string `json:"url,omitempty"`
	Protocol string `json:"protocol,omitempty"`
	Service  string `json:"service,omitempty"`
	Title    string `json:"title,omitempty"`
	Product  string `json:"product,omitempty"`
}

func (t *ToolSet) assetIntelSearch() actool.CoreTool {
	return readTool(
		"asset_intel_search",
		"在当前任务已登记的授权范围内调用 FOFA、Quake、Hunter 或 ZoomEye 查询资产。必须提供已授权的 domain 或 ip；查询结果会再次按任务范围过滤，只返回候选，不会自动写入资产、扩大范围或发起目标探测。外部 API 可能消耗用户额度。需要通过服务进程环境变量配置 API 凭据；未配置时明确报错。",
		obj(map[string]any{
			"provider": map[string]any{
				"type":        "string",
				"enum":        []string{"fofa", "quake", "hunter", "zoomeye"},
				"description": "资产测绘服务",
			},
			"domain": map[string]any{
				"type":        "string",
				"description": "必须落在当前任务已登记授权范围内的域名",
			},
			"ip": map[string]any{
				"type":        "string",
				"description": "必须落在当前任务已登记授权范围内的 IP",
			},
			"port": map[string]any{
				"type":        "integer",
				"description": "可选端口过滤",
			},
			"page": map[string]any{
				"type": "integer", "description": "页码，默认 1，最大 10",
			},
			"page_size": map[string]any{
				"type": "integer", "description": "每页数量，默认 20，最大 100",
			},
		}, "provider"),
		func(ctx context.Context, raw json.RawMessage) (actool.Result, error) {
			var input struct {
				Provider string `json:"provider"`
				Domain   string `json:"domain"`
				IP       string `json:"ip"`
				Port     int    `json:"port"`
				Page     int    `json:"page"`
				PageSize int    `json:"page_size"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return actool.Errorf("invalid input: " + err.Error()), nil
			}
			input.Provider = strings.ToLower(strings.TrimSpace(input.Provider))
			rawDomain := strings.TrimSpace(input.Domain)
			input.Domain = normalizeIntelDomain(input.Domain)
			if rawDomain != "" && input.Domain == "" {
				return actool.Errorf("domain 格式无效"), nil
			}
			input.IP = strings.TrimSpace(input.IP)
			if input.Page <= 0 {
				input.Page = 1
			}
			if input.Page > 10 {
				input.Page = 10
			}
			if input.PageSize <= 0 {
				input.PageSize = 20
			}
			if input.PageSize > 100 {
				input.PageSize = 100
			}
			if input.Port < 0 || input.Port > 65535 {
				return actool.Errorf("port 必须在 1-65535 范围内"), nil
			}
			if input.Domain == "" && input.IP == "" {
				return actool.Errorf("必须提供 domain 或 ip，不能执行宽泛的全网查询"), nil
			}
			if input.IP != "" && net.ParseIP(input.IP) == nil {
				return actool.Errorf("ip 必须是有效 IPv4 或 IPv6 地址"), nil
			}
			taskID := t.taskID
			if taskID <= 0 {
				taskID = RunInfoFrom(ctx).TaskID
			}
			if taskID <= 0 || t.as == nil {
				return actool.Errorf("资产检索需要任务上下文和已初始化的授权范围"), nil
			}
			scopes, err := t.as.ListTaskScope(taskID)
			if err != nil {
				return actool.Errorf("读取任务授权范围失败"), nil
			}
			if input.Domain != "" && !intelDomainInScope(input.Domain, scopes) {
				return actool.Errorf("domain 不在当前任务已登记的授权范围内；请先由授权负责人确认并登记范围"), nil
			}
			if input.IP != "" && !intelIPInScope(input.IP, scopes) {
				return actool.Errorf("ip 不在当前任务已登记的授权范围内；请先由授权负责人确认并登记范围"), nil
			}
			candidates, total, err := searchAssetIntel(ctx, assetIntelDeadline(), assetIntelInput{
				Provider: input.Provider, Domain: input.Domain, IP: input.IP,
				Port: input.Port, Page: input.Page, PageSize: input.PageSize,
			})
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			allowed := make([]intelCandidate, 0, len(candidates))
			for _, candidate := range candidates {
				if intelCandidateInScope(candidate, scopes) {
					allowed = append(allowed, candidate)
				}
			}
			return jsonResult(map[string]any{
				"provider": input.Provider, "page": input.Page, "page_size": input.PageSize,
				"provider_total": total, "returned": len(allowed), "filtered_out_of_scope": len(candidates) - len(allowed),
				"candidates": allowed,
				"note":       "候选来自外部测绘结果，不等于目标授权或存活证明；未自动写入资产或任务范围。",
			})
		},
	)
}

func normalizeIntelDomain(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimSuffix(value, ".")
	if value == "" || strings.ContainsAny(value, `/\:*?"<>|@ `) {
		return ""
	}
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		return ""
	}
	ascii, err := idna.Lookup.ToASCII(value)
	if err != nil || len(ascii) > 253 {
		return ""
	}
	for _, label := range strings.Split(ascii, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return ""
		}
		for _, char := range label {
			if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-') {
				return ""
			}
		}
	}
	return ascii
}

func intelDomainInScope(domain string, scopes []db.TaskScope) bool {
	domain = normalizeIntelDomain(domain)
	if domain == "" || net.ParseIP(domain) != nil {
		return false
	}
	for _, scope := range scopes {
		switch scope.Kind {
		case "root_domain":
			root := normalizeIntelDomain(scope.Domain)
			if root != "" && (domain == root || strings.HasSuffix(domain, "."+root)) {
				return true
			}
		case "subdomain":
			if domain == normalizeIntelDomain(scope.Domain) {
				return true
			}
		}
	}
	return false
}

func intelIPInScope(ip string, scopes []db.TaskScope) bool {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return false
	}
	for _, scope := range scopes {
		if scope.Kind != "ip" && scope.Kind != "cidr" {
			continue
		}
		_, network, err := net.ParseCIDR(scope.Net)
		if err == nil && network.Contains(parsed) {
			return true
		}
	}
	return false
}

func intelCandidateInScope(candidate intelCandidate, scopes []db.TaskScope) bool {
	if candidate.Domain != "" {
		domain := candidate.Domain
		if parsed, err := url.Parse(domain); err == nil && parsed.Hostname() != "" {
			domain = parsed.Hostname()
		} else if host, _, err := net.SplitHostPort(domain); err == nil {
			domain = host
		}
		if net.ParseIP(domain) != nil {
			if intelIPInScope(domain, scopes) {
				return true
			}
		} else if intelDomainInScope(domain, scopes) {
			return true
		}
	}
	if candidate.IP != "" && intelIPInScope(candidate.IP, scopes) {
		return true
	}
	if candidate.URL != "" {
		if parsed, err := url.Parse(candidate.URL); err == nil {
			if ip := net.ParseIP(parsed.Hostname()); ip != nil {
				return intelIPInScope(ip.String(), scopes)
			}
			return intelDomainInScope(parsed.Hostname(), scopes)
		}
	}
	return false
}

func intelQuery(domain, ip string, port int) (string, error) {
	parts := []string{}
	if domain != "" {
		if normalizeIntelDomain(domain) != domain {
			return "", fmt.Errorf("domain 格式无效")
		}
		parts = append(parts, fmt.Sprintf(`domain="%s"`, domain))
	}
	if ip != "" {
		if net.ParseIP(ip) == nil {
			return "", fmt.Errorf("ip 格式无效")
		}
		parts = append(parts, fmt.Sprintf(`ip="%s"`, ip))
	}
	if port > 0 {
		parts = append(parts, fmt.Sprintf(`port="%d"`, port))
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("查询条件不能为空")
	}
	return strings.Join(parts, " AND "), nil
}

func providerIntelQuery(provider, domain, ip string, port int) (string, error) {
	if provider != "fofa" && provider != "quake" && provider != "hunter" && provider != "zoomeye" {
		return "", fmt.Errorf("provider 仅支持 fofa、quake、hunter、zoomeye")
	}
	if _, err := intelQuery(domain, ip, port); err != nil {
		return "", err
	}
	parts := []string{}
	switch provider {
	case "fofa":
		if domain != "" {
			parts = append(parts, fmt.Sprintf(`domain="%s"`, domain))
		}
		if ip != "" {
			parts = append(parts, fmt.Sprintf(`ip="%s"`, ip))
		}
		if port > 0 {
			parts = append(parts, fmt.Sprintf(`port="%d"`, port))
		}
		return strings.Join(parts, "&&"), nil
	case "quake":
		if domain != "" {
			parts = append(parts, fmt.Sprintf(`domain:"%s"`, domain))
		}
		if ip != "" {
			parts = append(parts, fmt.Sprintf(`ip:"%s"`, ip))
		}
		if port > 0 {
			parts = append(parts, fmt.Sprintf(`port:"%d"`, port))
		}
		return strings.Join(parts, " AND "), nil
	case "hunter":
		if domain != "" {
			parts = append(parts, fmt.Sprintf(`domain.suffix="%s"`, domain))
		}
		if ip != "" {
			parts = append(parts, fmt.Sprintf(`ip="%s"`, ip))
		}
		if port > 0 {
			parts = append(parts, fmt.Sprintf(`port="%d"`, port))
		}
		return strings.Join(parts, " && "), nil
	case "zoomeye":
		if domain != "" {
			parts = append(parts, fmt.Sprintf(`domain="%s"`, domain))
		}
		if ip != "" {
			parts = append(parts, fmt.Sprintf(`ip="%s"`, ip))
		}
		if port > 0 {
			parts = append(parts, fmt.Sprintf(`port="%d"`, port))
		}
		return strings.Join(parts, " AND "), nil
	}
	return "", fmt.Errorf("provider 查询语法不可用")
}

func intelAccounts(provider string) []intelCredentials {
	get := func(label, email string, keys ...string) intelCredentials {
		credential := intelCredentials{label: label}
		if email != "" {
			credential.email = strings.TrimSpace(os.Getenv(email))
		}
		for _, key := range keys {
			if credential.key = strings.TrimSpace(os.Getenv(key)); credential.key != "" {
				break
			}
		}
		return credential
	}
	switch provider {
	case "fofa":
		return nonEmptyAccounts(
			get("primary", "FOFA_EMAIL", "FOFA_KEY", "FOFA_API_KEY"),
			get("backup", "FOFA_EMAIL_BACKUP", "FOFA_KEY_BACKUP", "FOFA_API_KEY_BACKUP"),
			get("backup2", "FOFA_EMAIL_BACKUP2", "FOFA_KEY_BACKUP2", "FOFA_API_KEY_BACKUP2"),
		)
	case "quake":
		return nonEmptyAccounts(
			get("primary", "", "QUAKE_API_KEY"),
			get("backup", "", "QUAKE_API_KEY_BACKUP"),
		)
	case "hunter":
		return nonEmptyAccounts(
			get("primary", "", "HUNTER_API_KEY"),
			get("backup", "", "HUNTER_API_KEY_BACKUP"),
		)
	case "zoomeye":
		return nonEmptyAccounts(
			get("primary", "", "ZOOMEYE_API_KEY"),
			get("backup", "", "ZOOMEYE_API_KEY_BACKUP"),
		)
	default:
		return nil
	}
}

func nonEmptyAccounts(accounts ...intelCredentials) []intelCredentials {
	out := make([]intelCredentials, 0, len(accounts))
	for _, account := range accounts {
		if account.key != "" {
			out = append(out, account)
		}
	}
	return out
}

type assetIntelInput struct {
	Provider string `json:"provider"`
	Domain   string `json:"domain"`
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

func searchAssetIntel(ctx context.Context, client *http.Client, input assetIntelInput) ([]intelCandidate, any, error) {
	if input.Provider != "fofa" && input.Provider != "quake" && input.Provider != "hunter" && input.Provider != "zoomeye" {
		return nil, nil, fmt.Errorf("provider 仅支持 fofa、quake、hunter、zoomeye")
	}
	if client == nil {
		client = assetIntelDeadline()
	}
	query, err := providerIntelQuery(input.Provider, input.Domain, input.IP, input.Port)
	if err != nil {
		return nil, nil, err
	}
	credentials := intelAccounts(input.Provider)
	if len(credentials) == 0 {
		return nil, nil, fmt.Errorf("%s API 凭据未配置", strings.ToUpper(input.Provider))
	}
	var lastErr error
	for _, credential := range credentials {
		rows, total, retry, err := searchIntelAccount(ctx, client, input, query, credential)
		if err == nil {
			return rows, total, nil
		}
		lastErr = err
		if !retry {
			break
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("外部服务请求失败")
	}
	return nil, nil, lastErr
}

func searchIntelAccount(ctx context.Context, client *http.Client, input assetIntelInput, query string, credential intelCredentials) ([]intelCandidate, any, bool, error) {
	switch input.Provider {
	case "fofa":
		return searchFOFA(ctx, client, query, input.Page, input.PageSize, credential)
	case "quake":
		return searchQuake(ctx, client, query, input.Page, input.PageSize, credential)
	case "hunter":
		return searchHunter(ctx, client, query, input.Page, input.PageSize, credential)
	case "zoomeye":
		return searchZoomEye(ctx, client, query, input.Page, input.PageSize, credential)
	}
	return nil, nil, false, fmt.Errorf("provider 不支持")
}

func intelHTTP(ctx context.Context, client *http.Client, method, endpoint string, headers map[string]string, body io.Reader) (map[string]any, int, error) {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, 0, fmt.Errorf("构造外部服务请求失败")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, 0, fmt.Errorf("外部服务连接失败")
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxIntelResponseBytes+1))
	if err != nil || len(payload) > maxIntelResponseBytes {
		return nil, response.StatusCode, fmt.Errorf("外部服务响应读取失败或超出大小限制")
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, response.StatusCode, fmt.Errorf("外部服务返回非 JSON 响应（HTTP %d）", response.StatusCode)
	}
	return decoded, response.StatusCode, nil
}

func searchFOFA(ctx context.Context, client *http.Client, query string, page, size int, credential intelCredentials) ([]intelCandidate, any, bool, error) {
	params := url.Values{
		"key":     {credential.key},
		"qbase64": {base64.StdEncoding.EncodeToString([]byte(query))},
		"page":    {strconv.Itoa(page)},
		"size":    {strconv.Itoa(size)},
		"fields":  {"host,ip,port,protocol,title"},
	}
	if credential.email != "" {
		params.Set("email", credential.email)
	}
	data, status, err := intelHTTP(ctx, client, http.MethodGet, "https://fofa.info/api/v1/search/all?"+params.Encode(),
		map[string]string{"Accept": "application/json", "User-Agent": "Yunkun-Console/asset-intel"}, nil)
	if err != nil {
		return nil, nil, true, err
	}
	if status == http.StatusTooManyRequests || status == http.StatusForbidden {
		return nil, nil, true, fmt.Errorf("FOFA 限流或权限不足")
	}
	if status < 200 || status >= 300 || data["error"] == true {
		return nil, nil, true, fmt.Errorf("FOFA 查询失败，HTTP %d", status)
	}
	raw, _ := data["results"].([]any)
	out := make([]intelCandidate, 0, len(raw))
	for _, item := range raw {
		row, ok := item.([]any)
		if !ok || len(row) == 0 {
			continue
		}
		candidate := intelCandidate{Provider: "fofa"}
		if len(row) > 0 {
			candidate.Domain = intelString(row[0])
		}
		if len(row) > 1 {
			candidate.IP = intelString(row[1])
		}
		if len(row) > 2 {
			candidate.Port = intelString(row[2])
		}
		if len(row) > 3 {
			candidate.Protocol = intelString(row[3])
		}
		if len(row) > 4 {
			candidate.Title = intelString(row[4])
		}
		out = append(out, candidate)
	}
	return out, data["size"], false, nil
}

func searchQuake(ctx context.Context, client *http.Client, query string, page, size int, credential intelCredentials) ([]intelCandidate, any, bool, error) {
	payload, _ := json.Marshal(map[string]any{
		"query": query, "start": (page - 1) * size, "size": size, "ignore_cache": false,
	})
	data, status, err := intelHTTP(ctx, client, http.MethodPost, "https://quake.360.net/api/v3/search/quake_service",
		map[string]string{"X-QuakeToken": credential.key, "Content-Type": "application/json", "User-Agent": "Yunkun-Console/asset-intel"},
		bytes.NewReader(payload))
	if err != nil {
		return nil, nil, true, err
	}
	if status == http.StatusTooManyRequests || status == http.StatusForbidden {
		return nil, nil, true, fmt.Errorf("Quake 限流或权限不足")
	}
	if status < 200 || status >= 300 || fmt.Sprint(data["code"]) != "0" {
		return nil, nil, true, fmt.Errorf("Quake 查询失败，HTTP %d", status)
	}
	raw, _ := data["data"].([]any)
	out := make([]intelCandidate, 0, len(raw))
	for _, item := range raw {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		candidate := intelCandidate{Provider: "quake", IP: intelString(row["ip"]), Port: intelString(row["port"]), Domain: firstIntelString(row["hostname"])}
		if service, ok := row["service"].(map[string]any); ok {
			candidate.Service = intelString(service["name"])
			if httpData, ok := service["http"].(map[string]any); ok {
				candidate.Title = intelString(httpData["title"])
			}
		}
		out = append(out, candidate)
	}
	return out, data["meta"], false, nil
}

func searchHunter(ctx context.Context, client *http.Client, query string, page, size int, credential intelCredentials) ([]intelCandidate, any, bool, error) {
	params := url.Values{
		"api-key":   {credential.key},
		"search":    {base64.RawURLEncoding.EncodeToString([]byte(query))},
		"page":      {strconv.Itoa(page)},
		"page_size": {strconv.Itoa(size)},
		"is_web":    {"3"},
	}
	data, status, err := intelHTTP(ctx, client, http.MethodGet, "https://hunter.qianxin.com/openApi/search?"+params.Encode(),
		map[string]string{"User-Agent": "Yunkun-Console/asset-intel"}, nil)
	if err != nil {
		return nil, nil, true, err
	}
	if status == http.StatusTooManyRequests || status == http.StatusForbidden {
		return nil, nil, true, fmt.Errorf("Hunter 限流或权限不足")
	}
	if status < 200 || status >= 300 || fmt.Sprint(data["code"]) != "200" {
		return nil, nil, true, fmt.Errorf("Hunter 查询失败，HTTP %d", status)
	}
	result, _ := data["data"].(map[string]any)
	raw, _ := result["arr"].([]any)
	out := make([]intelCandidate, 0, len(raw))
	for _, item := range raw {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, intelCandidate{
			Provider: "hunter", Domain: intelString(row["domain"]), IP: intelString(row["ip"]),
			Port: intelString(row["port"]), URL: intelString(row["url"]),
			Protocol: intelString(row["protocol"]), Title: intelString(row["title"]),
		})
	}
	return out, result["total"], false, nil
}

func searchZoomEye(ctx context.Context, client *http.Client, query string, page, size int, credential intelCredentials) ([]intelCandidate, any, bool, error) {
	body, _ := json.Marshal(map[string]any{
		"qbase64": base64.StdEncoding.EncodeToString([]byte(query)), "page": page, "pagesize": size,
		"sub_type": "web", "fields": "ip,port,domain,hostname,service,title,product",
	})
	data, status, err := intelHTTP(ctx, client, http.MethodPost, "https://api.zoomeye.ai/v2/search",
		map[string]string{"API-KEY": credential.key, "Content-Type": "application/json", "User-Agent": "Yunkun-Console/asset-intel"},
		bytes.NewReader(body))
	if err != nil {
		return nil, nil, true, err
	}
	if status == http.StatusTooManyRequests || status == http.StatusForbidden {
		return nil, nil, true, fmt.Errorf("ZoomEye 限流或权限不足")
	}
	if status < 200 || status >= 300 || fmt.Sprint(data["code"]) != "60000" {
		return nil, nil, true, fmt.Errorf("ZoomEye 查询失败，HTTP %d", status)
	}
	raw, _ := data["data"].([]any)
	out := make([]intelCandidate, 0, len(raw))
	for _, item := range raw {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, intelCandidate{
			Provider: "zoomeye", Domain: firstIntelString(row["domain"], row["hostname"]),
			IP: intelString(row["ip"]), Port: intelString(row["port"]),
			Service: intelString(row["service"]), Title: firstIntelString(row["title"]),
			Product: intelString(row["product"]),
		})
	}
	return out, data["total"], false, nil
}

func intelString(value any) string {
	var text string
	switch typed := value.(type) {
	case string:
		text = strings.TrimSpace(typed)
	case float64:
		text = strconv.FormatInt(int64(typed), 10)
	case json.Number:
		text = typed.String()
	default:
		return ""
	}
	runes := []rune(text)
	if len(runes) > maxIntelFieldRunes {
		return string(runes[:maxIntelFieldRunes]) + "…"
	}
	return text
}

func firstIntelString(values ...any) string {
	for _, value := range values {
		switch typed := value.(type) {
		case []any:
			for _, item := range typed {
				if text := intelString(item); text != "" {
					return text
				}
			}
		default:
			if text := intelString(value); text != "" {
				return text
			}
		}
	}
	return ""
}

func (t *ToolSet) threatBookLookup() actool.CoreTool {
	return readTool(
		"threatbook_lookup",
		"查询当前任务已授权 IP 或域名的 ThreatBook 威胁情报。只接受任务授权范围内的单个 IP 或域名；结果仅供风险研判，不改变范围或任务状态。需要配置 THREATBOOK_API_KEY 环境变量。",
		obj(map[string]any{
			"resource": map[string]any{"type": "string", "description": "当前任务授权范围内的 IP 或域名"},
			"scene":    map[string]any{"type": "string", "enum": []string{"ip", "dns"}, "description": "ip 查询 IP 信誉，dns 查询域名风险"},
		}, "resource"),
		func(ctx context.Context, raw json.RawMessage) (actool.Result, error) {
			var input struct {
				Resource string `json:"resource"`
				Scene    string `json:"scene"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return actool.Errorf("invalid input: " + err.Error()), nil
			}
			input.Resource = strings.TrimSpace(input.Resource)
			if input.Scene == "" {
				input.Scene = "ip"
			}
			taskID := t.taskID
			if taskID <= 0 {
				taskID = RunInfoFrom(ctx).TaskID
			}
			if taskID <= 0 || t.as == nil {
				return actool.Errorf("威胁情报查询需要任务上下文和授权范围"), nil
			}
			scopes, err := t.as.ListTaskScope(taskID)
			if err != nil {
				return actool.Errorf("读取任务授权范围失败"), nil
			}
			if ip := net.ParseIP(input.Resource); ip != nil {
				if !intelIPInScope(ip.String(), scopes) {
					return actool.Errorf("resource 不在当前任务已登记的授权范围内"), nil
				}
			} else if !intelDomainInScope(normalizeIntelDomain(input.Resource), scopes) {
				return actool.Errorf("resource 不是有效域名或不在当前任务已登记的授权范围内"), nil
			}
			if input.Scene != "ip" && input.Scene != "dns" {
				return actool.Errorf("scene 仅支持 ip 或 dns"), nil
			}
			if input.Scene == "ip" && net.ParseIP(input.Resource) == nil ||
				input.Scene == "dns" && net.ParseIP(input.Resource) != nil {
				return actool.Errorf("scene 与 resource 类型不匹配"), nil
			}
			result, err := threatBookRequest(ctx, assetIntelDeadline(), input.Resource, input.Scene)
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			return jsonResult(result)
		},
	)
}

func threatBookRequest(ctx context.Context, client *http.Client, resource, scene string) (map[string]any, error) {
	keys := nonEmptyAccounts(
		intelCredentials{label: "primary", key: strings.TrimSpace(os.Getenv("THREATBOOK_API_KEY"))},
		intelCredentials{label: "backup", key: strings.TrimSpace(os.Getenv("THREATBOOK_API_KEY_BACKUP"))},
	)
	if len(keys) == 0 {
		return nil, fmt.Errorf("ThreatBook API 凭据未配置")
	}
	path := "scene/ip_reputation"
	if scene == "dns" {
		path = "scene/dns"
	}
	for _, credential := range keys {
		params := url.Values{"apikey": {credential.key}, "resource": {resource}, "lang": {"zh"}}
		data, status, err := intelHTTP(ctx, client, http.MethodGet, "https://api.threatbook.cn/v3/"+path+"?"+params.Encode(),
			map[string]string{"User-Agent": "Yunkun-Console/asset-intel"}, nil)
		if err != nil {
			continue
		}
		if status >= 200 && status < 300 && fmt.Sprint(data["response_code"]) == "0" {
			encoded, _ := json.Marshal(data)
			if len(encoded) > maxIntelOutputBytes {
				return map[string]any{
					"response_code": data["response_code"],
					"verbose_msg":   intelString(data["verbose_msg"]),
					"truncated":     true,
					"note":          "威胁情报响应过大，仅保留状态摘要",
				}, nil
			}
			return data, nil
		}
	}
	return nil, fmt.Errorf("ThreatBook 查询失败，检查凭据、权限或额度")
}

func assetIntelDeadline() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if request.URL.Scheme != "https" || len(via) >= 3 || request.URL.Hostname() != via[0].URL.Hostname() {
				return fmt.Errorf("拒绝不安全的外部服务重定向")
			}
			return nil
		},
	}
}
