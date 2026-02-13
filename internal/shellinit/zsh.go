package shellinit

import (
	"fmt"
	"os"
	"path/filepath"
)

type zshShell struct{}

func (z *zshShell) Name() string { return "zsh" }

func (z *zshShell) HookCode(ghappBin string) string {
	return fmt.Sprintf(`gh() {
    local _ghapp_token
    _ghapp_token=$(%q token 2>/dev/null)
    if [ -n "$_ghapp_token" ]; then
        GH_TOKEN="$_ghapp_token" command gh "$@"
    else
        command gh "$@"
    fi
}`, ghappBin)
}

func (z *zshShell) EvalLine(ghappBin string) string {
	return fmt.Sprintf(`eval "$(%q auth shell-init)"`, ghappBin)
}

func (z *zshShell) RCFilePath() (string, error) {
	// Respect ZDOTDIR if set
	if zdotdir := os.Getenv("ZDOTDIR"); zdotdir != "" {
		return filepath.Join(zdotdir, ".zshrc"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".zshrc"), nil
}

func (z *zshShell) UsesConfD() bool          { return false }
func (z *zshShell) ConfDFilePath() (string, error) { return "", nil }
