package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.BaseURL == "" {
		t.Errorf("expected non-empty BaseURL")
	}
	if cfg.Model == "" {
		t.Errorf("expected non-empty Model")
	}
	if cfg.DefaultPrivacy != "unlisted" {
		t.Errorf("expected unlisted default privacy, got %q", cfg.DefaultPrivacy)
	}
}

func TestLoadConfig(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("TALK_CUT_MODEL", "")

	// Create fake ~/.config/talk_cut/config.json
	confDir := filepath.Join(tempHome, ".config", "talk_cut")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatalf("failed creating test dir: %v", err)
	}

	confJSON := `{
		"openrouter_key": "sk-or-v1-file-key",
		"model": "deepseek/deepseek-chat",
		"default_privacy": "private"
	}`
	if err := os.WriteFile(filepath.Join(confDir, "config.json"), []byte(confJSON), 0o600); err != nil {
		t.Fatalf("failed writing config.json: %v", err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.OpenRouterKey != "sk-or-v1-file-key" {
		t.Errorf("expected key from file, got %q", cfg.OpenRouterKey)
	}
	if cfg.Model != "deepseek/deepseek-chat" {
		t.Errorf("expected model from file, got %q", cfg.Model)
	}
	if cfg.DefaultPrivacy != "private" {
		t.Errorf("expected privacy from file, got %q", cfg.DefaultPrivacy)
	}

	// Test environment variable override
	t.Setenv("OPENROUTER_API_KEY", "sk-or-v1-env-override")
	t.Setenv("TALK_CUT_MODEL", "google/gemini-2.5-flash")

	cfgEnv, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig with env failed: %v", err)
	}
	if cfgEnv.OpenRouterKey != "sk-or-v1-env-override" {
		t.Errorf("expected env override key, got %q", cfgEnv.OpenRouterKey)
	}
	if cfgEnv.Model != "google/gemini-2.5-flash" {
		t.Errorf("expected env override model, got %q", cfgEnv.Model)
	}
}
