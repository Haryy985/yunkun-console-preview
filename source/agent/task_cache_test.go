package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	actool "github.com/Autumn-27/norma/tool"
)

func callTaskCache(t *testing.T, tool actool.CoreTool, input any, wantError bool) string {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	result, err := tool.Call(t.Context(), raw, nil)
	if err != nil || result.IsError != wantError {
		t.Fatalf("call %s: %s, %v", raw, result.Flatten(), err)
	}
	return result.Flatten()
}

func TestTaskCacheCleanupOnlyPreviewedFiles(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "tasks", "42", "i7")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	tool := taskCacheTool(runDir)
	if got := callTaskCache(t, tool, map[string]any{"action": "preview"}, false); !strings.Contains(got, `"files":[]`) {
		t.Fatalf("unexpected preview: %s", got)
	}
	cache := filepath.Join(runDir, "cache")
	script := filepath.Join(cache, "bundle.js")
	if err := os.WriteFile(script, []byte("temporary"), 0o600); err != nil {
		t.Fatal(err)
	}
	evidence := filepath.Join(runDir, "evidence.js")
	if err := os.WriteFile(evidence, []byte("retain"), 0o600); err != nil {
		t.Fatal(err)
	}
	callTaskCache(t, tool, map[string]any{"action": "delete", "files": []string{"bundle.js"}}, true)
	if got := callTaskCache(t, tool, map[string]any{"action": "preview"}, false); !strings.Contains(got, "bundle.js") {
		t.Fatal("preview omitted the candidate")
	}
	for _, name := range []string{"../evidence.js", "evidence.js", "cache/bundle.js", taskCacheMarker} {
		callTaskCache(t, tool, map[string]any{"action": "delete", "files": []string{name}}, true)
	}
	if err := os.WriteFile(script, []byte("changed since preview"), 0o600); err != nil {
		t.Fatal(err)
	}
	callTaskCache(t, tool, map[string]any{"action": "delete", "files": []string{"bundle.js"}}, true)
	callTaskCache(t, tool, map[string]any{"action": "preview"}, false)
	callTaskCache(t, tool, map[string]any{"action": "delete", "files": []string{"bundle.js"}}, false)
	if _, err := os.Stat(script); !os.IsNotExist(err) {
		t.Fatal("previewed cache script was not removed")
	}
	if _, err := os.Stat(evidence); err != nil {
		t.Fatal("task evidence was removed")
	}
}

func TestTaskCacheRefusesUnmanagedAndLinkedDirectories(t *testing.T) {
	root := t.TempDir()
	runDir := filepath.Join(root, "tasks", "42", "i7")
	if err := os.MkdirAll(filepath.Join(runDir, "cache"), 0o700); err != nil {
		t.Fatal(err)
	}
	tool := taskCacheTool(runDir)
	callTaskCache(t, tool, map[string]any{"action": "preview"}, true)
	if err := os.Remove(filepath.Join(runDir, "cache")); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(runDir, "cache")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink privilege unavailable: %v", err)
		}
		t.Fatal(err)
	}
	callTaskCache(t, tool, map[string]any{"action": "preview"}, true)
	if _, err := os.Stat(filepath.Join(outside, taskCacheMarker)); !os.IsNotExist(err) {
		t.Fatal("linked directory was modified")
	}
}

func TestTaskCacheToolCatalogBindings(t *testing.T) {
	for _, seed := range BuiltinToolSeeds() {
		if seed.Key != "cleanup_task_cache" {
			continue
		}
		for _, role := range []string{"worker", "planner", "mainagent", "auto", "pentest"} {
			found := false
			for _, bound := range seed.Agents {
				if bound == role {
					found = true
				}
			}
			if !found {
				t.Fatalf("cleanup unavailable for %s: %+v", role, seed.Agents)
			}
		}
		return
	}
	t.Fatal("cleanup tool not seeded")
}
