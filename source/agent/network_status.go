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
		"查询本任务的真实网络配置、调度策略、出口验证与未受管工具。round_robin 表示服务端为新请求自动轮询同池合格节点，node 只是下一可用节点快照，不是固定绑定；明确代理连接失败时服务端可对安全请求有限换节点重试，耗尽或凭据不可用时停止。fixed 表示用户手动固定节点。configured 不代表全部流量强制代理；不得用其他工具绕过策略或改用直连。返回信息不包含供应商凭据。",
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
