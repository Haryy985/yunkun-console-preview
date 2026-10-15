package agent

import (
	"context"
	"encoding/json"

	actool "github.com/Autumn-27/norma/tool"
)

// NetworkStatus is server-owned and never accepts a model-selected task ID.
var NetworkStatus func(taskID int64) (map[string]any, error)

func networkStatusTool(taskID int64) actool.CoreTool {
	return readTool("network_status",
		"查询本任务的真实网络配置、代理租约、最近出口验证与未受管工具。configured 仅表示已配置，不代表全部流量强制代理；绑定节点失效时停止相关请求，不得自行绕过或改用直连。返回信息不包含供应商凭据。",
		obj(map[string]any{}),
		func(_ context.Context, _ json.RawMessage) (actool.Result, error) {
			status, err := NetworkStatus(taskID)
			if err != nil {
				return actool.Errorf("network status unavailable"), nil
			}
			b, err := json.Marshal(status)
			if err != nil {
				return actool.Errorf("network status unavailable"), nil
			}
			return actool.Text(string(b)), nil
		})
}
