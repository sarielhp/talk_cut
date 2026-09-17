// Package config manages configuration resolution and discovery for talk_cut.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config stores runtime configuration parameters.
type Config struct {
	OpenRouterKey   string `json:"openrouter_key"`
	Model           string `json:"model"`
	BaseURL         string `json:"base_url"`
	YouTubeSecrets  string `json:"youtube_secrets"`
	DefaultPrivacy  string `json:"default_privacy"`
	PreferredLayout string `json:"preferred_layout"` // "slides", "clean", "speaker", "gallery"
}

// DefaultConfig returns baseline configuration settings.
func DefaultConfig() Config {
	return Config{
		Model:           "google/gemini-2.5-flash-lite",
		BaseURL:         "https://openrouter.ai/api/v1",
		DefaultPrivacy:  "unlisted",
		PreferredLayout: "slides",
	}
}

// LoadConfig resolves configuration using prioritized resolution:
// 1. ~/.config/talk_cut/config.json
// 2. Environment variables (OPENROUTER_API_KEY, TALK_CUT_MODEL)
func LoadConfig() (Config, error) {
	cfg := DefaultConfig()

	home, err := os.UserHomeDir()
	if err == nil {
		loadFromTalkCutConfig(&cfg, home)
	}

	// Environment variables override file configuration
	if envKey := os.Getenv("OPENROUTER_API_KEY"); envKey != "" {
		cfg.OpenRouterKey = strings.TrimSpace(envKey)
	}
	if envModel := os.Getenv("TALK_CUT_MODEL"); envModel != "" {
		cfg.Model = strings.TrimSpace(envModel)
	}

	return cfg, nil
}

// loadFromTalkCutConfig reads ~/.config/talk_cut/config.json if present.
func loadFromTalkCutConfig(cfg *Config, home string) {
	path := filepath.Join(home, ".config", "talk_cut", "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	var stored Config
	if err := json.Unmarshal(data, &stored); err == nil {
		if stored.OpenRouterKey != "" {
			cfg.OpenRouterKey = stored.OpenRouterKey
		}
		if stored.Model != "" {
			cfg.Model = stored.Model
		}
		if stored.BaseURL != "" {
			cfg.BaseURL = stored.BaseURL
		}
		if stored.YouTubeSecrets != "" {
			cfg.YouTubeSecrets = stored.YouTubeSecrets
		}
		if stored.DefaultPrivacy != "" {
			cfg.DefaultPrivacy = stored.DefaultPrivacy
		}
		if stored.PreferredLayout != "" {
			cfg.PreferredLayout = stored.PreferredLayout
		}
	}
}

// SaveConfig persists the current configuration to ~/.config/talk_cut/config.json.
func (c Config) SaveConfig() error {
	home, homeErr := os.UserHomeDir()
	if homeErr != nil {
		return fmt.Errorf("locating home directory: %w", homeErr)
	}

	dir := filepath.Join(home, ".config", "talk_cut")
	if dirErr := os.MkdirAll(dir, 0o755); dirErr != nil {
		return fmt.Errorf("creating config dir %q: %w", dir, dirErr)
	}

	path := filepath.Join(dir, "config.json")
	data, marshalErr := json.MarshalIndent(c, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("marshaling config: %w", marshalErr)
	}

	if writeErr := os.WriteFile(path, data, 0o600); writeErr != nil {
		return fmt.Errorf("writing config file: %w", writeErr)
	}

	return nil
}
