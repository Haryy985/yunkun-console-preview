package agent

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Autumn-27/norma/llm"
)

func TestCompactionWindowFollowsUpstreamPolicy(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  Config
		want int
	}{
		{"default", Config{}, 200000},
		{"unconfirmed", Config{ContextWindowK: 200}, 200000},
		{"small", Config{ContextWindowK: 8, ContextWindowConfirmed: true}, 32000},
		{"configured", Config{ContextWindowK: 128}, 128000},
		{"large", Config{ContextWindowK: 2000}, 1000000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.CompactionWindow(); got != tc.want {
				t.Fatalf("window = %d, want %d", got, tc.want)
			}
		})
	}
	if got := compactionConfig(0).ContextWindow; got != 200000 {
		t.Fatalf("default compaction window = %d", got)
	}
}

func TestProviderDoesNotRejectLargeRequestLocally(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["max_tokens"] != float64(32768) {
			t.Errorf("output cap changed: %v", body["max_tokens"])
		}
		messages, _ := body["messages"].([]any)
		if len(messages) == 0 {
			t.Error("history missing from provider request")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	defer srv.Close()
	provider, err := (Config{Format: llm.FormatOpenAI, Model: "fixture", APIKey: "fixture", BaseURL: srv.URL}).NewProvider()
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, err = provider.Complete(t.Context(), llm.CompletionRequest{
		Messages:  []llm.Message{llm.UserText(strings.Repeat("large-history-", 18000))},
		MaxTokens: 32768,
	})
	if err != nil || calls != 1 {
		t.Fatalf("large request did not reach provider: calls=%d err=%v", calls, err)
	}
}
