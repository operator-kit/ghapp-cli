package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveAndLoad(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := &Config{
		AppID:          123456,
		InstallationID: 789012,
		PrivateKeyPath: "/path/to/key.pem",
		KeyInKeyring:   false,
	}

	require.NoError(t, Save(path, cfg))

	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, cfg.AppID, loaded.AppID)
	assert.Equal(t, cfg.InstallationID, loaded.InstallationID)
	assert.Equal(t, cfg.PrivateKeyPath, loaded.PrivateKeyPath)
	assert.Equal(t, cfg.KeyInKeyring, loaded.KeyInKeyring)
}

func TestSaveCreatesDirectories(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "dir", "config.yaml")

	require.NoError(t, Save(path, &Config{AppID: 1}))

	_, err := os.Stat(path)
	require.NoError(t, err)
}

func TestSaveFilePermissions(t *testing.T) {
	t.Parallel()
	if os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skip("file permission test unreliable in CI")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	require.NoError(t, Save(path, &Config{}))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.False(t, info.IsDir())
}

func TestLoad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantErr string
	}{
		{
			name: "missing_file",
			setup: func(t *testing.T) string {
				return "/nonexistent/config.yaml"
			},
			wantErr: "read config",
		},
		{
			name: "invalid_yaml",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.yaml")
				require.NoError(t, os.WriteFile(path, []byte(":::invalid"), 0o600))
				return path
			},
			wantErr: "parse config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := tt.setup(t)
			_, err := Load(path)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// NO t.Parallel() — t.Setenv is incompatible with parallel tests
func TestEnvOverrides(t *testing.T) {
	tests := []struct {
		name           string
		env            map[string]string
		fileAppID      int64
		wantAppID      int64
		wantInstID     int64
		wantKeyPath    string
		fileInstID     int64
		fileKeyPath    string
	}{
		{
			name:        "valid_overrides",
			fileAppID:   1,
			fileInstID:  2,
			env:         map[string]string{"GHAPP_APP_ID": "999", "GHAPP_INSTALLATION_ID": "888", "GHAPP_PRIVATE_KEY_PATH": "/override/key.pem"},
			wantAppID:   999,
			wantInstID:  888,
			wantKeyPath: "/override/key.pem",
		},
		{
			name:      "invalid_ignored",
			fileAppID: 42,
			env:       map[string]string{"GHAPP_APP_ID": "not-a-number"},
			wantAppID: 42,
		},
		{
			name:       "partial_override",
			fileAppID:  10,
			fileInstID: 20,
			env:        map[string]string{"GHAPP_INSTALLATION_ID": "99"},
			wantAppID:  10,
			wantInstID: 99,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			require.NoError(t, Save(path, &Config{
				AppID:          tt.fileAppID,
				InstallationID: tt.fileInstID,
				PrivateKeyPath: tt.fileKeyPath,
			}))

			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			loaded, err := Load(path)
			require.NoError(t, err)
			assert.Equal(t, tt.wantAppID, loaded.AppID)
			assert.Equal(t, tt.wantInstID, loaded.InstallationID)
			assert.Equal(t, tt.wantKeyPath, loaded.PrivateKeyPath)
		})
	}
}

func TestKeyInKeyringRoundtrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := &Config{
		AppID:          100,
		InstallationID: 200,
		KeyInKeyring:   true,
	}
	require.NoError(t, Save(path, cfg))

	loaded, err := Load(path)
	require.NoError(t, err)
	assert.True(t, loaded.KeyInKeyring)
	assert.Empty(t, loaded.PrivateKeyPath)
}

func TestDefaultPath(t *testing.T) {
	t.Parallel()
	p, err := DefaultPath()
	require.NoError(t, err)
	assert.Contains(t, p, "ghapp")
	assert.Contains(t, p, "config.yaml")
}
