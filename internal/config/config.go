package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	AppID          int64  `yaml:"app_id"`
	InstallationID int64  `yaml:"installation_id"`
	PrivateKeyPath string `yaml:"private_key_path,omitempty"`
	KeyInKeyring   bool   `yaml:"key_in_keyring"`

	// Cached app metadata (fetched once during auth configure)
	AppSlug   string `yaml:"app_slug,omitempty"`
	BotUserID int64  `yaml:"bot_user_id,omitempty"`

	// Backup of previous git identity (restored on auth reset)
	PrevGitUserName  string `yaml:"prev_git_user_name,omitempty"`
	PrevGitUserEmail string `yaml:"prev_git_user_email,omitempty"`
}

// DefaultPath returns ~/.config/ghapp/config.yaml
func DefaultPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	return filepath.Join(configDir, "ghapp", "config.yaml"), nil
}

// Load reads config from path, then applies env overrides.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	applyEnvOverrides(&cfg)
	return &cfg, nil
}

// Save writes config to path, creating directories as needed.
func Save(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("GHAPP_APP_ID"); v != "" {
		var id int64
		if _, err := fmt.Sscanf(v, "%d", &id); err == nil {
			cfg.AppID = id
		}
	}
	if v := os.Getenv("GHAPP_INSTALLATION_ID"); v != "" {
		var id int64
		if _, err := fmt.Sscanf(v, "%d", &id); err == nil {
			cfg.InstallationID = id
		}
	}
	if v := os.Getenv("GHAPP_PRIVATE_KEY_PATH"); v != "" {
		cfg.PrivateKeyPath = v
	}
}
