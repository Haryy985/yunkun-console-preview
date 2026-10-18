package agent

import (
	"fmt"
	"strings"

	"yunkun/console/db"
)

// FindingValidation is the reviewable proof contract for Agent-created findings.
// It does not replace the original captured traffic or execution trace.
type FindingValidation struct {
	Authorization   string  `json:"authorization"`
	Asset           string  `json:"asset"`
	Identity        string  `json:"identity"`
	Preconditions   string  `json:"preconditions"`
	Baseline        string  `json:"baseline"`
	Mutation        string  `json:"mutation"`
	ObservedDelta   string  `json:"observed_delta"`
	SecurityImpact  string  `json:"security_impact"`
	Cleanup         string  `json:"cleanup"`
	EvidenceStepIDs []int64 `json:"evidence_step_ids"`
}

func validateFindingProof(v FindingValidation, refs []db.TrafficRef, ts *db.ExplorationStore, intentID int64) error {
	for _, field := range []struct{ name, value string }{
		{"authorization", v.Authorization}, {"asset", v.Asset},
		{"identity", v.Identity}, {"preconditions", v.Preconditions},
		{"baseline", v.Baseline}, {"mutation", v.Mutation},
		{"observed_delta", v.ObservedDelta}, {"security_impact", v.SecurityImpact},
		{"cleanup", v.Cleanup},
	} {
		if strings.TrimSpace(field.value) == "" || len(field.value) > 2000 {
			return fmt.Errorf("validation.%s 不能为空且不得超过 2000 字符；证据不足请先记待验证事实", field.name)
		}
	}
	if len(refs) == 0 && len(v.EvidenceStepIDs) == 0 {
		return fmt.Errorf("validation.evidence_step_ids 或已核实的 traffic_refs 至少提供一种真实证据引用")
	}
	if len(v.EvidenceStepIDs) > 10 {
		return fmt.Errorf("validation.evidence_step_ids 最多 10 条")
	}
	if len(v.EvidenceStepIDs) > 0 {
		if ts == nil || intentID <= 0 {
			return fmt.Errorf("引用执行步骤时必须明确本任务的 intent_id")
		}
		seen := make(map[int64]bool, len(v.EvidenceStepIDs))
		for _, id := range v.EvidenceStepIDs {
			if id <= 0 || seen[id] {
				return fmt.Errorf("validation.evidence_step_ids 包含无效或重复 ID")
			}
			seen[id] = true
		}
		steps, err := ts.ActivityByIDs(v.EvidenceStepIDs)
		if err != nil {
			return fmt.Errorf("核实证据步骤失败: %w", err)
		}
		if len(steps) != len(seen) {
			return fmt.Errorf("证据步骤不存在或不属于当前任务")
		}
		for _, step := range steps {
			if step.Kind != "tool_result" || step.IsError || step.NodeID == nil ||
				*step.NodeID != intentID || strings.TrimSpace(step.Detail) == "" ||
				step.Tool == "report_finding" || step.Tool == "record_fact" ||
				step.Tool == "propose_rag_change" {
				return fmt.Errorf("证据步骤 #%d 不是本意图成功执行的工具结果", step.ID)
			}
		}
	}
	return nil
}

func findingProofText(v FindingValidation, evidence string) string {
	return fmt.Sprintf("授权依据：%s\n目标：%s\n测试身份：%s\n前置条件：%s\n正常基线：%s\n受控变异：%s\n实际差异：%s\n安全影响：%s\n清理与回滚：%s\n执行证据步骤：%v\n\n原始验证记录：\n%s",
		v.Authorization, v.Asset, v.Identity, v.Preconditions, v.Baseline,
		v.Mutation, v.ObservedDelta, v.SecurityImpact, v.Cleanup, v.EvidenceStepIDs, evidence)
}
