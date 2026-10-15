package agent

import (
	"context"
	"encoding/json"
	"testing"

	"yunkun/console/db"
	"yunkun/console/llmrec"
	"github.com/Autumn-27/norma/agentcore"
	"github.com/Autumn-27/norma/llm"
	"github.com/Autumn-27/norma/permission"
	"github.com/Autumn-27/norma/tool"
)

func TestCaptureRunLinksToolToProducingRequest(t *testing.T) {
	turn := 0
	provider := captureUsageProvider{stream: func(_ context.Context, yield func(llm.StreamEvent, error) bool) {
		turn++
		events := []llm.StreamEvent{{Type: llm.SETextDelta, Text: "done"}, {Type: llm.SEMessageDelta, StopReason: "end_turn"}}
		if turn == 1 {
			events = []llm.StreamEvent{
				{Type: llm.SEToolUseStart, ToolID: "call-1", ToolName: "TraceProbe"},
				{Type: llm.SEToolInputJSON, Text: `{}`},
				{Type: llm.SEMessageDelta, StopReason: "tool_use"},
			}
		}
		for _, event := range events {
			if !yield(event, nil) {
				return
			}
		}
	}}
	recorder := llmrec.Wrap(provider, nil, "fixture", "", "", "", func() bool { return false })
	probe := tool.Build(tool.Spec{
		Name: "TraceProbe", Schema: map[string]any{"type": "object"},
		Run: func(context.Context, json.RawMessage, *tool.ToolContext) (tool.Result, error) {
			return tool.Text("probe result"), nil
		},
	})
	var activities []db.Activity
	_, _, err := captureRun(t.Context(), agentcore.Options{
		Provider: recorder, Tools: []tool.CoreTool{probe},
		PermissionMode: permission.ModeBypass, WorkingDir: t.TempDir(), MaxTurns: 2,
	}, "run probe", func(a db.Activity) { activities = append(activities, a) })
	if err != nil {
		t.Fatal(err)
	}
	var useID, resultID string
	for _, activity := range activities {
		if activity.ToolUseID != "call-1" {
			continue
		}
		var metadata struct {
			RequestID string `json:"llm_request_id"`
		}
		if err := json.Unmarshal(activity.Metadata, &metadata); err != nil {
			t.Fatal(err)
		}
		switch activity.Kind {
		case "tool_use":
			useID = metadata.RequestID
		case "tool_result":
			resultID = metadata.RequestID
		}
	}
	if len(useID) != 32 || resultID != useID || turn != 2 {
		t.Fatalf("tool request attribution: use=%q result=%q turns=%d", useID, resultID, turn)
	}
}
