package agent

import (
	"context"
	"encoding/json"
	"io"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Autumn-27/norma/llm"
)

func reasonlessToolHistory() []llm.Message {
	return []llm.Message{
		llm.UserText("check status"),
		{Role: llm.RoleAssistant, Content: []llm.ContentBlock{
			{Type: llm.BlockToolUse, ID: "call_1", Name: "network_status", Input: []byte(`{}`)},
		}},
		{Role: llm.RoleUser, Content: []llm.ContentBlock{
			{Type: llm.BlockToolResult, ToolUseID: "call_1", Content: []llm.ContentBlock{llm.TextBlock("available")}},
		}},
	}
}

func TestReasoningCompatibilityRetryKeepsToolHistory(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		name := "complete"
		if streaming {
			name = "stream"
		}
		t.Run(name, func(t *testing.T) {
			calls, disabledCalls := 0, 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body struct {
					Thinking *struct {
						Type string `json:"type"`
					} `json:"thinking"`
					Messages []struct {
						Role             string          `json:"role"`
						ReasoningContent string          `json:"reasoning_content"`
						ToolCalls        json.RawMessage `json:"tool_calls"`
						ToolCallID       string          `json:"tool_call_id"`
					} `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				toolCall, toolResult := false, false
				for _, msg := range body.Messages {
					if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
						toolCall = true
						if msg.ReasoningContent != "" {
							t.Error("invented reasoning_content for a prior model's tool call")
						}
					}
					if msg.Role == "tool" && msg.ToolCallID == "call_1" {
						toolResult = true
					}
				}
				if !toolCall || !toolResult {
					t.Errorf("tool history was changed: call=%v result=%v", toolCall, toolResult)
				}
				if body.Thinking == nil {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = io.WriteString(w, `{"error":{"message":"The reasoning_content in the thinking mode must be passed back to the API."}}`)
					return
				}
				if body.Thinking.Type != "disabled" {
					t.Errorf("thinking type = %q", body.Thinking.Type)
				}
				disabledCalls++
				if streaming {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
					_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
					_, _ = io.WriteString(w, "data: [DONE]\n\n")
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
				}
			}))
			defer srv.Close()
			provider, err := (Config{Format: llm.FormatOpenAI, Model: "fixture", BaseURL: srv.URL, APIKey: "fixture"}).NewProvider()
			if err != nil {
				t.Fatal(err)
			}
			req := llm.CompletionRequest{Messages: reasonlessToolHistory(), MaxTokens: 512}
			for i := 0; i < 2; i++ {
				if streaming {
					var output strings.Builder
					for ev, err := range provider.Stream(context.Background(), req) {
						if err != nil {
							t.Fatal(err)
						}
						if ev.Type == llm.SETextDelta {
							output.WriteString(ev.Text)
						}
					}
					if output.String() != "ok" {
						t.Fatalf("output = %q", output.String())
					}
				} else {
					msg, _, _, err := provider.Complete(context.Background(), req)
					if err != nil || msg.Text() != "ok" {
						t.Fatalf("response = %q, err = %v", msg.Text(), err)
					}
				}
			}
			if calls != 3 || disabledCalls != 2 {
				t.Fatalf("calls = %d, disabled calls = %d; want 3 and 2", calls, disabledCalls)
			}
		})
	}
}

func TestReasoningCompatibilityRequiresExactRejection(t *testing.T) {
	if missingReasoningError(nil) ||
		missingReasoningError(context.DeadlineExceeded) ||
		missingReasoningError(&ModelResponseError{Reason: "reasoning_content"}) ||
		missingReasoningError(assertError("status 400: invalid API key")) {
		t.Fatal("unrelated errors must not trigger compatibility retry")
	}
	p := &reasoningCompatProvider{enabled: true}
	if p.eligible(llm.CompletionRequest{Messages: []llm.Message{llm.UserText("hello")}}) ||
		p.eligible(llm.CompletionRequest{Messages: reasonlessToolHistory(), Thinking: "enabled"}) {
		t.Fatal("explicit thinking or a fresh conversation must not be changed")
	}
}

type assertError string

func (e assertError) Error() string { return string(e) }

type stagedReasoningProvider struct {
	llm.Provider
	calls    int
	disabled []bool
	output   bool
}

func (p *stagedReasoningProvider) Stream(ctx context.Context, _ llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	p.calls++
	p.disabled = append(p.disabled, requestNeedsThinkingDisabled(ctx))
	return func(yield func(llm.StreamEvent, error) bool) {
		if !yield(llm.StreamEvent{Type: llm.SEMessageStart}, nil) {
			return
		}
		if p.output {
			if !yield(llm.StreamEvent{Type: llm.SETextDelta, Text: "partial"}, nil) {
				return
			}
		}
		yield(llm.StreamEvent{}, assertError("status 400: reasoning_content must be passed back"))
	}
}

func TestReasoningCompatibilityStreamCommitBoundary(t *testing.T) {
	for _, output := range []bool{false, true} {
		t.Run(map[bool]string{false: "start-only", true: "partial-output"}[output], func(t *testing.T) {
			inner := &stagedReasoningProvider{output: output}
			provider := &reasoningCompatProvider{Provider: inner, enabled: true}
			events := []llm.StreamEventType{}
			var gotErr error
			for event, err := range provider.Stream(context.Background(), llm.CompletionRequest{Messages: reasonlessToolHistory()}) {
				if err != nil {
					gotErr = err
					break
				}
				events = append(events, event.Type)
			}
			if gotErr == nil {
				t.Fatal("expected gateway rejection")
			}
			if output {
				if inner.calls != 1 || len(events) != 2 || events[0] != llm.SEMessageStart || events[1] != llm.SETextDelta {
					t.Fatalf("partial output must not retry: calls=%d events=%v", inner.calls, events)
				}
			} else if inner.calls != 2 || len(events) != 1 || events[0] != llm.SEMessageStart || len(inner.disabled) != 2 || inner.disabled[0] || !inner.disabled[1] {
				t.Fatalf("start-only rejection must retry without duplicate start: calls=%d events=%v disabled=%v", inner.calls, events, inner.disabled)
			}
		})
	}
}

func TestReasoningCompatibilityPreservesExplicitThinking(t *testing.T) {
	inner := &stagedReasoningProvider{}
	provider := &reasoningCompatProvider{Provider: inner, enabled: true}
	for _, err := range provider.Stream(context.Background(), llm.CompletionRequest{Messages: reasonlessToolHistory(), Thinking: "enabled"}) {
		if err != nil {
			break
		}
	}
	if inner.calls != 1 || inner.disabled[0] {
		t.Fatalf("explicit thinking was altered: calls=%d disabled=%v", inner.calls, inner.disabled)
	}
}
