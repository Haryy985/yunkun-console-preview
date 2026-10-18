package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/Autumn-27/norma/llm"
)

type thinkingDisabledKey struct{}

func requestNeedsThinkingDisabled(ctx context.Context) bool {
	v, _ := ctx.Value(thinkingDisabledKey{}).(bool)
	return v
}

// The SDK's request builder leaves implicit gateway thinking untouched. Set
// the wire field here so it does not fabricate reasoning_content in old turns.
func withThinkingDisabled(req *http.Request) (*http.Request, error) {
	if !strings.HasSuffix(req.URL.Path, "/chat/completions") {
		return req, fmt.Errorf("thinking compatibility: unexpected endpoint")
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return req, fmt.Errorf("thinking compatibility: read request: %w", err)
	}
	_ = req.Body.Close()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return req, fmt.Errorf("thinking compatibility: invalid request JSON: %w", err)
	}
	if _, set := fields["thinking"]; set {
		return req, fmt.Errorf("thinking compatibility: explicit thinking setting must not be overridden")
	}
	fields["thinking"] = json.RawMessage(`{"type":"disabled"}`)
	encoded, err := json.Marshal(fields)
	if err != nil {
		return req, fmt.Errorf("thinking compatibility: encode request: %w", err)
	}
	out := req.Clone(req.Context())
	out.Body = io.NopCloser(bytes.NewReader(encoded))
	out.ContentLength = int64(len(encoded))
	out.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(encoded)), nil
	}
	return out, nil
}

func missingReasoningOnToolCall(req llm.CompletionRequest) bool {
	for _, msg := range req.Messages {
		if msg.Role != llm.RoleAssistant {
			continue
		}
		toolCall, reasoning := false, false
		for _, block := range msg.Content {
			toolCall = toolCall || block.Type == llm.BlockToolUse
			reasoning = reasoning || (block.Type == llm.BlockThinking && block.Thinking != "")
		}
		if toolCall && !reasoning {
			return true
		}
	}
	return false
}

func missingReasoningError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "status 400") &&
		strings.Contains(s, "reasoning_content") &&
		strings.Contains(s, "must be passed back")
}

// Retry only a rejected, output-free request. Once a gateway proves it has
// implicit thinking, subsequent mixed-model history avoids the rejected probe.
type reasoningCompatProvider struct {
	llm.Provider
	enabled bool
	learned atomic.Bool
}

func (p *reasoningCompatProvider) eligible(req llm.CompletionRequest) bool {
	return p.enabled && req.Thinking == "" && missingReasoningOnToolCall(req)
}

func (p *reasoningCompatProvider) Stream(ctx context.Context, req llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	return func(yield func(llm.StreamEvent, error) bool) {
		eligible := p.eligible(req)
		disabled := eligible && p.learned.Load()
		callCtx := ctx
		if disabled {
			callCtx = context.WithValue(ctx, thinkingDisabledKey{}, true)
		}
		emitted := false
		var pendingStart *llm.StreamEvent
		for event, err := range p.Provider.Stream(callCtx, req) {
			if err != nil && eligible && !disabled && !emitted && ctx.Err() == nil && missingReasoningError(err) {
				retrySucceeded := true
				for retryEvent, retryErr := range p.Provider.Stream(context.WithValue(ctx, thinkingDisabledKey{}, true), req) {
					if retryErr != nil {
						retrySucceeded = false
					}
					if !yield(retryEvent, retryErr) {
						return
					}
				}
				if retrySucceeded {
					p.learned.Store(true)
				}
				return
			}
			// Some adapters announce a message before the gateway rejects it.
			// Do not expose that pre-output event until the request is committed.
			if err == nil && event.Type == llm.SEMessageStart && !emitted {
				pendingStart = &event
				continue
			}
			if pendingStart != nil {
				if !yield(*pendingStart, nil) {
					return
				}
				pendingStart = nil
			}
			if err == nil && event.Type != llm.SEMessageStart {
				emitted = true
			}
			if !yield(event, err) {
				return
			}
		}
		if pendingStart != nil {
			yield(*pendingStart, nil)
		}
	}
}

func (p *reasoningCompatProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	eligible := p.eligible(req)
	callCtx := ctx
	if eligible && p.learned.Load() {
		callCtx = context.WithValue(ctx, thinkingDisabledKey{}, true)
	}
	msg, stop, usage, err := p.Provider.Complete(callCtx, req)
	if err == nil || !eligible || requestNeedsThinkingDisabled(callCtx) || ctx.Err() != nil || !missingReasoningError(err) {
		return msg, stop, usage, err
	}
	msg, stop, usage, err = p.Provider.Complete(context.WithValue(ctx, thinkingDisabledKey{}, true), req)
	if err == nil {
		p.learned.Store(true)
	}
	return msg, stop, usage, err
}
