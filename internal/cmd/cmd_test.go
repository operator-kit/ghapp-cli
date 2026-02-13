package cmd

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/operator-kit/ghapp-cli/internal/auth"
	"github.com/operator-kit/ghapp-cli/internal/authstate"
	"github.com/operator-kit/ghapp-cli/internal/config"
	"github.com/operator-kit/ghapp-cli/internal/selfupdate"
	"github.com/operator-kit/ghapp-cli/internal/shellinit"
	"github.com/operator-kit/ghapp-cli/internal/tokencache"
)

type mockTokenGenerator struct {
	result *auth.TokenResult
	err    error
	calls  int
}

func (m *mockTokenGenerator) Generate(appID, installationID int64, privateKey []byte) (*auth.TokenResult, error) {
	m.calls++
	return m.result, m.err
}

func setupTestConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "test.pem")
	require.NoError(t, os.WriteFile(keyPath, []byte("fake-pem"), 0o600))

	cfgDir := filepath.Join(dir, "config")
	cfgFile := filepath.Join(cfgDir, "config.yaml")
	require.NoError(t, config.Save(cfgFile, &config.Config{
		AppID:          123,
		InstallationID: 456,
		PrivateKeyPath: keyPath,
	}))
	return cfgFile
}

func saveRestore(t *testing.T) {
	t.Helper()
	origGen := tokenGenerator
	origCfg := cfg
	origCfgPath := cfgPath
	origCacheDir := tokencache.DirOverride
	origUpdateDir := selfupdate.DirOverride
	origNoCache := noCache
	origGhAuth := ghAuthFlag
	origRemoveKey := removeKey
	origShellOverride := shellinit.ShellOverride
	// Isolate token cache to temp dir
	tokencache.DirOverride = t.TempDir()
	selfupdate.DirOverride = t.TempDir()
	t.Cleanup(func() {
		tokenGenerator = origGen
		cfg = origCfg
		cfgPath = origCfgPath
		tokencache.DirOverride = origCacheDir
		selfupdate.DirOverride = origUpdateDir
		noCache = origNoCache
		ghAuthFlag = origGhAuth
		removeKey = origRemoveKey
		shellinit.ShellOverride = origShellOverride
	})
}

// isolateHome creates a sandboxed home directory so E2E tests don't touch
// the real git config, shell rc files, or gh hosts.yml.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()

	// Git isolation
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	// Go os.UserConfigDir() isolation
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))

	// Create empty .bashrc so shell hook install has a target
	require.NoError(t, os.WriteFile(filepath.Join(home, ".bashrc"), nil, 0o644))

	return home
}

// setupE2E combines isolateHome + saveRestore + config with pre-populated
// app identity (avoids real API calls in configureGitIdentity).
func setupE2E(t *testing.T) (home string, buf *bytes.Buffer) {
	t.Helper()
	home = isolateHome(t)
	saveRestore(t)

	keyPath := filepath.Join(home, "test.pem")
	require.NoError(t, os.WriteFile(keyPath, []byte("fake-pem"), 0o600))

	// Config dir lives inside the isolated home
	cfgDir := filepath.Join(home, ".config", "ghapp")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))
	cfgFile := filepath.Join(cfgDir, "config.yaml")
	require.NoError(t, config.Save(cfgFile, &config.Config{
		AppID:          123,
		InstallationID: 456,
		PrivateKeyPath: keyPath,
		AppSlug:        "testbot",
		BotUserID:      99999,
	}))
	cfgPath = cfgFile

	mock := &mockTokenGenerator{
		result: &auth.TokenResult{
			Token:     "ghs_e2e_token",
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	tokenGenerator = mock

	buf = new(bytes.Buffer)
	rootCmd.SetOut(buf)
	return home, buf
}

func TestVersionCmd(t *testing.T) {
	SetVersionInfo("1.0.0", "abc123", "2024-01-01")

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"version"})
	require.NoError(t, rootCmd.Execute())

	assert.Contains(t, buf.String(), "1.0.0")
	assert.Contains(t, buf.String(), "abc123")
}

func TestTokenCmd(t *testing.T) {
	cfgFile := setupTestConfig(t)
	saveRestore(t)

	mock := &mockTokenGenerator{
		result: &auth.TokenResult{
			Token:     "ghs_test_token_123",
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	tokenGenerator = mock
	cfgPath = cfgFile

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"token"})
	require.NoError(t, rootCmd.Execute())

	assert.Equal(t, "ghs_test_token_123\n", buf.String())
	assert.Equal(t, 1, mock.calls)
}

func TestCredentialHelperGet(t *testing.T) {
	cfgFile := setupTestConfig(t)
	saveRestore(t)

	mock := &mockTokenGenerator{
		result: &auth.TokenResult{
			Token:     "ghs_cred_helper_token",
			ExpiresAt: time.Unix(1700000000, 0),
		},
	}
	tokenGenerator = mock
	cfgPath = cfgFile

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetIn(strings.NewReader("protocol=https\nhost=github.com\n\n"))
	rootCmd.SetArgs([]string{"credential-helper", "get"})
	require.NoError(t, rootCmd.Execute())

	output := buf.String()
	assert.Contains(t, output, "username=x-access-token")
	assert.Contains(t, output, "password=ghs_cred_helper_token")
	assert.Contains(t, output, "password_expiry_utc=1700000000")
}

func TestCredentialHelperStore(t *testing.T) {
	cfgFile := setupTestConfig(t)
	saveRestore(t)
	cfgPath = cfgFile

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"credential-helper", "store"})
	require.NoError(t, rootCmd.Execute())
	assert.Empty(t, buf.String())
}

func TestCredentialHelperNonGitHub(t *testing.T) {
	cfgFile := setupTestConfig(t)
	saveRestore(t)

	mock := &mockTokenGenerator{}
	tokenGenerator = mock
	cfgPath = cfgFile

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetIn(strings.NewReader("protocol=https\nhost=gitlab.com\n\n"))
	rootCmd.SetArgs([]string{"credential-helper", "get"})
	require.NoError(t, rootCmd.Execute())

	assert.Empty(t, buf.String())
	assert.Equal(t, 0, mock.calls)
}

// --- Setup command tests ---

func testPEMPath(t *testing.T) string {
	t.Helper()
	// Write the test PEM to a temp file so setup can read it
	pemData, err := os.ReadFile(filepath.Join("..", "auth", "testdata", "test.pem"))
	require.NoError(t, err)
	dir := t.TempDir()
	p := filepath.Join(dir, "test.pem")
	require.NoError(t, os.WriteFile(p, pemData, 0o600))
	return p
}

func TestSetupCmd(t *testing.T) {
	saveRestore(t)

	keyPath := testPEMPath(t)
	cfgDir := t.TempDir()
	cfgFile := filepath.Join(cfgDir, "config.yaml")
	cfgPath = cfgFile

	mock := &mockTokenGenerator{
		result: &auth.TokenResult{
			Token:     "ghs_setup_token",
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	tokenGenerator = mock

	// Pipe: AppID, InstallationID, key path, then "n" for configure prompt
	input := "123\n456\n" + keyPath + "\nn\n"

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetIn(strings.NewReader(input))
	rootCmd.SetArgs([]string{"setup"})
	require.NoError(t, rootCmd.Execute())

	output := buf.String()
	assert.Contains(t, output, "OK")
	assert.Contains(t, output, "Config saved")

	// Verify config was actually saved
	loaded, err := config.Load(cfgFile)
	require.NoError(t, err)
	assert.Equal(t, int64(123), loaded.AppID)
	assert.Equal(t, int64(456), loaded.InstallationID)
}

func TestSetupCmd_SetsCfgBeforeAuthConfigure(t *testing.T) {
	saveRestore(t)

	keyPath := testPEMPath(t)
	cfgDir := t.TempDir()
	cfgFile := filepath.Join(cfgDir, "config.yaml")
	cfgPath = cfgFile

	mock := &mockTokenGenerator{
		result: &auth.TokenResult{
			Token:     "ghs_setup_token",
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	tokenGenerator = mock

	// Answer "n" to configure prompt — we just verify cfg is set
	// after setup completes (the bug was cfg remaining nil)
	input := "123\n456\n" + keyPath + "\nn\n"

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetIn(strings.NewReader(input))
	rootCmd.SetArgs([]string{"setup"})
	require.NoError(t, rootCmd.Execute())

	// Regression: cfg must be set from setup's newCfg so that
	// runAuthConfigure (if called) doesn't hit nil pointer panic
	require.NotNil(t, cfg, "cfg must be set after setup")
	assert.Equal(t, int64(123), cfg.AppID)
	assert.Equal(t, int64(456), cfg.InstallationID)
	assert.Equal(t, keyPath, cfg.PrivateKeyPath)
}

func TestSetupCmd_InvalidPEM(t *testing.T) {
	saveRestore(t)

	dir := t.TempDir()
	badPEM := filepath.Join(dir, "bad.pem")
	require.NoError(t, os.WriteFile(badPEM, []byte("not-a-pem"), 0o600))

	cfgPath = filepath.Join(dir, "config.yaml")

	input := "123\n456\n" + badPEM + "\n"

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetIn(strings.NewReader(input))
	rootCmd.SetArgs([]string{"setup"})
	err := rootCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid PEM")
}

func TestSetupCmd_InvalidAppID(t *testing.T) {
	saveRestore(t)
	cfgPath = filepath.Join(t.TempDir(), "config.yaml")

	input := "abc\n"

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetIn(strings.NewReader(input))
	rootCmd.SetArgs([]string{"setup"})
	err := rootCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid App ID")
}

// --- validatePEM tests ---

func TestValidatePEM(t *testing.T) {
	t.Parallel()

	pemData, err := os.ReadFile(filepath.Join("..", "auth", "testdata", "test.pem"))
	require.NoError(t, err)

	tests := []struct {
		name    string
		data    []byte
		wantErr string
	}{
		{
			name: "valid_pkcs1",
			data: pemData,
		},
		{
			name:    "no_pem_block",
			data:    []byte("just random bytes"),
			wantErr: "invalid PEM file",
		},
		{
			name:    "invalid_key",
			data:    []byte("-----BEGIN RSA PRIVATE KEY-----\nYmFkZGF0YQ==\n-----END RSA PRIVATE KEY-----"),
			wantErr: "invalid private key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validatePEM(tt.data)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

// --- promptInt64 tests ---

func TestPromptInt64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr string
	}{
		{name: "valid", input: "123\n", want: 123},
		{name: "invalid", input: "abc\n", wantErr: "invalid"},
		{name: "empty", input: "\n", wantErr: "invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reader := bufio.NewReader(strings.NewReader(tt.input))
			got, err := promptInt64(reader, &bytes.Buffer{}, "Test")
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- confirm tests ---

func TestConfirm(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "y", input: "y\n", want: true},
		{name: "yes", input: "yes\n", want: true},
		{name: "n", input: "n\n", want: false},
		{name: "empty", input: "\n", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := new(bytes.Buffer)
			rootCmd.SetOut(buf)
			rootCmd.SetIn(strings.NewReader(tt.input))
			got := confirm(rootCmd, "test?")
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- E2E tests (isolated home) ---

// gitCfg reads a git global config key from the isolated environment.
func gitCfg(t *testing.T, key string) string {
	t.Helper()
	out, err := exec.Command("git", "config", "--global", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func TestAuthConfigure_E2E(t *testing.T) {
	_, buf := setupE2E(t)

	// "y" = proceed, "y" = switch to bot identity
	rootCmd.SetIn(strings.NewReader("y\ny\n"))
	rootCmd.SetArgs([]string{"auth", "configure", "--gh-auth", "none"})
	require.NoError(t, rootCmd.Execute())

	output := buf.String()
	assert.Contains(t, output, "Git credential helper configured")
	assert.Contains(t, output, "Git identity set to testbot[bot]")

	// Verify git config was written to isolated gitconfig
	assert.Contains(t, gitCfg(t, "credential.https://github.com.helper"), "credential-helper")
	assert.Equal(t, "testbot[bot]", gitCfg(t, "user.name"))
	assert.Contains(t, gitCfg(t, "user.email"), "testbot[bot]@users.noreply.github.com")
	assert.Equal(t, "git@github.com:", gitCfg(t, `url.https://github.com/.insteadOf`))

	// Verify gh hosts.yml written
	configDir, err := os.UserConfigDir()
	require.NoError(t, err)
	hostsPath := filepath.Join(configDir, "gh", "hosts.yml")
	hostsData, err := os.ReadFile(hostsPath)
	require.NoError(t, err)
	assert.Contains(t, string(hostsData), "github.com")

	// Verify auth state saved
	state, err := authstate.LoadDefault()
	require.NoError(t, err)
	assert.NotNil(t, state)
}

func TestAuthReset_E2E(t *testing.T) {
	_, buf := setupE2E(t)

	// First: configure
	rootCmd.SetIn(strings.NewReader("y\ny\n"))
	rootCmd.SetArgs([]string{"auth", "configure", "--gh-auth", "none"})
	require.NoError(t, rootCmd.Execute())

	// Sanity: credential helper is set
	assert.NotEmpty(t, gitCfg(t, "credential.https://github.com.helper"))

	// Now: reset
	buf.Reset()
	rootCmd.SetIn(strings.NewReader("y\n"))
	rootCmd.SetArgs([]string{"auth", "reset"})
	require.NoError(t, rootCmd.Execute())

	output := buf.String()
	assert.Contains(t, output, "Git credential helper removed")

	// Verify git config entries removed
	assert.Empty(t, gitCfg(t, "credential.https://github.com.helper"))
	assert.Empty(t, gitCfg(t, `url.https://github.com/.insteadOf`))
}

func TestAuthStatus_E2E(t *testing.T) {
	_, buf := setupE2E(t)

	// Configure first
	rootCmd.SetIn(strings.NewReader("y\ny\n"))
	rootCmd.SetArgs([]string{"auth", "configure", "--gh-auth", "none"})
	require.NoError(t, rootCmd.Execute())

	// Check status
	buf.Reset()
	rootCmd.SetArgs([]string{"auth", "status"})
	require.NoError(t, rootCmd.Execute())

	output := buf.String()
	assert.Contains(t, output, "App ID:")
	assert.Contains(t, output, "Git helper:")
	assert.Contains(t, output, "credential-helper")
	assert.Contains(t, output, "testbot[bot]")
}

func TestAuthConfigure_ShellFunction_E2E(t *testing.T) {
	home, buf := setupE2E(t)

	// Override shell detection so the test doesn't depend on parent process
	shellinit.ShellOverride = shellinit.ShellByName("bash")

	// "y" = proceed, "y" = switch to bot identity
	rootCmd.SetIn(strings.NewReader("y\ny\n"))
	rootCmd.SetArgs([]string{"auth", "configure", "--gh-auth", "shell-function"})
	require.NoError(t, rootCmd.Execute())

	output := buf.String()
	assert.Contains(t, output, "Shell hook installed")

	// Verify managed block in .bashrc
	bashrc, err := os.ReadFile(filepath.Join(home, ".bashrc"))
	require.NoError(t, err)
	assert.Contains(t, string(bashrc), "ghapp initialize")
	assert.Contains(t, string(bashrc), "auth shell-init")

	// Verify auth state saved with shell-function mode
	state, err := authstate.LoadDefault()
	require.NoError(t, err)
	assert.Equal(t, "shell-function", state.GhAuthMode)
	assert.NotEmpty(t, state.ShellHooks)
}

func TestFullFlow_Setup_Configure_Token_E2E(t *testing.T) {
	home, buf := setupE2E(t)

	// Override cfgPath to let setup create its own config
	keyPath := filepath.Join(home, "test.pem")
	pemData, err := os.ReadFile(filepath.Join("..", "auth", "testdata", "test.pem"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(keyPath, pemData, 0o600))

	cfgFile := filepath.Join(home, ".config", "ghapp", "config.yaml")
	cfgPath = cfgFile

	// Setup: AppID=123, InstallID=456, key path, then "n" to skip configure
	input := "123\n456\n" + keyPath + "\nn\n"
	rootCmd.SetIn(strings.NewReader(input))
	rootCmd.SetArgs([]string{"setup"})
	require.NoError(t, rootCmd.Execute())

	assert.Contains(t, buf.String(), "Config saved")

	// Verify config was persisted
	loaded, err := config.Load(cfgFile)
	require.NoError(t, err)
	assert.Equal(t, int64(123), loaded.AppID)

	// Now get a token using the saved config
	buf.Reset()
	rootCmd.SetArgs([]string{"token"})
	require.NoError(t, rootCmd.Execute())
	assert.Equal(t, "ghs_e2e_token\n", buf.String())
}
