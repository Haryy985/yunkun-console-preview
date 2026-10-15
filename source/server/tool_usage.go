package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	actool "github.com/Autumn-27/norma/tool"

	"yunkun/console/agent"
	"yunkun/console/db"
)

type toolUsageRecorder interface {
	InsertToolUsage(*db.ToolUsage) error
}

// meteredTool records one row immediately before a catalog tool is invoked. It
// embeds the resolved tool so schema overrides, defaults and permission behavior
// remain unchanged.
type meteredTool struct {
	actool.CoreTool
	recorder toolUsageRecorder
	toolKey  string
	agentKey string
	ri       agent.RunInfo
}

func meterTool(t actool.CoreTool, recorder toolUsageRecorder, toolKey, agentKey string, ri agent.RunInfo) actool.CoreTool {
	if recorder == nil {
		return t
	}
	return &meteredTool{CoreTool: t, recorder: recorder, toolKey: toolKey, agentKey: agentKey, ri: ri}
}

func (m *meteredTool) ValidateCall(ctx context.Context) error {
	if catalog, ok := m.recorder.(interface {
		GetToolContext(context.Context, string) (*db.Tool, error)
	}); ok {
		checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		row, err := catalog.GetToolContext(checkCtx, m.toolKey)
		cancel()
		if err != nil {
			return fmt.Errorf("读取工具授权失败，执行已停止: %w", err)
		}
		if row == nil || !row.Enabled || !contains(row.Agents, m.agentKey) {
			return fmt.Errorf("工具 %s 已停用或不再授权给当前 Agent", m.toolKey)
		}
	}
	return nil
}

func (m *meteredTool) Call(ctx context.Context, input json.RawMessage, tc *actool.ToolContext) (actool.Result, error) {
	if err := m.ValidateCall(ctx); err != nil {
		return actool.Result{}, err
	}
	if err := m.recorder.InsertToolUsage(&db.ToolUsage{
		ToolKey:       m.toolKey,
		AgentKey:      m.agentKey,
		TaskID:        m.ri.TaskID,
		ExplorationID: m.ri.ExplorationID,
		IntentID:      m.ri.IntentID,
		SessionID:     m.ri.SessionID,
	}); err != nil {
		log.Printf("[toolusage] insert %s: %v", m.toolKey, err)
	}
	return m.CoreTool.Call(ctx, input, tc)
}
