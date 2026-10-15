package agent

import (
	"context"
	"encoding/json"
	"errors"
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
		yield(llm.StreamEvent{Type: llm.SETextDelta, Text: "ok"}, nil)
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
		{"unknown", Config{}, 0, 64000, 8192, "unknown_local_policy"},
		{"unconfirmed", Config{ContextWindowK: 200}, 0, 64000, 8192, "unconfirmed_local_policy"},
		{"small_unconfirmed", Config{ContextWindowK: 8}, 0, 8000, 2000, "unconfirmed_local_policy"},
		{"confirmed", Config{ContextWindowK: 8, ContextWindowConfirmed: true}, 0, 8000, 2000, "confirmed_profile"},
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
			if err != nil || prepared.MaxTokens != tc.output {
				t.Fatalf("output=%d err=%v", prepared.MaxTokens, err)
			}
		})
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

func TestRequestBudgetRejectsInvalidEncodingAndOversizedOutput(t *testing.T) {
	p := &budgetProvider{}
	for _, req := range []llm.CompletionRequest{
		{MaxTokens: 64000},
		{Tools: []llm.ToolSchema{{InputSchema: map[string]any{"private": make(chan int)}}}},
	} {
		if _, err := p.prepare(req); err == nil || strings.Contains(err.Error(), "private") {
			t.Fatalf("unexpected error: %v", err)
		}
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
