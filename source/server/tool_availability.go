package server

import (
	"context"
	"encoding/json"
	"errors"

	actool "github.com/Autumn-27/norma/tool"
)

type unavailableTool struct{ actool.CoreTool }

func (t *unavailableTool) ValidateCall(context.Context) error {
	return errors.New("工具配置数据源不可用，本次执行已停止；请在数据库恢复后重试")
}

func (t *unavailableTool) Call(ctx context.Context, _ json.RawMessage, _ *actool.ToolContext) (actool.Result, error) {
	return actool.Result{}, t.ValidateCall(ctx)
}
