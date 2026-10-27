package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	actool "github.com/Autumn-27/norma/tool"
)

const taskCacheMarker = ".yunkun-managed-cache"
const taskCacheSignature = "yunkun task scratch cache v1\n"

type cacheStamp struct {
	Size  int64
	MTime time.Time
}

// Only the worker's managed scratch directory is eligible. Existing task
// evidence, arbitrary downloads, and directories are deliberately excluded.
func openTaskCache(runDir string) (*os.Root, error) {
	resolved, err := filepath.EvalSymlinks(runDir)
	same := filepath.Clean(resolved) == filepath.Clean(runDir)
	if os.PathSeparator == '\\' {
		same = strings.EqualFold(filepath.Clean(resolved), filepath.Clean(runDir))
	}
	if err != nil || !same {
		return nil, fmt.Errorf("task workspace is missing or linked")
	}
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.Lstat("cache")
	switch {
	case os.IsNotExist(err):
		if err = root.Mkdir("cache", 0o700); err != nil {
			return nil, err
		}
		cache, err := root.OpenRoot("cache")
		if err != nil {
			return nil, err
		}
		if err := cache.WriteFile(taskCacheMarker, []byte(taskCacheSignature), 0o600); err != nil {
			cache.Close()
			return nil, err
		}
		return cache, nil
	case err != nil:
		return nil, err
	case !info.IsDir() || info.Mode()&os.ModeSymlink != 0:
		return nil, fmt.Errorf("cache is not a regular directory")
	}
	cache, err := root.OpenRoot("cache")
	if err != nil {
		return nil, err
	}
	marker, err := cache.ReadFile(taskCacheMarker)
	if err != nil || string(marker) != taskCacheSignature {
		cache.Close()
		return nil, fmt.Errorf("cache directory is not managed by this task")
	}
	return cache, nil
}

func cacheCandidate(name string) bool {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name ||
		strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, ".") {
		return false
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".js", ".mjs", ".cjs", ".map", ".css", ".html":
		return true
	}
	return false
}

func taskCacheTool(runDir string) actool.CoreTool {
	if runDir != "" {
		// Prepare the managed directory before Bash/Write can place temporary
		// scripts there. A pre-existing unmarked directory stays untouched.
		if cache, err := openTaskCache(runDir); err == nil {
			cache.Close()
		}
	}
	var mu sync.Mutex
	previewed := map[string]cacheStamp{}
	return writeTool("cleanup_task_cache",
		"预览或清理本次会话/意图 cache/ 中下载的前端 JS、source map、CSS、HTML 临时文件。先 preview，再传入所选文件名 delete；只删除预览后未变化的普通文件，不处理目录、任务证据、数据库或其它路径。重要证据先移到工作目录（非 cache/）。",
		obj(map[string]any{
			"action": map[string]any{"type": "string", "enum": []string{"preview", "delete"}},
			"files": map[string]any{"type": "array", "items": map[string]any{"type": "string"},
				"description": "delete 时选择 preview 返回的文件名（不含 cache/ 路径），最多 100 个"},
		}, "action"),
		func(_ context.Context, raw json.RawMessage) (actool.Result, error) {
			var in struct {
				Action string   `json:"action"`
				Files  []string `json:"files"`
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return actool.Errorf("invalid input"), nil
			}
			mu.Lock()
			defer mu.Unlock()
			cache, err := openTaskCache(runDir)
			if err != nil {
				return actool.Errorf("cache unavailable: " + err.Error()), nil
			}
			defer cache.Close()
			switch in.Action {
			case "preview":
				dir, err := cache.Open(".")
				if err != nil {
					return actool.Errorf(err.Error()), nil
				}
				entries, err := dir.ReadDir(201)
				dir.Close()
				if err != nil || len(entries) > 200 {
					previewed = map[string]cacheStamp{}
					return actool.Errorf("cache has too many entries or cannot be listed"), nil
				}
				previewed = map[string]cacheStamp{}
				out := make([]map[string]any, 0, len(entries))
				for _, entry := range entries {
					name := entry.Name()
					if !cacheCandidate(name) {
						continue
					}
					fi, err := cache.Lstat(name)
					if err != nil || !fi.Mode().IsRegular() {
						continue
					}
					previewed[name] = cacheStamp{fi.Size(), fi.ModTime()}
					out = append(out, map[string]any{"name": name, "size": fi.Size()})
				}
				b, _ := json.Marshal(map[string]any{"directory": "cache/", "files": out})
				return actool.Text(string(b)), nil
			case "delete":
				if len(in.Files) == 0 || len(in.Files) > 100 {
					return actool.Errorf("select 1-100 files from preview"), nil
				}
				seen := map[string]bool{}
				for _, name := range in.Files {
					stamp, ok := previewed[name]
					if !cacheCandidate(name) || !ok || seen[name] {
						return actool.Errorf("file was not previewed or is invalid: " + name), nil
					}
					seen[name] = true
					fi, err := cache.Lstat(name)
					if err != nil || !fi.Mode().IsRegular() ||
						fi.Size() != stamp.Size || !fi.ModTime().Equal(stamp.MTime) {
						return actool.Errorf("file changed since preview: " + name), nil
					}
				}
				removed := make([]string, 0, len(in.Files))
				for _, name := range in.Files {
					if err := cache.Remove(name); err != nil {
						return actool.Errorf(fmt.Sprintf("removed %d files; %s: %v", len(removed), name, err)), nil
					}
					delete(previewed, name)
					removed = append(removed, name)
				}
				b, _ := json.Marshal(map[string]any{"removed": removed})
				return actool.Text(string(b)), nil
			default:
				return actool.Errorf("action must be preview or delete"), nil
			}
		})
}
