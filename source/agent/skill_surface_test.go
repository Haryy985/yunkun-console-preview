package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func callSurfaceTool(t *testing.T, payload any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewToolSet(nil, "worker").analyzeWebSurface().Call(context.Background(), raw, nil)
	if err != nil {
		t.Fatalf("tool call returned transport error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Flatten())
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(result.Flatten()), &decoded); err != nil {
		t.Fatalf("decode result: %v; raw=%s", err, result.Flatten())
	}
	return decoded
}

func TestAnalyzeWebSurfaceExtractsHTMLAndJSWithoutFetching(t *testing.T) {
	out := callSurfaceTool(t, map[string]any{
		"content_type": "html",
		"base_url":     "https://app.example.test/admin/",
		"content": `<html><body>
			<a href="/dashboard">Home</a>
			<script src="../assets/main.js"></script>
			<form method="post" action="/api/login">
				<input name="username"><input name="password">
			</form>
			<script>fetch("/api/v2/profile"); const route = "/settings/account";</script>
		</body></html>`,
	})
	if got := out["source"]; got != "provided_artifact" {
		t.Fatalf("source = %v", got)
	}
	apis := out["api_paths"].([]any)
	if len(apis) != 2 || apis[0] != "/api/login" || apis[1] != "/api/v2/profile" {
		t.Fatalf("api_paths = %#v", apis)
	}
	routes := out["route_paths"].([]any)
	if len(routes) != 1 || routes[0] != "/settings/account" {
		t.Fatalf("route_paths = %#v", routes)
	}
	links := out["links"].([]any)
	if len(links) != 1 || links[0] != "https://app.example.test/dashboard" {
		t.Fatalf("links = %#v", links)
	}
	forms := out["forms"].([]any)
	if len(forms) != 1 {
		t.Fatalf("forms = %#v", forms)
	}
	form := forms[0].(map[string]any)
	if form["method"] != "POST" || form["action"] != "https://app.example.test/api/login" {
		t.Fatalf("form = %#v", form)
	}
	params := form["params"].([]any)
	if len(params) != 2 || params[0] != "password" || params[1] != "username" {
		t.Fatalf("form params = %#v", params)
	}
	scripts := out["scripts"].([]any)
	if len(scripts) != 1 || scripts[0] != "https://app.example.test/assets/main.js" {
		t.Fatalf("scripts = %#v", scripts)
	}
}

func TestAnalyzeWebSurfaceRejectsInvalidOrOversizedInput(t *testing.T) {
	for _, input := range []map[string]any{
		{"content": "x", "content_type": "html", "base_url": "file:///etc/passwd"},
		{"content": "x", "content_type": "unknown"},
		{"content": strings.Repeat("x", maxSurfaceInputBytes+1), "content_type": "javascript"},
	} {
		raw, _ := json.Marshal(input)
		result, err := NewToolSet(nil, "worker").analyzeWebSurface().Call(context.Background(), raw, nil)
		if err != nil {
			t.Fatalf("unexpected transport error: %v", err)
		}
		if !result.IsError {
			t.Errorf("expected safe input rejection, got %s", result.Flatten())
		}
	}
}

func TestAnalyzeWebSurfaceIsBoundToWorkerAndPentest(t *testing.T) {
	seeds := map[string]ToolSeed{}
	for _, seed := range BuiltinToolSeeds() {
		seeds[seed.Key] = seed
	}
	seed, ok := seeds["analyze_web_surface"]
	if !ok {
		t.Fatal("analyze_web_surface missing from tool catalog")
	}
	bound := map[string]bool{}
	for _, name := range seed.Agents {
		bound[name] = true
	}
	if !bound["worker"] || !bound["pentest"] {
		t.Fatalf("analyze_web_surface bindings = %v, want worker and pentest", seed.Agents)
	}
}
