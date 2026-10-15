package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	actool "github.com/Autumn-27/norma/tool"
)

func TestResourceNetworkSummaryAllowlist(t *testing.T) {
	summary := resourceNetworkSummary(map[string]any{
		"status": "ready", "mode": "pool", "bound": true, "enforced": true,
		"managed_tools": []string{strings.Repeat("x", 500)}, "node": "password-secret",
		"binding": "password-secret", "error": "password-secret",
	})
	body, _ := json.Marshal(summary)
	if strings.Contains(string(body), "secret") || summary["enforced"] != false ||
		summary["status"] != "ready" || len(summary["managed_tools"].([]string)[0]) != 160 {
		t.Fatalf("unsafe summary: %s", body)
	}
}

func TestResourceDiscoveryRequiresTaskAndStore(t *testing.T) {
	tool := NewToolSet(nil, "fixture").availableResources()
	res, err := tool.Call(t.Context(), json.RawMessage(`{}`), nil)
	if err != nil || !res.IsError {
		t.Fatalf("resource discovery without task/store: %v %v", res, err)
	}
}

func TestResourceAssemblyRespectsToolPermissions(t *testing.T) {
	oldResolve, oldAugment, oldNetwork := ToolResolve, ToolAugment, NetworkTools
	t.Cleanup(func() { ToolResolve, ToolAugment, NetworkTools = oldResolve, oldAugment, oldNetwork })
	ToolAugment, NetworkTools = nil, nil
	base := []actool.CoreTool{NewToolSet(nil, "fixture").availableResources()}
	for _, allowed := range []bool{true, false} {
		ToolResolve = func(_ context.Context, _ string, tools []actool.CoreTool) []actool.CoreTool {
			if allowed {
				return tools
			}
			return nil
		}
		out, def, cleanup := AugmentTools(t.Context(), "worker", base)
		cleanup()
		if (len(out) == 1) != allowed || (def.ResourceGuidance != "") != allowed {
			t.Fatal("resource tool/guidance ignored final permissions")
		}
		system, _ := deferredSystem("custom system", def)
		if strings.Contains(strings.Join(system, "\n"), "available_resources") != allowed {
			t.Fatal("system resource guidance ignored permissions")
		}
	}
	seeds := BuiltinToolSeeds()
	bindings := map[string]bool{}
	for _, seed := range seeds {
		if seed.Key == "available_resources" {
			for _, role := range seed.Agents {
				bindings[role] = true
			}
		}
	}
	for _, role := range []string{"planner", "worker", "mainagent"} {
		if !bindings[role] {
			t.Fatalf("missing resource binding for %s", role)
		}
	}
}
