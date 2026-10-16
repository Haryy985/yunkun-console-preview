package agent

import (
	"context"
	"fmt"
	"iter"
	"strings"

	"github.com/Autumn-27/norma/llm"
)

// ModelResponseError never includes a response body, prompt or credential.
// Incomplete streams may be retried only before any output reaches the caller.
type ModelResponseError struct {
	Format     string
	Reason     string
	Incomplete bool
}

func (e *ModelResponseError) Error() string {
	return fmt.Sprintf("model response invalid (%s): %s; check model API format, endpoint and streaming compatibility", e.Format, e.Reason)
}

type validatedProvider struct {
	llm.Provider
	format string
}

func (p *validatedProvider) validate(hasContent bool, stop string, stopped bool) error {
	if !hasContent && stop != "max_tokens" {
		return &ModelResponseError{Format: p.format, Reason: "no usable text or tool calls returned"}
	}
	// The Responses adapter represents response.completed/incomplete with a
	// message_delta only; Anthropic and Chat Completions also emit message_stop.
	if stop == "" || (!stopped && p.format != "openai-responses") {
		return &ModelResponseError{Format: p.format, Reason: "response ended without a completion marker", Incomplete: true}
	}
	return nil
}

func (p *validatedProvider) Stream(ctx context.Context, req llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	return func(yield func(llm.StreamEvent, error) bool) {
		hasContent, stopped := false, false
		stop := ""
		for event, err := range p.Provider.Stream(ctx, req) {
			if err != nil {
				yield(llm.StreamEvent{}, err)
				return
			}
			switch event.Type {
			case llm.SETextDelta:
				hasContent = hasContent || strings.TrimSpace(event.Text) != ""
			case llm.SEToolUseStart:
				hasContent = hasContent || (event.ToolID != "" && event.ToolName != "")
			case llm.SEMessageDelta:
				if event.StopReason != "" {
					stop = event.StopReason
				}
			case llm.SEMessageStop:
				stopped = true
			}
			if !yield(event, nil) {
				return
			}
		}
		if err := ctx.Err(); err != nil {
			yield(llm.StreamEvent{}, err)
			return
		}
		// A reasoning-only max_tokens turn belongs to the harness's bounded
		// continuation path. A normal empty turn must never mean task completion.
		if err := p.validate(hasContent, stop, stopped); err != nil {
			yield(llm.StreamEvent{}, err)
		}
	}
}

func (p *validatedProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	msg, stop, usage, err := p.Provider.Complete(ctx, req)
	if err != nil {
		return msg, stop, usage, err
	}
	if err := ctx.Err(); err != nil {
		return msg, stop, usage, err
	}
	hasContent := false
	for _, block := range msg.Content {
		switch block.Type {
		case llm.BlockText:
			hasContent = hasContent || strings.TrimSpace(block.Text) != ""
		case llm.BlockToolUse:
			hasContent = hasContent || (block.ID != "" && block.Name != "")
		}
	}
	return msg, stop, usage, p.validate(hasContent, stop, true)
}
