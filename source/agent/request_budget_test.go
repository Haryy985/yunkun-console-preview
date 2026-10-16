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
	"testing"
	"time"

	"github.com/Autumn-27/norma/agentcore"
	"github.com/Autumn-27/norma/llm"
	actool "github.com/Autumn-27/norma/tool"
)

type budgetProbe struct {
	calls int
	last  llm.CompletionRequest
}

func (p *budgetProbe) Stream(_ context.Context, req llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	return func(yield func(llm.StreamEvent, error) bool) {
		p.calls++
		p.last = req
		for _, ev := range []llm.StreamEvent{
			{Type: llm.SETextDelta, Text: "ok"},
			{Type: llm.SEMessageDelta, StopReason: "end_turn"},
			{Type: llm.SEMessageStop},
		} {
			if !yield(ev, nil) {
				return
			}
		}
	}
}

func (p *budgetProbe) Complete(_ context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	p.calls++
	p.last = req
	return llm.UserText("ok"), "end_turn", llm.Usage{}, nil
}

func TestRequestBudgetCapacityAndOutput(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		cfg                    Config
		request, limit, output int
		source                 string
	}{
		{"unknown", Config{}, 0, 64000, 0, "unknown_local_policy"},
		{"unconfirmed", Config{ContextWindowK: 200}, 0, 64000, 0, "unconfirmed_local_policy"},
		{"small_unconfirmed", Config{ContextWindowK: 8}, 0, 8000, -1, "unconfirmed_local_policy"},
		{"confirmed", Config{ContextWindowK: 8, ContextWindowConfirmed: true}, 0, 8000, -1, "confirmed_profile"},
		{"configured_cap", Config{MaxTokens: 512}, 1024, 64000, 512, "unknown_local_policy"},
		{"smaller_request", Config{MaxTokens: 512}, 128, 64000, 128, "unknown_local_policy"},
		{"explicit_request", Config{}, 16000, 64000, 16000, "unknown_local_policy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if limit, source := tc.cfg.requestBudget(); limit != tc.limit || source != tc.source {
				t.Fatalf("capacity = %d %s", limit, source)
			}
			p := &budgetProvider{config: tc.cfg}
			prepared, err := p.prepare(llm.CompletionRequest{MaxTokens: tc.request})
			if err != nil || (tc.output >= 0 && prepared.MaxTokens != tc.output) ||
				(tc.output < 0 && (prepared.MaxTokens <= 0 || prepared.MaxTokens >= defaultOutputBudget)) {
				t.Fatalf("output=%d err=%v", prepared.MaxTokens, err)
			}
		})
	}
}

func TestRequestBudgetCompactionWindowMatchesGuard(t *testing.T) {
	if got := compactionConfig(0).ContextWindow; got != unknownContextBudget {
		t.Fatalf("default compaction window = %d, want %d", got, unknownContextBudget)
	}
	for _, tc := range []struct {
		name string
		cfg  Config
		want int
	}{
		{"unknown", Config{}, 64000},
		{"unconfirmed_large", Config{ContextWindowK: 200}, 64000},
		{"unconfirmed_small", Config{ContextWindowK: 48}, 48000},
		{"confirmed", Config{ContextWindowK: 200, ContextWindowConfirmed: true}, 200000},
		{"confirmed_small", Config{ContextWindowK: 8, ContextWindowConfirmed: true}, 8000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.CompactionWindow(); got != tc.want {
				t.Fatalf("compaction window = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestRequestBudgetShrinksExcessOutputOnFirstTurn(t *testing.T) {
	probe := &budgetProbe{}
	p := &budgetProvider{Provider: probe}
	req := llm.CompletionRequest{System: []string{strings.Repeat("x", 100000)}, MaxTokens: 32768}
	if _, _, _, err := p.Complete(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if probe.calls != 1 || probe.last.MaxTokens >= req.MaxTokens || probe.last.MaxTokens < 28000 {
		t.Fatalf("first request rejected or output not adjusted: calls=%d maxTokens=%d", probe.calls, probe.last.MaxTokens)
	}
	if req.MaxTokens != 32768 {
		t.Fatal("caller request was mutated")
	}
	for range p.Stream(t.Context(), req) {
	}
	if probe.calls != 2 || probe.last.MaxTokens >= req.MaxTokens {
		t.Fatal("stream request did not apply the same output cap")
	}
}

func TestRequestBudgetPreservesProviderDefaultUntilCapacityRequiresCap(t *testing.T) {
	probe := &budgetProbe{}
	p := &budgetProvider{Provider: probe}
	if _, _, _, err := p.Complete(t.Context(), llm.CompletionRequest{Messages: []llm.Message{llm.UserText("hello")}}); err != nil {
		t.Fatal(err)
	}
	if probe.calls != 1 || probe.last.MaxTokens != 0 {
		t.Fatalf("unconfigured output changed provider default: calls=%d output=%d", probe.calls, probe.last.MaxTokens)
	}
	if _, _, _, err := p.Complete(t.Context(), llm.CompletionRequest{System: []string{strings.Repeat("x", 174000)}}); err != nil {
		t.Fatal(err)
	}
	if probe.last.MaxTokens <= 0 || probe.last.MaxTokens >= defaultOutputBudget {
		t.Fatalf("near-capacity request did not receive a bounded output: %d", probe.last.MaxTokens)
	}
}

func TestRequestBudgetCountsAllInputBeforeDelivery(t *testing.T) {
	secret := strings.Repeat("private-prompt-", 6000)
	for _, field := range []string{"system", "schema", "history", "nested_tool", "signature"} {
		t.Run(field, func(t *testing.T) {
			req := llm.CompletionRequest{MaxTokens: 321}
			switch field {
			case "system":
				req.System = []string{secret}
			case "schema":
				req.Tools = []llm.ToolSchema{{Name: "probe", InputSchema: map[string]any{"description": secret}}}
			case "history":
				req.Messages = []llm.Message{llm.UserText(secret)}
			case "nested_tool":
				req.Messages = []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{
					llm.ToolResultText("call", secret, false),
				}}}
			case "signature":
				req.Messages = []llm.Message{{Role: llm.RoleAssistant, Content: []llm.ContentBlock{
					{Type: llm.BlockThinking, Thinking: "thinking", Signature: secret},
				}}}
			}
			probe := &budgetProbe{}
			p := &budgetProvider{Provider: probe, config: Config{ContextWindowK: 8, ContextWindowConfirmed: true}}
			_, _, _, err := p.Complete(t.Context(), req)
			var budget *RequestBudgetError
			if !errors.As(err, &budget) || budget.Limit != 8000 || budget.OutputReserve != 321 {
				t.Fatalf("expected typed budget error, got %v", err)
			}
			if strings.Contains(err.Error(), "private-prompt") {
				t.Fatal("request data leaked into error")
			}
			gotError := false
			for _, err := range p.Stream(t.Context(), req) {
				gotError = errors.As(err, &budget)
			}
			if !gotError || probe.calls != 0 {
				t.Fatalf("stream rejected=%v underlying calls=%d", gotError, probe.calls)
			}
		})
	}
	probe := &budgetProbe{}
	p := &budgetProvider{Provider: probe}
	for range p.Stream(t.Context(), llm.CompletionRequest{}) {
	}
	if _, _, _, err := p.Complete(t.Context(), llm.CompletionRequest{MaxTokens: 123}); err != nil {
		t.Fatal(err)
	}
	if probe.calls != 2 || probe.last.MaxTokens != 123 {
		t.Fatal("small request or actual cap not forwarded")
	}
}

func TestRequestBudgetHonorsSmallExplicitOutputNearLimit(t *testing.T) {
	p := &budgetProvider{}
	req := llm.CompletionRequest{System: []string{strings.Repeat("x", 174000)}, MaxTokens: 321}
	prepared, err := p.prepare(req)
	if err != nil || prepared.MaxTokens != 321 {
		t.Fatalf("small explicit output rejected: output=%d err=%v", prepared.MaxTokens, err)
	}
	req.MaxTokens = 32768
	prepared, err = p.prepare(req)
	if err != nil || prepared.MaxTokens <= minForwardOutput || prepared.MaxTokens >= req.MaxTokens {
		t.Fatalf("large output should shrink to remaining space: output=%d err=%v", prepared.MaxTokens, err)
	}
}

func TestRequestBudgetShrinksToRemainingSpaceNearLimit(t *testing.T) {
	p := &budgetProvider{}
	req := llm.CompletionRequest{
		System:    []string{strings.Repeat("x", 184000)},
		MaxTokens: 32768,
	}
	prepared, err := p.prepare(req)
	if err != nil {
		t.Fatalf("near-limit request should shrink output instead of failing: %v", err)
	}
	if prepared.MaxTokens <= minForwardOutput || prepared.MaxTokens >= req.MaxTokens {
		t.Fatalf("unexpected shrunk output: %d", prepared.MaxTokens)
	}
}

func TestRequestBudgetAllowsSmallUnknownNearEdgeProbe(t *testing.T) {
	p := &budgetProvider{}
	req := llm.CompletionRequest{
		System:    []string{strings.Repeat("x", 194000)},
		MaxTokens: 32768,
	}
	prepared, err := p.prepare(req)
	if err != nil || prepared.MaxTokens != minForwardOutput {
		t.Fatalf("near-edge unknown request should use bounded probe: output=%d err=%v", prepared.MaxTokens, err)
	}
}

func TestRequestBudgetSDKFirstTurnSucceeds(t *testing.T) {
	probe := &budgetProbe{}
	cfg := Config{MaxTokens: 32768}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	_, err := agentcore.Run(ctx, agentcore.Options{
		Provider:     &budgetProvider{Provider: probe, config: cfg},
		SystemPrompt: []string{strings.Repeat("x", 100000)},
		MaxTokens:    cfg.MaxTokens,
		Compaction:   compactionConfig(cfg.CompactionWindow()),
	}, "hello")
	if err != nil || probe.calls != 1 || probe.last.MaxTokens >= cfg.MaxTokens {
		t.Fatalf("first SDK turn failed: calls=%d output=%d err=%v", probe.calls, probe.last.MaxTokens, err)
	}
}

func TestRequestBudgetRejectsInvalidEncodingAndOversizedInput(t *testing.T) {
	p := &budgetProvider{}
	for _, req := range []llm.CompletionRequest{
		{System: []string{strings.Repeat("x", 300000)}, MaxTokens: 64000},
		{Tools: []llm.ToolSchema{{InputSchema: map[string]any{"private": make(chan int)}}}},
	} {
		if _, err := p.prepare(req); err == nil || strings.Contains(err.Error(), "private") {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	prepared, err := p.prepare(llm.CompletionRequest{MaxTokens: 64000})
	if err != nil || prepared.MaxTokens >= 64000 {
		t.Fatalf("oversized output should fit the local budget: output=%d err=%v", prepared.MaxTokens, err)
	}
}

func TestRequestBudgetWireOutputCaps(t *testing.T) {
	for _, tc := range []struct {
		format   llm.Format
		field    string
		response string
	}{
		{llm.FormatAnthropic, "max_tokens", `{"id":"m","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`},
		{llm.FormatOpenAI, "max_tokens", `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`},
		{llm.FormatOpenAI, "max_completion_tokens", `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`},
		{llm.FormatOpenAIResponses, "max_output_tokens", `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":1}}`},
	} {
		t.Run(string(tc.format)+"/"+tc.field, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body[tc.field] != float64(777) {
					t.Errorf("wire cap = %v, want 777", body[tc.field])
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tc.response)
			}))
			defer srv.Close()
			cfg := Config{Format: tc.format, Model: "test", APIKey: "test", BaseURL: srv.URL, MaxTokens: 777}
			if tc.field == "max_completion_tokens" {
				cfg.MaxTokensField = tc.field
			}
			provider, err := cfg.NewProvider()
			if err != nil {
				t.Fatal(err)
			}
			if _, _, _, err := provider.Complete(t.Context(), llm.CompletionRequest{Messages: []llm.Message{llm.UserText("hello")}}); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("calls=%d", calls)
			}
		})
	}
}

func TestRequestBudgetWireProviderDefaults(t *testing.T) {
	for _, tc := range []struct {
		format llm.Format
		want   float64
		body   string
		window int
	}{
		{llm.FormatAnthropic, 8192, `{"id":"m","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`, 0},
		{llm.FormatOpenAI, 0, `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`, 0},
		{llm.FormatAnthropic, -1, `{"id":"m","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`, 8},
	} {
		t.Run(fmt.Sprintf("%s/%dk", tc.format, tc.window), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				got, present := body["max_tokens"].(float64)
				if (tc.want >= 0 && (got != tc.want || present != (tc.want != 0))) ||
					(tc.want < 0 && (!present || got <= 0 || got >= 8000)) {
					t.Errorf("wire max_tokens = %v (present=%v), want %v", got, present, tc.want)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()
			provider, err := (Config{Format: tc.format, Model: "test", APIKey: "test", BaseURL: srv.URL,
				ContextWindowK: tc.window, ContextWindowConfirmed: tc.window > 0}).NewProvider()
			if err != nil {
				t.Fatal(err)
			}
			if _, _, _, err := provider.Complete(t.Context(), llm.CompletionRequest{Messages: []llm.Message{llm.UserText("hello")}}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRequestBudgetSDKOverflowTerminates(t *testing.T) {
	probe := &budgetProbe{}
	provider := &budgetProvider{Provider: probe, config: Config{ContextWindowK: 8, ContextWindowConfirmed: true}}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	_, err := agentcore.Run(ctx, agentcore.Options{
		Provider: provider, SystemPrompt: []string{strings.Repeat("x", 50000)},
		Tools:      []actool.CoreTool{},
		Compaction: compactionConfig(32000),
	}, "hello")
	if err == nil || errors.Is(err, context.DeadlineExceeded) || probe.calls != 0 {
		t.Fatalf("overflow must terminate before delivery: calls=%d err=%v", probe.calls, err)
	}
}
