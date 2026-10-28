package agent

import (
	"encoding/json"
	"strings"
	"testing"

	actool "github.com/Autumn-27/norma/tool"
)

// Names captured from the user's original ARTEX source on 2026-10-27.
// Keep these base capabilities; permissions and task scope still apply at runtime.
func TestUpstreamToolSurfacePreserved(t *testing.T) {
	ts := NewToolSet(nil, "")
	for _, tc := range []struct {
		role  string
		tools []actool.CoreTool
		want  string
	}{
		{"mainagent", ts.MainAgentTools(), "add_company_scope add_hint add_intent add_task_scope expand_digest get_worker_output get_worker_trace graph_overview insert_assets list_assets list_facts list_findings list_untested_assets node_detail record_fact report_finding search_all_worker_traces set_constraints set_goals steer_work"},
		{"planner", ts.PlannerTools(), "add_company_scope add_intent add_task_scope expand_digest get_worker_output get_worker_trace goal_met graph_overview kill_work list_assets list_companies list_facts list_findings list_goals list_untested_assets node_detail prove_goal report_finding search_all_worker_traces steer_work"},
		{"worker", ts.WorkerTools(), "get_worker_trace insert_assets list_assets list_findings node_detail record_fact report_finding search_all_worker_traces"},
		{"sdk", actool.DefaultTools(), "Bash Edit Glob Grep LS MultiEdit Read Write sleep"},
	} {
		t.Run(tc.role, func(t *testing.T) {
			got := make(map[string]bool)
			for _, tool := range tc.tools {
				if got[tool.Name()] {
					t.Errorf("duplicate tool %q", tool.Name())
				}
				got[tool.Name()] = true
			}
			for _, name := range strings.Fields(tc.want) {
				if !got[name] {
					t.Errorf("upstream capability %q removed", name)
				}
			}
		})
	}
}

func TestReportFindingDescriptionUsesOriginalContract(t *testing.T) {
	tool := NewToolSet(nil, "").addFinding()
	description := tool.Description()
	if !strings.Contains(description, "记录确认的漏洞") || strings.Contains(description, "validation") {
		t.Fatalf("original report_finding contract not restored: %q", description)
	}
	raw, err := json.Marshal(tool.InputSchema())
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	for _, required := range schema.Required {
		if required == "validation" || required == "evidence" || required == "learning" {
			t.Fatalf("original contract made %q mandatory: %s", required, raw)
		}
	}
	for _, field := range []string{"validation", "learning"} {
		if _, exists := schema.Properties[field]; exists {
			t.Fatalf("new finding workflow field %q remains in original tool schema: %s", field, raw)
		}
	}
	if _, exists := schema.Properties["evidence"]; !exists {
		t.Fatalf("original optional evidence field missing: %s", raw)
	}
}
