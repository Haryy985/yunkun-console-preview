package llmrec

import (
	"context"
	"testing"

	"github.com/Autumn-27/norma/llm"
)

func TestRequestTraceIsScoped(t *testing.T) {
	firstCtx, first := WithRequestTrace(context.Background())
	secondCtx, second := WithRequestTrace(context.Background())
	markRequest(firstCtx, "request-1")
	markRequest(secondCtx, "request-2")
	if first.ID() != "request-1" || second.ID() != "request-2" {
		t.Fatalf("traces leaked: first=%q second=%q", first.ID(), second.ID())
	}
	markRequest(firstCtx, "request-3")
	if first.ID() != "request-3" || second.ID() != "request-2" {
		t.Fatalf("trace did not advance per call: first=%q second=%q", first.ID(), second.ID())
	}
}

func TestRecorderMarksRequestTrace(t *testing.T) {
	ctx, trace := WithRequestTrace(t.Context())
	recorder := Wrap(completeProvider{}, nil, "fixture", "", "", "", func() bool { return false })
	if _, _, _, err := recorder.Complete(ctx, llm.CompletionRequest{}); err != nil {
		t.Fatal(err)
	}
	first := trace.ID()
	if len(first) != 32 {
		t.Fatalf("complete request ID absent: %q", first)
	}
	for _, err := range recorder.Stream(ctx, llm.CompletionRequest{}) {
		if err != nil {
			t.Fatal(err)
		}
	}
	if second := trace.ID(); len(second) != 32 || second == first {
		t.Fatalf("stream request ID absent or stale: %q", second)
	}
}
