package llmrec

import (
	"context"
	"fmt"
	"iter"
	"os"
	"testing"

	"github.com/Autumn-27/norma/llm"
	"github.com/Autumn-27/norma/transcript"
	"github.com/google/uuid"
	"yunkun/console/db"
)

type completeProvider struct{}

func (completeProvider) Stream(context.Context, llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	return func(func(llm.StreamEvent, error) bool) {}
}

func (completeProvider) Complete(context.Context, llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	return llm.Message{
		Role: llm.RoleAssistant,
		Content: []llm.ContentBlock{
			{Type: llm.BlockThinking, Thinking: "reasoning"},
			llm.TextBlock("answer"),
		},
	}, "stop", llm.Usage{InputTokens: 7, OutputTokens: 3}, nil
}

func TestTaskIDContextUsesExplicitRegistryID(t *testing.T) {
	ctx := WithTaskID(context.Background(), " 42 ")
	if got := TaskIDFrom(ctx); got != "42" {
		t.Fatalf("TaskIDFrom()=%q want 42", got)
	}
	if got := TaskIDFrom(WithTaskID(ctx, "   ")); got != "42" {
		t.Fatalf("blank task id should preserve parent context, got %q", got)
	}
	if got := TaskIDFrom(nil); got != "" {
		t.Fatalf("nil context returned %q", got)
	}
}

func TestParseSessionFallbackIsExplorationScoped(t *testing.T) {
	taskID, worker := parseSession("exp12-worker-i99")
	if taskID != "12" || worker != "worker" {
		t.Fatalf("parseSession()=(%q,%q)", taskID, worker)
	}
	if taskID, worker := parseSession("not-a-task"); taskID != "" || worker != "" {
		t.Fatalf("unexpected non-session parse: (%q,%q)", taskID, worker)
	}
}

func TestRequestIDIsUniqueAndOpaque(t *testing.T) {
	first, second := newRequestID(), newRequestID()
	if len(first) != 32 || len(second) != 32 || first == second {
		t.Fatalf("request ids must be distinct 128-bit hex values: %q, %q", first, second)
	}
}

func TestMergeUsageSnapshotKeepsSplitFieldsAndDeduplicatesCumulativeFrames(t *testing.T) {
	total, delta := mergeUsageSnapshot(llm.Usage{}, llm.Usage{InputTokens: 23})
	if total.InputTokens != 23 || delta.InputTokens != 23 {
		t.Fatalf("initial input snapshot: total=%+v delta=%+v", total, delta)
	}
	total, delta = mergeUsageSnapshot(total, llm.Usage{InputTokens: 23, OutputTokens: 5})
	if total.InputTokens != 23 || total.OutputTokens != 5 || delta.InputTokens != 0 || delta.OutputTokens != 5 {
		t.Fatalf("split/cumulative frame: total=%+v delta=%+v", total, delta)
	}
	total, delta = mergeUsageSnapshot(total, llm.Usage{InputTokens: 23, OutputTokens: 5})
	if total.InputTokens != 23 || total.OutputTokens != 5 || hasUsage(delta) {
		t.Fatalf("duplicate frame was counted: total=%+v delta=%+v", total, delta)
	}
	total, delta = mergeUsageSnapshot(total, llm.Usage{InputTokens: 31, OutputTokens: 9, CacheReadTokens: 4})
	if total.InputTokens != 31 || total.OutputTokens != 9 || total.CacheReadTokens != 4 ||
		delta.InputTokens != 8 || delta.OutputTokens != 4 || delta.CacheReadTokens != 4 {
		t.Fatalf("monotonic advance: total=%+v delta=%+v", total, delta)
	}
}

func TestRAGPromptCandidatesOnlyToolResults(t *testing.T) {
	payload := `{"retrieval_log_id":42,"results":[{"chunk_id":7,"content":"exact chunk"}]}`
	req := llm.CompletionRequest{Messages: []llm.Message{
		{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.BlockText, Text: payload}}},
		{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{Type: llm.BlockToolUse, ID: "call-1", Name: "rag_search"}}},
		{Content: []llm.ContentBlock{{Type: llm.BlockToolResult, ToolUseID: "call-1", Content: []llm.ContentBlock{llm.TextBlock(payload)}}}},
	}}
	items := ragPromptCandidates(req)
	if len(items) != 1 || items[0].RetrievalLogID != 42 || items[0].ChunkID != 7 ||
		items[0].ToolUseID != "call-1" || items[0].Content != "exact chunk" {
		t.Fatalf("unexpected prompt candidates: %+v", items)
	}
}

func TestCompleteForwardsAtomicResponse(t *testing.T) {
	recorder := Wrap(completeProvider{}, nil, "model", "profile", "", "", func() bool { return false })
	msg, stopReason, usage, err := recorder.Complete(context.Background(), llm.CompletionRequest{})
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if got := msg.Text(); got != "answer" {
		t.Fatalf("Complete() text=%q want answer", got)
	}
	if stopReason != "stop" {
		t.Fatalf("Complete() stop reason=%q want stop", stopReason)
	}
	if usage.InputTokens != 7 || usage.OutputTokens != 3 {
		t.Fatalf("Complete() usage=%+v", usage)
	}
}

type meteredStreamProvider struct{ completeProvider }

func (meteredStreamProvider) Stream(context.Context, llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	return func(yield func(llm.StreamEvent, error) bool) {
		if !yield(llm.StreamEvent{Type: llm.SEMessageStart, Usage: llm.Usage{InputTokens: 23}}, nil) {
			return
		}
		if !yield(llm.StreamEvent{Type: llm.SEMessageDelta, Usage: llm.Usage{InputTokens: 23, OutputTokens: 5}}, nil) {
			return
		}
		if !yield(llm.StreamEvent{Type: llm.SEMessageDelta, Usage: llm.Usage{InputTokens: 23, OutputTokens: 5}}, nil) {
			return
		}
		yield(llm.StreamEvent{Type: llm.SEMessageStop}, nil)
	}
}

func TestStreamForwardsOnlyUsageAdvances(t *testing.T) {
	recorder := Wrap(meteredStreamProvider{}, nil, "fixture", "profile", "", "", func() bool { return false })
	var downstream llm.Usage
	for event, err := range recorder.Stream(context.Background(), llm.CompletionRequest{}) {
		if err != nil {
			t.Fatal(err)
		}
		downstream.Add(event.Usage)
	}
	if downstream.InputTokens != 23 || downstream.OutputTokens != 5 {
		t.Fatalf("downstream usage counted cumulative snapshots more than once: %+v", downstream)
	}
}

func TestSideUsageRecordedOnceOnConsumerCancellation(t *testing.T) {
	dsn := os.Getenv("YUNKUN_PG_DSN")
	if dsn == "" {
		t.Skip("requires isolated YUNKUN_PG_DSN")
	}
	pg, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Close()
	if err := pg.EnsureLLMUsageTable(); err != nil {
		t.Fatal(err)
	}
	for _, early := range []bool{false, true} {
		profile := "btw-metering-" + uuid.NewString()
		ctx, cancel := context.WithCancel(transcript.WithSessionID(t.Context(), "exp0-btw-test"))
		recorder := Wrap(meteredStreamProvider{}, pg, "fixture", profile, "", "", func() bool { return false })
		for event, err := range recorder.Stream(ctx, llm.CompletionRequest{}) {
			if err != nil {
				t.Fatal(err)
			}
			if early && event.Type == llm.SEMessageDelta {
				cancel()
				break // Consumer exits before the provider can return a cancellation event.
			}
		}
		cancel()
		var count, input, output int
		var worker, status string
		err = pg.QueryRow(`SELECT count(*),max(input_tokens),max(output_tokens),max(worker),max(status)
FROM llm_usage WHERE profile_name=$1`, profile).Scan(&count, &input, &output, &worker, &status)
		_, _ = pg.Exec(`DELETE FROM llm_usage WHERE profile_name=$1`, profile)
		if err != nil {
			t.Fatal(err)
		}
		wantStatus := "ok"
		if early {
			wantStatus = "error"
		}
		if count != 1 || input != 23 || output != 5 || worker != "btw" || status != wantStatus {
			t.Fatalf("early=%v: count=%d usage=%d/%d worker=%q status=%q", early, count, input, output, worker, status)
		}
	}
}

func TestConfirmedContextWindowCapturedPerRequest(t *testing.T) {
	dsn := os.Getenv("YUNKUN_PG_DSN")
	if dsn == "" {
		t.Skip("requires isolated YUNKUN_PG_DSN")
	}
	pg, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Close()
	if err := pg.EnsureLLMUsageTable(); err != nil {
		t.Fatal(err)
	}
	taskID := uuid.NewString()
	for _, capacity := range []int{0, 128000} {
		recorder := Wrap(completeProvider{}, pg, "fixture", "test", "", "", func() bool { return false })
		recorder.WithConfirmedContextWindow(capacity)
		if _, _, _, err := recorder.Complete(WithTaskID(t.Context(), taskID), llm.CompletionRequest{}); err != nil {
			t.Fatal(err)
		}
		latest, err := pg.LatestTaskLLMUsage(taskID)
		if err != nil || latest == nil || latest.InputTokens != 7 || latest.ContextWindowTokens != capacity {
			t.Fatalf("capacity=%d latest=%+v err=%v", capacity, latest, err)
		}
	}
	_, _ = pg.Exec(`DELETE FROM llm_usage WHERE task_id=$1`, taskID)
}

func TestProviderRequestRecordsOnlyPersistedRAGToolResult(t *testing.T) {
	dsn := os.Getenv("YUNKUN_PG_DSN")
	if dsn == "" {
		t.Skip("requires isolated YUNKUN_PG_DSN")
	}
	pg, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Close()
	if err := pg.EnsureLLMUsageTable(); err != nil {
		t.Fatal(err)
	}
	taskID := uuid.NewString()
	var logID int64
	err = pg.QueryRow(`INSERT INTO rag_retrieval_logs(task_id,query,results)
VALUES ($1,'fixture',$2::jsonb) RETURNING id`, taskID, `[{"chunk_id":7,"content":"exact chunk"}]`).Scan(&logID)
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Exec(`DELETE FROM rag_retrieval_logs WHERE id=$1`, logID)
	payload := fmt.Sprintf(`{"retrieval_log_id":%d,"results":[{"chunk_id":7,"content":"exact chunk"}]}`, logID)
	request := llm.CompletionRequest{Messages: []llm.Message{
		{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.TextBlock(payload)}},
		{Content: []llm.ContentBlock{{Type: llm.BlockToolResult, ToolUseID: "call-1", Content: []llm.ContentBlock{llm.TextBlock(payload)}}}},
	}}
	recorder := Wrap(completeProvider{}, pg, "fixture", "test", "", "", func() bool { return false })
	if _, _, _, err := recorder.Complete(WithTaskID(t.Context(), taskID), request); err != nil {
		t.Fatal(err)
	}
	inclusions, err := pg.ListTaskRAGPromptInclusions(taskID, 10)
	if err != nil || len(inclusions) != 1 || inclusions[0].ToolUseID != "call-1" {
		t.Fatalf("provider inclusion=%+v err=%v", inclusions, err)
	}
	usage, err := pg.LatestTaskLLMUsage(taskID)
	if err != nil || usage == nil || usage.RequestID != inclusions[0].RequestID {
		t.Fatalf("inclusion request must match metered call: usage=%+v inclusion=%+v err=%v", usage, inclusions, err)
	}
	_, _ = pg.Exec(`DELETE FROM llm_usage WHERE task_id=$1`, taskID)
}
