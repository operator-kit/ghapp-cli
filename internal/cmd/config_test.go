package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/operator-kit/ghapp-cli/internal/auth"
	"github.com/operator-kit/ghapp-cli/internal/config"
)

// cmdMockKeyring implements auth.KeyringProvider for cmd-level tests.
type cmdMockKeyring struct {
	store map[string]string
}

func newCmdMockKeyring() *cmdMockKeyring {
	return &cmdMockKeyring{store: make(map[string]string)}
}

func (m *cmdMockKeyring) Set(service, user, password string) error {
	m.store[service+"/"+user] = password
	return nil
}

func (m *cmdMockKeyring) Get(service, user string) (string, error) {
	v, ok := m.store[service+"/"+user]
	if !ok {
		return "", fmt.Errorf("not found")
	}
	return v, nil
}

func (m *cmdMockKeyring) Delete(service, user string) error {
	delete(m.store, service+"/"+user)
	return nil
}

func TestConfigSet(t *testing.T) {
	saveRestore(t)

	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	cfgPath = cfgFile

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"config", "set", "--app-id", "123", "--installation-id", "456", "--private-key-path", "/path/to/key.pem"})
	require.NoError(t, rootCmd.Execute())

	assert.Contains(t, buf.String(), "Config saved")

	loaded, err := config.Load(cfgFile)
	require.NoError(t, err)
	assert.Equal(t, int64(123), loaded.AppID)
	assert.Equal(t, int64(456), loaded.InstallationID)
	assert.Equal(t, "/path/to/key.pem", loaded.PrivateKeyPath)
}

func TestConfigSet_Partial(t *testing.T) {
	saveRestore(t)

	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	cfgPath = cfgFile

	// Pre-populate
	require.NoError(t, config.Save(cfgFile, &config.Config{
		AppID:          111,
		InstallationID: 222,
		PrivateKeyPath: "/existing/key.pem",
	}))

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"config", "set", "--app-id", "999"})
	require.NoError(t, rootCmd.Execute())

	loaded, err := config.Load(cfgFile)
	require.NoError(t, err)
	assert.Equal(t, int64(999), loaded.AppID)
	assert.Equal(t, int64(222), loaded.InstallationID)
	assert.Equal(t, "/existing/key.pem", loaded.PrivateKeyPath)
}

func TestConfigSet_ImportKey(t *testing.T) {
	saveRestore(t)

	mk := newCmdMockKeyring()
	restore := auth.SetKeyringProvider(mk)
	t.Cleanup(restore)

	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	cfgPath = cfgFile

	pemFile := filepath.Join(dir, "test.pem")
	require.NoError(t, os.WriteFile(pemFile, []byte("fake-pem-data"), 0o600))

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"config", "set", "--app-id", "123", "--import-key", pemFile})
	require.NoError(t, rootCmd.Execute())

	loaded, err := config.Load(cfgFile)
	require.NoError(t, err)
	assert.True(t, loaded.KeyInKeyring)
	assert.Empty(t, loaded.PrivateKeyPath)
	assert.Equal(t, int64(123), loaded.AppID)

	// Verify key stored in mock keyring
	assert.Equal(t, "fake-pem-data", mk.store["ghapp-cli/private-key"])
}

func TestConfigSet_MutualExclusion(t *testing.T) {
	saveRestore(t)
	cfgPath = filepath.Join(t.TempDir(), "config.yaml")

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"config", "set", "--private-key-path", "/a.pem", "--import-key", "/b.pem"})
	err := rootCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestConfigGet(t *testing.T) {
	saveRestore(t)

	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	cfgPath = cfgFile

	require.NoError(t, config.Save(cfgFile, &config.Config{
		AppID:          123,
		InstallationID: 456,
		PrivateKeyPath: "/path/to/key.pem",
	}))

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"config", "get"})
	require.NoError(t, rootCmd.Execute())

	output := buf.String()
	assert.Contains(t, output, "app-id: 123")
	assert.Contains(t, output, "installation-id: 456")
	assert.Contains(t, output, "private-key-path: /path/to/key.pem")
	assert.Contains(t, output, "key-in-keyring: false")
}

func TestConfigGet_SingleKey(t *testing.T) {
	saveRestore(t)

	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	cfgPath = cfgFile

	require.NoError(t, config.Save(cfgFile, &config.Config{
		AppID:          123,
		InstallationID: 456,
	}))

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"config", "get", "app-id"})
	require.NoError(t, rootCmd.Execute())

	assert.Equal(t, "123\n", buf.String())
}

func TestConfigPath(t *testing.T) {
	saveRestore(t)

	cfgFile := filepath.Join(t.TempDir(), "config.yaml")
	cfgPath = cfgFile

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"config", "path"})
	require.NoError(t, rootCmd.Execute())

	assert.Equal(t, cfgFile+"\n", buf.String())
}

func TestAuthConfigure_NonInteractive(t *testing.T) {
	_, buf := setupE2E(t)

	// --gh-auth flag makes it fully non-interactive
	rootCmd.SetArgs([]string{"auth", "configure", "--gh-auth", "none"})
	require.NoError(t, rootCmd.Execute())

	output := buf.String()
	assert.Contains(t, output, "Git credential helper configured")
	assert.Contains(t, output, "gh CLI configured")
}
