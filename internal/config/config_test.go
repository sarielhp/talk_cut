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
	if cfg.KeyFile != "~/.config/auth/openrouter_api_key" {
		t.Errorf("expected default KeyFile ~/.config/auth/openrouter_api_key, got %q", cfg.KeyFile)
	}
}

func TestResolvePath(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	// Create a test file in ~/.config/auth/test_key
	authDir := filepath.Join(tempHome, ".config", "auth")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("failed creating auth dir: %v", err)
	}
	keyPath := filepath.Join(authDir, "test_key")
	if err := os.WriteFile(keyPath, []byte("secret"), 0o600); err != nil {
		t.Fatalf("failed creating key file: %v", err)
	}

	// 1. Test ~ expansion
	expanded, err := ResolvePath("~/some/path")
	if err != nil {
		t.Fatalf("ResolvePath failed: %v", err)
	}
	if expanded != filepath.Join(tempHome, "some", "path") {
		t.Errorf("unexpected expansion: %q", expanded)
	}

	// 2. Test bare filename auto-resolution in ~/.config/auth/
	bareResolved, err := ResolvePath("test_key")
	if err != nil {
		t.Fatalf("ResolvePath bare failed: %v", err)
	}
	if bareResolved != keyPath {
		t.Errorf("expected %q, got %q", keyPath, bareResolved)
	}
}

func TestGetAPIKey(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("OPENROUTER_API_KEY", "")

	authDir := filepath.Join(tempHome, ".config", "auth")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("failed creating auth dir: %v", err)
	}
	keyPath := filepath.Join(authDir, "openrouter_api_key")
	if err := os.WriteFile(keyPath, []byte("sk-or-v1-my-key\n"), 0o600); err != nil {
		t.Fatalf("failed writing key: %v", err)
	}

	cfg := Config{
		KeyFile: "~/.config/auth/openrouter_api_key",
	}

	key, err := cfg.GetAPIKey()
	if err != nil {
		t.Fatalf("GetAPIKey failed: %v", err)
	}
	if key != "sk-or-v1-my-key" {
		t.Errorf("expected trimmed key, got %q", key)
	}

	// Test OPENROUTER_API_KEY env takes precedence
	t.Setenv("OPENROUTER_API_KEY", "sk-or-v1-env-override")
	envKey, err := cfg.GetAPIKey()
	if err != nil {
		t.Fatalf("GetAPIKey with env failed: %v", err)
	}
	if envKey != "sk-or-v1-env-override" {
		t.Errorf("expected env key override, got %q", envKey)
	}
}
