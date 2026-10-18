package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	actool "github.com/Autumn-27/norma/tool"
	"golang.org/x/net/html"
)

const (
	maxSurfaceInputBytes = 256 << 10
	maxSurfaceItems      = 100
)

var (
	surfacePathPattern = regexp.MustCompile(`["'\x60](/[A-Za-z0-9_./{}:$?&=%-]{1,240})["'\x60]`)
	apiPathPattern     = regexp.MustCompile(`(?i)/(?:api|rest|service|services|gateway|graphql|v[0-9]+|web|admin|backend|open|ajax|rpc)(?:/|$)`)
	apiInlinePattern   = regexp.MustCompile(`(?i)(?:fetch|axios|ajax|url|endpoint|request)\s*(?:\(|:|=)\s*["'\x60]([^"'\x60]{1,240})["'\x60]`)
	assetExtPattern    = regexp.MustCompile(`(?i)\.(?:js|css|png|jpe?g|svg|gif|woff2?|ttf|ico|map|mp4|webp)(?:\?|$)`)
)

type surfaceForm struct {
	Method string   `json:"method"`
	Action string   `json:"action"`
	Params []string `json:"params,omitempty"`
}

type surfaceAnalyzer struct {
	baseURL *url.URL
	apis    map[string]struct{}
	routes  map[string]struct{}
	links   map[string]struct{}
	scripts map[string]struct{}
	forms   map[string]*surfaceForm
}

func newSurfaceAnalyzer(base string) (*surfaceAnalyzer, error) {
	var parsed *url.URL
	if strings.TrimSpace(base) != "" {
		u, err := url.Parse(strings.TrimSpace(base))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
			return nil, fmt.Errorf("base_url 必须是有效的 HTTP(S) URL")
		}
		parsed = u
	}
	return &surfaceAnalyzer{
		baseURL: parsed,
		apis:    map[string]struct{}{},
		routes:  map[string]struct{}{},
		links:   map[string]struct{}{},
		scripts: map[string]struct{}{},
		forms:   map[string]*surfaceForm{},
	}, nil
}

func (a *surfaceAnalyzer) resolve(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "#") || strings.HasPrefix(raw, "javascript:") {
		return ""
	}
	if a.baseURL == nil {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return a.baseURL.ResolveReference(u).String()
}

func (a *surfaceAnalyzer) addPath(raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" || assetExtPattern.MatchString(raw) {
		return
	}
	if apiPathPattern.MatchString(raw) {
		a.apis[raw] = struct{}{}
		return
	}
	if strings.HasPrefix(raw, "/") {
		a.routes[raw] = struct{}{}
	}
}

func (a *surfaceAnalyzer) analyzeJS(text string) {
	for _, match := range surfacePathPattern.FindAllStringSubmatch(text, -1) {
		if len(match) == 2 {
			a.addPath(match[1])
		}
	}
	for _, match := range apiInlinePattern.FindAllStringSubmatch(text, -1) {
		if len(match) == 2 && strings.HasPrefix(match[1], "/") {
			a.addPath(match[1])
		}
	}
}

func (a *surfaceAnalyzer) analyzeHTML(text string) error {
	root, err := html.Parse(strings.NewReader(text))
	if err != nil {
		return err
	}
	var currentForm *surfaceForm
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode {
			attrs := make(map[string]string, len(n.Attr))
			for _, attr := range n.Attr {
				attrs[strings.ToLower(attr.Key)] = attr.Val
			}
			switch strings.ToLower(n.Data) {
			case "a", "area":
				if link := a.resolve(attrs["href"]); link != "" {
					a.links[link] = struct{}{}
				}
			case "script":
				if src := a.resolve(attrs["src"]); src != "" {
					a.scripts[src] = struct{}{}
				}
			case "form":
				action := attrs["action"]
				if action == "" {
					action = a.currentPageURL()
				} else {
					action = a.resolve(action)
				}
				if parsed, err := url.Parse(action); err == nil {
					a.addPath(parsed.Path)
				}
				method := strings.ToUpper(strings.TrimSpace(attrs["method"]))
				if method == "" {
					method = "GET"
				}
				key := method + " " + action
				currentForm = a.forms[key]
				if currentForm == nil {
					currentForm = &surfaceForm{Method: method, Action: action}
					a.forms[key] = currentForm
				}
			case "input", "select", "textarea", "button":
				if currentForm != nil && strings.TrimSpace(attrs["name"]) != "" {
					currentForm.Params = append(currentForm.Params, strings.TrimSpace(attrs["name"]))
				}
			}
			for _, attr := range n.Attr {
				a.analyzeJS(attr.Val)
			}
		}
		if n.Type == html.TextNode && n.Parent != nil && n.Parent.Type == html.ElementNode &&
			strings.EqualFold(n.Parent.Data, "script") {
			a.analyzeJS(n.Data)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "form") {
			currentForm = nil
		}
	}
	visit(root)
	return nil
}

func (a *surfaceAnalyzer) currentPageURL() string {
	if a.baseURL == nil {
		return ""
	}
	return a.baseURL.String()
}

func sortedBounded(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	if len(out) > maxSurfaceItems {
		out = out[:maxSurfaceItems]
	}
	return out
}

func (t *ToolSet) analyzeWebSurface() actool.CoreTool {
	return readTool(
		"analyze_web_surface",
		"从已捕获或用户提供的 HTML/JavaScript 文本中提取表单、脚本地址、路由和疑似 API 路径。此工具只分析输入内容，不会请求目标、不跟随链接、不提交表单，也不会扩大任务授权范围。输入不得包含不必要的 Cookie、口令或其他凭据；输出是候选线索，需结合原始证据复核。",
		obj(map[string]any{
			"content": map[string]any{
				"type":        "string",
				"description": "已捕获的 HTML 或 JavaScript 文本，最多 256 KiB",
			},
			"content_type": map[string]any{
				"type":        "string",
				"enum":        []string{"html", "javascript"},
				"description": "输入内容类型",
			},
			"base_url": map[string]any{
				"type":        "string",
				"description": "原始页面 URL，用于解析相对地址；只用于标注，不会发出请求",
			},
		}, "content", "content_type"),
		func(_ context.Context, raw json.RawMessage) (actool.Result, error) {
			var input struct {
				Content     string `json:"content"`
				ContentType string `json:"content_type"`
				BaseURL     string `json:"base_url"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return actool.Errorf("invalid input: " + err.Error()), nil
			}
			if strings.TrimSpace(input.Content) == "" {
				return actool.Errorf("content 不能为空"), nil
			}
			if len(input.Content) > maxSurfaceInputBytes || !utf8.ValidString(input.Content) {
				return actool.Errorf("content 超出 256 KiB 限制或不是有效 UTF-8"), nil
			}
			analyzer, err := newSurfaceAnalyzer(input.BaseURL)
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			switch strings.ToLower(strings.TrimSpace(input.ContentType)) {
			case "html":
				if err := analyzer.analyzeHTML(input.Content); err != nil {
					return actool.Errorf("HTML 解析失败: " + err.Error()), nil
				}
			case "javascript":
				analyzer.analyzeJS(input.Content)
			default:
				return actool.Errorf("content_type 仅支持 html 或 javascript"), nil
			}
			forms := make([]surfaceForm, 0, len(analyzer.forms))
			for _, form := range analyzer.forms {
				sort.Strings(form.Params)
				form.Params = uniqueStrings(form.Params)
				forms = append(forms, *form)
			}
			sort.Slice(forms, func(i, j int) bool {
				if forms[i].Method == forms[j].Method {
					return forms[i].Action < forms[j].Action
				}
				return forms[i].Method < forms[j].Method
			})
			if len(forms) > maxSurfaceItems {
				forms = forms[:maxSurfaceItems]
			}
			out := map[string]any{
				"forms":       forms,
				"api_paths":   sortedBounded(analyzer.apis),
				"route_paths": sortedBounded(analyzer.routes),
				"links":       sortedBounded(analyzer.links),
				"scripts":     sortedBounded(analyzer.scripts),
				"truncated":   len(analyzer.forms) > maxSurfaceItems || len(analyzer.apis) > maxSurfaceItems || len(analyzer.routes) > maxSurfaceItems || len(analyzer.links) > maxSurfaceItems || len(analyzer.scripts) > maxSurfaceItems,
				"source":      "provided_artifact",
			}
			return jsonResult(out)
		},
	)
}

func uniqueStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}
