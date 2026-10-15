package llmrec

import (
	"context"
	"sync"
)

type requestTraceKey struct{}

// RequestTrace holds the request that produced the current agent event.
// Each capture run owns its own trace; concurrent agents do not share it.
type RequestTrace struct {
	mu sync.RWMutex
	id string
}

func WithRequestTrace(ctx context.Context) (context.Context, *RequestTrace) {
	trace := &RequestTrace{}
	return context.WithValue(ctx, requestTraceKey{}, trace), trace
}

func (t *RequestTrace) ID() string {
	if t == nil {
		return ""
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.id
}

func markRequest(ctx context.Context, id string) {
	trace, _ := ctx.Value(requestTraceKey{}).(*RequestTrace)
	if trace == nil {
		return
	}
	trace.mu.Lock()
	trace.id = id
	trace.mu.Unlock()
}
