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
	minForwardOutput     = 1024
	unknownBudgetOverage = 16384
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
	outputReserve := req.MaxTokens
	if outputReserve <= 0 {
		outputReserve = defaultOutputBudget
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
	// Keep a small tokenizer-estimation margin without consuming the entire
	// tail of an unknown 64K window. The request estimate already includes
	// structural overhead; a fixed 5% reserve made otherwise executable
	// planner turns fail before the provider was contacted.
	safety := min(max(512, limit/64), limit/8)
	available := limit - safety - estimated
	// A configured output cap is an upper bound, not a minimum reservation.
	// When the request is close to the local ceiling, forward the largest
	// output that still fits instead of rejecting a potentially executable
	// planner turn. Keep a small floor so the provider cannot receive a
	// request that is guaranteed to be empty.
	minimumOutput := min(outputReserve, minForwardOutput)
	if available < minimumOutput {
		// Unknown provider capacities are guarded conservatively, but the
		// neutral estimate intentionally overcounts JSON and token boundaries.
		// Permit a small near-edge probe with the minimum output budget; a
		// materially oversized request must still fail locally.
		if source == "unknown_local_policy" && estimated <= limit+unknownBudgetOverage {
			req.MaxTokens = minimumOutput
			return req, nil
		}
		return req, &RequestBudgetError{
			EstimatedInput: estimated, OutputReserve: outputReserve,
			SafetyReserve: safety, Limit: limit, CapacitySource: source,
		}
	}
	if available < outputReserve {
		req.MaxTokens = available
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
