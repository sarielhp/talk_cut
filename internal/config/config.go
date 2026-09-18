// Package config manages configuration resolution and discovery for talk_cut.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config stores runtime configuration parameters without embedding raw secret tokens.
type Config struct {
	KeyFile          string            `json:"key_file"`
	Model            string            `json:"model"`
	BaseURL          string            `json:"base_url"`
	YouTubeSecrets   string            `json:"youtube_secrets"`
	YouTubeTokenFile string            `json:"youtube_token_file,omitempty"`
	DefaultChannel   string            `json:"default_channel,omitempty"`
	Channels         map[string]string `json:"channels,omitempty"`
	DefaultPrivacy   string            `json:"default_privacy"`
	PreferredLayout  string            `json:"preferred_layout"` // "slides", "clean", "speaker", "gallery"
}

// DefaultConfig returns baseline configuration settings.
func DefaultConfig() Config {
	return Config{
		KeyFile:          "~/.config/auth/openrouter_api_key",
		Model:            "google/gemini-2.5-flash-lite",
		BaseURL:          "https://openrouter.ai/api/v1",
		DefaultPrivacy:   "public",
		PreferredLayout:  "slides",
		YouTubeSecrets:   "~/.config/auth/youtube_client_secrets.json",
		YouTubeTokenFile: "~/.config/auth/youtube_token.json",
	}
}

// LoadConfig resolves configuration from ~/.config/talk_cut/config.json with environment overrides.
func LoadConfig() (Config, error) {
	cfg := DefaultConfig()

	home, err := os.UserHomeDir()
	if err == nil {
		loadFromTalkCutConfig(&cfg, home)
	}

	// Environment overrides
	if envKeyFile := os.Getenv("TALK_CUT_KEY_FILE"); envKeyFile != "" {
		cfg.KeyFile = strings.TrimSpace(envKeyFile)
	}
	if envModel := os.Getenv("TALK_CUT_MODEL"); envModel != "" {
		cfg.Model = strings.TrimSpace(envModel)
	}
	if envSec := os.Getenv("TALK_CUT_YOUTUBE_SECRETS"); envSec != "" {
		cfg.YouTubeSecrets = strings.TrimSpace(envSec)
	}
	if envTok := os.Getenv("TALK_CUT_YOUTUBE_TOKEN"); envTok != "" {
		cfg.YouTubeTokenFile = strings.TrimSpace(envTok)
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
		if stored.KeyFile != "" {
			cfg.KeyFile = stored.KeyFile
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
		if stored.YouTubeTokenFile != "" {
			cfg.YouTubeTokenFile = stored.YouTubeTokenFile
		}
		if stored.DefaultChannel != "" {
			cfg.DefaultChannel = stored.DefaultChannel
		}
		if stored.Channels != nil {
			cfg.Channels = stored.Channels
		}
		if stored.DefaultPrivacy != "" {
			cfg.DefaultPrivacy = stored.DefaultPrivacy
		}
		if stored.PreferredLayout != "" {
			cfg.PreferredLayout = stored.PreferredLayout
		}
	}
}

// ResolveChannelTokenFile returns the file path for a channel's OAuth2 token.
// If channel is empty, it uses c.DefaultChannel, or falls back to "default".
// The token file is named ~/.config/auth/youtube_<channel>.json.
func (c Config) ResolveChannelTokenFile(channel string) string {
	channel = strings.TrimSpace(channel)
	if channel == "" {
		channel = strings.TrimSpace(c.DefaultChannel)
	}
	if channel == "" {
		if c.YouTubeTokenFile != "" {
			return c.YouTubeTokenFile
		}
		channel = "default"
	}

	if c.Channels != nil {
		if path, ok := c.Channels[channel]; ok && path != "" {
			return path
		}
	}

	return fmt.Sprintf("~/.config/auth/youtube_%s.json", channel)
}

// SetChannelToken registers a token file path for the named channel.
func (c *Config) SetChannelToken(channel, path string) {
	channel = strings.TrimSpace(channel)
	if channel == "" {
		channel = "default"
	}
	if c.Channels == nil {
		c.Channels = make(map[string]string)
	}
	c.Channels[channel] = path
	if c.DefaultChannel == "" {
		c.DefaultChannel = channel
	}
}

// GetAPIKey resolves and reads the API key.
// Priority:
// 1. OPENROUTER_API_KEY environment variable (if explicitly set)
// 2. The key file specified in c.KeyFile (e.g. ~/.config/auth/openrouter_api_key)
func (c Config) GetAPIKey() (string, error) {
	if envKey := os.Getenv("OPENROUTER_API_KEY"); strings.TrimSpace(envKey) != "" {
		return strings.TrimSpace(envKey), nil
	}

	resolvedPath, err := ResolvePath(c.KeyFile)
	if err != nil {
		return "", fmt.Errorf("resolving key file path %q: %w", c.KeyFile, err)
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return "", fmt.Errorf("reading API key from %q: %w", resolvedPath, err)
	}

	key := strings.TrimSpace(string(data))
	if key == "" {
		return "", fmt.Errorf("API key file %q is empty", resolvedPath)
	}

	return key, nil
}

// ResolvePath expands ~ to user's home directory and resolves bare filenames into ~/.config/auth/.
func ResolvePath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("empty path")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating home directory: %w", err)
	}

	if p == "~" {
		return home, nil
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:]), nil
	}

	// If given a bare filename like "openrouter_api_key", check ~/.config/auth/ first
	if !strings.Contains(p, string(filepath.Separator)) {
		authCandidate := filepath.Join(home, ".config", "auth", p)
		if _, statErr := os.Stat(authCandidate); statErr == nil {
			return authCandidate, nil
		}
	}

	return filepath.Clean(p), nil
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

	if c.DefaultPrivacy == "" {
		c.DefaultPrivacy = "public"
	}

	path := filepath.Join(dir, "config.json")
	data, marshalErr := json.MarshalIndent(c, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("marshaling config: %w", marshalErr)
	}

	if writeErr := os.WriteFile(path, data, 0o644); writeErr != nil {
		return fmt.Errorf("writing config file: %w", writeErr)
	}

	return nil
}
