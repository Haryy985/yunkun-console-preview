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
		"查询本任务的真实网络配置、调度策略、出口验证与工具覆盖。round_robin 表示服务端为每个新受管 HTTP 请求/CONNECT 隧道自动轮询同池合格节点；明确代理连接失败时服务端会冷却故障节点并有限切换重试，耗尽或凭据不可用时停止，不跨池、不直连。fixed 表示用户手动固定节点。Bash 子进程通过 HTTP_PROXY/HTTPS_PROXY/ALL_PROXY 接入任务网关，但环境变量不能约束显式绕过代理或自行创建原始 socket 的程序；不得规避任务授权范围。返回信息不包含供应商凭据。",
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
