package agent

import "testing"

func TestFromEnvYunkunSettings(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("YUNKUN_LLM_PROVIDER", "openai")
	t.Setenv("YUNKUN_LLM_MODEL", "new-model")
	t.Setenv("YUNKUN_LLM_STREAM", "false")
	got, ok := FromEnv()
	if !ok || got.Model != "new-model" || got.APIKey != "test-key" || got.Stream {
		t.Fatalf("Yunkun LLM settings: %+v, ok=%v", got, ok)
	}
}

func TestFromEnvRequiresAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("YUNKUN_LLM_PROVIDER", "")
	t.Setenv("YUNKUN_LLM_MODEL", "")
	t.Setenv("YUNKUN_LLM_STREAM", "")
	got, ok := FromEnv()
	if ok {
		t.Fatalf("missing API key must disable environment configuration: %+v", got)
	}
}
