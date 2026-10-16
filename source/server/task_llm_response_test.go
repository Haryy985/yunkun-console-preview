package server

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Autumn-27/norma/harness"
	"github.com/Autumn-27/norma/llm"
	"yunkun/console/agent"
)

func TestModelResponseSkipsWorkerReplay(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reason harness.TerminalReason
		err    error
		want   bool
	}{
		{"empty", harness.ReasonModelError, &agent.ModelResponseError{Reason: "no usable text or tool calls returned"}, false},
		{"incomplete", harness.ReasonModelError, &agent.ModelResponseError{Reason: "missing completion marker", Incomplete: true}, false},
		{"wrapped", harness.ReasonModelError, fmt.Errorf("worker: %w", &agent.ModelResponseError{Reason: "empty"}), false},
		{"network", harness.ReasonModelError, errors.New("temporary network error"), true},
		{"completed", harness.ReasonCompleted, errors.New("temporary network error"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := retryableWorkerModelError(tc.reason, tc.err); got != tc.want {
				t.Fatalf("retry=%t want %t", got, tc.want)
			}
		})
	}
}

func TestModelResponseRetryClassification(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want bool
	}{
		{&agent.ModelResponseError{Reason: "no usable text or tool calls returned"}, false},
		{fmt.Errorf("wrapped: %w", &agent.ModelResponseError{Reason: "no usable text or tool calls returned"}), false},
		{&agent.ModelResponseError{Reason: "response ended without a completion marker", Incomplete: true}, true},
	} {
		if got := isRetryableStreamError(tc.err); got != tc.want {
			t.Fatalf("retry(%v)=%t want %t", tc.err, got, tc.want)
		}
	}
}

func TestTaskLLMInvalidResponseFailsWithoutReplay(t *testing.T) {
	for _, streaming := range []bool{true, false} {
		t.Run(fmt.Sprintf("stream_%t", streaming), func(t *testing.T) {
			responseErr := &agent.ModelResponseError{Reason: "no usable text or tool calls returned"}
			provider := &scriptedLLMProvider{err: responseErr}
			hooks := taskLLMStreamHooks{
				current: func() (taskLLMSelection, error) {
					return taskLLMSelection{profileID: 11, provider: provider}, nil
				},
			}
			var err error
			if streaming {
				_, err = collectTaskLLMStream(streamTaskLLM(context.Background(), "probe", llm.CompletionRequest{}, hooks))
			} else {
				_, _, _, err = completeTaskLLM(context.Background(), "probe", llm.CompletionRequest{}, hooks)
			}
			if !errors.Is(err, responseErr) || provider.calls != 1 {
				t.Fatalf("invalid response must fail once: calls=%d err=%v", provider.calls, err)
			}
		})
	}
}

func TestTaskLLMIncompleteResponseDoesNotReplayDeliveredTool(t *testing.T) {
	responseErr := &agent.ModelResponseError{Reason: "response ended without a completion marker", Incomplete: true}
	provider := &scriptedLLMProvider{
		events: []llm.StreamEvent{
			{Type: llm.SEToolUseStart, ToolID: "call_1", ToolName: "local_probe"},
			{Type: llm.SEToolInputJSON, Text: "{}"},
		},
		err: responseErr,
	}
	hooks := taskLLMStreamHooks{
		current: func() (taskLLMSelection, error) {
			return taskLLMSelection{profileID: 11, provider: provider}, nil
		},
	}
	events, err := collectTaskLLMStream(streamTaskLLM(context.Background(), "probe", llm.CompletionRequest{}, hooks))
	if !errors.Is(err, responseErr) || provider.calls != 1 || len(events) != 2 {
		t.Fatalf("delivered tool must not replay: calls=%d events=%d err=%v", provider.calls, len(events), err)
	}
}
