package agent

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"yunkun/console/db"
	"yunkun/console/rag"
	actool "github.com/Autumn-27/norma/tool"
)

func retrievalConversationID(sessionID string) *int64 {
	if !strings.HasPrefix(sessionID, "conv-") {
		return nil
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(sessionID, "conv-"), 10, 64)
	if err != nil || id <= 0 {
		return nil
	}
	return &id
}

func ragCitations(results []*db.RAGSearchResult) []map[string]any {
	citations := make([]map[string]any, 0, len(results))
	for _, result := range results {
		citations = append(citations, map[string]any{
			"chunk_id":      result.ChunkID,
			"document_id":   result.DocumentID,
			"document_name": result.DocumentName,
			"source_uri":    result.SourceURI,
			"heading_path":  result.HeadingPath,
			"page_from":     result.PageFrom,
			"page_to":       result.PageTo,
			"line_from":     result.LineFrom,
			"line_to":       result.LineTo,
		})
	}
	return citations
}

func (t *ToolSet) ragSearch() actool.CoreTool {
	return readTool(
		"rag_search",
		"检索本任务已授权且启用的 RAG 知识库。未指定时按授权时间、知识库 ID 选择首个，可用 available_resources 查询。混合排序后返回短摘录、来源、chunk_id 和 retrieval_log_id；单条正文最多 1800 UTF-8 字节、整份 JSON 最多 32 KiB，content_truncated 表示正文不完整，omitted_count 表示省略或去重。不能据摘录推断全文条件，也不代替真实目标验证。",
		obj(map[string]any{
			"query": map[string]any{
				"type": "string", "description": "要检索的问题或关键词",
			},
			"knowledge_base_id": map[string]any{
				"type": "integer", "description": "知识库 id，可选；不传则选择当前任务已授权且启用的首个知识库",
			},
			"top_k": map[string]any{
				"type": "integer", "description": "返回数量，默认 5，最大 20",
			},
		}, "query"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			if t.ragDB == nil {
				return actool.Errorf("rag_search 未启用: RAG 数据库未初始化"), nil
			}
			var args struct {
				Query           string `json:"query"`
				KnowledgeBaseID int64  `json:"knowledge_base_id"`
				TopK            int    `json:"top_k"`
			}
			if err := json.Unmarshal(in, &args); err != nil {
				return actool.Errorf("invalid input: " + err.Error()), nil
			}
			args.Query = strings.TrimSpace(args.Query)
			if args.Query == "" {
				return actool.Errorf("query 不能为空"), nil
			}
			if args.TopK <= 0 || args.TopK > 20 {
				args.TopK = 5
			}
			taskID := t.taskID
			runInfo := RunInfoFrom(ctx)
			if taskID <= 0 {
				taskID = runInfo.TaskID
			}
			if taskID <= 0 {
				return actool.Errorf("需要任务上下文及明确授权才能检索知识库"), nil
			}
			var baseID int64
			if args.KnowledgeBaseID > 0 {
				baseID = args.KnowledgeBaseID
			} else {
				ids, err := t.ragDB.TaskRAGKnowledgeBaseIDs(taskID)
				if err != nil {
					return actool.Errorf("读取知识库失败: " + err.Error()), nil
				}
				if len(ids) > 0 {
					baseID = ids[0]
				}
				if baseID == 0 {
					return actool.Errorf("任务没有获准访问的知识库"), nil
				}
			}
			allowed, err := t.ragDB.TaskCanAccessRAGKnowledgeBase(taskID, baseID)
			if err != nil {
				return actool.Errorf("检查知识库授权失败: " + err.Error()), nil
			}
			if !allowed {
				return actool.Errorf("任务无权访问该知识库"), nil
			}
			base, err := t.ragDB.GetRAGKnowledgeBase(baseID)
			if err != nil {
				return actool.Errorf("读取知识库失败: " + err.Error()), nil
			}
			if base == nil || !base.Enabled {
				return actool.Errorf("知识库不存在或未启用"), nil
			}
			model := strings.TrimSpace(base.EmbeddingModel)
			var queryVector []float32
			if embedder, embedErr := rag.ResolveEmbedder(base.EmbeddingProvider, base.EmbeddingModel, 256, true); embedErr == nil {
				queryVector, _ = embedder.Embed(args.Query)
				model = embedder.Model()
			}
			started := time.Now()
			results, mode, err := t.ragDB.SearchRAGChunksHybrid(baseID, args.Query, model, queryVector, args.TopK)
			if err != nil {
				return actool.Errorf("RAG 检索失败: " + err.Error()), nil
			}
			latency := time.Since(started).Milliseconds()
			matchedCount := len(results)
			results, err = boundedRAGResults(baseID, mode, latency, results)
			if err != nil {
				return actool.Errorf("检索结果无法安全编码"), nil
			}
			agentID := strings.TrimSpace(t.worker)
			if agentID == "" {
				agentID = strings.TrimSpace(runInfo.AgentID)
			}
			if agentID == "" {
				agentID = "agent"
			}
			logID, err := t.ragDB.PublishTaskRAGRetrieval(ctx, taskID, baseID, agentID, retrievalConversationID(runInfo.SessionID), args.Query, args.TopK, mode, results, latency)
			if err != nil {
				if err == db.ErrTaskRAGAccessDenied {
					return actool.Errorf("检索期间知识库授权已撤销，结果已丢弃"), nil
				}
				return actool.Errorf("写入检索日志失败: " + err.Error()), nil
			}
			return jsonResult(ragSearchPayload(baseID, logID, mode, latency, results, matchedCount))
		},
	)
}

func (t *ToolSet) proposeRAGChange() actool.CoreTool {
	return writeTool("propose_rag_change",
		"向知识库提交新增或修订建议。每一条必须由管理员人工审核；提交后不会立即进入检索。提供可追溯的来源和理由，新发现的方法应标记待验证，不要包含凭据或敏感请求体。用户指定的任务目标和约束应使用任务工具，不应改写成 RAG。",
		obj(map[string]any{
			"knowledge_base_id":  intp("目标知识库 ID"),
			"kind":               map[string]any{"type": "string", "enum": []string{"create", "update"}},
			"target_document_id": intp("修订时的目标文档 ID；新增时不传"),
			"title":              str("标题"),
			"content":            str("拟发布的完整 Markdown 内容；修订时提供替换后的完整内容"),
			"rationale":          str("新增或修订理由，标注尚未验证的推测"),
			"source_ref":         str("证据来源或记录 ID，可供人工复核"),
		}, "knowledge_base_id", "kind", "title", "content", "rationale", "source_ref"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			if t.ragDB == nil {
				return actool.Errorf("RAG 数据库未初始化"), nil
			}
			taskID := t.taskID
			if taskID == 0 {
				taskID = RunInfoFrom(ctx).TaskID
			}
			if taskID <= 0 {
				return actool.Errorf("需要真实任务上下文才能提交知识提案"), nil
			}
			var args struct {
				KnowledgeBaseID  int64  `json:"knowledge_base_id"`
				Kind             string `json:"kind"`
				TargetDocumentID *int64 `json:"target_document_id"`
				Title            string `json:"title"`
				Content          string `json:"content"`
				Rationale        string `json:"rationale"`
				SourceRef        string `json:"source_ref"`
			}
			if err := json.Unmarshal(in, &args); err != nil {
				return actool.Errorf("invalid input: " + err.Error()), nil
			}
			agentID := strings.TrimSpace(t.worker)
			if agentID == "" {
				agentID = strings.TrimSpace(RunInfoFrom(ctx).AgentID)
			}
			if agentID == "" {
				agentID = "agent"
			}
			p, err := t.ragDB.CreateRAGProposal(db.RAGChangeProposal{
				KnowledgeBaseID: args.KnowledgeBaseID, TaskID: taskID, AgentID: agentID,
				Kind: args.Kind, TargetDocumentID: args.TargetDocumentID, Title: args.Title,
				Content: args.Content, Rationale: args.Rationale, SourceRef: args.SourceRef,
			})
			if err != nil {
				return actool.Errorf("提交知识提案失败: " + err.Error()), nil
			}
			return jsonResult(map[string]any{"proposal_id": p.ID, "status": p.Status, "knowledge_base_id": p.KnowledgeBaseID})
		})
}
