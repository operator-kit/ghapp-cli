package authstate

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// AuthState tracks the current gh authentication mode and installed hooks.
type AuthState struct {
	GhAuthMode    string          `yaml:"gh_auth_mode,omitempty"`    // "shell-function", "path-shim", ""
	ShellHooks    []ShellHookInfo `yaml:"shell_hooks_installed,omitempty"`
	GhappGhPath     string          `yaml:"ghapp_gh_path,omitempty"`     // PATH shim: path to wrapper binary
	WrapperChecksum string          `yaml:"wrapper_checksum,omitempty"` // PATH shim: SHA256 of wrapper
	URLRewrite      bool            `yaml:"url_rewrite,omitempty"`      // whether ghapp set insteadOf
}

// ShellHookInfo records a shell hook installation.
type ShellHookInfo struct {
	ShellName   string    `yaml:"shell_name"`
	FilePath    string    `yaml:"file_path"`
	InstalledAt time.Time `yaml:"installed_at"`
}

// DefaultPath returns ~/.config/ghapp/auth-state.yaml.
func DefaultPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	return filepath.Join(configDir, "ghapp", "auth-state.yaml"), nil
}

// Load reads auth state from path. Returns empty state if file missing.
func Load(path string) (*AuthState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &AuthState{}, nil
		}
		return nil, fmt.Errorf("read auth state: %w", err)
	}

	var state AuthState
	if err := yaml.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse auth state: %w", err)
	}
	return &state, nil
}

// Save writes auth state to path, creating directories as needed.
func Save(path string, state *AuthState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create auth state dir: %w", err)
	}

	data, err := yaml.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal auth state: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write auth state: %w", err)
	}
	return nil
}

// LoadDefault loads from the default path.
func LoadDefault() (*AuthState, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return Load(path)
}

// SaveDefault saves to the default path.
func SaveDefault(state *AuthState) error {
	path, err := DefaultPath()
	if err != nil {
		return err
	}
	return Save(path, state)
}

// Clear removes the auth state file.
func Clear() error {
	path, err := DefaultPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
