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
	if cfg.DefaultPrivacy != "public" {
		t.Errorf("expected default privacy public, got %q", cfg.DefaultPrivacy)
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

func TestChannelTokenResolution(t *testing.T) {
	cfg := DefaultConfig()

	// Default fallback
	defPath := cfg.ResolveChannelTokenFile("")
	if defPath != "~/.config/auth/youtube_token.json" {
		t.Errorf("expected default token path, got %q", defPath)
	}

	// Specific channel name generates youtube_<channel>.json
	seminarPath := cfg.ResolveChannelTokenFile("seminar")
	if seminarPath != "~/.config/auth/youtube_seminar.json" {
		t.Errorf("expected seminar token path, got %q", seminarPath)
	}

	// Setting custom channel
	cfg.SetChannelToken("theory", "~/.config/auth/custom_theory.json")
	if cfg.DefaultChannel != "theory" {
		t.Errorf("expected default channel to be set to theory, got %q", cfg.DefaultChannel)
	}
	customPath := cfg.ResolveChannelTokenFile("theory")
	if customPath != "~/.config/auth/custom_theory.json" {
		t.Errorf("expected custom theory path, got %q", customPath)
	}
}

func TestSaveConfigAndDefaultPlaylist(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	cfg := DefaultConfig()
	cfg.DefaultChannel = "CompGeom"
	cfg.DefaultPlaylist = "PL_test_playlist_123"

	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.DefaultChannel != "CompGeom" {
		t.Errorf("expected DefaultChannel CompGeom, got %q", loaded.DefaultChannel)
	}
	if loaded.DefaultPlaylist != "PL_test_playlist_123" {
		t.Errorf("expected DefaultPlaylist PL_test_playlist_123, got %q", loaded.DefaultPlaylist)
	}
}

func TestWhisperConfigResolution(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	// Mock whisper executable and model in fake home
	binDir := filepath.Join(tempHome, ".local", "bin")
	_ = os.MkdirAll(binDir, 0o755)
	mockBin := filepath.Join(binDir, "whisper-cli-rocm")
	_ = os.WriteFile(mockBin, []byte("#!/bin/sh\n"), 0o755)

	modelsDir := filepath.Join(tempHome, ".local", "share", "whisper-models")
	_ = os.MkdirAll(modelsDir, 0o755)
	mockModel := filepath.Join(modelsDir, "ggml-large-v3-turbo.bin")
	_ = os.WriteFile(mockModel, []byte("model"), 0o644)

	cfg := DefaultConfig()
	resolvedBin := cfg.ResolveWhisperBin()
	if resolvedBin != mockBin {
		t.Errorf("expected auto-resolved bin %q, got %q", mockBin, resolvedBin)
	}

	resolvedModel := cfg.ResolveWhisperModel()
	if resolvedModel != mockModel {
		t.Errorf("expected auto-resolved model %q, got %q", mockModel, resolvedModel)
	}

	// Test explicit config takes priority
	cfg.WhisperBin = mockBin
	cfg.WhisperModel = mockModel
	if cfg.ResolveWhisperBin() != mockBin {
		t.Errorf("expected configured bin %q", mockBin)
	}

	// Test env overrides
	t.Setenv("TALK_CUT_WHISPER_BIN", "/custom/bin/whisper")
	t.Setenv("TALK_CUT_WHISPER_MODEL", "/custom/model.bin")
	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if loaded.WhisperBin != "/custom/bin/whisper" {
		t.Errorf("expected env WhisperBin, got %q", loaded.WhisperBin)
	}
	if loaded.WhisperModel != "/custom/model.bin" {
		t.Errorf("expected env WhisperModel, got %q", loaded.WhisperModel)
	}
}
