package shellinit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllShells(t *testing.T) {
	t.Parallel()
	shells := AllShells()
	assert.Len(t, shells, 4)
	names := make([]string, len(shells))
	for i, sh := range shells {
		names[i] = sh.Name()
	}
	assert.Contains(t, names, "bash")
	assert.Contains(t, names, "zsh")
	assert.Contains(t, names, "fish")
	assert.Contains(t, names, "powershell")
}

func TestShellByName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		want string
	}{
		{"bash", "bash"},
		{"zsh", "zsh"},
		{"fish", "fish"},
		{"powershell", "powershell"},
		{"BASH", "bash"},
		{"unknown", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sh := ShellByName(tt.name)
			if tt.want == "" {
				assert.Nil(t, sh)
			} else {
				require.NotNil(t, sh)
				assert.Equal(t, tt.want, sh.Name())
			}
		})
	}
}

func TestBashHookCode(t *testing.T) {
	t.Parallel()
	sh := &bashShell{}
	code := sh.HookCode("/usr/local/bin/ghapp")
	assert.Contains(t, code, "gh()")
	assert.Contains(t, code, `"/usr/local/bin/ghapp" token`)
	assert.Contains(t, code, "GH_TOKEN=")
	assert.Contains(t, code, "command gh")
}

func TestZshHookCode(t *testing.T) {
	t.Parallel()
	sh := &zshShell{}
	code := sh.HookCode("/usr/local/bin/ghapp")
	assert.Contains(t, code, "gh()")
	assert.Contains(t, code, "command gh")
}

func TestFishHookCode(t *testing.T) {
	t.Parallel()
	sh := &fishShell{}
	code := sh.HookCode("/usr/local/bin/ghapp")
	assert.Contains(t, code, "function gh")
	assert.Contains(t, code, "command gh $argv")
}

func TestPwshHookCode(t *testing.T) {
	t.Parallel()
	sh := &pwshShell{}
	code := sh.HookCode("/usr/local/bin/ghapp")
	assert.Contains(t, code, "function gh")
	assert.Contains(t, code, "$env:GH_TOKEN")
}

func TestEvalLines(t *testing.T) {
	t.Parallel()
	tests := []struct {
		shell    Shell
		contains string
	}{
		{&bashShell{}, `eval`},
		{&zshShell{}, `eval`},
		{&fishShell{}, `| source`},
		{&pwshShell{}, `Invoke-Expression`},
	}
	for _, tt := range tests {
		t.Run(tt.shell.Name(), func(t *testing.T) {
			t.Parallel()
			line := tt.shell.EvalLine("/path/to/ghapp")
			assert.Contains(t, line, tt.contains)
			assert.Contains(t, line, "auth shell-init")
		})
	}
}

func TestFishUsesConfD(t *testing.T) {
	t.Parallel()
	assert.True(t, (&fishShell{}).UsesConfD())
	assert.False(t, (&bashShell{}).UsesConfD())
	assert.False(t, (&zshShell{}).UsesConfD())
	assert.False(t, (&pwshShell{}).UsesConfD())
}

// --- RC file tests ---

func TestManagedBlock(t *testing.T) {
	t.Parallel()
	block := ManagedBlock("eval line here")
	assert.True(t, strings.HasPrefix(block, markerStart))
	assert.True(t, strings.HasSuffix(block, markerEnd))
	assert.Contains(t, block, "eval line here")
	assert.Contains(t, block, "managed by")
}

func TestInstallAndUninstallHook_RCFile(t *testing.T) {
	dir := t.TempDir()
	sh := &testShell{rcPath: filepath.Join(dir, ".bashrc")}

	path, err := InstallHook(sh, "/usr/local/bin/ghapp")
	require.NoError(t, err)
	assert.Equal(t, sh.rcPath, path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, markerStart)
	assert.Contains(t, content, markerEnd)
	assert.Contains(t, content, "auth shell-init")

	// HasHook should return true
	assert.True(t, HasHook(sh))

	// Uninstall
	require.NoError(t, UninstallHook(sh))
	data, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(data), markerStart)

	assert.False(t, HasHook(sh))
}

func TestInstallHook_PreservesExistingContent(t *testing.T) {
	dir := t.TempDir()
	rcPath := filepath.Join(dir, ".bashrc")

	existing := "# my custom config\nexport FOO=bar\n"
	require.NoError(t, os.WriteFile(rcPath, []byte(existing), 0o644))

	sh := &testShell{rcPath: rcPath}
	_, err := InstallHook(sh, "/usr/local/bin/ghapp")
	require.NoError(t, err)

	data, err := os.ReadFile(rcPath)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "export FOO=bar")
	assert.Contains(t, content, markerStart)
}

func TestInstallHook_ReplacesExistingBlock(t *testing.T) {
	dir := t.TempDir()
	rcPath := filepath.Join(dir, ".bashrc")

	// Pre-existing block with old path
	old := "# prefix\n" + ManagedBlock(`eval "$(/old/ghapp auth shell-init)"`) + "\n# suffix\n"
	require.NoError(t, os.WriteFile(rcPath, []byte(old), 0o644))

	sh := &testShell{rcPath: rcPath}
	_, err := InstallHook(sh, "/new/ghapp")
	require.NoError(t, err)

	data, err := os.ReadFile(rcPath)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "/new/ghapp")
	assert.NotContains(t, content, "/old/ghapp")
	assert.Contains(t, content, "# prefix")
	assert.Contains(t, content, "# suffix")
	// Only one start marker
	assert.Equal(t, 1, strings.Count(content, markerStart))
}

func TestInstallHook_OrphanedMarkers(t *testing.T) {
	dir := t.TempDir()
	rcPath := filepath.Join(dir, ".bashrc")

	// Only start marker, no end
	require.NoError(t, os.WriteFile(rcPath, []byte(markerStart+"\nstuff\n"), 0o644))

	sh := &testShell{rcPath: rcPath}
	_, err := InstallHook(sh, "/usr/local/bin/ghapp")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "orphaned")
}

func TestInstallHook_ConfD(t *testing.T) {
	dir := t.TempDir()
	confDPath := filepath.Join(dir, "conf.d", "ghapp.fish")
	sh := &testShell{confD: true, confDPath: confDPath}

	path, err := InstallHook(sh, "/usr/local/bin/ghapp")
	require.NoError(t, err)
	assert.Equal(t, confDPath, path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), markerStart)
}

func TestUninstallHook_ConfD(t *testing.T) {
	dir := t.TempDir()
	confDDir := filepath.Join(dir, "conf.d")
	require.NoError(t, os.MkdirAll(confDDir, 0o755))
	confDPath := filepath.Join(confDDir, "ghapp.fish")
	require.NoError(t, os.WriteFile(confDPath, []byte("stuff"), 0o644))

	sh := &testShell{confD: true, confDPath: confDPath}
	require.NoError(t, UninstallHook(sh))

	_, err := os.Stat(confDPath)
	assert.True(t, os.IsNotExist(err))
}

func TestInstallHook_CreatesBackup(t *testing.T) {
	dir := t.TempDir()
	rcPath := filepath.Join(dir, ".bashrc")

	original := "# original content\n"
	require.NoError(t, os.WriteFile(rcPath, []byte(original), 0o644))

	sh := &testShell{rcPath: rcPath}
	_, err := InstallHook(sh, "/usr/local/bin/ghapp")
	require.NoError(t, err)

	backup, err := os.ReadFile(rcPath + ".ghapp-backup")
	require.NoError(t, err)
	assert.Equal(t, original, string(backup))
}

// testShell is a Shell implementation for tests with controllable paths.
type testShell struct {
	rcPath    string
	confD     bool
	confDPath string
}

func (t *testShell) Name() string                    { return "test" }
func (t *testShell) HookCode(ghappBin string) string { return "# test hook" }
func (t *testShell) EvalLine(ghappBin string) string {
	return `eval "$(` + ghappBin + ` auth shell-init)"`
}
func (t *testShell) RCFilePath() (string, error)   { return t.rcPath, nil }
func (t *testShell) UsesConfD() bool               { return t.confD }
func (t *testShell) ConfDFilePath() (string, error) { return t.confDPath, nil }
