package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"

	"github.com/Autumn-27/norma/llm"
)

const (
	unknownContextBudget = 64000
	defaultOutputBudget  = 8192
)

// RequestBudgetError carries numbers only, never prompts, tool data or keys.
// The context-length marker lets the existing SDK attempt reactive compaction.
type RequestBudgetError struct {
	EstimatedInput int
	OutputReserve  int
	SafetyReserve  int
	Limit          int
	CapacitySource string
}

func (e *RequestBudgetError) Error() string {
	return fmt.Sprintf("context length budget exceeded: local estimate input=%d output=%d safety=%d limit=%d capacity_source=%s; reduce input or compact history (estimate is not measured model usage)",
		e.EstimatedInput, e.OutputReserve, e.SafetyReserve, e.Limit, e.CapacitySource)
}

type budgetProvider struct {
	llm.Provider
	config Config
}

func (c Config) requestBudget() (int, string) {
	if c.ContextWindowK > 0 && c.ContextWindowK <= maxWindowK {
		window := c.ContextWindowK * 1000
		if c.ContextWindowConfirmed {
			return window, "confirmed_profile"
		}
		return min(window, unknownContextBudget), "unconfirmed_local_policy"
	}
	return unknownContextBudget, "unknown_local_policy"
}

func (p *budgetProvider) prepare(req llm.CompletionRequest) (llm.CompletionRequest, error) {
	limit, source := p.config.requestBudget()
	if p.config.MaxTokens > 0 && (req.MaxTokens <= 0 || req.MaxTokens > p.config.MaxTokens) {
		req.MaxTokens = p.config.MaxTokens
	}
	if req.MaxTokens <= 0 {
		req.MaxTokens = min(defaultOutputBudget, limit/4)
	}
	// Serializing the whole neutral request includes system segments, schemas,
	// signatures and recursively nested tool results. This deliberately estimates
	// rather than claiming compatibility with every provider's tokenizer.
	encoded, err := json.Marshal(req)
	if err != nil {
		return req, fmt.Errorf("cannot estimate model request budget: invalid request encoding")
	}
	ascii, other := 0, 0
	for _, b := range encoded {
		if b < 128 {
			ascii++
		} else {
			other++
		}
	}
	estimated := (ascii+2)/3 + other + 64 + 16*len(req.Messages) + 32*len(req.Tools)
	safety := min(max(512, limit/20), limit/4)
	if req.MaxTokens > limit-safety || estimated > limit-safety-req.MaxTokens {
		return req, &RequestBudgetError{
			EstimatedInput: estimated, OutputReserve: req.MaxTokens,
			SafetyReserve: safety, Limit: limit, CapacitySource: source,
		}
	}
	return req, nil
}

func (p *budgetProvider) Stream(ctx context.Context, req llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	return func(yield func(llm.StreamEvent, error) bool) {
		prepared, err := p.prepare(req)
		if err != nil {
			yield(llm.StreamEvent{}, err)
			return
		}
		for event, err := range p.Provider.Stream(ctx, prepared) {
			if !yield(event, err) {
				return
			}
		}
	}
}

func (p *budgetProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	prepared, err := p.prepare(req)
	if err != nil {
		return llm.Message{}, "", llm.Usage{}, err
	}
	return p.Provider.Complete(ctx, prepared)
}
