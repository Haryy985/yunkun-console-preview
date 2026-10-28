package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"

	actool "github.com/Autumn-27/norma/tool"
	"yunkun/console/agent"
	"yunkun/console/db"
	"yunkun/console/traffic"
)

func (s *Server) seedFindingWorkflowTools() {
	s.refreshDefaultFindingDescription()
	s.restoreOriginalFindingSchema()
	const hostSearchDescriptionFlag = "finding_workflow_tools_v3_host_search_description"
	if value, _, _ := s.m.pg.GetSetting(hostSearchDescriptionFlag); value != "true" {
		// Only replace the original built-in text. A user-edited description is
		// authoritative and must survive upgrades.
		legacy := "查询记录代理已抓取的目标流量（必须指定 host，可再按 URL 子串或正文关键词过滤）。body_contains 会在已抓取的请求/响应头与正文中做全文搜索，支持任意子串和中文（至少 3 个字符），可用来找响应里的密码、密钥、报错、内网地址等。仅返回极轻量索引(id/method/url/status/resp_len)，不含任何响应内容。默认只返回 3 条、每页最多 10 条；结果多时用 page 翻页（page=0 起）；要看某条的请求/响应原文用 traffic_get(id)。回看已访问资源、找端点先用它，避免重复 curl 同一 URL。"
		if _, err := s.m.pg.Exec(`UPDATE tools SET description=$1,updated_at=now() WHERE key='traffic_search' AND system AND description=$2`, traffic.TrafficSearchDescription, legacy); err != nil {
			// Log and leave the flag unset so the next startup retries; do not
			// return, or a transient error here would also skip the reporter
			// migration below — the two are independent.
			log.Printf("[evidence] upgrade traffic_search description: %v", err)
		} else {
			_ = s.m.pg.SetSetting(hostSearchDescriptionFlag, "true")
		}
	}
	const flag = "finding_workflow_tools_v2_reporter"
	if value, _, _ := s.m.pg.GetSetting(flag); value == "true" {
		return
	}
	for _, key := range []string{"report_finding", "add_hint", "add_task_hint"} {
		row, err := s.m.pg.GetTool(key)
		if err != nil {
			log.Printf("[evidence] load %s: %v", key, err)
			return
		}
		if row == nil || !row.System {
			continue
		}
		var schema map[string]any
		if err := json.Unmarshal(row.Schema, &schema); err != nil {
			log.Printf("[evidence] invalid schema for %s: %v", key, err)
			return
		}
		if schema == nil {
			log.Printf("[evidence] missing object schema for %s", key)
			return
		}
		props := objectProperty(schema, "properties")
		if key == "report_finding" {
			if _, exists := props["evidence_hint_id"]; !exists {
				props["evidence_hint_id"] = map[string]any{"type": "integer", "description": "可选：本任务中对应此漏洞的 hint ID；读取该提示保存的 traffic_refs 一并绑定，无提示时省略"}
			}
		} else {
			if _, exists := props["traffic_refs"]; !exists {
				props["traffic_refs"] = agent.HintTrafficSchema()
			}
			hints := objectProperty(props, "hints")
			if _, exists := hints["type"]; !exists {
				hints["type"] = "array"
			}
			items := objectProperty(hints, "items")
			if _, ok := items["type"]; !ok {
				items["type"] = "object"
			}
			itemProps := objectProperty(items, "properties")
			for name, value := range map[string]any{"text": strParam("提示内容"), "asset_ids": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}}, "traffic_refs": agent.HintTrafficSchema()} {
				if _, exists := itemProps[name]; !exists {
					itemProps[name] = value
				}
			}
		}
		raw, _ := json.Marshal(schema)
		result, err := s.m.pg.Exec(`UPDATE tools SET schema=$2::jsonb,updated_at=now() WHERE key=$1 AND system AND schema=$3::jsonb`, key, string(raw), string(row.Schema))
		if err != nil {
			log.Printf("[evidence] upgrade %s: %v", key, err)
			return
		}
		if n, _ := result.RowsAffected(); n != 1 {
			return
		} // preserve concurrent user edits
	}
	// Upgrade only the original default binding. Customized lists and enabled
	// flags survive; the one-time flag also preserves future user unbinding.
	readers := `["worker","reporter"]`
	for _, key := range []string{"traffic_search", "traffic_get", "traffic_blob"} {
		if _, err := s.m.pg.Exec(`UPDATE tools SET agents=$2::jsonb WHERE key=$1 AND system AND (agents='["worker"]'::jsonb OR (agents @> '["worker","planner","mainagent","auto","pentest"]'::jsonb AND jsonb_array_length(agents)=5))`, key, readers); err != nil {
			return
		}
	}
	if _, err := s.m.pg.Exec(`UPDATE tools SET agents=$1::jsonb WHERE key='get_finding_traffic' AND system AND agents @> '["auto","reporter"]'::jsonb AND jsonb_array_length(agents)=2`, `["auto","reporter","worker","planner","mainagent","pentest"]`); err != nil {
		return
	}
	// Replace the previous code default only; preserve customized binding lists.
	if _, err := s.m.pg.Exec(`UPDATE tools SET agents='["reporter"]'::jsonb WHERE key='bind_finding_traffic' AND system AND agents @> '["worker","planner","mainagent","auto","pentest"]'::jsonb AND jsonb_array_length(agents)=5`); err != nil {
		return
	}
	if err := s.m.pg.AddAgentToToolBinding("reporter", []string{"bind_finding_traffic"}); err != nil {
		return
	}
	_ = s.m.pg.SetSetting(flag, "true")
}

const previousFindingDescription = "仅记录证据齐全且有实际安全影响的已确认漏洞。不要把 CORS、IDOR、BOLA 作为独立正式漏洞；它们只有在已证明数据读取、权限提升或重大业务影响链时才按最终影响报告，否则用 record_fact 保存线索。validation 填写授权、目标、身份、基线、受控变异、实际差异、影响和清理；提供本任务成功工具结果的 evidence_step_ids 或核实的 traffic_refs。缺失则用 record_fact 记录待验证线索，不得硬报。返回 finding_id 与 finding_node_id。"
const restrictedFindingDescription = "仅记录证据齐全且有实际安全影响的已确认漏洞。本项目排除 CORS、IDOR、BOLA 专项和正式报告；相关观察只用 record_fact 保存线索，不得改标签或夸大影响绕过排除规则。validation 填写授权、目标、身份、基线、受控变异、实际差异、影响和清理；提供本任务成功工具结果的 evidence_step_ids 或核实的 traffic_refs。缺失则用 record_fact 记录待验证线索，不得硬报。返回 finding_id 与 finding_node_id。"

// Update only the previously shipped default. A customized tool description
// remains the administrator's choice and is never replaced here.
func (s *Server) refreshDefaultFindingDescription() {
	for _, seed := range agent.BuiltinToolSeeds() {
		if seed.Key != "report_finding" {
			continue
		}
		if _, err := s.m.pg.Exec(`UPDATE tools SET description=$1,updated_at=now()
WHERE key='report_finding' AND system AND description IN ($2,$3)`, seed.Desc, previousFindingDescription, restrictedFindingDescription); err != nil {
			log.Printf("[finding] refresh default description: %v", err)
		}
		return
	}
}

// Remove only finding-specific fields added by the review workflow.
func (s *Server) restoreOriginalFindingSchema() {
	row, err := s.m.pg.GetTool("report_finding")
	if err != nil || row == nil || !row.System {
		log.Printf("[finding] load report_finding schema: %v", err)
		return
	}
	var schema map[string]any
	if err := json.Unmarshal(row.Schema, &schema); err != nil {
		log.Printf("[finding] decode report_finding schema: %v", err)
		return
	}
	if schema == nil {
		return
	}
	existing := objectProperty(schema, "properties")
	changed := false
	if _, ok := existing["validation"]; ok {
		delete(existing, "validation")
		changed = true
	}
	if _, ok := existing["learning"]; ok {
		delete(existing, "learning")
		changed = true
	}
	required, _ := schema["required"].([]any)
	kept := make([]any, 0, len(required))
	for _, name := range required {
		if name != "validation" && name != "learning" {
			kept = append(kept, name)
		} else {
			changed = true
		}
	}
	schema["required"] = kept
	if !changed {
		return
	}
	raw, _ := json.Marshal(schema)
	if string(raw) == string(row.Schema) {
		return
	}
	res, err := s.m.pg.Exec(`UPDATE tools SET schema=$2::jsonb, updated_at=now()
WHERE key='report_finding' AND system AND schema=$1::jsonb`, string(row.Schema), string(raw))
	if err != nil {
		log.Printf("[finding] restore report_finding schema: %v", err)
		return
	}
	_, _ = res.RowsAffected()
}

func objectProperty(parent map[string]any, key string) map[string]any {
	value, ok := parent[key].(map[string]any)
	if !ok {
		value = map[string]any{}
		parent[key] = value
	}
	return value
}

func (s *Server) agentFindingTrafficAccess(ctx context.Context, id int64, write bool) error {
	if id <= 0 {
		return errors.New("finding_id 必须为独立漏洞记录 ID；不是探索节点 ID")
	}
	f, err := s.m.pg.GetFinding(id)
	if err != nil {
		return err
	}
	if f == nil {
		return fmt.Errorf("%w：finding_id=%d。证据工具使用独立漏洞记录 ID，请从 list_task_findings / get_task_node_detail 的 finding_id 字段读取；不要传 id / finding_node_id", db.ErrFindingNotFound, id)
	}
	if ri := agent.RunInfoFrom(ctx); ri.TaskID > 0 {
		task := s.m.ResolveTask(strconv.FormatInt(ri.TaskID, 10))
		if task == nil {
			return errors.New("任务不存在")
		}
		_, inherited, allowed := findingProvenanceInTask(task, f.TaskID)
		if !allowed {
			return errors.New("当前任务不可读取该漏洞")
		}
		if write && inherited {
			return errors.New("继承漏洞的流量证据只读，请到来源任务修改")
		}
	}
	return nil
}

func (s *Server) toolBindFindingTraffic() actool.CoreTool {
	return wrTool("bind_finding_traffic", "为已登记漏洞补绑经核实的真实 HTTP 流量。finding_id 使用独立漏洞记录 ID；不要传探索节点 ID。同批引用全部成功或全部失败，重复引用不覆盖已有说明。补绑会使已有报告标记待更新；不要为补包重新探测或重复创建漏洞。",
		objSchema(map[string]any{"finding_id": strParam("独立漏洞记录 ID，从 list_task_findings / get_task_node_detail 的 finding_id 字段读取"), "traffic_refs": agent.HintTrafficSchema()}, "finding_id", "traffic_refs"),
		func(ctx context.Context, raw json.RawMessage) (actool.Result, error) {
			if !s.m.pg.GetBool(settingAgentTrafficBinding, false) {
				return actool.Errorf("Agent 自动绑定流量已关闭；请在系统设置开启，或使用页面人工绑定。"), nil
			}
			var args struct {
				FindingID json.RawMessage `json:"finding_id"`
				Refs      []db.TrafficRef `json:"traffic_refs"`
			}
			if err := json.Unmarshal(raw, &args); err != nil {
				return actool.Errorf(err.Error()), nil
			}
			id := parseProfileID(args.FindingID)
			if err := s.agentFindingTrafficAccess(ctx, id, true); err != nil {
				return actool.Errorf(err.Error()), nil
			}
			if len(args.Refs) == 0 {
				return actool.Errorf("补绑需要至少一条已核实的 traffic_refs；无流量无需调用此工具"), nil
			}
			list, err := s.evidenceStore().Bind(ctx, id, args.Refs)
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			return jsonResult(trafficSummary(list))
		})
}
