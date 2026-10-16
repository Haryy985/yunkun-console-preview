package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Autumn-27/norma/agentcore"
	"github.com/Autumn-27/norma/llm"
	actool "github.com/Autumn-27/norma/tool"
)

type validationProbe struct {
	events []llm.StreamEvent
	err    error
}

func (p validationProbe) Stream(context.Context, llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	return func(yield func(llm.StreamEvent, error) bool) {
		for _, event := range p.events {
			if !yield(event, nil) {
				return
			}
		}
		if p.err != nil {
			yield(llm.StreamEvent{}, p.err)
		}
	}
}

func (p validationProbe) Complete(context.Context, llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	acc := llm.NewAccumulator()
	for _, event := range p.events {
		acc.Add(event)
	}
	return acc.Message(), acc.StopReason, acc.Usage, p.err
}

func TestModelResponseValidation(t *testing.T) {
	end := llm.StreamEvent{Type: llm.SEMessageDelta, StopReason: "end_turn"}
	stop := llm.StreamEvent{Type: llm.SEMessageStop}
	text := llm.StreamEvent{Type: llm.SETextDelta, Text: "ok"}
	for _, tc := range []struct {
		name   string
		events []llm.StreamEvent
		want   string
	}{
		{"empty", nil, "no usable"},
		{"empty_terminal", []llm.StreamEvent{end, stop}, "no usable"},
		{"whitespace", []llm.StreamEvent{{Type: llm.SETextDelta, Text: " \n"}, end, stop}, "no usable"},
		{"thinking_only", []llm.StreamEvent{{Type: llm.SEThinkingDelta, Text: "thinking"}, end, stop}, "no usable"},
		{"text", []llm.StreamEvent{text, end, stop}, ""},
		{"tool_only", []llm.StreamEvent{{Type: llm.SEToolUseStart, ToolID: "call_1", ToolName: "probe"}, {Type: llm.SEToolInputJSON, Text: "{}"}, {Type: llm.SEMessageDelta, StopReason: "tool_use"}, stop}, ""},
		{"truncated_reasoning", []llm.StreamEvent{{Type: llm.SEThinkingDelta, Text: "thinking"}, {Type: llm.SEMessageDelta, StopReason: "max_tokens"}, stop}, ""},
		{"hidden_reasoning_limit", []llm.StreamEvent{{Type: llm.SEMessageDelta, StopReason: "max_tokens"}, stop}, ""},
		{"missing_reason", []llm.StreamEvent{text, stop}, "completion marker"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &validatedProvider{Provider: validationProbe{events: tc.events}, format: "anthropic"}
			var streamErr error
			for _, err := range p.Stream(t.Context(), llm.CompletionRequest{}) {
				if err != nil {
					streamErr = err
				}
			}
			_, _, _, completeErr := p.Complete(t.Context(), llm.CompletionRequest{})
			for _, err := range []error{streamErr, completeErr} {
				if tc.want == "" {
					if err != nil {
						t.Fatal(err)
					}
				} else {
					var responseErr *ModelResponseError
					if !errors.As(err, &responseErr) || !strings.Contains(err.Error(), tc.want) {
						t.Fatalf("want %q, got %v", tc.want, err)
					}
				}
			}
		})
	}
}

func TestModelResponseStreamLifecycle(t *testing.T) {
	sentinel := errors.New("transport interrupted")
	p := &validatedProvider{Provider: validationProbe{err: sentinel}, format: "anthropic"}
	for _, err := range p.Stream(t.Context(), llm.CompletionRequest{}) {
		if err != sentinel {
			t.Fatalf("transport error changed: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	p.Provider = validationProbe{}
	for _, err := range p.Stream(ctx, llm.CompletionRequest{}) {
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation lost: %v", err)
		}
	}
	// Stop consuming mid-response: no trailing validation/error may be yielded.
	p.Provider = validationProbe{events: []llm.StreamEvent{{Type: llm.SETextDelta, Text: "partial"}}}
	count := 0
	for range p.Stream(t.Context(), llm.CompletionRequest{}) {
		count++
		break
	}
	if count != 1 {
		t.Fatalf("events after consumer stopped: %d", count)
	}
	var streamErr error
	for _, err := range p.Stream(t.Context(), llm.CompletionRequest{}) {
		streamErr = err
	}
	var responseErr *ModelResponseError
	if !errors.As(streamErr, &responseErr) || !responseErr.Incomplete {
		t.Fatalf("partial response accepted: %v", streamErr)
	}
}

func TestModelResponseWrongWireFormatFailsRun(t *testing.T) {
	for _, response := range []string{
		"<html>gateway unavailable</html>",
		`{"choices":[{"message":{"content":"wrong protocol"}}]}`,
		"data: {\"id\":\"\",\"object\":\"\",\"choices\":null,\"usage\":null}\n\n",
	} {
		t.Run(fmt.Sprintf("body_%d", len(response)), func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, response)
			}))
			defer srv.Close()
			p, err := (Config{Format: llm.FormatAnthropic, BaseURL: srv.URL, APIKey: "test", Model: "test"}).NewProvider()
			if err != nil {
				t.Fatal(err)
			}
			_, err = agentcore.Run(t.Context(), agentcore.Options{
				Provider: p, Tools: []actool.CoreTool{}, MaxTurns: 3,
			}, "Reply OK")
			if err == nil || !strings.Contains(err.Error(), "no usable") || calls.Load() != 1 {
				t.Fatalf("invalid response must fail once: calls=%d err=%v", calls.Load(), err)
			}
		})
	}
}

// Exercise the real HTTP adapters and harness, including the tool result sent
// back on the next request. Only the in-memory probe tool is available.
func TestModelResponseToolRoundTrip(t *testing.T) {
	for _, format := range []llm.Format{llm.FormatAnthropic, llm.FormatOpenAI, llm.FormatOpenAIResponses} {
		for _, streaming := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/stream_%t", format, streaming), func(t *testing.T) {
				var requests, tools atomic.Int32
				var sawResult atomic.Bool
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					raw, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
					var body map[string]any
					if err := json.Unmarshal(raw, &body); err != nil {
						t.Error(err)
					}
					if body["stream"] != streaming {
						t.Errorf("stream=%v", body["stream"])
					}
					first := requests.Add(1) == 1
					if !first {
						sawResult.Store(strings.Contains(string(raw), "local-probe-result"))
					}
					writeValidationResponse(w, format, streaming, first)
				}))
				defer srv.Close()
				provider, err := (Config{
					Format: format, BaseURL: srv.URL, APIKey: "test", Model: "test",
					Retry: RetryConfig{ConnectAttempts: -1, EmptyAttempts: -1},
				}).NewProvider()
				if err != nil {
					t.Fatal(err)
				}
				probe := actool.Build(actool.Spec{
					Name: "local_probe", Description: "Read an in-memory value",
					Schema:   map[string]any{"type": "object", "properties": map[string]any{}},
					ReadOnly: func(json.RawMessage) bool { return true },
					Run: func(context.Context, json.RawMessage, *actool.ToolContext) (actool.Result, error) {
						tools.Add(1)
						return actool.Text("local-probe-result"), nil
					},
				})
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				result, err := agentcore.Run(ctx, agentcore.Options{
					Provider: provider, Tools: []actool.CoreTool{probe},
					NonStreaming: !streaming, MaxTurns: 3, MaxTokens: 1024,
				}, "Read the local probe and reply OK")
				if err != nil || !strings.Contains(result, "OK") || tools.Load() != 1 || requests.Load() != 2 || !sawResult.Load() {
					t.Fatalf("round trip: result=%q err=%v tools=%d requests=%d sawResult=%t",
						result, err, tools.Load(), requests.Load(), sawResult.Load())
				}
			})
		}
	}
}

func writeValidationResponse(w http.ResponseWriter, format llm.Format, streaming, toolCall bool) {
	w.Header().Set("Content-Type", "application/json")
	if streaming {
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(v any) {
			raw, _ := json.Marshal(v)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", raw)
		}
		switch format {
		case llm.FormatAnthropic:
			emit(map[string]any{"type": "message_start", "message": map[string]any{"usage": map[string]int{"input_tokens": 1}}})
			reason := "end_turn"
			if toolCall {
				reason = "tool_use"
				emit(map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "tool_use", "id": "call_1", "name": "local_probe"}})
				emit(map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "input_json_delta", "partial_json": "{}"}})
			} else {
				emit(map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": "OK"}})
			}
			emit(map[string]any{"type": "message_delta", "delta": map[string]string{"stop_reason": reason}})
			emit(map[string]string{"type": "message_stop"})
		case llm.FormatOpenAI:
			delta := map[string]any{"content": "OK"}
			reason := "stop"
			if toolCall {
				delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "call_1", "type": "function", "function": map[string]string{"name": "local_probe", "arguments": "{}"}}}}
				reason = "tool_calls"
			}
			emit(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": delta}}})
			emit(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": reason}}})
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
		case llm.FormatOpenAIResponses:
			if toolCall {
				emit(map[string]any{"type": "response.output_item.added", "item": map[string]string{"type": "function_call", "call_id": "call_1", "name": "local_probe"}})
				emit(map[string]string{"type": "response.function_call_arguments.delta", "delta": "{}"})
			} else {
				emit(map[string]string{"type": "response.output_text.delta", "delta": "OK"})
			}
			emit(map[string]any{"type": "response.completed", "response": map[string]string{"status": "completed"}})
		}
		return
	}
	switch format {
	case llm.FormatAnthropic:
		content := []any{map[string]string{"type": "text", "text": "OK"}}
		reason := "end_turn"
		if toolCall {
			content = []any{map[string]any{"type": "tool_use", "id": "call_1", "name": "local_probe", "input": map[string]any{}}}
			reason = "tool_use"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"role": "assistant", "content": content, "stop_reason": reason})
	case llm.FormatOpenAI:
		msg := map[string]any{"role": "assistant", "content": "OK"}
		reason := "stop"
		if toolCall {
			msg = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "call_1", "type": "function", "function": map[string]string{"name": "local_probe", "arguments": "{}"}}}}
			reason = "tool_calls"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": msg, "finish_reason": reason}}})
	case llm.FormatOpenAIResponses:
		output := []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]string{"type": "output_text", "text": "OK"}}}}
		if toolCall {
			output = []any{map[string]string{"type": "function_call", "call_id": "call_1", "name": "local_probe", "arguments": "{}"}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output": output})
	}
}
