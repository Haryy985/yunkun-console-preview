package config

import "testing"

func TestCORSOriginsDefaultsToSameOriginOnly(t *testing.T) {
	t.Setenv("YUNKUN_CORS_ORIGINS", "")
	t.Setenv("YUNKUN_CONFIG", t.TempDir()+"\\missing-config.json")
	if got := CORSOrigins(); len(got) != 0 {
		t.Fatalf("CORS origins = %#v, want empty", got)
	}
}

func TestCORSOriginsEnvironmentOverridesConfig(t *testing.T) {
	t.Setenv("YUNKUN_CORS_ORIGINS", "https://console.example, https://admin.example ")
	got := CORSOrigins()
	if len(got) != 2 || got[0] != "https://console.example" || got[1] != "https://admin.example" {
		t.Fatalf("CORS origins = %#v", got)
	}
}
