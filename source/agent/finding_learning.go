package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"yunkun/console/db"
)

type findingLearning struct {
	Reusable        bool   `json:"reusable"`
	KnowledgeBaseID int64  `json:"knowledge_base_id"`
	Title           string `json:"title"`
	Method          string `json:"method"`
	Applicability   string `json:"applicability"`
	Limitations     string `json:"limitations"`
}

var sensitiveLearning = regexp.MustCompile(`(?i)(https?://|(?:authorization|cookie|password|passwd|secret|api[_-]?key|access[_-]?token)\s*[:=]|bearer\s+\S+|-----BEGIN [A-Z ]+PRIVATE KEY-----)`)

func (t *ToolSet) canProposeRAG(ctx context.Context) error {
	row, err := t.ragDB.GetToolContext(ctx, "propose_rag_change")
	if err != nil {
		return fmt.Errorf("读取知识提案权限失败: %w", err)
	}
	if row == nil || !row.Enabled {
		return fmt.Errorf("知识提案工具已禁用")
	}
	agentID := strings.TrimSpace(RunInfoFrom(ctx).AgentID)
	if agentID == "" {
		agentID = strings.TrimSpace(t.worker)
	}
	for _, allowed := range row.Agents {
		if allowed == agentID {
			return nil
		}
	}
	return fmt.Errorf("当前 Agent 未获知识提案权限")
}

func (l findingLearning) proposal(taskID, findingID int64, agentID string) (db.RAGChangeProposal, error) {
	if !l.Reusable {
		return db.RAGChangeProposal{}, nil
	}
	if l.KnowledgeBaseID <= 0 || strings.TrimSpace(l.Title) == "" ||
		strings.TrimSpace(l.Method) == "" || strings.TrimSpace(l.Applicability) == "" ||
		strings.TrimSpace(l.Limitations) == "" ||
		len(l.Title) > 200 || len(l.Method) > 4000 ||
		len(l.Applicability) > 1000 || len(l.Limitations) > 1000 {
		return db.RAGChangeProposal{}, fmt.Errorf("可复用经验须指定已授权知识库、标题、方法、适用条件和局限")
	}
	if sensitiveLearning.MatchString(l.Title + "\n" + l.Method + "\n" + l.Applicability + "\n" + l.Limitations) {
		return db.RAGChangeProposal{}, fmt.Errorf("知识提案包含 URL 或疑似凭据，请只提交脱敏后的通用方法")
	}
	content := fmt.Sprintf("## 验证方法\n\n%s\n\n## 适用条件\n\n%s\n\n## 局限与待复核点\n\n%s",
		strings.TrimSpace(l.Method), strings.TrimSpace(l.Applicability), strings.TrimSpace(l.Limitations))
	return db.RAGChangeProposal{
		KnowledgeBaseID: l.KnowledgeBaseID, TaskID: taskID, AgentID: agentID,
		Kind: "create", Title: strings.TrimSpace(l.Title), Content: content,
		Rationale: fmt.Sprintf("来自任务 #%d 已验证漏洞 #%d；请人工审核通用性与脱敏。", taskID, findingID),
		SourceRef: fmt.Sprintf("finding:%d", findingID),
	}, nil
}
