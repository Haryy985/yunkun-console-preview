package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	actool "github.com/Autumn-27/norma/tool"
	"yunkun/console/db"
)

const (
	maxCrawlPages       = 20
	maxCrawlDepth       = 3
	maxCrawlRequests    = 40
	maxCrawlBodyBytes   = 1 << 20
	crawlRequestTimeout = 8 * time.Second
	crawlDelay          = 100 * time.Millisecond
)

type crawlQueueItem struct {
	url   string
	depth int
}

type crawlSummary struct {
	Pages       int           `json:"pages"`
	Requests    int           `json:"requests"`
	Forms       []surfaceForm `json:"forms"`
	APIPaths    []string      `json:"api_paths"`
	RoutePaths  []string      `json:"route_paths"`
	Links       []string      `json:"links"`
	Scripts     []string      `json:"scripts"`
	Failures    []string      `json:"failures,omitempty"`
	Truncated   bool          `json:"truncated"`
	ScopeNotice string        `json:"scope_notice"`
}

func (t *ToolSet) crawlWebSurface() actool.CoreTool {
	return readTool(
		"crawl_web_surface",
		"对当前任务授权范围内的单一 Web origin 做有限、只读的 GET 页面发现，并解析 HTML/JS。仅同源 HTTP(S)，不发送 Cookie/Authorization、不提交表单、不调用表单 action、不访问跨源链接；每个地址必须在任务范围内，响应受大小/数量/超时限制。GET 请求仍可能触发目标副作用，因此该工具默认不绑定，需管理员明确启用。结果只作候选，不扩大任务范围。",
		obj(map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "任务已授权范围内的 HTTP(S) 起始 URL，不得包含账号、口令或敏感查询参数",
			},
			"max_pages": map[string]any{
				"type":        "integer",
				"description": "最多抓取页面数，默认 10，最大 20",
			},
			"max_depth": map[string]any{
				"type":        "integer",
				"description": "同源链接深度，默认 2，最大 3",
			},
		}, "url"),
		func(ctx context.Context, raw json.RawMessage) (actool.Result, error) {
			var input struct {
				URL      string `json:"url"`
				MaxPages int    `json:"max_pages"`
				MaxDepth int    `json:"max_depth"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return actool.Errorf("invalid input: " + err.Error()), nil
			}
			taskID := t.taskID
			if taskID <= 0 {
				taskID = RunInfoFrom(ctx).TaskID
			}
			if taskID <= 0 || t.as == nil {
				return actool.Errorf("网页抓取需要任务上下文和已登记的授权范围"), nil
			}
			scopes, err := t.as.ListTaskScope(taskID)
			if err != nil {
				return actool.Errorf("读取任务授权范围失败"), nil
			}
			start, err := parseScopedCrawlURL(input.URL, scopes)
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			if input.MaxPages <= 0 {
				input.MaxPages = 10
			}
			if input.MaxPages > maxCrawlPages {
				input.MaxPages = maxCrawlPages
			}
			if input.MaxDepth <= 0 {
				input.MaxDepth = 2
			}
			if input.MaxDepth > maxCrawlDepth {
				input.MaxDepth = maxCrawlDepth
			}
			summary, err := crawlWebSurface(ctx, start, scopes, input.MaxPages, input.MaxDepth)
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			return jsonResult(summary)
		},
	)
}

func parseScopedCrawlURL(raw string, scopes []db.TaskScope) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.Fragment != "" {
		return nil, fmt.Errorf("url 必须是无凭据、无片段的有效 HTTP(S) URL")
	}
	if !crawlURLInScope(parsed, scopes) {
		return nil, fmt.Errorf("url 主机不在当前任务已登记的授权范围内")
	}
	for key := range parsed.Query() {
		switch strings.ToLower(key) {
		case "token", "access_token", "refresh_token", "api_key", "apikey", "key", "auth", "authorization", "password", "passwd", "session", "code", "ticket":
			return nil, fmt.Errorf("url 查询参数疑似包含凭据；请移除后重试")
		}
	}
	parsed.Fragment = ""
	return parsed, nil
}

func crawlURLInScope(target *url.URL, scopes []db.TaskScope) bool {
	host := target.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		return intelIPInScope(ip.String(), scopes)
	}
	return intelDomainInScope(host, scopes)
}

func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) &&
		strings.EqualFold(a.Hostname(), b.Hostname()) &&
		effectivePort(a) == effectivePort(b)
}

func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

func scopedCrawlerTransport(scopes []db.TaskScope) *http.Transport {
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 15 * time.Second}
	return &http.Transport{
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		IdleConnTimeout:       15 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, fmt.Errorf("目标地址格式无效")
			}
			ips := []net.IPAddr{}
			if literal := net.ParseIP(host); literal != nil {
				ips = append(ips, net.IPAddr{IP: literal})
			} else {
				ips, err = net.DefaultResolver.LookupIPAddr(ctx, host)
				if err != nil {
					return nil, fmt.Errorf("目标域名解析失败")
				}
			}
			for _, resolved := range ips {
				ip := resolved.IP
				if ip == nil || ip.IsUnspecified() || ip.IsMulticast() {
					continue
				}
				privateOrSpecial := !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
				if privateOrSpecial && !intelIPInScope(ip.String(), scopes) {
					continue
				}
				conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if dialErr == nil {
					return conn, nil
				}
			}
			return nil, fmt.Errorf("目标解析地址不在可访问的授权边界内")
		},
	}
}

func crawlWebSurface(ctx context.Context, start *url.URL, scopes []db.TaskScope, maxPages, maxDepth int) (*crawlSummary, error) {
	if start == nil || !crawlURLInScope(start, scopes) {
		return nil, fmt.Errorf("起始地址不在任务授权范围内")
	}
	if maxPages < 1 || maxPages > maxCrawlPages || maxDepth < 0 || maxDepth > maxCrawlDepth {
		return nil, fmt.Errorf("抓取上限无效")
	}
	origin := *start
	transport := scopedCrawlerTransport(scopes)
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		Timeout:   crawlRequestTimeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 3 || request.URL.User != nil || !sameOrigin(&origin, request.URL) || !crawlURLInScope(request.URL, scopes) {
				return fmt.Errorf("拒绝跨 origin 或越权重定向")
			}
			return nil
		},
	}
	summary := &crawlSummary{
		Forms: []surfaceForm{}, APIPaths: []string{}, RoutePaths: []string{},
		Links: []string{}, Scripts: []string{}, Failures: []string{},
		ScopeNotice: "仅同源、授权范围内的 GET；结果未经验证，不自动登记资产。",
	}
	aggregate, _ := newSurfaceAnalyzer(start.String())
	queue := []crawlQueueItem{{url: start.String(), depth: 0}}
	seenPages := map[string]bool{}
	seenScripts := map[string]bool{}
	fail := func(target string, err error) {
		if len(summary.Failures) < 20 {
			if parsed, parseErr := url.Parse(target); parseErr == nil {
				target = parsed.EscapedPath()
			}
			summary.Failures = append(summary.Failures, target+": "+err.Error())
		} else {
			summary.Truncated = true
		}
	}
	fetch := func(target string) ([]byte, string, int, error) {
		if summary.Requests >= maxCrawlRequests {
			return nil, "", 0, fmt.Errorf("已达到请求数上限")
		}
		parsed, err := parseScopedCrawlURL(target, scopes)
		if err != nil || !sameOrigin(&origin, parsed) {
			return nil, "", 0, fmt.Errorf("仅允许访问授权范围内的同源地址")
		}
		if summary.Requests > 0 {
			timer := time.NewTimer(crawlDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, "", 0, ctx.Err()
			case <-timer.C:
			}
		}
		reqCtx, cancel := context.WithTimeout(ctx, crawlRequestTimeout)
		defer cancel()
		request, err := http.NewRequestWithContext(reqCtx, http.MethodGet, parsed.String(), nil)
		if err != nil {
			return nil, "", 0, fmt.Errorf("构造请求失败")
		}
		request.Header.Set("Accept", "text/html,application/javascript,text/javascript,*/*;q=0.2")
		request.Header.Set("User-Agent", "Yunkun-Console-Surface-Review/1.0")
		summary.Requests++
		response, err := client.Do(request)
		if err != nil {
			return nil, "", 0, fmt.Errorf("请求失败或被范围策略阻止")
		}
		defer response.Body.Close()
		body, err := io.ReadAll(io.LimitReader(response.Body, maxCrawlBodyBytes+1))
		if err != nil {
			return nil, response.Header.Get("Content-Type"), response.StatusCode, fmt.Errorf("读取响应失败")
		}
		if len(body) > maxCrawlBodyBytes {
			body = body[:maxCrawlBodyBytes]
			summary.Truncated = true
		}
		return body, response.Header.Get("Content-Type"), response.StatusCode, nil
	}
	for len(queue) > 0 && summary.Pages < maxPages && summary.Requests < maxCrawlRequests {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		item := queue[0]
		queue = queue[1:]
		parsed, err := url.Parse(item.url)
		if err != nil || !sameOrigin(&origin, parsed) || !crawlURLInScope(parsed, scopes) {
			continue
		}
		key := parsed.String()
		if seenPages[key] || crawlExcludedPath(parsed.Path) {
			continue
		}
		seenPages[key] = true
		body, contentType, status, err := fetch(key)
		if err != nil {
			fail(key, err)
			continue
		}
		summary.Pages++
		if status < 200 || status >= 400 {
			fail(key, fmt.Errorf("HTTP %d", status))
		}
		pageAnalyzer, _ := newSurfaceAnalyzer(key)
		if strings.Contains(strings.ToLower(contentType), "html") || strings.HasPrefix(strings.TrimSpace(string(body)), "<") {
			if parseErr := pageAnalyzer.analyzeHTML(string(body)); parseErr != nil {
				fail(key, fmt.Errorf("HTML 解析失败"))
			}
		} else if strings.Contains(strings.ToLower(contentType), "javascript") || strings.HasSuffix(parsed.Path, ".js") {
			pageAnalyzer.analyzeJS(string(body))
		} else {
			continue
		}
		if len(pageAnalyzer.links) > maxSurfaceItems || len(pageAnalyzer.scripts) > maxSurfaceItems ||
			len(pageAnalyzer.apis) > maxSurfaceItems || len(pageAnalyzer.routes) > maxSurfaceItems {
			summary.Truncated = true
		}
		mergeSurfaceAnalyzers(aggregate, pageAnalyzer)
		if item.depth < maxDepth {
			for _, link := range sortedBounded(pageAnalyzer.links) {
				next, parseErr := url.Parse(link)
				if parseErr != nil || !sameOrigin(&origin, next) || !crawlURLInScope(next, scopes) || crawlExcludedPath(next.Path) {
					continue
				}
				if !seenPages[next.String()] {
					queue = append(queue, crawlQueueItem{url: next.String(), depth: item.depth + 1})
				}
			}
		}
		for _, script := range sortedBounded(pageAnalyzer.scripts) {
			if summary.Requests >= maxCrawlRequests {
				summary.Truncated = true
				break
			}
			scriptURL, parseErr := url.Parse(script)
			if parseErr != nil || !sameOrigin(&origin, scriptURL) || !crawlURLInScope(scriptURL, scopes) ||
				!strings.HasSuffix(strings.ToLower(scriptURL.Path), ".js") || seenScripts[scriptURL.String()] {
				continue
			}
			seenScripts[scriptURL.String()] = true
			scriptBody, scriptType, _, fetchErr := fetch(scriptURL.String())
			if fetchErr != nil {
				fail(scriptURL.String(), fetchErr)
				continue
			}
			if strings.Contains(strings.ToLower(scriptType), "javascript") || len(scriptBody) > 0 {
				scriptAnalyzer, _ := newSurfaceAnalyzer(scriptURL.String())
				scriptAnalyzer.analyzeJS(string(scriptBody))
				mergeSurfaceAnalyzers(aggregate, scriptAnalyzer)
			}
		}
	}
	if len(queue) > 0 || summary.Pages >= maxPages || summary.Requests >= maxCrawlRequests {
		summary.Truncated = true
	}
	summary.Forms = make([]surfaceForm, 0, len(aggregate.forms))
	for _, form := range aggregate.forms {
		action, err := url.Parse(form.Action)
		if err == nil && action.IsAbs() && !sameOrigin(&origin, action) {
			continue
		}
		sort.Strings(form.Params)
		form.Params = uniqueStrings(form.Params)
		summary.Forms = append(summary.Forms, *form)
	}
	if len(summary.Forms) > maxSurfaceItems {
		summary.Forms = summary.Forms[:maxSurfaceItems]
		summary.Truncated = true
	}
	sort.Slice(summary.Forms, func(i, j int) bool {
		if summary.Forms[i].Method == summary.Forms[j].Method {
			return summary.Forms[i].Action < summary.Forms[j].Action
		}
		return summary.Forms[i].Method < summary.Forms[j].Method
	})
	summary.APIPaths = sortedBounded(aggregate.apis)
	summary.RoutePaths = sortedBounded(aggregate.routes)
	for _, link := range sortedBounded(aggregate.links) {
		if parsed, err := url.Parse(link); err == nil && sameOrigin(&origin, parsed) {
			summary.Links = append(summary.Links, link)
		}
	}
	for _, script := range sortedBounded(aggregate.scripts) {
		if parsed, err := url.Parse(script); err == nil && sameOrigin(&origin, parsed) {
			summary.Scripts = append(summary.Scripts, script)
		}
	}
	return summary, nil
}

func crawlExcludedPath(path string) bool {
	path = strings.ToLower(path)
	for _, marker := range []string{"/logout", "/log-out", "/signout", "/sign-out", "/delete", "/remove", "/destroy"} {
		if strings.Contains(path, marker) {
			return true
		}
	}
	return false
}

func mergeSurfaceAnalyzers(dst, src *surfaceAnalyzer) {
	for value := range src.apis {
		dst.apis[value] = struct{}{}
	}
	for value := range src.routes {
		dst.routes[value] = struct{}{}
	}
	for value := range src.links {
		dst.links[value] = struct{}{}
	}
	for value := range src.scripts {
		dst.scripts[value] = struct{}{}
	}
	for key, form := range src.forms {
		if existing := dst.forms[key]; existing != nil {
			existing.Params = append(existing.Params, form.Params...)
			continue
		}
		copy := *form
		copy.Params = append([]string(nil), form.Params...)
		dst.forms[key] = &copy
	}
}
