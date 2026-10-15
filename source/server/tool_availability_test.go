package server

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"yunkun/console/agent"
	"yunkun/console/db"
	actool "github.com/Autumn-27/norma/tool"
)

type policyRecorder struct {
	row   *db.Tool
	err   error
	usage int
}

func (r *policyRecorder) GetToolContext(context.Context, string) (*db.Tool, error) {
	return r.row, r.err
}
func (r *policyRecorder) InsertToolUsage(*db.ToolUsage) error { r.usage++; return nil }

func TestToolAvailabilityStopsOnOutageAndRevocation(t *testing.T) {
	calls := 0
	base := actool.Build(actool.Spec{Name: "fixture", Run: func(context.Context, json.RawMessage, *actool.ToolContext) (actool.Result, error) {
		calls++
		return actool.Text("done"), nil
	}})
	blocked := &unavailableTool{CoreTool: base}
	if _, err := blocked.Call(t.Context(), nil, nil); err == nil || calls != 0 {
		t.Fatal("unavailable configuration allowed a call")
	}
	rec := &policyRecorder{row: &db.Tool{Key: "fixture", Enabled: true, Agents: []string{"worker"}}}
	tool := meterTool(base, rec, "fixture", "worker", agent.RunInfo{})
	if _, err := tool.Call(t.Context(), nil, nil); err != nil || calls != 1 {
		t.Fatalf("normal authorized call failed: %v", err)
	}
	for _, mode := range []string{"offline", "disabled", "unbound", "removed"} {
		rec.err = nil
		rec.row = &db.Tool{Key: "fixture", Enabled: true, Agents: []string{"worker"}}
		switch mode {
		case "offline":
			rec.err = errors.New("database offline")
		case "disabled":
			rec.row.Enabled = false
		case "unbound":
			rec.row.Agents = []string{"planner"}
		case "removed":
			rec.row = nil
		}
		if _, err := tool.Call(t.Context(), nil, nil); err == nil || calls != 1 || rec.usage != 1 {
			t.Fatalf("%s allowed side effects", mode)
		}
	}
}

func TestProxyFetchPreservesUnavailableToolGate(t *testing.T) {
	base := actool.NewWebFetch(actool.WebFetchConfig{})
	tool := &taskProxyFetch{CoreTool: &unavailableTool{CoreTool: base}}
	// The nil server deliberately proves the gate runs before any routing I/O.
	if _, err := tool.Call(t.Context(), json.RawMessage(`{"url":"http://example.invalid"}`), nil); err == nil {
		t.Fatal("proxy routing bypassed unavailable tool policy")
	}
}
