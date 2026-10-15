package agent

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"yunkun/console/db"
	"yunkun/console/intercept"
	"yunkun/console/llmrec"
	"yunkun/console/sidequestion"
	"github.com/Autumn-27/norma/agentcore"
	"github.com/Autumn-27/norma/harness"
	"github.com/Autumn-27/norma/llm"
)

// captureRun drives one agent turn-to-completion over Session.Prompt and emits a
// coalesced ActivityRecord per execution step (tool_use / tool_result / text /
// thinking / result). It is shared by every LLM agent in the system (worker,
// planner, …) so their execution is visible instead of a black box — the old
// agentcore.Run discarded every event. The emitted records carry only
// Kind/Tool/ToolUseID/IsError/Summary/Detail; the caller's emit fills in
// IntentID/Worker. Returns the final assistant text + terminal error.
//
// KindText/KindThinking arrive as streaming deltas (one event per fragment); a
// contiguous run is coalesced into a single record so the trace shows whole
// messages, not dozens of fragments.
func captureRun(ctx context.Context, opts agentcore.Options, input string, emit func(db.Activity)) (string, harness.TerminalReason, error) {
	s := newRunSession(ctx, opts)
	defer s.Close() // release the session's background-task manager (temp dir + processes)
	return captureRunSession(ctx, s, input, emit)
}

// captureRunSession is captureRun over an existing session, so a caller can run
// multiple prompts on the SAME conversation (e.g. a settlement round that reuses
// the worker's accumulated context after the main run hit max_turns).
func captureRunSession(ctx context.Context, s *agentcore.Session, input string, emit func(db.Activity)) (string, harness.TerminalReason, error) {
	ctx, auditTrace := intercept.WithTrace(ctx, input, approvalHistory(s.Messages()))
	ctx, requestTrace := llmrec.WithRequestTrace(ctx)
	defer auditTrace.Finish()
	var reason harness.TerminalReason
	rec := func(r db.Activity) {
		if r.Kind == "text" || r.Kind == "tool_result" {
			auditTrace.Append(db.InterceptContextEntry{Kind: r.Kind, Tool: r.Tool, ToolUseID: r.ToolUseID, Text: r.Detail, IsError: r.IsError})
		}
		if emit != nil {
			emit(r)
		}
	}
	toolNames := map[string]string{} // tool_use id -> name, to label results
	toolRequests := map[string]string{}
	toolInputs := map[string]string{}

	var tbuf strings.Builder
	var tkind string
	flush := func() {
		if tbuf.Len() == 0 {
			return
		}
		s := strings.TrimSpace(tbuf.String())
		k := tkind
		tbuf.Reset()
		tkind = ""
		if s != "" {
			rec(db.Activity{Kind: k, Summary: firstLine(s, 200), Detail: s})
		}
	}
	addDelta := func(kind, text string) {
		if text == "" {
			return
		}
		if tkind != "" && tkind != kind {
			flush()
		}
		tkind = kind
		tbuf.WriteString(text)
	}
	lastTool := &runTrace{startedAt: time.Now()}
	var lastUsage *llm.Usage

	var finalText string
	var rerr error
	for ev, err := range s.Prompt(ctx, input) {
		if err != nil {
			flush()
			if ctx.Err() != nil { // engine/user cancellation, not a provider failure
				sum, detail := terminalText(ctx, &harness.Terminal{Reason: reason, Err: ctx.Err()}, lastTool)
				rec(activityWithUsage(db.Activity{Kind: "result", Summary: firstLine(sum, 400), Detail: detail}, lastUsage))
				return finalText, reason, ctx.Err()
			}
			rec(activityWithUsage(db.Activity{Kind: "result", IsError: true, Summary: "执行出错: " + err.Error(), Detail: err.Error()}, lastUsage))
			return finalText, reason, err
		}
		switch ev.Kind {
		case harness.KindToolUse:
			if ev.ToolUse == nil {
				continue
			}
			flush()
			toolNames[ev.ToolUse.ID] = ev.ToolUse.Name
			toolRequests[ev.ToolUse.ID] = requestTrace.ID()
			in := string(ev.ToolUse.Input)
			toolInputs[ev.ToolUse.ID] = in
			lastTool.start(ev.ToolUse.ID, ev.ToolUse.Name, in)
			auditTrace.Start(ev.ToolUse.ID, ev.ToolUse.Name, ev.ToolUse.Input)
			rec(db.Activity{Kind: "tool_use", Tool: ev.ToolUse.Name, ToolUseID: ev.ToolUse.ID,
				Summary: ev.ToolUse.Name + " " + firstLine(in, 200), Detail: in,
				Metadata: toolRequestMetadata(toolRequests[ev.ToolUse.ID], nil)})
		case harness.KindToolResult:
			if ev.ToolResult == nil {
				continue
			}
			flush()
			out := blocksText(ev.ToolResult.Content)
			lastTool.done(ev.ToolResult.ToolUseID)
			tool := toolNames[ev.ToolResult.ToolUseID]
			input := toolInputs[ev.ToolResult.ToolUseID]
			delete(toolInputs, ev.ToolResult.ToolUseID)
			auditTrace.Complete(ev.ToolResult.ToolUseID, out, ev.ToolResult.IsError)
			rec(db.Activity{Kind: "tool_result", Tool: tool, ToolUseID: ev.ToolResult.ToolUseID,
				IsError: ev.ToolResult.IsError, Summary: firstLine(out, 200), Detail: out,
				Metadata: toolRequestMetadata(toolRequests[ev.ToolResult.ToolUseID],
					toolResultRecordRef(tool, input, out, ev.ToolResult.IsError))})
		case harness.KindText:
			addDelta("text", ev.Text)
		case harness.KindThinking:
			addDelta("thinking", ev.Text)
		case harness.KindUsage:
			// live cumulative token usage (per model turn). Emitted as a non-rendered
			// "usage" activity carrying only the token fields; the UI uses the latest
			// one for a running session's live token count. Don't flush() here — the
			// buffered final-answer text must stay for the KindResult de-dup.
			if ev.Usage != nil {
				u := *ev.Usage
				lastUsage = &u
				rec(db.Activity{Kind: "usage",
					InputTokens: &u.InputTokens, OutputTokens: &u.OutputTokens,
					CacheReadTokens: &u.CacheReadTokens, CacheWriteTokens: &u.CacheWriteTokens})
			}
		case harness.KindResult:
			if ev.Terminal != nil {
				if ev.Terminal.Reason != harness.ReasonAbortedStreaming {
					sidequestion.Finish(ctx, ev.Terminal.Messages)
				}
				finalText = ev.Terminal.Text
				reason = ev.Terminal.Reason
				// the buffered tail text usually equals Terminal.Text (final answer);
				// drop it to avoid a duplicate record, the result row carries it.
				if tkind == "text" && strings.TrimSpace(tbuf.String()) == strings.TrimSpace(ev.Terminal.Text) {
					tbuf.Reset()
					tkind = ""
				}
				flush() // flush any trailing thinking / non-final text
				sum, detail := ev.Terminal.Text, ev.Terminal.Text
				if sum == "" || ev.Terminal.Reason == harness.ReasonAbortedTools || ev.Terminal.Reason == harness.ReasonAbortedStreaming {
					sum, detail = terminalText(ctx, ev.Terminal, lastTool)
				}
				u := ev.Terminal.Usage // cumulative token usage for this session
				rec(db.Activity{Kind: "result", IsError: ev.Terminal.Err != nil,
					Summary: firstLine(sum, 400), Detail: detail,
					InputTokens: &u.InputTokens, OutputTokens: &u.OutputTokens,
					CacheReadTokens: &u.CacheReadTokens, CacheWriteTokens: &u.CacheWriteTokens})
				if ev.Terminal.Err != nil {
					rerr = ev.Terminal.Err
				}
			}
		}
	}
	flush() // safety: any unflushed text if the stream ended without KindResult
	return finalText, reason, rerr
}

func toolRequestMetadata(requestID string, existing json.RawMessage) json.RawMessage {
	if requestID == "" {
		return existing
	}
	metadata := map[string]any{"llm_request_id": requestID}
	if len(existing) > 0 {
		var ref struct {
			RecordRef json.RawMessage `json:"record_ref"`
		}
		if json.Unmarshal(existing, &ref) == nil && len(ref.RecordRef) > 0 {
			metadata["record_ref"] = ref.RecordRef
		}
	}
	raw, _ := json.Marshal(metadata)
	return raw
}

// toolResultRecordRef links the captured call ID to a persisted result without
// copying tool output (which can contain credentials) into activity metadata.
func toolResultRecordRef(tool, input, output string, isError bool) json.RawMessage {
	if isError {
		return nil
	}
	var ref struct {
		Kind            string `json:"kind"`
		ID              string `json:"id"`
		NodeID          string `json:"node_id,omitempty"`
		KnowledgeBaseID string `json:"knowledge_base_id,omitempty"`
	}
	switch tool {
	case "traffic_get":
		var args struct {
			ID string `json:"id"`
		}
		if json.Unmarshal([]byte(input), &args) != nil || args.ID == "" ||
			len(args.ID) > 128 || strings.TrimSpace(args.ID) != args.ID ||
			!strings.HasPrefix(output, "=== REQUEST ===\n") ||
			!strings.Contains(output, "\n\n=== RESPONSE ===\n") {
			return nil
		}
		ref.Kind, ref.ID = "traffic_read", args.ID
	case "rag_search":
		var result struct {
			RetrievalLogID int64 `json:"retrieval_log_id"`
		}
		if json.Unmarshal([]byte(output), &result) != nil || result.RetrievalLogID <= 0 {
			return nil
		}
		ref.Kind, ref.ID = "rag_retrieval", strconv.FormatInt(result.RetrievalLogID, 10)
	case "propose_rag_change":
		var result struct {
			ProposalID      int64 `json:"proposal_id"`
			KnowledgeBaseID int64 `json:"knowledge_base_id"`
		}
		if json.Unmarshal([]byte(output), &result) != nil || result.ProposalID <= 0 || result.KnowledgeBaseID <= 0 {
			return nil
		}
		ref.Kind, ref.ID = "rag_proposal", strconv.FormatInt(result.ProposalID, 10)
		ref.KnowledgeBaseID = strconv.FormatInt(result.KnowledgeBaseID, 10)
	case "report_finding":
		header, body, ok := strings.Cut(output, "\n")
		if !ok || !strings.HasPrefix(header, "finding recorded: ") {
			return nil
		}
		var result struct {
			FindingID     string `json:"finding_id"`
			FindingNodeID string `json:"finding_node_id"`
		}
		if json.Unmarshal([]byte(body), &result) != nil {
			return nil
		}
		id, idErr := strconv.ParseInt(result.FindingID, 10, 64)
		nodeID, nodeErr := strconv.ParseInt(result.FindingNodeID, 10, 64)
		if idErr != nil || nodeErr != nil || id <= 0 || nodeID <= 0 ||
			header != "finding recorded: "+result.FindingNodeID {
			return nil
		}
		ref.Kind, ref.ID, ref.NodeID = "finding", result.FindingID, result.FindingNodeID
	default:
		return nil
	}
	raw, _ := json.Marshal(map[string]any{"record_ref": ref})
	return raw
}

func activityWithUsage(activity db.Activity, usage *llm.Usage) db.Activity {
	if usage == nil {
		return activity
	}
	u := *usage
	activity.InputTokens = &u.InputTokens
	activity.OutputTokens = &u.OutputTokens
	activity.CacheReadTokens = &u.CacheReadTokens
	activity.CacheWriteTokens = &u.CacheWriteTokens
	return activity
}

// blocksText concatenates the text of a tool-result's content blocks.
func blocksText(blocks []llm.ContentBlock) string {
	var b strings.Builder
	for _, bl := range blocks {
		if bl.Type == llm.BlockText && bl.Text != "" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(bl.Text)
		}
	}
	return b.String()
}

// firstLine returns a single-line, rune-capped preview for the summary column.
func firstLine(s string, max int) string {
	s = strings.TrimSpace(s)
	if before, _, found := strings.Cut(s, "\n"); found {
		s = before
	}
	if utf8.RuneCountInString(s) > max {
		s = string([]rune(s)[:max]) + "…"
	}
	return s
}

// Preserve the recorded session's visible messages, excluding thinking blocks.
// This is audit context; the judge receives only bounded, paired execution
// evidence selected from it, never assistant prose or thinking blocks.
func approvalHistory(messages []llm.Message) []db.InterceptContextEntry {
	var entries []db.InterceptContextEntry
	for _, message := range messages {
		for _, block := range message.Content {
			entry := db.InterceptContextEntry{Kind: string(message.Role)}
			switch block.Type {
			case llm.BlockText:
				entry.Text = block.Text
			case llm.BlockToolUse:
				entry.Kind, entry.Tool, entry.ToolUseID, entry.Text = "tool_use", block.Name, block.ID, string(block.Input)
			case llm.BlockToolResult:
				entry.Kind, entry.ToolUseID, entry.Text, entry.IsError = "tool_result", block.ToolUseID, blocksText(block.Content), block.IsError
			default:
				continue
			}
			entries = append(entries, entry)
		}
	}
	return entries
}
