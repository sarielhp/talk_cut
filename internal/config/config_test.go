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
}

func TestLoadConfigFromSwitcher(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("OPENROUTER_API_KEY", "")

	// Create fake opencode-switcher structure
	switcherDir := filepath.Join(tempHome, ".config", "opencode-switcher")
	profileDir := filepath.Join(switcherDir, "05")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("failed creating test dir: %v", err)
	}

	defJSON := `{"default": "05"}`
	if err := os.WriteFile(filepath.Join(switcherDir, "default.json"), []byte(defJSON), 0o644); err != nil {
		t.Fatalf("failed writing default.json: %v", err)
	}

	apiScript := `#!/usr/bin/env bash
export OPENROUTER_API_KEY="sk-or-v1-test-key-12345"
`
	if err := os.WriteFile(filepath.Join(profileDir, "API_key.sh"), []byte(apiScript), 0o644); err != nil {
		t.Fatalf("failed writing API_key.sh: %v", err)
	}

	confJSON := `{"small_model": "openrouter/google/gemini-2.5-flash-lite"}`
	if err := os.WriteFile(filepath.Join(profileDir, "config.json"), []byte(confJSON), 0o644); err != nil {
		t.Fatalf("failed writing config.json: %v", err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.OpenRouterKey != "sk-or-v1-test-key-12345" {
		t.Errorf("expected key from switcher, got %q", cfg.OpenRouterKey)
	}
	if cfg.Model != "google/gemini-2.5-flash-lite" {
		t.Errorf("expected model from switcher, got %q", cfg.Model)
	}
}
