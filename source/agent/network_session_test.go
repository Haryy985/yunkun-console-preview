package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Autumn-27/norma/agentcore"
	"github.com/Autumn-27/norma/llm"
	"github.com/Autumn-27/norma/tool"
)

func TestLateWebFetchRespectsToolVisibility(t *testing.T) {
	oldResolve, oldNetwork := ToolResolve, NetworkTools
	t.Cleanup(func() { ToolResolve, NetworkTools = oldResolve, oldNetwork })
	checked := false
	ToolResolve = func(ctx context.Context, key string, tools []tool.CoreTool) []tool.CoreTool {
		if key != "worker" || RunInfoFrom(ctx).TaskID != 42 {
			t.Fatalf("lost policy context: key=%s info=%+v", key, RunInfoFrom(ctx))
		}
		checked = true
		return nil
	}
	NetworkTools = func(_ context.Context, tools []tool.CoreTool) []tool.CoreTool {
		for _, item := range tools {
			if item.Name() == "WebFetch" {
				t.Fatal("disabled WebFetch reintroduced by late SDK assembly")
			}
		}
		return tools
	}
	ctx := WithRunInfo(t.Context(), RunInfo{TaskID: 42, AgentID: "worker"})
	_ = newRunSession(ctx, agentcore.Options{EnableWebFetch: true, Tools: []tool.CoreTool{}})
	if !checked {
		t.Fatal("late WebFetch skipped visibility resolution")
	}
}

func TestChatFinalWebFetchRoutingPreservesTaskContext(t *testing.T) {
	old := NetworkTools
	t.Cleanup(func() { NetworkTools = old })
	calls := 0
	NetworkTools = func(ctx context.Context, tools []tool.CoreTool) []tool.CoreTool {
		out := append([]tool.CoreTool(nil), tools...)
		for i, item := range out {
			if item.Name() != "WebFetch" {
				continue
			}
			ri := RunInfoFrom(ctx)
			if ri.TaskID != 42 || ri.SessionID != "retest-fixture" || ri.AgentID != "retester" {
				t.Errorf("lost task attribution: %+v", ri)
			}
			out[i] = tool.Build(tool.Spec{Name: "WebFetch", Schema: item.InputSchema(),
				Run: func(context.Context, json.RawMessage, *tool.ToolContext) (tool.Result, error) {
					calls++
					return tool.Text("controlled route"), nil
				}})
		}
		return out
	}
	turn := 0
	provider := captureUsageProvider{stream: func(_ context.Context, yield func(llm.StreamEvent, error) bool) {
		turn++
		events := []llm.StreamEvent{{Type: llm.SETextDelta, Text: "done"}, {Type: llm.SEMessageDelta, StopReason: "end_turn"}}
		if turn == 1 {
			events = []llm.StreamEvent{
				{Type: llm.SEToolUseStart, ToolID: "fetch-1", ToolName: "WebFetch"},
				{Type: llm.SEToolInputJSON, Text: `{"url":"http://should-never-resolve.invalid/"}`},
				{Type: llm.SEMessageDelta, StopReason: "tool_use"},
			}
		}
		for _, event := range events {
			if !yield(event, nil) {
				return
			}
		}
	}}
	ca := NewChatAgent(provider, "fixture", t.TempDir(), nil, 0)
	ca.SetProxy("http://original.invalid", "")
	perRun := ca.WithProxy("http://task.invalid", "")
	if ca.proxyAddr != "http://original.invalid" || perRun.proxyAddr != "http://task.invalid" {
		t.Fatal("per-task proxy mutated shared agent")
	}
	ctx := WithRunInfo(t.Context(), RunInfo{TaskID: 42})
	if _, err := perRun.Chat(ctx, "retester", "retest-fixture", "fetch", 2, 0, false, nil); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("late SDK WebFetch bypassed controlled tool: calls=%d", calls)
	}
}
