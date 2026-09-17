// Package config manages configuration resolution and discovery for talk_cut.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var openrouterKeyRegex = regexp.MustCompile(`OPENROUTER_API_KEY=["']?([^"'\s]+)["']?`)

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
// 1. Environment variables
// 2. ~/.config/talk_cut/config.json
// 3. ~/.config/opencode-switcher/ active profile (default.json -> API_key.sh)
// 4. ~/.config/auth/openrouter_api_key
func LoadConfig() (Config, error) {
	cfg := DefaultConfig()

	home, err := os.UserHomeDir()
	if err == nil {
		loadFromTalkCutConfig(&cfg, home)
		if cfg.OpenRouterKey == "" {
			loadFromOpenCodeSwitcher(&cfg, home)
		}
		if cfg.OpenRouterKey == "" {
			loadFromAuthDir(&cfg, home)
		}
	}

	// Environment variables always take final precedence
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
	}
}

// loadFromOpenCodeSwitcher checks ~/.config/opencode-switcher/ for active profile and key.
func loadFromOpenCodeSwitcher(cfg *Config, home string) {
	switcherDir := filepath.Join(home, ".config", "opencode-switcher")
	defaultFile := filepath.Join(switcherDir, "default.json")
	defaultData, err := os.ReadFile(defaultFile)
	if err != nil {
		return
	}

	var def struct {
		Default string `json:"default"`
	}
	if err := json.Unmarshal(defaultData, &def); err != nil || def.Default == "" {
		return
	}

	profileDir := filepath.Join(switcherDir, def.Default)
	keyScript := filepath.Join(profileDir, "API_key.sh")
	if scriptData, err := os.ReadFile(keyScript); err == nil {
		if matches := openrouterKeyRegex.FindStringSubmatch(string(scriptData)); len(matches) == 2 {
			cfg.OpenRouterKey = strings.TrimSpace(matches[1])
		}
	}

	// Read small_model or model from profile config if available
	configFile := filepath.Join(profileDir, "config.json")
	if confData, err := os.ReadFile(configFile); err == nil {
		var profConf struct {
			Model      string `json:"model"`
			SmallModel string `json:"small_model"`
		}
		if err := json.Unmarshal(confData, &profConf); err == nil {
			targetModel := profConf.SmallModel
			if targetModel == "" {
				targetModel = profConf.Model
			}
			if targetModel != "" {
				// Strip openrouter/ prefix if present
				cfg.Model = strings.TrimPrefix(targetModel, "openrouter/")
			}
		}
	}
}

// loadFromAuthDir checks ~/.config/auth/openrouter_api_key.
func loadFromAuthDir(cfg *Config, home string) {
	keyPath := filepath.Join(home, ".config", "auth", "openrouter_api_key")
	if data, err := os.ReadFile(keyPath); err == nil {
		key := strings.TrimSpace(string(data))
		if key != "" {
			cfg.OpenRouterKey = key
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
