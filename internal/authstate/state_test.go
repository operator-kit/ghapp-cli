package authstate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ghapp", "auth-state.yaml")

	state := &AuthState{
		GhAuthMode: "shell-function",
		ShellHooks: []ShellHookInfo{
			{
				ShellName:   "zsh",
				FilePath:    "/home/user/.zshrc",
				InstalledAt: time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC),
			},
		},
	}

	require.NoError(t, Save(path, state))

	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "shell-function", loaded.GhAuthMode)
	require.Len(t, loaded.ShellHooks, 1)
	assert.Equal(t, "zsh", loaded.ShellHooks[0].ShellName)
	assert.Equal(t, "/home/user/.zshrc", loaded.ShellHooks[0].FilePath)
}

func TestLoad_Missing(t *testing.T) {
	state, err := Load(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	require.NoError(t, err)
	assert.NotNil(t, state)
	assert.Empty(t, state.GhAuthMode)
}

func TestLoad_Corrupt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")
	require.NoError(t, os.WriteFile(path, []byte(":::invalid yaml"), 0o600))

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse auth state")
}

func TestSave_CreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deep", "nested", "state.yaml")

	require.NoError(t, Save(path, &AuthState{GhAuthMode: "path-shim"}))

	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "path-shim", loaded.GhAuthMode)
}

func TestPathShimFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")

	state := &AuthState{
		GhAuthMode:      "path-shim",
		GhappGhPath:     "/usr/local/bin/gh",
		WrapperChecksum: "abc123",
	}

	require.NoError(t, Save(path, state))

	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "/usr/local/bin/gh", loaded.GhappGhPath)
	assert.Equal(t, "abc123", loaded.WrapperChecksum)
}
